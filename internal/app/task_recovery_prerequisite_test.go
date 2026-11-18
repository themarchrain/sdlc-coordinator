package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/failure"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store/repository"
)

// A configured provider name says nothing about whether the runner can actually
// launch the CLI, so a setup failure must stay unrecoverable until something can
// prove the missing piece is back.
func TestRecoveryDoesNotTreatConfiguredProviderAsFixedPrerequisite(t *testing.T) {
	db := openRecoveryTenantDB(t)
	shared := engineShared{cfg: config.Config{}, logger: zap.NewNop()}
	shared.cfg.AgentRunner.Provider = "codex"
	engine, err := buildTenantEngine(db, 101, shared)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	repositoryID := "repo-prerequisite"
	if err := registeredrepo.NewStore(db).Create(ctx, &registeredrepo.RegisteredRepository{
		RepositoryID: repositoryID, DisplayName: "Prerequisite", Provider: "gitlab",
		BaseURL: "https://gitlab.example.com", PathWithNamespace: "group/prerequisite", Status: registeredrepo.StatusEnabled,
	}); err != nil {
		t.Fatal(err)
	}
	runID := "run-prerequisite"
	task := &model.Task{
		TaskID: "task-prerequisite", DedupeKey: "dedupe-prerequisite", Type: "bugfix", Role: "DeveloperWorker",
		RepositoryID: &repositoryID, TargetRepo: "group/prerequisite", Status: model.TaskStatusBlocked,
		CurrentRunID: &runID, AttemptCount: 5, ContextJSON: `{}`,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusBlocked}).Error; err != nil {
		t.Fatal(err)
	}
	finished := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	failureJSON := `{"version":1,"stage":"initialization","source":"runner","code":"` + failure.CodeCLIMissing + `","summary":"codex cli not found"}`
	if err := db.Create(&model.AgentExecution{
		RunID: runID, ExecutionID: runID, Provider: "codex", Status: model.AgentExecutionStatusFailed,
		RequestHash: "request-hash", FinishedAt: &finished, FailureJSON: failureJSON,
	}).Error; err != nil {
		t.Fatal(err)
	}
	assessment, err := engine.orch.AssessTaskRecovery(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("assess task recovery: %v", err)
	}
	if assessment.Eligible {
		t.Fatalf("configured provider name was treated as a fixed prerequisite: %+v", assessment)
	}
	if assessment.ReasonCode != "failure_prerequisite" {
		t.Fatalf("reason code = %q, want failure_prerequisite", assessment.ReasonCode)
	}
	// 没有任何生产探针能证明 CLI 或配置回来了，所以不能承诺"修好后重新评估"。
	if !strings.Contains(assessment.Remediation, "新建任务") {
		t.Fatalf("remediation still promises a recovery that cannot happen: %q", assessment.Remediation)
	}
	if strings.Contains(assessment.Remediation, "重新评估") {
		t.Fatalf("remediation still asks the operator to re-assess: %q", assessment.Remediation)
	}
	if _, err := repository.NewTaskRecoveryRepository(db).GetByRequest(ctx, task.TaskID, "request-prerequisite"); err == nil {
		t.Fatal("unexpected recovery record")
	}
}
