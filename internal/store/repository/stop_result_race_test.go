package repository

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
)

func TestStaleSaveCannotOverwriteStop(t *testing.T) {
	for _, target := range []string{"task", "run", "submission", "failure", "defer"} {
		t.Run(target, func(t *testing.T) {
			db, stops, task := stopFixture(t, model.TaskStatusRunning)
			run, _ := bindStopExecution(t, db, task, model.RunStatusRunning)
			submissions := NewResultSubmissionRepository(db)
			submission := &model.ResultSubmission{TaskID: task.TaskID, RunID: run.RunID, Source: "mcp", Role: task.Role, PayloadHash: "original-hash", RawPayloadJSON: `{"original":true}`, CanonicalJSON: `{}`}
			ctx := context.Background()
			if err := submissions.Accept(ctx, submission, time.Now().Add(time.Minute)); err != nil {
				t.Fatal(err)
			}
			staleTask, _ := NewTaskRepository(db).GetByTaskID(ctx, task.TaskID)
			staleRun, _ := NewRunRepository(db).GetByRunID(ctx, run.RunID)
			if _, err := stops.Stop(ctx, StopTarget{TaskID: task.TaskID}, "admin", "closed", 3); err != nil {
				t.Fatal(err)
			}
			switch target {
			case "task":
				staleTask.Status = model.TaskStatusSucceeded
				_ = NewTaskRepository(db).Update(ctx, staleTask)
			case "run":
				staleRun.Status = model.RunStatusSucceeded
				_ = NewRunRepository(db).Update(ctx, staleRun)
			case "submission":
				submission.Status = model.ResultSubmissionStatusSucceeded
				_ = submissions.Update(ctx, submission)
			case "failure":
				_, _, _ = submissions.FailFinalization(ctx, submission.SubmissionID, "late failure", time.Now(), 3)
			case "defer":
				_, _ = submissions.DeferReconciliationWait(ctx, submission.SubmissionID, "late writeback")
			}
			currentTask, _ := NewTaskRepository(db).GetByTaskID(ctx, task.TaskID)
			currentRun, _ := NewRunRepository(db).GetByRunID(ctx, run.RunID)
			currentSubmission, err := submissions.GetBySubmissionID(ctx, submission.SubmissionID)
			if err != nil {
				t.Fatal(err)
			}
			if currentTask.Status != model.TaskStatusInterrupted || currentRun.Status != model.RunStatusInterrupted || currentSubmission.Status != model.ResultSubmissionStatusInterrupted {
				t.Fatalf("late %s overwrote stop: %s/%s/%s", target, currentTask.Status, currentRun.Status, currentSubmission.Status)
			}
			if currentSubmission.RawPayloadJSON != submission.RawPayloadJSON || currentSubmission.PayloadHash != "original-hash" || currentSubmission.AttemptCount != 1 || currentTask.AttemptCount != 2 {
				t.Fatalf("stop history changed: %+v / %+v", currentTask, currentSubmission)
			}
		})
	}
}

func TestStoppedTaskRejectsReworkLifecycle(t *testing.T) {
	for _, next := range []model.TaskStatus{model.TaskStatusSucceeded, model.TaskStatusBlocked} {
		t.Run(string(next), func(t *testing.T) {
			db, stops, task := stopFixture(t, model.TaskStatusWaitingMerge)
			if err := db.Model(task).Update("type", model.TaskTypeReworkMergeRequest).Error; err != nil {
				t.Fatal(err)
			}
			bindStopExecution(t, db, task, model.RunStatusSucceeded)
			ctx := context.Background()
			if _, err := stops.Stop(ctx, StopTarget{TaskID: task.TaskID}, "admin", "closed", 3); err != nil {
				t.Fatal(err)
			}
			_, err := NewTaskRepository(db).FinalizeRework(ctx, task.DedupeKey, next, "late event", time.Now())
			current, _ := NewTaskRepository(db).GetByTaskID(ctx, task.TaskID)
			if err == nil || current.Status != model.TaskStatusInterrupted {
				t.Fatalf("rework lifecycle revived stopped task: status=%s err=%v", current.Status, err)
			}
		})
	}
}
