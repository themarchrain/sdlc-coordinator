package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"sdlc-coordinator/internal/failure"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"

	"gorm.io/gorm"
)

func TestRecoverBlockedTaskAtomic(t *testing.T) {
	db, task, run, orch := openRecoveryOrchestrator(t, failure.CodeProviderRateLimited)
	if _, err := orch.AdvanceTask(context.Background(), task.TaskID, model.TaskStatusPending); !errors.Is(err, ErrRecoveryRequiresAuditedPath) {
		t.Fatalf("generic blocked->pending was not fenced: %v", err)
	}
	assessment, err := orch.AssessTaskRecovery(context.Background(), task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if !assessment.Eligible {
		t.Fatalf("assessment not eligible: %+v", assessment)
	}
	response, err := orch.RecoverBlockedTask(context.Background(), task.TaskID, "member-1", RecoverTaskRequest{
		RequestID: "request-orch-atomic", BlockRevision: assessment.BlockRevision, Reason: "retry transient provider failure",
	})
	if err != nil {
		t.Fatalf("recover task: %v", err)
	}
	if response.Replayed || response.Task.Status != model.TaskStatusPending || response.Task.RecoveryGeneration != 1 || response.Recovery.RequestID != "request-orch-atomic" {
		t.Fatalf("unexpected recovery response: %+v", response)
	}
	var runs int64
	if err := db.Model(&model.Run{}).Where("task_id = ?", task.TaskID).Count(&runs).Error; err != nil || runs != 1 {
		t.Fatalf("recovery created extra run: count=%d err=%v", runs, err)
	}
	var storedRun model.Run
	if err := db.Where("run_id = ?", run.RunID).First(&storedRun).Error; err != nil || storedRun.RecoveryGeneration != 0 {
		t.Fatalf("old run changed generation: %+v err=%v", storedRun, err)
	}
}

func TestRecoveryUnknownRequiresAck(t *testing.T) {
	db, task, _, orch := openRecoveryOrchestrator(t, failure.CodeProviderFailed)
	_ = db
	assessment, err := orch.AssessTaskRecovery(context.Background(), task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if !assessment.RequiresRiskConfirmation {
		t.Fatalf("unknown assessment did not require risk: %+v", assessment)
	}
	if _, err := orch.RecoverBlockedTask(context.Background(), task.TaskID, "member-1", RecoverTaskRequest{RequestID: "request-orch-risk", BlockRevision: assessment.BlockRevision, Reason: "manual retry"}); !errors.Is(err, repository.ErrRecoveryRiskRequired) {
		t.Fatalf("missing risk acknowledgement error = %v", err)
	}
	current, _ := orch.GetTask(context.Background(), task.TaskID)
	if current.Status != model.TaskStatusBlocked || current.RecoveryGeneration != 0 {
		t.Fatalf("risk validation changed task: %+v", current)
	}
}

func TestRecoveryRejectsLateRunBeforeClaim(t *testing.T) {
	db, task, run, orch := openRecoveryOrchestrator(t, failure.CodeProviderRateLimited)
	assessment, err := orch.AssessTaskRecovery(context.Background(), task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orch.RecoverBlockedTask(context.Background(), task.TaskID, "member-1", RecoverTaskRequest{RequestID: "request-orch-late", BlockRevision: assessment.BlockRevision, Reason: "retry"}); err != nil {
		t.Fatal(err)
	}
	current, _ := orch.GetTask(context.Background(), task.TaskID)
	if current.CurrentRunID != nil || current.Status != model.TaskStatusPending {
		t.Fatalf("recovery window task = %+v", current)
	}
	if !isStaleRunForTask(current, run.RunID) {
		t.Fatal("nil current run in recovered generation was treated as current")
	}
	if err := AssertRunRecoveryCurrent(current, run); err == nil {
		t.Fatal("old run passed generation assertion")
	}
	_, err = orch.SubmitResult(context.Background(), SubmitResultRequest{
		RawPayload: []byte(`{}`),
		Canonical:  results.CanonicalResult{TaskID: task.TaskID, RunID: run.RunID, Status: results.StatusSucceeded},
	})
	if !errors.Is(err, repository.ErrResultNotAcceptable) {
		t.Fatalf("old canonical result replay error = %v", err)
	}
	submission := &model.ResultSubmission{
		TaskID: task.TaskID, RunID: run.RunID, Source: "mcp", Role: "DeveloperWorker",
		PayloadHash: "payload", RawPayloadJSON: `{}`, CanonicalJSON: `{}`,
	}
	if err := repository.NewResultSubmissionRepository(db).Accept(context.Background(), submission, time.Now().Add(time.Minute)); !errors.Is(err, repository.ErrRecoveryGenerationMismatch) && !errors.Is(err, repository.ErrResultNotAcceptable) {
		t.Fatalf("old submission was accepted: %v", err)
	}
	_, err = repository.NewActionAdmissionRepository(db).AdmitAction(context.Background(), repository.ActionAdmissionRequest{
		TaskID: task.TaskID, RunID: run.RunID, Kind: model.ActionGitPush, ActionKey: "late-push", Attempt: 1, LeaseOwner: "owner", LeaseGeneration: 1,
	})
	if err == nil {
		t.Fatal("late git admission was accepted before claim")
	}
}

func openRecoveryOrchestrator(t *testing.T, code string) (*gorm.DB, *model.Task, *model.Run, *Orchestrator) {
	t.Helper()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.ResultSubmission{}, &model.Writeback{}, &model.Effect{}, &model.ActionAdmission{}, &model.WorkItemLease{}, &model.DeveloperDeliveryCheckpoint{}, &model.MergeGateAudit{}, &model.StopRequest{}, &model.TaskRecovery{}); err != nil {
		t.Fatal(err)
	}
	repositoryID := "repo-recovery-orchestrator"
	runID := "run-recovery-orchestrator"
	task := &model.Task{
		TaskID: "task-recovery-orchestrator", DedupeKey: "dedupe-recovery-orchestrator", Type: "bugfix", Role: "DeveloperWorker",
		RepositoryID: &repositoryID, TargetRepo: "group/project", Status: model.TaskStatusBlocked, AttemptCount: 2,
		CurrentRunID: &runID, ContextJSON: `{}`,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusFailed}
	if err := db.Create(run).Error; err != nil {
		t.Fatal(err)
	}
	finished := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	failureJSON := `{"version":1,"stage":"provider","source":"provider","code":"` + code + `","summary":"provider failure"}`
	execution := &model.AgentExecution{
		RunID: runID, ExecutionID: runID, Provider: "codex", Status: model.AgentExecutionStatusFailed,
		RequestHash: "request-hash", FailureJSON: failureJSON, FinishedAt: &finished,
	}
	if err := db.Create(execution).Error; err != nil {
		t.Fatal(err)
	}
	reader := &RecoverySafetyReader{
		Store: repository.NewTaskRecoveryRepository(db),
		RepositoryLookup: fakeRecoveryRepositoryLookup{repo: &registeredrepo.RegisteredRepository{
			RepositoryID: repositoryID, Status: registeredrepo.StatusEnabled,
		}},
		Now: func() time.Time { return time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC) },
	}
	orch := New(repository.NewTaskRepository(db), repository.NewRunRepository(db),
		WithRecoveryFactsReader(reader),
		WithTaskRecoveryRepository(repository.NewTaskRecoveryRepository(db)),
		WithResultSubmissionRepository(repository.NewResultSubmissionRepository(db)),
	)
	return db, task, run, orch
}
