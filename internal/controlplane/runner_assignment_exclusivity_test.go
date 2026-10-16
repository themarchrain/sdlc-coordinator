package controlplane

import (
	"context"
	"errors"
	"testing"

	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"
)

func TestReplaceRunnerAssignmentsRejectsEndpointSharedAcrossTenants(t *testing.T) {
	repository := openTestControlPlane(t)
	if err := repository.Migrate(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, tenant := range []*Tenant{
		{ID: 1, Slug: "acme", DisplayName: "Acme"},
		{ID: 2, Slug: "globex", DisplayName: "Globex"},
	} {
		if err := repository.CreateTenant(ctx, tenant); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(repository)
	endpoint := "http://runner-a:8081"
	if err := service.ReplaceRunnerAssignments(ctx, nil, 1, []string{endpoint}); err != nil {
		t.Fatal(err)
	}
	if err := service.ReplaceRunnerAssignments(ctx, nil, 2, []string{endpoint}); !errors.Is(err, ErrRunnerEndpointAssigned) {
		t.Fatalf("cross-tenant assignment error = %v, want ErrRunnerEndpointAssigned", err)
	}
	for _, item := range []struct {
		tenantID uint
		want     int
	}{{1, 1}, {2, 0}} {
		rows, err := repository.ListRunnerAssignments(ctx, item.tenantID)
		if err != nil || len(rows) != item.want {
			t.Fatalf("tenant %d assignments = %v, error = %v", item.tenantID, rows, err)
		}
	}
}

// MySQL 上的独占裁决：唯一索引冲突错误须穿过 service 事务边界仍能被
// IsDuplicateKey 识别并翻译为 ErrRunnerEndpointAssigned（防未来更换错误
// 包裹方式时静默退化）。
func TestReplaceRunnerAssignmentsRejectsEndpointSharedAcrossTenantsOnMySQL(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	db := storetest.OpenDatabase(t, admin, "cp_endpoint_excl")
	repository := NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, tenant := range []*Tenant{
		{ID: 1, Slug: "acme", DisplayName: "Acme"},
		{ID: 2, Slug: "globex", DisplayName: "Globex"},
	} {
		if err := repository.CreateTenant(ctx, tenant); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(repository)
	endpoint := "http://runner-a:8081"
	if err := service.ReplaceRunnerAssignments(ctx, nil, 1, []string{endpoint}); err != nil {
		t.Fatal(err)
	}
	err := service.ReplaceRunnerAssignments(ctx, nil, 2, []string{endpoint})
	if !errors.Is(err, ErrRunnerEndpointAssigned) {
		t.Fatalf("cross-tenant assignment error = %v, want ErrRunnerEndpointAssigned", err)
	}
	if !store.IsDuplicateKey(err) {
		t.Fatalf("error must stay classifiable as duplicate key, got %v", err)
	}
	rows, err := repository.ListRunnerAssignments(ctx, 2)
	if err != nil || len(rows) != 0 {
		t.Fatalf("tenant 2 assignments = %v, error = %v", rows, err)
	}
}
