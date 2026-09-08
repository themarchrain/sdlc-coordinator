package api

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)


// TestHTTPRouteMatrixCoversEveryRegisteredRoute 锁定完备性红线：现役 HTTP
// 路由必须有矩阵行；漏登即红。
func TestLegacyAuthRoutesAreRemovedFromGinAndCatalog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newFullyWiredRouter(t)
	for _, info := range router.Routes() {
		if strings.HasPrefix(info.Path, "/api/auth") {
			t.Errorf("legacy auth still registered: %s %s", info.Method, info.Path)
		}
	}
	login := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"admin123"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(login, req)
	if login.Code != http.StatusNotFound {
		t.Fatalf("POST /api/auth/login = %d, want 404 (removed, not 410)", login.Code)
	}
	session := httptest.NewRecorder()
	router.ServeHTTP(session, httptest.NewRequest(http.MethodGet, "/api/auth/session", nil))
	if session.Code != http.StatusNotFound {
		t.Fatalf("GET /api/auth/session = %d, want 404 (removed, not 410)", session.Code)
	}
	for _, row := range HTTPRouteMatrix() {
		if strings.HasPrefix(row.Path, "/api/auth") {
			t.Errorf("catalog still lists %s %s", row.Method, row.Path)
		}
	}
}

func TestHTTPRouteMatrixCoversEveryRegisteredRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newFullyWiredRouter(t)

	registered := map[string]struct{}{}
	var ginKeys []string
	for _, info := range router.Routes() {
		key := routeKey(info.Method, info.Path)
		if _, exists := registered[key]; exists {
			continue
		}
		registered[key] = struct{}{}
		ginKeys = append(ginKeys, key)
	}
	sort.Strings(ginKeys)

	catalog := map[string]MatrixRow{}
	var catalogKeys []string
	for _, row := range HTTPRouteMatrix() {
		key := routeKey(row.Method, row.Path)
		if prev, exists := catalog[key]; exists {
			t.Fatalf("duplicate matrix row %s (class %s vs %s)", key, prev.Class, row.Class)
		}
		catalog[key] = row
		catalogKeys = append(catalogKeys, key)
	}
	sort.Strings(catalogKeys)

	var missing []string
	for _, key := range ginKeys {
		if _, ok := catalog[key]; !ok {
			missing = append(missing, key)
		}
	}
	var extra []string
	for _, key := range catalogKeys {
		if _, ok := registered[key]; !ok {
			extra = append(extra, key)
		}
	}
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("route matrix mismatch\nmissing from catalog (%d):\n  %s\nextra in catalog (%d):\n  %s",
			len(missing), strings.Join(missing, "\n  "),
			len(extra), strings.Join(extra, "\n  "))
	}
}

func TestFrozenRuntimeWebhookAndMCPRoutesStayFrozen(t *testing.T) {
	for _, row := range HTTPRouteMatrix() {
		switch row.Class {
		case ClassRuntime, ClassWebhook, ClassMCP:
			if !row.Frozen {
				t.Errorf("%s %s: non-human channel must be frozen", row.Method, row.Path)
			}
			if row.CurrentMiddleware != row.TargetMiddleware {
				t.Errorf("%s %s: frozen route must not change middleware (%q -> %q)",
					row.Method, row.Path, row.CurrentMiddleware, row.TargetMiddleware)
			}
			if row.NonHuman == ChannelNone {
				t.Errorf("%s %s: frozen route must declare a non-human channel", row.Method, row.Path)
			}
		default:
			if row.Frozen {
				t.Errorf("%s %s: only runtime/webhook/mcp may be frozen", row.Method, row.Path)
			}
		}
	}
}

func TestSPAPageMatrixCoversKnownAdminPages(t *testing.T) {
	want := []string{
		"GET /admin/login",
		"GET /admin/",
		"GET /admin/setup",
		"GET /admin/settings",
		"GET /admin/people",
		"GET /admin/requirements",
		"GET /admin/repositories",
		"GET /admin/tasks",
		"GET /admin/explorer",
		"GET /admin/connectors",
		"GET /admin/operations",
		"GET /admin/agents",
		"GET /admin/portal/login",
		"GET /admin/portal/register",
		"GET /admin/portal/select-tenant",
		"GET /admin/portal/change-password",
		"GET /admin/org",
		"GET /admin/super-admin/tenants",
		"GET /admin/super-admin/tenants/:id",
		"GET /admin/super-admin/audit-log",
	}
	got := map[string]struct{}{}
	for _, row := range SPARouteMatrix() {
		got[routeKey(row.Method, row.Path)] = struct{}{}
	}
	var missing []string
	for _, key := range want {
		if _, ok := got[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("SPA matrix missing pages: %s", strings.Join(missing, ", "))
	}
}


func TestAdminClassCurrentMiddlewareMatchesTarget(t *testing.T) {
	for _, row := range HTTPRouteMatrix() {
		if row.Class != ClassAdmin {
			continue
		}
		if row.CurrentMiddleware != row.TargetMiddleware {
			t.Errorf("%s %s: admin route still on %q, want portal matrix %q",
				row.Method, row.Path, row.CurrentMiddleware, row.TargetMiddleware)
		}
	}
}

func newFullyWiredRouter(t *testing.T) *gin.Engine {
	t.Helper()
	return newFullyWiredWorld(t).Router
}
