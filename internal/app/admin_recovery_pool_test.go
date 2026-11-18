package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/controlplane"
	api "sdlc-coordinator/internal/http"
	handlers "sdlc-coordinator/internal/http/handlers"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/tenantdb"
)

// doAdminRequest drives the admin entry the way a signed-in browser does, so
// the request passes the real session middleware and engine selection.
func doAdminRequest(t *testing.T, router http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: handlers.PortalSessionCookieName, Value: token})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

type tenantRecoveryState struct {
	Status            model.TaskStatus
	Generation        uint64
	LastError         string
	UpdatedAt         string
	Recoveries        int64
	ExitConfirmations int64
}

func readTenantRecoveryState(t *testing.T, resolver tenantdb.UnmanagedResolver, tenantID uint, taskID string) tenantRecoveryState {
	t.Helper()
	handle, err := resolver.OpenUnmanaged(tenantID)
	if err != nil {
		t.Fatalf("open tenant %d store: %v", tenantID, err)
	}
	defer func() {
		if err := handle.Close(); err != nil {
			t.Fatalf("close tenant %d store: %v", tenantID, err)
		}
	}()
	var task model.Task
	if err := handle.DB.Where("task_id = ?", taskID).First(&task).Error; err != nil {
		t.Fatalf("read task %s from tenant %d: %v", taskID, tenantID, err)
	}
	var recoveries, confirmations int64
	if err := handle.DB.Model(&model.TaskRecovery{}).Where("task_id = ?", taskID).Count(&recoveries).Error; err != nil {
		t.Fatalf("count recoveries: %v", err)
	}
	if err := handle.DB.Model(&model.ExecutionExitConfirmation{}).Where("task_id = ?", taskID).Count(&confirmations).Error; err != nil {
		t.Fatalf("count exit confirmations: %v", err)
	}
	return tenantRecoveryState{
		Status: task.Status, Generation: task.RecoveryGeneration, LastError: task.LastError,
		UpdatedAt:  task.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Recoveries: recoveries, ExitConfirmations: confirmations,
	}
}

func TestAdminRecoveryUsesTheProductionEnginePool(t *testing.T) {
	ctx := context.Background()
	controlDB, err := store.Open("file:" + model.NewID("pool_control") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open control db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := controlDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	controlRepository := controlplane.NewRepository(controlDB)
	if err := controlRepository.Migrate(); err != nil {
		t.Fatalf("migrate control plane: %v", err)
	}
	control := controlplane.NewService(controlRepository)
	rootID, err := control.CreateSuperAdminUser(ctx, "pool-root", "pool-test-password")
	if err != nil {
		t.Fatalf("seed super admin: %v", err)
	}
	actor := &controlplane.Actor{ID: rootID, Username: "pool-root"}
	firstTenant, err := control.CreateTenant(ctx, actor, controlplane.TenantInput{Slug: "pool-first", DisplayName: "Pool First"})
	if err != nil {
		t.Fatalf("seed first tenant: %v", err)
	}
	secondTenant, err := control.CreateTenant(ctx, actor, controlplane.TenantInput{Slug: "pool-second", DisplayName: "Pool Second"})
	if err != nil {
		t.Fatalf("seed second tenant: %v", err)
	}
	signIn := func(username string, tenantID uint) string {
		t.Helper()
		user, err := control.CreateUser(ctx, username, "pool-test-password")
		if err != nil {
			t.Fatalf("create user %s: %v", username, err)
		}
		if _, err := control.AddMembership(ctx, actor, tenantID, user.ID, controlplane.RoleTenantAdmin); err != nil {
			t.Fatalf("add membership for %s: %v", username, err)
		}
		result, err := control.Login(ctx, username, "pool-test-password", "10.0.0.1")
		if err != nil {
			t.Fatalf("login %s: %v", username, err)
		}
		return result.Token
	}
	firstToken := signIn("pool-admin-first", firstTenant.ID)
	secondToken := signIn("pool-admin-second", secondTenant.ID)

	// 两个独立 SQLite 租户库，经生产的文件解析器落盘。
	resolver := tenantdb.NewSqliteFileResolver(&tenantdb.Store{DB: controlDB}, t.TempDir(), 4)
	t.Cleanup(resolver.Close)
	for _, tenantID := range []uint{firstTenant.ID, secondTenant.ID} {
		handle, err := resolver.OpenUnmanaged(tenantID)
		if err != nil {
			t.Fatalf("open tenant %d store: %v", tenantID, err)
		}
		if err := migrateBusinessStore(handle.DB); err != nil {
			t.Fatalf("migrate tenant %d store: %v", tenantID, err)
		}
		if err := handle.Close(); err != nil {
			t.Fatalf("close tenant %d store: %v", tenantID, err)
		}
	}

	runnerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/executions/exec-pool-a" {
			http.NotFound(w, request)
			return
		}
		_ = json.NewEncoder(w).Encode(runner.Execution{
			ExecutionID: "exec-pool-a", Status: runner.StatusInterrupted, RequestHash: "request-hash",
		})
	}))
	t.Cleanup(runnerServer.Close)

	finished := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	run := &model.Run{RunID: "run-pool-a", TaskID: "task-pool-a", WorkerRole: "DeveloperWorker",
		Status: model.RunStatusBlocked, RunnerEndpoint: runnerServer.URL}
	task := &model.Task{TaskID: "task-pool-a", DedupeKey: "pool-a", Type: "bugfix", Role: "DeveloperWorker",
		Status: model.TaskStatusBlocked, CurrentRunID: &run.RunID, Objective: "只在租户 A 存在"}
	execution := &model.AgentExecution{RunID: run.RunID, ExecutionID: "exec-pool-a", Provider: "codex",
		Status: model.AgentExecutionStatusInterrupted, RequestHash: "request-hash", FinishedAt: &finished}
	ownerStore, err := resolver.OpenUnmanaged(firstTenant.ID)
	if err != nil {
		t.Fatalf("open first tenant store: %v", err)
	}
	if err := ownerStore.DB.Create(run).Error; err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := repository.NewTaskRepository(ownerStore.DB).Create(ctx, task); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := ownerStore.DB.Create(execution).Error; err != nil {
		t.Fatalf("seed execution: %v", err)
	}
	if err := ownerStore.Close(); err != nil {
		t.Fatalf("close first tenant store: %v", err)
	}

	shared := engineShared{cfg: config.Config{}, logger: zap.NewNop()}
	pool := newEnginePool(resolver, control, shared, zap.NewNop())
	t.Cleanup(pool.CloseTenantEngines)
	router := api.NewRouter(nil,
		api.WithRuntimeLogger(zap.NewNop()),
		api.WithControlPlaneService(control),
		api.WithTenantStoreResolver(resolver),
		api.WithAdminOperationEngines(pool),
	)

	recoveryPath := "/api/admin/tasks/task-pool-a/recovery"
	assessment := doAdminRequest(t, router, http.MethodGet, recoveryPath, firstToken, nil)
	if assessment.Code != http.StatusOK {
		t.Fatalf("owner tenant read = %d body=%s", assessment.Code, assessment.Body.String())
	}
	var view struct {
		TaskID                string `json:"task_id"`
		BlockRevision         string `json:"block_revision"`
		UnconfirmedExecutions []struct {
			ExecutionID string `json:"execution_id"`
			Fingerprint string `json:"fingerprint"`
		} `json:"unconfirmed_executions"`
	}
	if err := json.Unmarshal(assessment.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode assessment: %v body=%s", err, assessment.Body.String())
	}
	if view.TaskID != "task-pool-a" || view.BlockRevision == "" {
		t.Fatalf("unexpected assessment identity: %s", assessment.Body.String())
	}
	if len(view.UnconfirmedExecutions) != 1 || view.UnconfirmedExecutions[0].ExecutionID != "exec-pool-a" {
		t.Fatalf("unexpected unconfirmed executions: %s", assessment.Body.String())
	}
	fingerprint := view.UnconfirmedExecutions[0].Fingerprint
	if fingerprint != model.ExecutionExitFingerprint(execution) {
		t.Fatalf("assessment fingerprint = %q, want the seeded execution fingerprint", fingerprint)
	}

	// 正向对照：让退出确认真的走完引擎、租户库与远端执行读取。
	confirmation := doAdminRequest(t, router, http.MethodPost, "/api/admin/tasks/task-pool-a/execution-exit", firstToken, map[string]any{
		"request_id": "request-pool-first", "execution_id": "exec-pool-a",
		"expected_fingerprint": fingerprint, "reason": "已在机器上确认进程退出",
	})
	if confirmation.Code != http.StatusOK {
		t.Fatalf("owner exit confirmation = %d body=%s", confirmation.Code, confirmation.Body.String())
	}
	var stored struct {
		ExecutionID string `json:"execution_id"`
		RemoteState string `json:"remote_state"`
	}
	if err := json.Unmarshal(confirmation.Body.Bytes(), &stored); err != nil {
		t.Fatalf("decode exit confirmation: %v", err)
	}
	if stored.ExecutionID != "exec-pool-a" || stored.RemoteState != model.ExecutionExitRemoteTerminal {
		t.Fatalf("unexpected exit confirmation: %s", confirmation.Body.String())
	}

	before := readTenantRecoveryState(t, resolver, firstTenant.ID, "task-pool-a")
	crossTenantBodies := map[string]map[string]any{
		"recovery": {
			"request_id": "request-pool-second", "block_revision": view.BlockRevision, "reason": "跨租户尝试恢复",
		},
		"execution-exit": {
			"request_id": "request-pool-second-exit", "execution_id": "exec-pool-a",
			"expected_fingerprint": fingerprint, "reason": "跨租户确认退出",
		},
	}
	crossTenantChecks := []struct {
		method, path, bodyKey string
	}{
		{method: http.MethodGet, path: recoveryPath},
		{method: http.MethodPost, path: recoveryPath, bodyKey: "recovery"},
		{method: http.MethodPost, path: "/api/admin/tasks/task-pool-a/execution-exit", bodyKey: "execution-exit"},
	}
	for _, check := range crossTenantChecks {
		var body any
		if check.bodyKey != "" {
			body = crossTenantBodies[check.bodyKey]
		}
		response := doAdminRequest(t, router, check.method, check.path, secondToken, body)
		if response.Code != http.StatusNotFound {
			t.Fatalf("cross tenant %s %s = %d body=%s", check.method, check.path, response.Code, response.Body.String())
		}
		var failure struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil || failure.Error.Code != "not_found" {
			t.Fatalf("cross tenant %s %s did not report a missing task: %s", check.method, check.path, response.Body.String())
		}
	}

	after := readTenantRecoveryState(t, resolver, firstTenant.ID, "task-pool-a")
	if after != before {
		t.Fatalf("cross tenant requests touched the owner store: before=%+v after=%+v", before, after)
	}
}
