package api

import (
	"context"
	"net/http"
	"testing"

	"sdlc-coordinator/internal/admin"
	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/tenantdb"

	"github.com/gin-gonic/gin"
)

type fixedTenantResolver struct {
	main   *tenantdb.Store
	tenant *tenantdb.Store
}

func (resolver fixedTenantResolver) Main() *tenantdb.Store { return resolver.main }

func (resolver fixedTenantResolver) Handle(_ context.Context, tenantID uint) (*tenantdb.Store, error) {
	if tenantID == 0 {
		return nil, tenantdb.ErrInvalidTenantID
	}
	return resolver.tenant, nil
}

func (resolver fixedTenantResolver) Invalidate(uint) {}

func TestAdminQueryUsesResolvedTenantStoreNotConstructorDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fallbackDB := openHTTPTestDB(t)
	tenantDB := openHTTPTestDB(t)
	if err := store.Migrate(fallbackDB, &model.Task{}); err != nil {
		t.Fatalf("migrate fallback: %v", err)
	}
	if err := store.Migrate(tenantDB, &model.Task{}); err != nil {
		t.Fatalf("migrate tenant: %v", err)
	}
	if err := tenantDB.Create(&model.Task{
		TaskID: "task-tenant-store", DedupeKey: "task-tenant-store", Type: "test", Role: "TesterWorker", Status: model.TaskStatusPending,
	}).Error; err != nil {
		t.Fatalf("seed tenant task: %v", err)
	}

	cpRepo := controlplane.NewRepository(fallbackDB)
	if err := cpRepo.Migrate(); err != nil {
		t.Fatalf("migrate control plane: %v", err)
	}
	cp := controlplane.NewService(cpRepo)
	resolver := fixedTenantResolver{
		main:   &tenantdb.Store{DB: fallbackDB},
		tenant: &tenantdb.Store{DB: tenantDB},
	}
	router := NewRouter(nil,
		WithAdminQueryService(admin.NewQueryService(fallbackDB)),
		WithControlPlaneService(cp),
		WithTenantStoreResolver(resolver),
	)
	world := &fullyWiredWorld{Router: router, ControlPlane: cp}
	tenantID := world.seedTenant(t, "acme")
	token := world.seedUserWithRole(t, tenantID, "t3-member", controlplane.RoleMember)

	resp := world.portalDo(t, http.MethodGet, "/api/admin/tasks?page=1&page_size=10", token, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	body := decodeJSONMap(t, resp)
	total, _ := body["total"].(float64)
	if total != 1 {
		t.Fatalf("query must read the resolved tenant store (want total=1), got %#v", body)
	}
}
