package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

func stopFixture(t *testing.T, status model.TaskStatus) (*gorm.DB, *StopRepository, *model.Task) {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
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
	lease := time.Now().Add(time.Hour)
	task := &model.Task{TaskID: "task-stop", DedupeKey: "original-dedupe", Type: "develop_from_issue", Role: "DeveloperWorker", Status: status, AttemptCount: 2, LeaseUntil: &lease}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	return db, NewStopRepository(db), task
}

func bindStopExecution(t *testing.T, db *gorm.DB, task *model.Task, runStatus model.RunStatus) (*model.Run, *model.AgentExecution) {
	t.Helper()
	run := &model.Run{RunID: "current-run", TaskID: task.TaskID, WorkerRole: task.Role, Status: runStatus}
	if err := db.Create(run).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(task).Update("current_run_id", run.RunID).Error; err != nil {
		t.Fatal(err)
	}
	execution := &model.AgentExecution{RunID: run.RunID, ExecutionID: "execution-stop", Provider: "codex", Status: model.AgentExecutionStatusRunning, RequestHash: "identity-hash", RunnerEndpoint: "http://runner-original:8090"}
	if err := db.Create(execution).Error; err != nil {
		t.Fatal(err)
	}
	return run, execution
}

func TestStopTaskStateMatrix(t *testing.T) {
	for _, status := range []model.TaskStatus{"pending", "claimed", "running", "finalizing", "retry_pending", "waiting_upstream", "waiting_merge", "human_required", "blocked"} {
		t.Run(string(status), func(t *testing.T) {
			db, repo, task := stopFixture(t, status)
			decision, err := repo.Stop(context.Background(), StopTarget{TaskID: task.TaskID}, "admin-original", "abandoned", 3)
			if err != nil {
				t.Fatalf("stop %s: %v", status, err)
			}
			current, err := NewTaskRepository(db).GetByTaskID(context.Background(), task.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			if current.Status != model.TaskStatusInterrupted || current.LeaseUntil != nil || current.AttemptCount != 2 || current.DedupeKey != "original-dedupe" || current.CurrentRunID != nil {
				t.Fatalf("incorrect stopped task: %+v", current)
			}
			if decision.Stop.StopID == "" || decision.Stop.RequestedBy != "admin-original" || decision.Stop.ReasonCode != "manual_stop" || decision.Stop.DeliveryStatus != model.StopDeliveryConfirmed || decision.Stop.ConfirmedAt == nil || decision.Stop.RequestedAt.IsZero() {
				t.Fatalf("incorrect stop audit: %+v", decision.Stop)
			}
			var runs, executions int64
			db.Model(&model.Run{}).Count(&runs)
			db.Model(&model.AgentExecution{}).Count(&executions)
			if runs != 0 || executions != 0 {
				t.Fatalf("stop created placeholders: runs=%d executions=%d", runs, executions)
			}
		})
	}
}

func TestStopCurrentRunCascadesAndPreservesHistory(t *testing.T) {
	for _, active := range []model.RunStatus{model.RunStatusPending, model.RunStatusRunning, model.RunStatusFinalizing} {
		t.Run(string(active), func(t *testing.T) {
			db, repo, task := stopFixture(t, model.TaskStatusFinalizing)
			run, execution := bindStopExecution(t, db, task, active)
			old := &model.Run{RunID: "old-run", TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusFailed, OutputJSON: "historical-result"}
			if err := db.Create(old).Error; err != nil {
				t.Fatal(err)
			}
			submission := &model.ResultSubmission{RunID: run.RunID, TaskID: task.TaskID, Role: task.Role, Source: "mcp", PayloadHash: "payload-hash", RawPayloadJSON: `{"original":true}`, CanonicalJSON: "{}", Status: model.ResultSubmissionStatusProcessing, AttemptCount: 2, LeaseUntil: task.LeaseUntil}
			if err := db.Create(submission).Error; err != nil {
				t.Fatal(err)
			}
			decision, err := repo.Stop(context.Background(), StopTarget{RunID: run.RunID}, "admin", "", 3)
			if err != nil {
				t.Fatal(err)
			}
			if decision.Stop.RunID != run.RunID || decision.Stop.ExecutionID != execution.ExecutionID || decision.Stop.RunnerEndpoint != execution.RunnerEndpoint || decision.Stop.DeliveryStatus != model.StopDeliveryPending {
				t.Fatalf("lost cancel target: %+v", decision.Stop)
			}
			if err := db.First(run, run.ID).Error; err != nil {
				t.Fatal(err)
			}
			if run.Status != model.RunStatusInterrupted || run.CompletedAt == nil {
				t.Fatalf("active run not interrupted: %+v", run)
			}
			if err := db.First(old, old.ID).Error; err != nil {
				t.Fatal(err)
			}
			if old.Status != model.RunStatusFailed || old.OutputJSON != "historical-result" {
				t.Fatalf("history changed: %+v", old)
			}
			submissionID := submission.ID
			submission = &model.ResultSubmission{}
			if err := db.First(submission, submissionID).Error; err != nil {
				t.Fatal(err)
			}
			if submission.Status != "interrupted" || submission.LeaseUntil != nil || submission.AttemptCount != 2 || submission.PayloadHash != "payload-hash" || submission.RawPayloadJSON != `{"original":true}` {
				t.Fatalf("submission lost original facts: %+v", submission)
			}
			if err := db.First(execution, execution.ID).Error; err != nil {
				t.Fatal(err)
			}
			if execution.Status != model.AgentExecutionStatusRunning {
				t.Fatal("stop falsely claimed execution exited")
			}
			current, _ := NewTaskRepository(db).GetByTaskID(context.Background(), task.TaskID)
			if current.CurrentRunID == nil || *current.CurrentRunID != run.RunID || current.AttemptCount != 2 {
				t.Fatalf("stop rewrote current identity: %+v", current)
			}
		})
	}
}

func TestStopIdempotentAndStaleTargetsRejected(t *testing.T) {
	db, repo, task := stopFixture(t, model.TaskStatusRunning)
	run, execution := bindStopExecution(t, db, task, model.RunStatusRunning)
	old := &model.Run{RunID: "old", TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusFailed}
	if err := db.Create(old).Error; err != nil {
		t.Fatal(err)
	}
	oldExecution := &model.AgentExecution{RunID: old.RunID, ExecutionID: "old-execution", Provider: "codex", RequestHash: "old", Status: model.AgentExecutionStatusFailed}
	if err := db.Create(oldExecution).Error; err != nil {
		t.Fatal(err)
	}
	for _, target := range []StopTarget{{RunID: old.RunID}, {ExecutionID: oldExecution.ExecutionID}} {
		if _, err := repo.Stop(context.Background(), target, "admin", "", 3); !errors.Is(err, ErrStopNotCurrentRun) {
			t.Fatalf("stale target accepted: %v", err)
		}
	}
	first, err := repo.Stop(context.Background(), StopTarget{ExecutionID: execution.ExecutionID}, "original-admin", "original reason", 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []StopTarget{{TaskID: task.TaskID}, {RunID: run.RunID}, {ExecutionID: execution.ExecutionID}} {
		repeated, err := repo.Stop(context.Background(), target, "later-admin", "changed reason", 3)
		if err != nil {
			t.Fatal(err)
		}
		if repeated.Stop.StopID != first.Stop.StopID || repeated.Stop.RequestedBy != "original-admin" || repeated.Stop.Reason != "original reason" || !repeated.Stop.RequestedAt.Equal(first.Stop.RequestedAt) {
			t.Fatalf("repeat rewrote original decision: %+v", repeated.Stop)
		}
	}
	var count int64
	db.Model(&model.StopRequest{}).Count(&count)
	if count != 1 {
		t.Fatalf("stop decisions = %d", count)
	}
}

func TestStopSuccessfulTaskAllowsOnlyExecutionCleanup(t *testing.T) {
	db, repo, task := stopFixture(t, model.TaskStatusSucceeded)
	run, execution := bindStopExecution(t, db, task, model.RunStatusSucceeded)
	for _, target := range []StopTarget{{TaskID: task.TaskID}, {RunID: run.RunID}} {
		if _, err := repo.Stop(context.Background(), target, "admin", "", 3); !errors.Is(err, ErrStopAlreadyTerminal) {
			t.Fatalf("successful delivery rewritten: %v", err)
		}
	}
	decision, err := repo.Stop(context.Background(), StopTarget{ExecutionID: execution.ExecutionID}, "admin", "cleanup", 3)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Stop.Scope != model.StopScopeExecutionCleanup || decision.Stop.DedupeKey != "execution_cleanup:execution-stop" || decision.Task.Status != model.TaskStatusSucceeded || decision.Run.Status != model.RunStatusSucceeded {
		t.Fatalf("cleanup lost successful delivery: %+v", decision)
	}
	var taskStops int64
	db.Model(&model.StopRequest{}).Where("dedupe_key = ?", "task:"+task.TaskID).Count(&taskStops)
	if taskStops != 0 {
		t.Fatal("cleanup created task-level notification veto")
	}
}

func TestStopTransactionRollback(t *testing.T) {
	db, repo, task := stopFixture(t, model.TaskStatusRunning)
	run, _ := bindStopExecution(t, db, task, model.RunStatusRunning)
	if err := db.Exec("CREATE TRIGGER reject_stop BEFORE UPDATE ON runs WHEN NEW.status = 'interrupted' BEGIN SELECT RAISE(ABORT, 'injected stop failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Stop(context.Background(), StopTarget{TaskID: task.TaskID}, "admin", "", 3); err == nil || !strings.Contains(err.Error(), "injected stop failure") {
		t.Fatalf("stop did not reach the injected storage failure: %v", err)
	}
	current, _ := NewTaskRepository(db).GetByTaskID(context.Background(), task.TaskID)
	var count int64
	db.Model(&model.StopRequest{}).Count(&count)
	db.First(run, run.ID)
	if current.Status != model.TaskStatusRunning || current.LeaseUntil == nil || run.Status != model.RunStatusRunning || count != 0 {
		t.Fatalf("partial stop committed: task=%+v run=%+v audits=%d", current, run, count)
	}
}

func TestStopReasonUnicodeLimit(t *testing.T) {
	_, repo, task := stopFixture(t, model.TaskStatusPending)
	for _, reason := range []string{strings.Repeat("中", 501), strings.Repeat("x", 501)} {
		if _, err := repo.Stop(context.Background(), StopTarget{TaskID: task.TaskID}, "admin", reason, 3); !errors.Is(err, ErrInvalidStopRequest) {
			t.Fatalf("oversized reason accepted: %v", err)
		}
	}
	if _, err := repo.Stop(context.Background(), StopTarget{TaskID: task.TaskID}, "", "", 3); !errors.Is(err, ErrInvalidStopRequest) {
		t.Fatalf("anonymous actor accepted: %v", err)
	}
	if _, err := repo.Stop(context.Background(), StopTarget{TaskID: task.TaskID}, "admin", strings.Repeat("中", 500), 3); err != nil {
		t.Fatalf("500 Unicode characters rejected: %v", err)
	}
}

func TestStopPreservesTerminalRunsAndSubmissionHistory(t *testing.T) {
	for _, status := range []model.RunStatus{model.RunStatusSucceeded, model.RunStatusFailed, model.RunStatusBlocked} {
		t.Run(string(status), func(t *testing.T) {
			db, repo, task := stopFixture(t, model.TaskStatusBlocked)
			run, _ := bindStopExecution(t, db, task, status)
			finished := time.Date(2026, 10, 20, 1, 0, 0, 0, time.UTC)
			if err := db.Model(run).Updates(map[string]any{"completed_at": finished, "output_json": "original output"}).Error; err != nil {
				t.Fatal(err)
			}
			submission := &model.ResultSubmission{RunID: run.RunID, TaskID: task.TaskID, Role: task.Role, Source: "mcp", PayloadHash: "original", RawPayloadJSON: "{}", CanonicalJSON: "{}", Status: model.ResultSubmissionStatusSucceeded, CompletedAt: &finished}
			if err := db.Create(submission).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := repo.Stop(context.Background(), StopTarget{TaskID: task.TaskID}, "admin", "", 3); err != nil {
				t.Fatal(err)
			}
			var currentRun model.Run
			if err := db.First(&currentRun, run.ID).Error; err != nil {
				t.Fatal(err)
			}
			var currentSubmission model.ResultSubmission
			if err := db.First(&currentSubmission, submission.ID).Error; err != nil {
				t.Fatal(err)
			}
			if currentRun.Status != status || currentRun.CompletedAt == nil || !currentRun.CompletedAt.Equal(finished) || currentRun.OutputJSON != "original output" || currentSubmission.Status != model.ResultSubmissionStatusSucceeded || currentSubmission.CompletedAt == nil || !currentSubmission.CompletedAt.Equal(finished) {
				t.Fatalf("stop changed completed history: run=%+v submission=%+v", currentRun, currentSubmission)
			}
		})
	}
	for _, test := range []struct {
		name     string
		attempts int
		want     model.ResultSubmissionStatus
	}{
		{"recoverable", 2, model.ResultSubmissionStatusInterrupted}, {"exhausted", 3, model.ResultSubmissionStatusFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, repo, task := stopFixture(t, model.TaskStatusFinalizing)
			run, _ := bindStopExecution(t, db, task, model.RunStatusFinalizing)
			submission := &model.ResultSubmission{RunID: run.RunID, TaskID: task.TaskID, Role: task.Role, Source: "mcp", PayloadHash: "original", RawPayloadJSON: "{}", CanonicalJSON: "{}", Status: model.ResultSubmissionStatusFailed, AttemptCount: test.attempts}
			if err := db.Create(submission).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := repo.Stop(context.Background(), StopTarget{TaskID: task.TaskID}, "admin", "", 3); err != nil {
				t.Fatal(err)
			}
			var current model.ResultSubmission
			if err := db.First(&current, submission.ID).Error; err != nil {
				t.Fatal(err)
			}
			if current.Status != test.want || current.AttemptCount != test.attempts || current.PayloadHash != "original" {
				t.Fatalf("incorrect failed submission outcome: %+v", current)
			}
		})
	}
}

func TestStopAndStartCommitOrders(t *testing.T) {
	for _, stopFirst := range []bool{true, false} {
		name := "start_first"
		if stopFirst {
			name = "stop_first"
		}
		t.Run(name, func(t *testing.T) {
			db, repo, task := stopFixture(t, model.TaskStatusPending)
			now := time.Now().UTC()
			run := &model.Run{RunID: "start-race", WorkerRole: task.Role}
			start := func() error {
				_, err := NewTaskRepository(db).StartRunAtomic(context.Background(), task.TaskID, run, now.Add(time.Hour), now)
				return err
			}
			if !stopFirst {
				if err := start(); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := repo.Stop(context.Background(), StopTarget{TaskID: task.TaskID}, "admin", "", 3); err != nil {
				t.Fatal(err)
			}
			if stopFirst && !errors.Is(start(), ErrTaskNotClaimable) {
				t.Fatal("start crossed committed stop")
			}
			var runs []model.Run
			if err := db.Where("task_id = ?", task.TaskID).Find(&runs).Error; err != nil {
				t.Fatal(err)
			}
			if stopFirst && len(runs) != 0 {
				t.Fatal("stop-first created a Run")
			}
			if !stopFirst && (len(runs) != 1 || runs[0].Status != model.RunStatusInterrupted) {
				t.Fatalf("start-first escaped stop: %+v", runs)
			}
		})
	}
}

func TestStopFreezesRunnerEndpoint(t *testing.T) {
	for _, repinFirst := range []bool{false, true} {
		name := "stop_first"
		if repinFirst {
			name = "repin_first"
		}
		t.Run(name, func(t *testing.T) {
			db, repo, task := stopFixture(t, model.TaskStatusRunning)
			run, execution := bindStopExecution(t, db, task, model.RunStatusRunning)
			executions := NewAgentExecutionRepository(db)
			endpoint := execution.RunnerEndpoint
			if repinFirst {
				if err := executions.UpdateRunnerEndpointIfUnchanged(context.Background(), run.RunID, endpoint, "runner-b"); err != nil {
					t.Fatal(err)
				}
				endpoint = "runner-b"
			}
			decision, err := repo.Stop(context.Background(), StopTarget{TaskID: task.TaskID}, "admin", "", 3)
			if err != nil {
				t.Fatal(err)
			}
			if decision.Stop.RunnerEndpoint != endpoint {
				t.Fatalf("stop lost admitted endpoint: %s", decision.Stop.RunnerEndpoint)
			}
			if err := executions.UpdateRunnerEndpointIfUnchanged(context.Background(), run.RunID, endpoint, "runner-after-stop"); !errors.Is(err, ErrTaskNotClaimable) {
				t.Errorf("execution endpoint advanced after stop: %v", err)
			}
			if err := NewRunRepository(db).PinRunnerEndpointIfEmpty(context.Background(), run.RunID, "runner-after-stop"); !errors.Is(err, ErrTaskNotClaimable) {
				t.Errorf("run endpoint pinned after stop: %v", err)
			}
			var current model.AgentExecution
			if err := db.First(&current, execution.ID).Error; err != nil {
				t.Fatal(err)
			}
			if current.RunnerEndpoint != endpoint {
				t.Fatalf("cancellation target drifted: snapshot=%s execution=%s", endpoint, current.RunnerEndpoint)
			}
		})
	}
}
