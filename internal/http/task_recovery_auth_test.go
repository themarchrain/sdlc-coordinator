package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store/repository"
	"time"
)

func TestRecoveryAuthMatrix(t *testing.T) {
	world := newFullyWiredWorld(t)
	tenantID := world.seedTenant(t, "recovery-acme")
	otherTenantID := world.seedTenant(t, "recovery-other")
	adminToken := world.seedUserWithRole(t, tenantID, "recovery-admin", controlplane.RoleTenantAdmin)
	memberToken := world.seedUserWithRole(t, tenantID, "recovery-member", controlplane.RoleMember)
	otherToken := world.seedUserWithRole(t, otherTenantID, "recovery-other-member", controlplane.RoleMember)

	tenantStore := world.defaultTenantStore(t)
	repoStore := registeredrepo.NewStore(tenantStore.DB)
	repositoryID := "repo-recovery-auth"
	if err := repoStore.Create(context.Background(), &registeredrepo.RegisteredRepository{
		RepositoryID: repositoryID, DisplayName: "Recovery", Provider: "gitlab",
		BaseURL: "https://gitlab.example.com", PathWithNamespace: "group/recovery", Status: registeredrepo.StatusEnabled,
	}); err != nil {
		t.Fatal(err)
	}
	runID := "run-recovery-auth"
	finished := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	if err := tenantStore.DB.Create(&model.Run{RunID: runID, TaskID: "task-recovery-auth", WorkerRole: "DeveloperWorker", Status: model.RunStatusFailed}).Error; err != nil {
		t.Fatal(err)
	}
	task := &model.Task{
		TaskID: "task-recovery-auth", DedupeKey: "recovery-auth", Type: "bugfix", Role: "DeveloperWorker",
		RepositoryID: &repositoryID, TargetRepo: "group/recovery", Status: model.TaskStatusBlocked,
		CurrentRunID: &runID, Objective: "recover me", ContextJSON: `{}`,
	}
	if err := repository.NewTaskRepository(tenantStore.DB).Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err := tenantStore.DB.Create(&model.AgentExecution{
		RunID: runID, ExecutionID: runID, Provider: "codex", Status: model.AgentExecutionStatusFailed,
		RequestHash: "request-hash", FinishedAt: &finished,
		FailureJSON: `{"version":1,"stage":"provider","source":"provider","code":"provider_failed","summary":"provider failed"}`,
	}).Error; err != nil {
		t.Fatal(err)
	}
	path := "/api/admin/tasks/" + task.TaskID + "/recovery"

	for name, token := range map[string]string{"member": memberToken, "tenant_admin": adminToken} {
		response := world.portalDo(t, http.MethodGet, path, token, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("%s GET recovery = %d body=%s", name, response.Code, response.Body.String())
		}
	}
	if response := world.portalDo(t, http.MethodGet, path, "", nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET recovery = %d body=%s", response.Code, response.Body.String())
	}
	_ = otherToken

	blockRevision := ""
	if response := world.portalDo(t, http.MethodGet, path, memberToken, nil); response.Code == http.StatusOK {
		blockRevision, _ = decodeJSONMap(t, response)["block_revision"].(string)
	}
	if blockRevision == "" {
		t.Fatal("GET recovery must return a block revision")
	}
	invalidActor := world.portalDo(t, http.MethodPost, path, memberToken, map[string]any{
		"request_id": "req-1", "block_revision": blockRevision, "reason": "retry", "actor_id": "999",
	})
	if invalidActor.Code != http.StatusBadRequest {
		t.Fatalf("forged actor field = %d body=%s", invalidActor.Code, invalidActor.Body.String())
	}
	missingAck := world.portalDo(t, http.MethodPost, path, memberToken, map[string]any{
		"request_id": "req-2", "block_revision": blockRevision, "reason": "retry",
	})
	if missingAck.Code != http.StatusBadRequest || !strings.Contains(missingAck.Body.String(), "recovery_risk_confirmation_required") {
		t.Fatalf("missing risk acknowledgement = %d body=%s", missingAck.Code, missingAck.Body.String())
	}
	oversized := world.portalDo(t, http.MethodPost, path, memberToken, map[string]any{
		"request_id": "req-3", "block_revision": blockRevision, "reason": strings.Repeat("x", 9000),
	})
	if oversized.Code != http.StatusBadRequest {
		t.Fatalf("oversized recovery body = %d body=%s", oversized.Code, oversized.Body.String())
	}
	for _, target := range []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPost, "/api/admin/tasks/" + task.TaskID + "/stop", map[string]any{"reason": "stop"}},
		{http.MethodPost, "/api/admin/tasks/" + task.TaskID + "/rebind", map[string]any{"repository_id": repositoryID}},
		{http.MethodPost, "/api/admin/scheduler/tick", map[string]any{}},
	} {
		response := world.portalDo(t, target.method, target.path, memberToken, target.body)
		if response.Code != http.StatusForbidden {
			t.Fatalf("member %s %s = %d body=%s", target.method, target.path, response.Code, response.Body.String())
		}
	}
}
