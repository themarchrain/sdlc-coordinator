package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"sdlc-coordinator/internal/config"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestNewReadyWiresControlPlaneAndSeedsSuperAdmin(t *testing.T) {
	// 自管临时目录：控制面 sqlite 句柄随进程存活（生产形态），TempDir 的
	// Windows 强制清理会因句柄未关而失败，这里尽力删除即可。
	dir, err := os.MkdirTemp("", "control-plane-test")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	dsn := "file:" + filepath.Join(dir, "control-plane.db") + "?_busy_timeout=1000"
	cfg := config.Config{
		HTTPAddr: ":0",
		ControlPlane: config.ControlPlaneSettings{
			DSN:                dsn,
			SuperAdminUsername: "root",
			SuperAdminPassword: "test-seed-password",
		},
	}
	application, err := NewReady(cfg, zap.NewNop(), openAppTestDB(t), nil, nil)
	if err != nil {
		t.Fatalf("new ready app: %v", err)
	}
	// 通过完整路由登录种子超管
	gin.SetMode(gin.TestMode)
	body, _ := json.Marshal(map[string]string{"username": "root", "password": "test-seed-password"})
	recorder := httptest.NewRecorder()
	application.Router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/portal/auth/login", bytes.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("seed super admin login = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestNewReadyWithoutControlPlaneConfigStartsClean(t *testing.T) {
	application, err := NewReady(config.Config{HTTPAddr: ":0"}, zap.NewNop(), openAppTestDB(t), nil, nil)
	if err != nil {
		t.Fatalf("new ready app: %v", err)
	}
	// 未配置时登录端点仍在（:memory: 控制面），但无种子账号 → 401
	gin.SetMode(gin.TestMode)
	body, _ := json.Marshal(map[string]string{"username": "root", "password": "whatever-pass"})
	recorder := httptest.NewRecorder()
	application.Router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/portal/auth/login", bytes.NewReader(body)))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without seed, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}
