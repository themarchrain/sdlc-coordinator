package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/configuration"

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
	application, err := NewReady(withTestControlPlane(t, cfg), zap.NewNop(), openAppTestDB(t), nil, nil)
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

func TestNewReadyRejectsMissingControlPlaneDSN(t *testing.T) {
	_, err := NewReady(config.Config{
		HTTPAddr: ":0", AgentRunner: config.AgentRunnerSettings{InternalToken: "app-test-internal-token"},
		ResultChannelSecret: strings.Repeat("01", 32),
	}, zap.NewNop(), openAppTestDB(t), nil, nil)
	if err == nil {
		t.Fatal("expected missing CONTROL_PLANE_DSN to fail startup")
	}
	if !strings.Contains(err.Error(), "CONTROL_PLANE_DSN") {
		t.Fatalf("err = %v, want CONTROL_PLANE_DSN in message", err)
	}
}

func TestNewReadyRejectsMissingSuperAdminSeed(t *testing.T) {
	dir, err := os.MkdirTemp("", "control-plane-noseed")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	_, err = NewReady(config.Config{
		HTTPAddr: ":0", AgentRunner: config.AgentRunnerSettings{InternalToken: "app-test-internal-token"},
		ResultChannelSecret: strings.Repeat("01", 32),
		ControlPlane: config.ControlPlaneSettings{
			DSN: "file:" + filepath.Join(dir, "control-plane.db") + "?_busy_timeout=1000",
		},
	}, zap.NewNop(), openAppTestDB(t), nil, nil)
	if err == nil {
		t.Fatal("expected missing CONTROL_PLANE_SUPERADMIN_* to fail startup")
	}
	if !strings.Contains(err.Error(), "CONTROL_PLANE_SUPERADMIN") {
		t.Fatalf("err = %v, want CONTROL_PLANE_SUPERADMIN in message", err)
	}
}

func TestNewBootstrapRejectsMissingControlPlaneConfig(t *testing.T) {
	_, err := NewBootstrap(config.BootstrapConfig{HTTPAddr: ":0"}, zap.NewNop(), openAppTestDB(t), nil, nil, configuration.ModeSetupPending)
	if err == nil {
		t.Fatal("expected bootstrap without control plane to fail startup")
	}
	if !strings.Contains(err.Error(), "CONTROL_PLANE") {
		t.Fatalf("err = %v, want CONTROL_PLANE in message", err)
	}
}
