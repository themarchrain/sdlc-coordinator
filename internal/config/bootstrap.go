package config

// BootstrapConfig 是启动日志、打开业务库、挂配置 UI 的固定设置；控制面
// DSN 与种子超管同属此层（setup 向导与 ready 共用装配）。
type BootstrapConfig struct {
	HTTPAddr     string
	DatabaseDSN  string
	LogLevel     string
	LogDir       string
	Environment  string
	ControlPlane ControlPlaneSettings
}

func LoadBootstrap() BootstrapConfig {
	return BootstrapConfig{
		HTTPAddr:     getEnv("COORDINATOR_HTTP_ADDR", ":8080"),
		DatabaseDSN:  getEnv("COORDINATOR_DATABASE_DSN", "file:sdlc-coordinator.db?_foreign_keys=on&_busy_timeout=5000"),
		LogLevel:     getEnv("COORDINATOR_LOG_LEVEL", "info"),
		LogDir:       getEnv("COORDINATOR_LOG_DIR", "logs"),
		Environment:  getEnv("COORDINATOR_ENVIRONMENT", "development"),
		ControlPlane: loadControlPlaneSettings(),
	}
}
