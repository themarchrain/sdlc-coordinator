package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"sdlc-coordinator/internal/configuration"
	"sdlc-coordinator/internal/orgconfig"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/tenantdb"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func newTenantConfigurationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open("file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "tenant-config.db")) + "?_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := orgconfig.NewStore(db).Migrate(); err != nil {
		t.Fatalf("migrate org config: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func newTenantConfigurationRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db := newTenantConfigurationTestDB(t)
	router := gin.New()
	group := router.Group("/api/admin", func(c *gin.Context) {
		setTenantStore(c, &tenantdb.Store{DB: db})
		c.Next()
	})
	NewAdminTenantConfigurationHandler().RegisterRoutes(group)
	return router, db
}

func performTenantConfigurationRequest(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func tenantConfigurationBody() map[string]any {
	return map[string]any{
		"config": map[string]any{
			"codehost": map[string]any{"provider": "github", "base_url": "https://github.com", "rework_allowed_user_ids": []string{"42"}},
			"im":       map[string]any{"provider": "", "reviewer_mentions": []string{}},
		},
		"secrets":           map[string]any{},
		"expected_revision": 0,
	}
}

func TestTenantConfigurationRoundTrip(t *testing.T) {
	router, _ := newTenantConfigurationRouter(t)

	get := performTenantConfigurationRequest(t, router, http.MethodGet, "/api/admin/tenant-configuration", nil)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"configured":false`) {
		t.Fatalf("initial get: %d %s", get.Code, get.Body.String())
	}

	put := performTenantConfigurationRequest(t, router, http.MethodPut, "/api/admin/tenant-configuration", tenantConfigurationBody())
	if put.Code != http.StatusOK {
		t.Fatalf("put: %d %s", put.Code, put.Body.String())
	}
	after := performTenantConfigurationRequest(t, router, http.MethodGet, "/api/admin/tenant-configuration", nil)
	body := after.Body.String()
	if !strings.Contains(body, `"revision":1`) || !strings.Contains(body, `"configured":true`) {
		t.Fatalf("after get: %s", body)
	}
	if !strings.Contains(body, `"rework_allowed_user_ids":["42"]`) {
		t.Fatalf("payload round trip mismatch: %s", body)
	}

	second := tenantConfigurationBody()
	second["expected_revision"] = 1
	secondPut := performTenantConfigurationRequest(t, router, http.MethodPut, "/api/admin/tenant-configuration", second)
	if secondPut.Code != http.StatusOK || !strings.Contains(secondPut.Body.String(), `"revision":2`) {
		t.Fatalf("second put must bump revision: %d %s", secondPut.Code, secondPut.Body.String())
	}

	// 基于过期 revision 的并发提交必须 409,而不是静默覆盖。
	stale := tenantConfigurationBody()
	stale["expected_revision"] = 1
	stalePut := performTenantConfigurationRequest(t, router, http.MethodPut, "/api/admin/tenant-configuration", stale)
	if stalePut.Code != http.StatusConflict || !strings.Contains(stalePut.Body.String(), "config_conflict") {
		t.Fatalf("stale put must conflict: %d %s", stalePut.Code, stalePut.Body.String())
	}
}

func TestTenantConfigurationValidateRejectsBadPayload(t *testing.T) {
	router, _ := newTenantConfigurationRouter(t)
	body := tenantConfigurationBody()
	config := body["config"].(map[string]any)
	codehost := config["codehost"].(map[string]any)
	codehost["provider"] = "svn"

	response := performTenantConfigurationRequest(t, router, http.MethodPut, "/api/admin/tenant-configuration", body)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "codehost.provider") {
		t.Fatalf("validate: %d %s", response.Code, response.Body.String())
	}
}

func TestTenantConfigurationValidateEndpointReportsValidity(t *testing.T) {
	router, _ := newTenantConfigurationRouter(t)

	valid := performTenantConfigurationRequest(t, router, http.MethodPost, "/api/admin/tenant-configuration/validate", tenantConfigurationBody())
	if valid.Code != http.StatusOK || !strings.Contains(valid.Body.String(), `"valid":true`) {
		t.Fatalf("valid check: %d %s", valid.Code, valid.Body.String())
	}

	body := tenantConfigurationBody()
	config := body["config"].(map[string]any)
	codehost := config["codehost"].(map[string]any)
	codehost["base_url"] = "not-a-url"
	invalid := performTenantConfigurationRequest(t, router, http.MethodPost, "/api/admin/tenant-configuration/validate", body)
	if invalid.Code != http.StatusOK || !strings.Contains(invalid.Body.String(), "codehost.base_url") {
		t.Fatalf("invalid check: %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestTenantConfigurationRejectsUnknownFields(t *testing.T) {
	router, _ := newTenantConfigurationRouter(t)
	body := tenantConfigurationBody()
	body["unknown"] = true

	response := performTenantConfigurationRequest(t, router, http.MethodPut, "/api/admin/tenant-configuration", body)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_request") {
		t.Fatalf("strict decode: %d %s", response.Code, response.Body.String())
	}
}

func TestTenantConfigurationWithoutTenantScopeForbidden(t *testing.T) {
	router := gin.New()
	NewAdminTenantConfigurationHandler().RegisterRoutes(router.Group("/api/admin"))
	response := performTenantConfigurationRequest(t, router, http.MethodGet, "/api/admin/tenant-configuration", nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without tenant store, got %d: %s", response.Code, response.Body.String())
	}
}

func TestTenantConfigurationMasksSecrets(t *testing.T) {
	router, _ := newTenantConfigurationRouter(t)
	body := tenantConfigurationBody()
	config := body["config"].(map[string]any)
	config["im"] = map[string]any{"provider": "telegram", "telegram": map[string]any{"default_chat_id": "-100"}, "reviewer_mentions": []string{}}
	body["secrets"] = map[string]any{"telegram_bot_token": "bot-secret-value"}

	put := performTenantConfigurationRequest(t, router, http.MethodPut, "/api/admin/tenant-configuration", body)
	if put.Code != http.StatusOK {
		t.Fatalf("put with secret: %d %s", put.Code, put.Body.String())
	}
	if strings.Contains(put.Body.String(), "bot-secret-value") {
		t.Fatalf("secret leaked in response: %s", put.Body.String())
	}
	if !strings.Contains(put.Body.String(), `"telegram_bot_token_configured":true`) {
		t.Fatalf("configured flag missing: %s", put.Body.String())
	}
}

// 保存/校验请求体必须有大小上限:超大集合载荷在解码前就拒收。
func TestTenantConfigurationUpdateRejectsOversizedBody(t *testing.T) {
	router, _ := newTenantConfigurationRouter(t)
	request := orgconfig.UpdateRequest{
		Config: orgconfig.EditableConfig{
			CodeHost: configuration.EditableCodeHostConfig{
				Provider: "gitlab", BaseURL: "https://gitlab.example.com",
				// 1 MiB 以上的白名单载荷:内容合法但体积超限。
				ReworkAllowedUserIDs: []string{strings.Repeat("7", tenantConfigMaxRequestBytes)},
			},
		},
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(payload) <= tenantConfigMaxRequestBytes {
		t.Fatalf("test payload must exceed the limit, got %d bytes", len(payload))
	}
	httpRequest := httptest.NewRequest(http.MethodPut, "/api/admin/tenant-configuration", bytes.NewReader(payload))
	httpRequest.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httpRequest)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized body must be rejected with 400, got %d: %s", response.Code, response.Body.String())
	}
}
