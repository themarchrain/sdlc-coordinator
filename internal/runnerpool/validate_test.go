package runnerpool

import (
	"strings"
	"testing"
)

// 启动校验（头条）：三类形态任一命中即拒绝启动（fail-fast）；
// 空表=合法放行，三类校验仅对已存在的行生效。
func TestValidateAssignments(t *testing.T) {
	t.Run("empty assignment table is legal", func(t *testing.T) {
		if err := ValidateAssignments(nil, map[uint]bool{}); err != nil {
			t.Fatalf("empty table must pass (A2): %v", err)
		}
	})

	t.Run("valid single-tenant multi-endpoint pool passes", func(t *testing.T) {
		assignments := []AssignmentShape{
			{TenantID: 7, EndpointURL: "http://runner-a:8081"},
			{TenantID: 7, EndpointURL: "https://runner-b:8081"},
		}
		if err := ValidateAssignments(assignments, map[uint]bool{7: true}); err != nil {
			t.Fatalf("valid assignments must pass: %v", err)
		}
	})

	t.Run("endpoint shared across tenants is rejected", func(t *testing.T) {
		assignments := []AssignmentShape{
			{TenantID: 7, EndpointURL: "http://shared-runner:8081"},
			{TenantID: 8, EndpointURL: "http://shared-runner:8081"},
		}
		err := ValidateAssignments(assignments, map[uint]bool{7: true, 8: true})
		if err == nil || !strings.Contains(err.Error(), "multiple tenants") {
			t.Fatalf("cross-tenant endpoint must be rejected, got %v", err)
		}
	})

	t.Run("assignment to missing tenant is rejected", func(t *testing.T) {
		assignments := []AssignmentShape{{TenantID: 99, EndpointURL: "http://runner-a:8081"}}
		err := ValidateAssignments(assignments, map[uint]bool{7: true})
		if err == nil || !strings.Contains(err.Error(), "missing tenant") {
			t.Fatalf("orphan assignment must be rejected, got %v", err)
		}
	})

	t.Run("endpoint without scheme is rejected", func(t *testing.T) {
		assignments := []AssignmentShape{{TenantID: 7, EndpointURL: "runner-a:8081"}}
		err := ValidateAssignments(assignments, map[uint]bool{7: true})
		if err == nil || !strings.Contains(err.Error(), "http") {
			t.Fatalf("scheme-less endpoint must be rejected, got %v", err)
		}
	})

	t.Run("endpoint without host is rejected", func(t *testing.T) {
		assignments := []AssignmentShape{{TenantID: 7, EndpointURL: "http://"}}
		err := ValidateAssignments(assignments, map[uint]bool{7: true})
		if err == nil || !strings.Contains(err.Error(), "host") {
			t.Fatalf("host-less endpoint must be rejected, got %v", err)
		}
	})
}
