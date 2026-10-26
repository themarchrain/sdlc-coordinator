package api

import (
	"context"
	"net/http"
	"sdlc-coordinator/internal/controlplane"
	"testing"
)

func TestAdminStopRoleMatrix(t *testing.T) {
	w := newFullyWiredWorld(t)
	tenant := w.seedTenant(t, "stop-auth")
	admin := w.seedUserWithRole(t, tenant, "stop-admin", controlplane.RoleTenantAdmin)
	member := w.seedUserWithRole(t, tenant, "stop-member", controlplane.RoleMember)
	super := w.loginSuperAdmin(t)
	for _, path := range []string{"/tasks/task/stop", "/runs/run/stop", "/agents/executions/executions/execution/stop"} {
		for _, tc := range []struct {
			token  string
			status int
		}{{"", 401}, {member, 403}, {super, 403}, {admin, 503}} {
			response := w.portalDo(t, http.MethodPost, "/api/admin"+path, tc.token, map[string]string{})
			if response.Code != tc.status {
				t.Fatalf("stop permission: %d want=%d body=%s", response.Code, tc.status, response.Body.String())
			}
		}
	}
	if err := w.ControlPlane.EnterSupportAccess(context.Background(), super, tenant, "stop support"); err != nil {
		t.Fatal(err)
	}
	response := w.portalDo(t, http.MethodPost, "/api/admin/tasks/task/stop", super, map[string]string{})
	if response.Code != 503 {
		t.Fatalf("support could not reach tenant stop: %d %s", response.Code, response.Body.String())
	}
}
