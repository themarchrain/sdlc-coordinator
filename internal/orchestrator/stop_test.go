package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

type stopService interface {
	StopTask(context.Context, string, StopActor, string) (*StopResult, error)
	StopRun(context.Context, string, StopActor, string) (*StopResult, error)
	StopExecution(context.Context, string, StopActor, string) (*StopResult, error)
}

func TestStopServiceThreeTargetsUseOneDecision(t *testing.T) {
	for _, target := range []string{"task", "run", "execution"} {
		t.Run(target, func(t *testing.T) {
			db, err := store.Open(testDatabaseDSN(t))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if sqlDB, err := db.DB(); err == nil {
					_ = sqlDB.Close()
				}
			})
			if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.ResultSubmission{}, &model.StopRequest{}, &model.ActionAdmission{}); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			runID := "current"
			task := &model.Task{TaskID: "task", DedupeKey: "task", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusRunning, CurrentRunID: &runID}
			if err := db.Create(task).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&model.AgentExecution{RunID: runID, ExecutionID: "execution", Provider: "codex", Status: model.AgentExecutionStatusRunning, RequestHash: "hash", RunnerEndpoint: "http://private-runner:8090"}).Error; err != nil {
				t.Fatal(err)
			}
			orch := New(repository.NewTaskRepository(db), repository.NewRunRepository(db), WithStopRepository(repository.NewStopRepository(db)))
			service, ok := any(orch).(stopService)
			if !ok {
				t.Fatal("orchestrator has no stop service")
			}
			stop := service.StopTask
			id := task.TaskID
			if target == "run" {
				stop, id = service.StopRun, runID
			}
			if target == "execution" {
				stop, id = service.StopExecution, "execution"
			}
			result, err := stop(ctx, id, StopActor{UserID: "session-user"}, "closed issue")
			if err != nil {
				t.Fatal(err)
			}
			if result.StopID == "" || result.TaskID != task.TaskID || result.RunID != runID || result.ExecutionID != "execution" || result.TaskStatus != model.TaskStatusInterrupted || result.RunStatus != model.RunStatusInterrupted || result.ExecutionStatus != model.AgentExecutionStatusRunning || result.ExitConfirmed || result.DeliveryStatus != model.StopDeliveryPending || result.RequestedBy != "session-user" || result.Reason != "closed issue" {
				t.Fatalf("incorrect result: %+v", result)
			}
			raw, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "private-runner") || strings.Contains(string(raw), "runner_endpoint") {
				t.Fatal("stop response exposes private routing")
			}
			repeated, err := service.StopTask(ctx, task.TaskID, StopActor{UserID: "another-user"}, "changed")
			if err != nil || repeated.StopID != result.StopID || repeated.RequestedBy != "session-user" {
				t.Fatalf("three targets diverge: %+v %v", repeated, err)
			}
		})
	}
}

func TestStopServiceWithoutTenantRepositoryFailsClosed(t *testing.T) {
	service, ok := any(New(nil, nil)).(stopService)
	if !ok {
		t.Fatal("orchestrator has no stop service")
	}
	if _, err := service.StopTask(context.Background(), "task", StopActor{UserID: "admin"}, ""); !errors.Is(err, ErrStopUnavailable) {
		t.Fatalf("missing tenant stop store fell back: %v", err)
	}
}
