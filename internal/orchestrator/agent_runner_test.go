package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

func TestAgentRunnerCoordinatorStartsIdempotentlyAndObservesResult(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.AgentExecutionEvent{}); err != nil {
		t.Fatal(err)
	}
	tasks := repository.NewTaskRepository(db)
	runs := repository.NewRunRepository(db)
	executions := repository.NewAgentExecutionRepository(db)
	runID := "run-agent-runner-core"
	task := &model.Task{TaskID: "task-agent-runner-core", DedupeKey: "agent-runner-core", Type: "review_mr", Role: "ReviewerWorker", Status: model.TaskStatusRunning, CurrentRunID: &runID}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := tasks.Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	client := &fakeReviewerRunnerClient{}
	coordinator := NewAgentRunnerCoordinator(tasks, runs, executions, client)
	request := runner.CreateExecutionRequest{
		ExecutionID: runID, TaskID: task.TaskID, Role: task.Role,
		WorkspacePath: t.TempDir(), PromptFile: "prompt.md", SchemaFile: "result-schema.json", TimeoutSeconds: 1800,
		Provider: runner.ProviderConfig{Name: "fake", Revision: 3, Model: "model-a", Effort: "high"},
	}

	first, err := coordinator.Start(context.Background(), task, run, request)
	if err != nil {
		t.Fatalf("start execution: %v", err)
	}
	second, err := coordinator.Start(context.Background(), task, run, request)
	if err != nil {
		t.Fatalf("repeat execution start: %v", err)
	}
	if first.ID != second.ID || client.createCalls != 1 {
		t.Fatalf("start was not idempotent: first=%#v second=%#v calls=%d", first, second, client.createCalls)
	}

	client.execution.Status = runner.StatusSucceeded
	client.execution.ResultHash = "result-hash"
	client.events = []runner.Event{{ExecutionID: runID, Seq: 1, Type: "accepted"}, {ExecutionID: runID, Seq: 2, Type: "succeeded"}}
	client.result = &runner.Result{ExecutionID: runID, ResultHash: "result-hash", StructuredOutput: json.RawMessage(`{"ok":true}`)}
	observed, err := coordinator.Observe(context.Background(), runID)
	if err != nil {
		t.Fatalf("observe execution: %v", err)
	}
	if observed.Execution.Status != model.AgentExecutionStatusSucceeded || observed.Execution.LastEventSeq != 2 {
		t.Fatalf("unexpected projection: %#v", observed.Execution)
	}
	if observed.Execution.ProviderModel != "model-a" || observed.Execution.ProviderEffort != "high" {
		t.Fatalf("provider metadata was not preserved: %#v", observed.Execution)
	}
	if observed.Result == nil || string(observed.Result.StructuredOutput) != `{"ok":true}` {
		t.Fatalf("unexpected result: %#v", observed.Result)
	}
	var persistedEvents []model.AgentExecutionEvent
	if err := db.Order("seq ASC").Find(&persistedEvents).Error; err != nil {
		t.Fatalf("list persisted execution events: %v", err)
	}
	if len(persistedEvents) != 2 || persistedEvents[0].RunID != runID || persistedEvents[1].Type != "succeeded" {
		t.Fatalf("unexpected persisted execution events: %#v", persistedEvents)
	}
	if _, err := coordinator.Observe(context.Background(), runID); err != nil {
		t.Fatalf("repeat observe: %v", err)
	}
	var count int64
	if err := db.Model(&model.AgentExecutionEvent{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("repeat observe duplicated events: count=%d err=%v", count, err)
	}
}

func TestAgentRunnerCoordinatorRejectsRunIdentityMismatch(t *testing.T) {
	coordinator := &AgentRunnerCoordinator{}
	task := &model.Task{TaskID: "task-1", Role: "ReviewerWorker"}
	run := &model.Run{RunID: "run-1", TaskID: "different-task", WorkerRole: "ReviewerWorker"}
	_, err := coordinator.Start(context.Background(), task, run, runner.CreateExecutionRequest{})
	if err == nil {
		t.Fatal("expected mismatched task and run to be rejected")
	}
}

func TestAgentRunnerCoordinatorProjectsFailureMessage(t *testing.T) {
	coordinator := &AgentRunnerCoordinator{now: time.Now}
	remote := &runner.Execution{
		ExecutionID: "run-failed", Provider: "codex-cli", Status: runner.StatusFailed,
		RequestHash: "request-hash", ErrorCode: "provider_failed", ErrorMessage: "invalid request: schema rejected",
	}
	projected, err := coordinator.project(remote, 1, 4, "model-a", "low")
	if err != nil {
		t.Fatalf("project failed execution: %v", err)
	}
	if projected.ErrorCode != remote.ErrorCode || projected.ErrorMessage != remote.ErrorMessage {
		t.Fatalf("failure detail was not projected: %#v", projected)
	}
}
