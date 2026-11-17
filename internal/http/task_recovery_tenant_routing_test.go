package api

import (
	"context"
	"net/http"
	"testing"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/http/handlers"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// tenantRecoveryEngines 按会话租户返回各租户的恢复引擎，替身生产引擎池的选库端口。
type tenantRecoveryEngines map[uint]handlers.AdminOperations

func (e tenantRecoveryEngines) OperationsFor(c *gin.Context) handlers.AdminOperations {
	session := handlers.CurrentPortalSession(c)
	if session == nil || session.EffectiveTenant == nil {
		return nil
	}
	return e[session.EffectiveTenant.ID]
}

func (e tenantRecoveryEngines) EffectsFor(*gin.Context) handlers.AdminEffectLookup {
	return nil
}

func openRecoveryRoutingDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openHTTPTestDB(t)
	if err := store.Migrate(db, &model.StopRequest{}, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.TaskRecovery{},
		&model.WorkItemLease{}, &model.ActionAdmission{}, &model.ResultSubmission{}, &model.Writeback{},
		&model.Effect{}, &model.ExecutionExitConfirmation{}, &model.DeveloperDeliveryCheckpoint{}, &model.MergeGateAudit{}); err != nil {
		t.Fatalf("migrate tenant db: %v", err)
	}
	return db
}

func newRecoveryRoutingEngine(t *testing.T, db *gorm.DB) *orchestrator.Orchestrator {
	t.Helper()
	recoveries := repository.NewTaskRecoveryRepository(db)
	return orchestrator.New(repository.NewTaskRepository(db), repository.NewRunRepository(db),
		orchestrator.WithTaskRecoveryRepository(recoveries),
		orchestrator.WithRecoveryFactsReader(&orchestrator.RecoverySafetyReader{Store: recoveries}),
	)
}

func TestAdminRecoveryResolvesTheStoreOfTheSessionTenant(t *testing.T) {
	firstDB := openRecoveryRoutingDB(t)
	secondDB := openRecoveryRoutingDB(t)
	engines := tenantRecoveryEngines{}
	world := newFullyWiredWorld(t, WithAdminOperationEngines(engines))
	firstTenant := world.seedTenant(t, "routing-first")
	secondTenant := world.seedTenant(t, "routing-second")
	engines[firstTenant] = newRecoveryRoutingEngine(t, firstDB)
	engines[secondTenant] = newRecoveryRoutingEngine(t, secondDB)
	firstToken := world.seedUserWithRole(t, firstTenant, "routing-first-admin", controlplane.RoleTenantAdmin)
	secondToken := world.seedUserWithRole(t, secondTenant, "routing-second-admin", controlplane.RoleTenantAdmin)

	// 任务只存在于第一个租户的库里。
	task := &model.Task{
		TaskID: "task-routing-first", DedupeKey: "routing-first", Type: "bugfix", Role: "DeveloperWorker",
		Status: model.TaskStatusBlocked, Objective: "first tenant only",
	}
	if err := repository.NewTaskRepository(firstDB).Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	path := "/api/admin/tasks/" + task.TaskID + "/recovery"

	if response := world.portalDo(t, http.MethodGet, path, firstToken, nil); response.Code != http.StatusOK {
		t.Fatalf("owner tenant read = %d body=%s", response.Code, response.Body.String())
	}
	if response := world.portalDo(t, http.MethodGet, path, secondToken, nil); response.Code != http.StatusNotFound {
		t.Fatalf("cross tenant read = %d body=%s", response.Code, response.Body.String())
	}
	if response := world.portalDo(t, http.MethodPost, path, secondToken, map[string]any{
		"request_id": "request-cross-tenant", "block_revision": "revision",
	}); response.Code != http.StatusNotFound {
		t.Fatalf("cross tenant recovery = %d body=%s", response.Code, response.Body.String())
	}

	// 跨租户的读写都没有碰到任务所有者的库。
	current, err := repository.NewTaskRepository(firstDB).GetByTaskID(context.Background(), task.TaskID)
	if err != nil || current.Status != model.TaskStatusBlocked {
		t.Fatalf("cross tenant recovery touched the owner task: %+v err=%v", current, err)
	}
}
