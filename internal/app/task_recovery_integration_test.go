package app

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/failure"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store/repository"
)

func TestProviderBlockedMemberRecoveryContinuesOriginalTask(t *testing.T) {
	db := openRecoveryTenantDB(t)
	repositoryID := "repo-recovery-integration"
	if err := registeredrepo.NewStore(db).Create(context.Background(), &registeredrepo.RegisteredRepository{
		RepositoryID: repositoryID, DisplayName: "Recovery integration", Provider: "gitlab",
		BaseURL: "https://gitlab.example.com", PathWithNamespace: "group/recovery",
		Status: registeredrepo.StatusEnabled,
	}); err != nil {
		t.Fatal(err)
	}
	runID := "run-recovery-original"
	finished := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	task := &model.Task{
		TaskID: "task-recovery-integration", DedupeKey: "recovery-integration", Type: "bugfix", Role: "DeveloperWorker",
		RepositoryID: &repositoryID, TargetRepo: "group/recovery", Status: model.TaskStatusBlocked,
		CurrentRunID: &runID, AttemptCount: 2, ContextJSON: `{}`,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusFailed}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.AgentExecution{
		RunID: runID, ExecutionID: runID, Provider: "codex", Status: model.AgentExecutionStatusFailed,
		RequestHash: "request-hash", FinishedAt: &finished,
		FailureJSON: `{"version":1,"stage":"provider","source":"provider","code":"` + failure.CodeProviderRateLimited + `","summary":"provider rate limited"}`,
	}).Error; err != nil {
		t.Fatal(err)
	}
	engine, err := buildTenantEngine(db, 303, engineShared{cfg: config.Config{}, logger: zap.NewNop()})
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := engine.orch.AssessTaskRecovery(context.Background(), task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if !assessment.Eligible || assessment.RequiresRiskConfirmation {
		t.Fatalf("expected eligible transient recovery: %+v", assessment)
	}
	response, err := engine.orch.RecoverBlockedTask(context.Background(), task.TaskID, "member-17", orchestrator.RecoverTaskRequest{
		RequestID: "request-integration", BlockRevision: assessment.BlockRevision, Reason: "provider recovered",
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Task.Status != model.TaskStatusPending || response.Task.RecoveryGeneration != 1 {
		t.Fatalf("recovery did not return the original task to pending: %+v", response.Task)
	}
	newRun, current, err := engine.orch.StartRun(context.Background(), task.TaskID, &model.Run{RunID: "run-recovery-next", WorkerRole: "DeveloperWorker"})
	if err != nil {
		t.Fatal(err)
	}
	if newRun.RecoveryGeneration != 1 || current.RecoveryGeneration != 1 || current.Status != model.TaskStatusRunning {
		t.Fatalf("new recovery round was not dispatched on the original task: run=%+v task=%+v", newRun, current)
	}
	if current.AttemptCount != 3 || current.RetryCycleBaseCount != 2 || current.RetryCycleMaxAttempts != 3 {
		t.Fatalf("recovery budget was not preserved for the new round: %+v", current)
	}
	stored, err := repository.NewTaskRepository(db).GetByTaskID(context.Background(), task.TaskID)
	if err != nil || stored.RecoveryGeneration != 1 || stored.DedupeKey != task.DedupeKey {
		t.Fatalf("original task identity was not preserved: %+v err=%v", stored, err)
	}
}

func TestUnknownRiskRecoveryCanFailImmediately(t *testing.T) {
	db := openRecoveryTenantDB(t)
	repositoryID := "repo-recovery-unknown"
	if err := registeredrepo.NewStore(db).Create(context.Background(), &registeredrepo.RegisteredRepository{
		RepositoryID: repositoryID, DisplayName: "Recovery unknown", Provider: "gitlab",
		BaseURL: "https://gitlab.example.com", PathWithNamespace: "group/unknown", Status: registeredrepo.StatusEnabled,
	}); err != nil {
		t.Fatal(err)
	}
	runID := "run-recovery-unknown"
	finished := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	task := &model.Task{TaskID: "task-recovery-unknown", DedupeKey: "recovery-unknown", Type: "bugfix", Role: "DeveloperWorker", RepositoryID: &repositoryID, TargetRepo: "group/unknown", Status: model.TaskStatusBlocked, CurrentRunID: &runID, AttemptCount: 1, ContextJSON: `{}`}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusFailed}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.AgentExecution{RunID: runID, ExecutionID: runID, Provider: "codex", Status: model.AgentExecutionStatusFailed, RequestHash: "request-hash", FinishedAt: &finished, FailureJSON: `{"version":1,"stage":"provider","source":"provider","code":"provider_failed","summary":"unknown provider failure"}`}).Error; err != nil {
		t.Fatal(err)
	}
	engine, err := buildTenantEngine(db, 305, engineShared{cfg: config.Config{}, logger: zap.NewNop()})
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := engine.orch.AssessTaskRecovery(context.Background(), task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if !assessment.Eligible || !assessment.RequiresRiskConfirmation || assessment.Classification != "unknown" {
		t.Fatalf("unknown recovery was not risk-gated: %+v", assessment)
	}
	response, err := engine.orch.RecoverBlockedTask(context.Background(), task.TaskID, "member-17", orchestrator.RecoverTaskRequest{
		RequestID: "request-unknown", BlockRevision: assessment.BlockRevision, Reason: "accepted unknown risk", AcknowledgeUnknownRisk: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Task.Status != model.TaskStatusPending || response.Task.RecoveryGeneration != 1 {
		t.Fatalf("unknown recovery did not return to pending: %+v", response.Task)
	}
	if _, current, err := engine.orch.StartRun(context.Background(), task.TaskID, &model.Run{RunID: "run-recovery-unknown-next", WorkerRole: "DeveloperWorker"}); err != nil || current.AttemptCount != 2 {
		t.Fatalf("unknown recovery did not start a bounded new attempt: task=%+v err=%v", current, err)
	}
}

func TestRecoveryPersistsAcrossEngineRestart(t *testing.T) {
	db := openRecoveryTenantDB(t)
	repositoryID := "repo-recovery-restart"
	if err := registeredrepo.NewStore(db).Create(context.Background(), &registeredrepo.RegisteredRepository{
		RepositoryID: repositoryID, DisplayName: "Recovery restart", Provider: "gitlab",
		BaseURL: "https://gitlab.example.com", PathWithNamespace: "group/restart", Status: registeredrepo.StatusEnabled,
	}); err != nil {
		t.Fatal(err)
	}
	runID := "run-recovery-restart"
	finished := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	task := &model.Task{TaskID: "task-recovery-restart", DedupeKey: "recovery-restart", Type: "bugfix", Role: "DeveloperWorker", RepositoryID: &repositoryID, TargetRepo: "group/restart", Status: model.TaskStatusBlocked, CurrentRunID: &runID, AttemptCount: 1, ContextJSON: `{}`}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusFailed}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.AgentExecution{RunID: runID, ExecutionID: runID, Provider: "codex", Status: model.AgentExecutionStatusFailed, RequestHash: "request-hash", FinishedAt: &finished, FailureJSON: `{"version":1,"stage":"provider","source":"provider","code":"` + failure.CodeProviderRateLimited + `","summary":"rate limited"}`}).Error; err != nil {
		t.Fatal(err)
	}
	engine, err := buildTenantEngine(db, 304, engineShared{cfg: config.Config{}, logger: zap.NewNop()})
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := engine.orch.AssessTaskRecovery(context.Background(), task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.orch.RecoverBlockedTask(context.Background(), task.TaskID, "member-17", orchestrator.RecoverTaskRequest{
		RequestID: "request-restart", BlockRevision: assessment.BlockRevision, Reason: "manual retry",
	}); err != nil {
		t.Fatal(err)
	}
	restarted, err := buildTenantEngine(db, 304, engineShared{cfg: config.Config{}, logger: zap.NewNop()})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := restarted.orch.GetTask(context.Background(), task.TaskID)
	if err != nil || stored.Status != model.TaskStatusPending || stored.RecoveryGeneration != 1 {
		t.Fatalf("recovery did not persist across engine restart: %+v err=%v", stored, err)
	}
}
