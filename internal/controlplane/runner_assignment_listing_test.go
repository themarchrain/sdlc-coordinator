package controlplane

import (
	"context"
	"testing"
)

// 启动校验消费端需要跨租户全量列举 runner 指派（启动校验与租户池路由的数据面）；
// 既有 ListRunnerAssignments 只按租户列举。
func TestListAllRunnerAssignmentsSpansTenants(t *testing.T) {
	repository := openTestControlPlane(t)
	ctx := context.Background()
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate control plane: %v", err)
	}
	for _, tenant := range []*Tenant{
		{ID: 1, Slug: "acme", DisplayName: "Acme"},
		{ID: 2, Slug: "globex", DisplayName: "Globex"},
	} {
		if err := repository.CreateTenant(ctx, tenant); err != nil {
			t.Fatalf("create tenant %s: %v", tenant.Slug, err)
		}
	}
	if err := repository.ReplaceRunnerAssignments(ctx, 1, []string{"http://runner-a"}); err != nil {
		t.Fatalf("replace assignments for tenant 1: %v", err)
	}
	if err := repository.ReplaceRunnerAssignments(ctx, 2, []string{"http://runner-b", "http://runner-c"}); err != nil {
		t.Fatalf("replace assignments for tenant 2: %v", err)
	}

	rows, err := repository.ListAllRunnerAssignments(ctx)
	if err != nil {
		t.Fatalf("list all assignments: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 assignments across tenants, got %d: %#v", len(rows), rows)
	}
	seen := map[uint]map[string]bool{}
	for _, row := range rows {
		if seen[row.TenantID] == nil {
			seen[row.TenantID] = map[string]bool{}
		}
		seen[row.TenantID][row.EndpointURL] = true
	}
	if !seen[1]["http://runner-a"] || !seen[2]["http://runner-b"] || !seen[2]["http://runner-c"] {
		t.Fatalf("assignments incomplete: %#v", seen)
	}
}
