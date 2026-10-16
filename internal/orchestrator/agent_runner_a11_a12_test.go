package orchestrator

import (
	"context"
	"errors"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/runner"
)

func TestRestartRecoveredInterruptedExecutionConvergesThroughCoordinator(t *testing.T) {
	w := newIntentWorld(t)
	ctx := context.Background()
	hash, err := runner.RequestHash(w.request)
	if err != nil {
		t.Fatalf("request hash: %v", err)
	}
	w.insertIntent(t, "runner-a", hash)
	w.runnerA.execution = &runner.Execution{
		ExecutionID: w.run.RunID, TaskID: w.task.TaskID, Role: w.task.Role,
		Provider: "claude-cli", Status: runner.StatusInterrupted, RequestHash: hash,
		ErrorCode: "runner_restarted",
	}

	observation, err := w.coord.Observe(ctx, w.run.RunID)
	if err != nil {
		t.Fatalf("observe interrupted execution: %v", err)
	}
	if observation.Execution.Status != model.AgentExecutionStatusInterrupted || observation.Execution.ErrorCode != "runner_restarted" {
		t.Fatalf("interrupted recovery did not converge: %+v", observation.Execution)
	}
	stored, err := w.executions.GetByRunID(ctx, w.run.RunID)
	if err != nil || stored.Status != model.AgentExecutionStatusInterrupted || w.runnerA.createCalls != 0 {
		t.Fatalf("interrupted recovery must persist without replay: execution=%+v creates=%d err=%v", stored, w.runnerA.createCalls, err)
	}
}

func TestAcceptedExecutionMissingAfterRunnerVolumeLossRemainsUnknown(t *testing.T) {
	for _, operation := range []string{"start", "observe"} {
		t.Run(operation, func(t *testing.T) {
			w := newIntentWorld(t)
			ctx := context.Background()
			hash, err := runner.RequestHash(w.request)
			if err != nil {
				t.Fatalf("request hash: %v", err)
			}
			w.insertIntent(t, "runner-a", hash)

			// Model Create being accepted before the coordinator can persist its
			// projection, followed by loss of the Runner's durable execution volume.
			if _, err := w.runnerA.Create(ctx, w.request); err != nil {
				t.Fatalf("initial accepted create: %v", err)
			}
			w.runnerA.execution = nil

			switch operation {
			case "start":
				_, err = w.coord.Start(ctx, w.task, w.run, w.request)
			case "observe":
				_, err = w.coord.Observe(ctx, w.run.RunID)
			}
			if !errors.Is(err, errDispatchOutcomeUnknown) {
				t.Fatalf("%s error = %v, want unknown outcome", operation, err)
			}
			if w.runnerA.createCalls != 1 {
				t.Fatalf("volume loss caused a second execution: creates=%d", w.runnerA.createCalls)
			}

			stored, err := w.executions.GetByRunID(ctx, w.run.RunID)
			if err != nil || stored.Status != model.AgentExecutionStatusDispatching {
				t.Fatalf("unknown outcome must preserve dispatch intent: execution=%+v err=%v", stored, err)
			}
		})
	}
}

func TestAmbiguousCreateAndRunner404DoNotFailOrRotate(t *testing.T) {
	w := newIntentWorld(t)
	w.runnerA.createErr = errors.New("connection reset after request was accepted")

	projection, err := w.coord.Start(context.Background(), w.task, w.run, w.request)
	if err != nil {
		t.Fatalf("unknown create outcome should remain pending: %v", err)
	}
	if projection.Status != model.AgentExecutionStatusDispatching {
		t.Fatalf("projection status = %s, want dispatching", projection.Status)
	}
	if w.runnerA.createCalls != 1 || w.runnerB.createCalls != 0 {
		t.Fatalf("ambiguous 404 must not create again or rotate: runnerA=%d runnerB=%d", w.runnerA.createCalls, w.runnerB.createCalls)
	}
	stored, err := w.executions.GetByRunID(context.Background(), w.run.RunID)
	if err != nil || stored.Status != model.AgentExecutionStatusDispatching {
		t.Fatalf("unknown create outcome must preserve dispatch intent: execution=%+v err=%v", stored, err)
	}
}
