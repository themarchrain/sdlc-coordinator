package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store/repository"
)

type fakeWorldExecutionExitReader struct {
	remote *runner.Execution
	err    error
}

func (f *fakeWorldExecutionExitReader) Get(context.Context, string) (*runner.Execution, error) {
	return f.remote, f.err
}

func seedInterruptedExecutionTask(t *testing.T, world *fullyWiredWorld, taskID string) {
	t.Helper()
	tenantStore := world.defaultTenantStore(t)
	repositoryID := "repo-" + taskID
	if err := registeredrepo.NewStore(tenantStore.DB).Create(context.Background(), &registeredrepo.RegisteredRepository{
		RepositoryID: repositoryID, DisplayName: "Exit", Provider: "gitlab",
		BaseURL: "https://gitlab.example.com", PathWithNamespace: "group/exit", Status: registeredrepo.StatusEnabled,
	}); err != nil {
		t.Fatal(err)
	}
	runID := "run-" + taskID
	if err := tenantStore.DB.Create(&model.Run{RunID: runID, TaskID: taskID, WorkerRole: "DeveloperWorker", Status: model.RunStatusBlocked}).Error; err != nil {
		t.Fatal(err)
	}
	task := &model.Task{
		TaskID: taskID, DedupeKey: taskID, Type: "bugfix", Role: "DeveloperWorker",
		RepositoryID: &repositoryID, TargetRepo: "group/exit", Status: model.TaskStatusBlocked,
		CurrentRunID: &runID, Objective: "confirm exit", ContextJSON: `{}`,
	}
	if err := repository.NewTaskRepository(tenantStore.DB).Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	finished := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	if err := tenantStore.DB.Create(&model.AgentExecution{
		RunID: runID, ExecutionID: runID, Provider: "codex", Status: model.AgentExecutionStatusInterrupted,
		RequestHash: "request-hash", ErrorCode: "runner_restarted", FinishedAt: &finished,
	}).Error; err != nil {
		t.Fatal(err)
	}
	world.exitReader.remote = &runner.Execution{ExecutionID: runID, Status: runner.StatusInterrupted, RequestHash: "request-hash"}
}

// exitBody builds the request the operator's page sends: the execution and the
// evidence fingerprint it displayed.
func exitBody(t *testing.T, world *fullyWiredWorld, executionID, requestID string) map[string]any {
	t.Helper()
	var execution model.AgentExecution
	if err := world.defaultTenantStore(t).DB.Where("execution_id = ?", executionID).First(&execution).Error; err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"request_id": requestID, "reason": "checked the runner host",
		"execution_id": executionID, "expected_fingerprint": model.ExecutionExitFingerprint(&execution),
	}
}

func TestExecutionExitConfirmationAuthMatrix(t *testing.T) {
	world := newFullyWiredWorld(t)
	tenantID := world.seedTenant(t, "exit-acme")
	adminToken := world.seedUserWithRole(t, tenantID, "exit-admin", controlplane.RoleTenantAdmin)
	memberToken := world.seedUserWithRole(t, tenantID, "exit-member", controlplane.RoleMember)
	seedInterruptedExecutionTask(t, world, "task-exit-http")
	path := "/api/admin/tasks/task-exit-http/execution-exit"
	body := exitBody(t, world, "run-task-exit-http", "request-exit-http")

	if response := world.portalDo(t, http.MethodPost, path, memberToken, body); response.Code != http.StatusForbidden {
		t.Fatalf("member execution exit confirmation = %d body=%s", response.Code, response.Body.String())
	}
	if response := world.portalDo(t, http.MethodPost, path, "", body); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated execution exit confirmation = %d body=%s", response.Code, response.Body.String())
	}
	if response := world.portalDo(t, http.MethodPost, "/api/admin/tasks/task-exit-missing/execution-exit", adminToken, body); response.Code != http.StatusNotFound {
		t.Fatalf("unknown task execution exit confirmation = %d body=%s", response.Code, response.Body.String())
	}
	accepted := world.portalDo(t, http.MethodPost, path, adminToken, body)
	if accepted.Code != http.StatusOK {
		t.Fatalf("tenant admin execution exit confirmation = %d body=%s", accepted.Code, accepted.Body.String())
	}
	payload := decodeJSONMap(t, accepted)
	if payload["remote_state"] != model.ExecutionExitRemoteTerminal {
		t.Fatalf("remote state = %v", payload["remote_state"])
	}
	if response := world.portalDo(t, http.MethodPost, path, adminToken, map[string]any{"request_id": "request-exit-blank", "reason": "  "}); response.Code != http.StatusBadRequest {
		t.Fatalf("blank reason = %d body=%s", response.Code, response.Body.String())
	}
	missingBinding := map[string]any{"request_id": "request-exit-unbound", "reason": "checked the runner host"}
	if response := world.portalDo(t, http.MethodPost, path, adminToken, missingBinding); response.Code != http.StatusBadRequest {
		t.Fatalf("missing execution binding = %d body=%s", response.Code, response.Body.String())
	}
}

func TestExecutionExitConfirmationBindsTheNamedExecution(t *testing.T) {
	world := newFullyWiredWorld(t)
	tenantID := world.seedTenant(t, "exit-binding")
	adminToken := world.seedUserWithRole(t, tenantID, "exit-binding-admin", controlplane.RoleTenantAdmin)
	seedInterruptedExecutionTask(t, world, "task-exit-own")
	seedInterruptedExecutionTask(t, world, "task-exit-other")
	path := "/api/admin/tasks/task-exit-own/execution-exit"

	foreign := exitBody(t, world, "run-task-exit-other", "request-exit-foreign")
	if response := world.portalDo(t, http.MethodPost, path, adminToken, foreign); response.Code != http.StatusNotFound {
		t.Fatalf("foreign execution = %d body=%s", response.Code, response.Body.String())
	}
	stale := exitBody(t, world, "run-task-exit-own", "request-exit-stale")
	stale["expected_fingerprint"] = "fingerprint-of-another-execution-state"
	if response := world.portalDo(t, http.MethodPost, path, adminToken, stale); response.Code != http.StatusConflict {
		t.Fatalf("stale fingerprint = %d body=%s", response.Code, response.Body.String())
	}
	if response := world.portalDo(t, http.MethodPost, path, adminToken, stale); decodeJSONMap(t, response)["error"] == nil {
		t.Fatalf("stale fingerprint was accepted: %s", response.Body.String())
	}
}

func TestExecutionExitConfirmationReportsUnconfirmableExecutions(t *testing.T) {
	world := newFullyWiredWorld(t)
	tenantID := world.seedTenant(t, "exit-unconfirmable")
	adminToken := world.seedUserWithRole(t, tenantID, "exit-unconfirmable-admin", controlplane.RoleTenantAdmin)
	seedInterruptedExecutionTask(t, world, "task-exit-finished")
	store := world.defaultTenantStore(t)
	if err := store.DB.Model(&model.AgentExecution{}).Where("execution_id = ?", "run-task-exit-finished").
		Update("status", model.AgentExecutionStatusFailed).Error; err != nil {
		t.Fatal(err)
	}
	path := "/api/admin/tasks/task-exit-finished/execution-exit"
	if response := world.portalDo(t, http.MethodPost, path, adminToken, exitBody(t, world, "run-task-exit-finished", "request-exit-finished")); response.Code != http.StatusConflict {
		t.Fatalf("execution with its own exit proof = %d body=%s", response.Code, response.Body.String())
	}

	seedInterruptedExecutionTask(t, world, "task-exit-unknown-status")
	world.exitReader.remote = &runner.Execution{ExecutionID: "run-task-exit-unknown-status", Status: "", RequestHash: "request-hash"}
	unknownPath := "/api/admin/tasks/task-exit-unknown-status/execution-exit"
	if response := world.portalDo(t, http.MethodPost, unknownPath, adminToken, exitBody(t, world, "run-task-exit-unknown-status", "request-exit-unknown")); response.Code != http.StatusConflict {
		t.Fatalf("unknown remote status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestExecutionExitConfirmationRejectsRunningRemoteExecution(t *testing.T) {
	world := newFullyWiredWorld(t)
	tenantID := world.seedTenant(t, "exit-running")
	adminToken := world.seedUserWithRole(t, tenantID, "exit-running-admin", controlplane.RoleTenantAdmin)
	seedInterruptedExecutionTask(t, world, "task-exit-running")
	world.exitReader.remote = &runner.Execution{ExecutionID: "run-task-exit-running", Status: runner.StatusRunning, RequestHash: "request-hash"}
	response := world.portalDo(t, http.MethodPost, "/api/admin/tasks/task-exit-running/execution-exit", adminToken,
		exitBody(t, world, "run-task-exit-running", "request-exit-running"))
	if response.Code != http.StatusConflict {
		t.Fatalf("running remote execution = %d body=%s", response.Code, response.Body.String())
	}
}
