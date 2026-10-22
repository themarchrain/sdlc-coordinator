package app

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
	"sdlc-coordinator/internal/adapters/agentrunner"
	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/store/storetest"
)

type stopIntegrationProvider struct{ started chan struct{} }

func (p *stopIntegrationProvider) Execute(ctx context.Context, _ runner.ProviderRequest, _ runner.EmitFunc) (json.RawMessage, error) {
	close(p.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestTenantEngineStopUsesOriginalRunnerAndConverges(t *testing.T) {
	for _, role := range []string{"DeveloperWorker", "TesterWorker", "ReviewerWorker"} {
		t.Run(role, func(t *testing.T) { tenantEngineStopIntegration(t, false, role) })
	}
}

func TestMySQLTwoCoordinatorsStopOriginalRunner(t *testing.T) {
	tenantEngineStopIntegration(t, true, "DeveloperWorker")
}

func tenantEngineStopIntegration(t *testing.T, useMySQL bool, role string) {
	provider := &stopIntegrationProvider{started: make(chan struct{})}
	original, err := runner.NewService(t.TempDir(), map[string]runner.Provider{"fake": provider})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(runner.NewHTTPHandler(original, runner.WithWorkspaceOperations(nil, "test-token")))
	defer server.Close()
	otherProvider := &stopIntegrationProvider{started: make(chan struct{})}
	wrong, err := runner.NewService(t.TempDir(), map[string]runner.Provider{"fake": otherProvider})
	if err != nil {
		t.Fatal(err)
	}
	fallback := httptest.NewServer(runner.NewHTTPHandler(wrong, runner.WithWorkspaceOperations(nil, "test-token")))
	defer fallback.Close()
	dsn := "file:" + model.NewID("stop_engine") + "?mode=memory&cache=shared"
	if useMySQL {
		dsn = storetest.NewDatabase(t, storetest.RequireMySQL(t), "stop_engines")
	}
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := migrateBusinessStore(db); err != nil {
		t.Fatal(err)
	}
	request := runner.CreateExecutionRequest{ExecutionID: "run-stop", TaskID: "task-stop", Role: role, WorkspacePath: t.TempDir(), PromptFile: "prompt.md", SchemaFile: "schema.json", Provider: runner.ProviderConfig{Name: "fake"}}
	remote, err := original.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	<-provider.started
	if _, err := wrong.Create(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	<-otherProvider.started
	defer func() {
		_, _ = wrong.Cancel(request.ExecutionID)
		waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		wrong.Shutdown(waitCtx)
	}()
	for _, row := range []any{&model.Task{TaskID: request.TaskID, DedupeKey: request.TaskID, Type: "develop_from_issue", Role: request.Role, Status: model.TaskStatusRunning, CurrentRunID: &request.ExecutionID}, &model.Run{RunID: request.ExecutionID, TaskID: request.TaskID, Status: model.RunStatusRunning, RunnerEndpoint: server.URL}, &model.AgentExecution{RunID: request.ExecutionID, ExecutionID: request.ExecutionID, RunnerEndpoint: server.URL, Provider: "fake", RequestHash: remote.RequestHash, Status: model.AgentExecutionStatusRunning}} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	shared := engineShared{cfg: config.Config{AgentRunner: config.AgentRunnerSettings{ServiceURL: fallback.URL, InternalToken: "test-token"}}, logger: zap.NewNop(), defaultRunner: agentrunner.NewClient(fallback.URL, nil)}
	engine, err := buildTenantEngine(db, 7, shared)
	if err != nil {
		t.Fatal(err)
	}
	var secondEngine *tenantEngine
	if useMySQL {
		second, err := store.Open(dsn)
		if err != nil {
			t.Fatal(err)
		}
		secondSQL, _ := second.DB()
		defer secondSQL.Close()
		secondEngine, err = buildTenantEngine(second, 7, shared)
		if err != nil {
			t.Fatal(err)
		}
	}
	result, err := engine.orch.StopTask(context.Background(), request.TaskID, orchestrator.StopActor{UserID: "admin"}, "closed")
	if err != nil {
		t.Fatal(err)
	}
	if result.TaskStatus != model.TaskStatusInterrupted || result.ExitConfirmed {
		t.Fatalf("bad initial result: %+v", result)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if secondEngine != nil {
			if err := secondEngine.orch.ReconcileStops(context.Background(), 20); err != nil {
				t.Fatal(err)
			}
		}
		stop, err := repository.NewStopRepository(db).GetForTask(context.Background(), request.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		if stop.ConfirmationKind == model.StopConfirmationExecutionExited {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("stop failed to converge: %+v", stop)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if other, err := wrong.Get(request.ExecutionID); err != nil || other.Status != runner.StatusRunning {
		t.Fatalf("stop affected unrelated Runner execution: %+v %v", other, err)
	}
	ended, err := original.Get(request.ExecutionID)
	if err != nil || ended.Status != runner.StatusCancelled {
		t.Fatalf("original provider still active: %+v %v", ended, err)
	}
}
