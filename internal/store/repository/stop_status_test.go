package repository

import (
	"context"
	"sdlc-coordinator/internal/model"
	"testing"
)

func TestStopCapabilityRejectsHistoryAndOffersCleanup(t *testing.T) {
	db, repo, task := stopFixture(t, model.TaskStatusRunning)
	run, execution := bindStopExecution(t, db, task, model.RunStatusRunning)
	capability, ok := any(repo).(interface {
		CanStop(context.Context, StopTarget) (bool, error)
	})
	if !ok {
		t.Fatal("stop capability missing from drawer query")
	}
	for _, target := range []StopTarget{{TaskID: task.TaskID}, {RunID: run.RunID}, {ExecutionID: execution.ExecutionID}} {
		allowed, err := capability.CanStop(context.Background(), target)
		if err != nil || !allowed {
			t.Fatalf("current target hidden: %v %v", allowed, err)
		}
	}
	old := &model.Run{RunID: "old", TaskID: task.TaskID, Status: model.RunStatusRunning}
	if err := db.Create(old).Error; err != nil {
		t.Fatal(err)
	}
	if allowed, err := capability.CanStop(context.Background(), StopTarget{RunID: old.RunID}); err != nil || allowed {
		t.Fatalf("history offered stop: %v %v", allowed, err)
	}
	if err := db.Model(task).Update("status", model.TaskStatusSucceeded).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(run).Update("status", model.RunStatusSucceeded).Error; err != nil {
		t.Fatal(err)
	}
	if allowed, err := capability.CanStop(context.Background(), StopTarget{TaskID: task.TaskID}); err != nil || allowed {
		t.Fatalf("success task offered stop: %v %v", allowed, err)
	}
	if allowed, err := capability.CanStop(context.Background(), StopTarget{ExecutionID: execution.ExecutionID}); err != nil || !allowed {
		t.Fatalf("live leftover cleanup hidden: %v %v", allowed, err)
	}
}
