package app

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/configuration"
	"sdlc-coordinator/internal/http/handlers"
)

// 生产装配路径验证(对策:防止测试 Nop 回退掩盖生产漏注入):
// NewReady 构造的 MCP handler 拿到的必须是传入 App 的那个共享 logger 实例,
// 不是 Nop 也不是新造;bootstrap 模式不创建 MCP handler(setup 阶段无业务
// 路由,handler 只在 ready 装配)。

// readyModeMCPHandler 从 NewReady 装配产物中提取 MCP handler(经
// WithMCPHandler 注入 router options,handlers 包外只经 router 可达)。
func readyModeMCPHandler(application *App) *handlers.MCPHandler {
	return application.mcpHandler
}

func TestReadyAssemblyInjectsSharedLoggerIntoMCPHandler(t *testing.T) {
	db := openAppTestDB(t)
	core, _ := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)

	application, err := NewReady(withTestControlPlane(t, config.Config{HTTPAddr: ":0"}), logger, db, nil, nil)
	if err != nil {
		t.Fatalf("new ready app: %v", err)
	}
	defer application.pool.CloseTenantEngines()

	handler := readyModeMCPHandler(application)
	if handler == nil {
		t.Fatal("ready assembly must create the MCP handler")
	}
	// 指针相等:同一实例,非复制、非 Nop。
	if handler.GetLogger() != logger {
		t.Fatal("MCP handler must receive the app's shared logger instance")
	}
}

func TestBootstrapAssemblyDoesNotCreateMCPHandler(t *testing.T) {
	db := openAppTestDB(t)
	repository := configuration.NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate configuration: %v", err)
	}
	service := configuration.NewService(repository, configuration.Resolution{Mode: configuration.ModeSetupPending})
	application, err := NewBootstrap(withTestBootstrap(t, config.BootstrapConfig{HTTPAddr: ":0"}), zap.NewNop(), db, service, configuration.NewShutdownSignal(), configuration.ModeSetupPending)
	if err != nil {
		t.Fatalf("new bootstrap app: %v", err)
	}
	if readyModeMCPHandler(application) != nil {
		t.Fatal("bootstrap assembly must not create the MCP handler")
	}
}
