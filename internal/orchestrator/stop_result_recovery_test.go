package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"

	"gorm.io/gorm"
)

func stoppedResultFixture(t *testing.T) (*gorm.DB, *Orchestrator, *model.Task, *model.Run) {
	t.Helper()
	db, err := store.Open("file:" + model.NewID("stop-result") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.ResultSubmission{}, &model.StopRequest{}, &model.Dependency{}, &model.Artifact{}, &model.Effect{}); err != nil {
		t.Fatal(err)
	}
	runID := "run"
	task := &model.Task{TaskID: "task", DedupeKey: "original", Role: "TesterWorker", Type: "test", Status: model.TaskStatusRunning, CurrentRunID: &runID}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(run).Error; err != nil {
		t.Fatal(err)
	}
	orch := New(repository.NewTaskRepository(db), repository.NewRunRepository(db), WithStopRepository(repository.NewStopRepository(db)), WithResultSubmissionRepository(repository.NewResultSubmissionRepository(db)), WithDependencyRepository(repository.NewDependencyRepository(db)), WithArtifactRepository(repository.NewArtifactRepository(db)), WithEffectRepository(repository.NewEffectRepository(db)))
	return db, orch, task, run
}

func TestStoppedTaskRejectsLateResultAndNextTask(t *testing.T) {
	for _, entry := range []string{"apply", "submit", "replay", "recovery"} {
		t.Run(entry, func(t *testing.T) {
			db, orch, task, run := stoppedResultFixture(t)
			ctx := context.Background()
			canonical := results.CanonicalResult{TaskID: task.TaskID, RunID: run.RunID, Status: results.StatusSucceeded, Summary: "late result", NextTasks: []results.NextTaskIntent{{Role: "DeveloperWorker", Objective: "must not start"}}}
			request := SubmitResultRequest{Source: "mcp", Canonical: canonical}
			if entry == "replay" || entry == "recovery" {
				raw, hash, err := results.Canonicalize(canonical)
				if err != nil {
					t.Fatal(err)
				}
				submission := &model.ResultSubmission{TaskID: task.TaskID, RunID: run.RunID, Source: "mcp", Role: task.Role, PayloadHash: hash, RawPayloadJSON: string(raw), CanonicalJSON: string(raw)}
				if err := orch.resultSubmissions.Accept(ctx, submission, time.Now().Add(-time.Minute)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := orch.StopTask(ctx, task.TaskID, StopActor{UserID: "admin"}, "closed"); err != nil {
				t.Fatal(err)
			}
			var err error
			switch entry {
			case "apply":
				_, _, err = orch.ApplyCanonicalResult(ctx, canonical)
			case "submit", "replay":
				_, err = orch.SubmitResult(ctx, request)
			case "recovery":
				var recovered []ResultFinalizationResult
				recovered, err = orch.FinalizeRecoverableSubmissions(ctx, 20)
				if len(recovered) != 0 {
					t.Fatalf("stopped submission recovered: %+v", recovered)
				}
			}
			if entry != "recovery" && (err == nil || errors.Is(err, ErrResultProcessing)) {
				t.Errorf("late result must return terminal conflict, got %v", err)
			}
			current, _ := orch.GetTask(ctx, task.TaskID)
			var count int64
			if err := db.Model(&model.Task{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if current.Status != model.TaskStatusInterrupted || count != 1 {
				t.Fatalf("stopped result created work: status=%s tasks=%d", current.Status, count)
			}
		})
	}
}

type stopDuringArtifact struct {
	ArtifactRepository
	stop func() error
}

func (s stopDuringArtifact) UpsertByIdentity(ctx context.Context, artifact *model.Artifact) error {
	if err := s.ArtifactRepository.UpsertByIdentity(ctx, artifact); err != nil {
		return err
	}
	return s.stop()
}

func TestStopWinsCompletion(t *testing.T) {
	db, orch, task, run := stoppedResultFixture(t)
	ctx := context.Background()
	orch.artifacts = stopDuringArtifact{ArtifactRepository: orch.artifacts, stop: func() error {
		_, err := orch.StopTask(ctx, task.TaskID, StopActor{UserID: "admin"}, "during finalization")
		return err
	}}
	_, err := orch.SubmitResult(ctx, SubmitResultRequest{Canonical: results.CanonicalResult{TaskID: task.TaskID, RunID: run.RunID, Status: results.StatusSucceeded, Artifacts: map[string]any{"test_result": "passed"}, NextTasks: []results.NextTaskIntent{{Role: "DeveloperWorker", Objective: "must not start"}}}})
	if err == nil {
		t.Fatal("stop lost to late completion")
	}
	current, _ := orch.GetTask(ctx, task.TaskID)
	currentRun, _ := orch.GetRun(ctx, run.RunID)
	var count int64
	if err := db.Model(&model.Task{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if current.Status != model.TaskStatusInterrupted || currentRun.Status != model.RunStatusInterrupted || count != 1 {
		t.Fatalf("stop did not fence completion: %s/%s tasks=%d", current.Status, currentRun.Status, count)
	}
}

func TestCompletionWinsStop(t *testing.T) {
	_, orch, task, run := stoppedResultFixture(t)
	ctx := context.Background()
	response, err := orch.SubmitResult(ctx, SubmitResultRequest{Canonical: results.CanonicalResult{TaskID: task.TaskID, RunID: run.RunID, Status: results.StatusSucceeded}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orch.StopTask(ctx, task.TaskID, StopActor{UserID: "admin"}, "late stop"); !errors.Is(err, repository.ErrStopAlreadyTerminal) {
		t.Fatalf("completed task was not preserved: %v", err)
	}
	if response.Task.Status != model.TaskStatusSucceeded || response.Run.Status != model.RunStatusSucceeded || response.Submission.Status != model.ResultSubmissionStatusSucceeded {
		t.Fatalf("inconsistent completed result: %+v", response)
	}
}

func TestResultFinalizationRollsBackTaskRunAndNextTasks(t *testing.T) {
	db, orch, task, run := stoppedResultFixture(t)
	injected := errors.New("injected run persistence failure")
	if err := db.Callback().Update().Before("gorm:update").Register("fail_result_run", func(tx *gorm.DB) {
		if tx.Statement.Table == "runs" {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, _, err := orch.ApplyCanonicalResult(context.Background(), results.CanonicalResult{TaskID: task.TaskID, RunID: run.RunID, Status: results.StatusSucceeded, NextTasks: []results.NextTaskIntent{{Role: "DeveloperWorker", Objective: "child"}}})
	if !errors.Is(err, injected) {
		t.Fatalf("expected persistence failure, got %v", err)
	}
	current, _ := orch.GetTask(context.Background(), task.TaskID)
	var count int64
	if err := db.Model(&model.Task{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if current.Status != model.TaskStatusRunning || count != 1 {
		t.Fatalf("partial result committed: status=%s tasks=%d", current.Status, count)
	}
}

func TestRecoverLegacyCompletedRunWithProcessingSubmission(t *testing.T) {
	db, orch, task, run := stoppedResultFixture(t)
	ctx := context.Background()
	canonical := results.CanonicalResult{TaskID: task.TaskID, RunID: run.RunID, Status: results.StatusSucceeded}
	raw, hash, err := results.Canonicalize(canonical)
	if err != nil {
		t.Fatal(err)
	}
	submission := &model.ResultSubmission{TaskID: task.TaskID, RunID: run.RunID, Source: "mcp", Role: task.Role, PayloadHash: hash, RawPayloadJSON: string(raw), CanonicalJSON: string(raw)}
	if err := orch.resultSubmissions.Accept(ctx, submission, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(task).Update("status", model.TaskStatusSucceeded).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(run).Update("status", model.RunStatusSucceeded).Error; err != nil {
		t.Fatal(err)
	}
	recovered, err := orch.FinalizeRecoverableSubmissions(ctx, 20)
	if err != nil || len(recovered) != 1 || recovered[0].Status != model.ResultSubmissionStatusSucceeded {
		t.Fatalf("legacy completion did not settle submission: %+v err=%v", recovered, err)
	}
}
