package config

// BootstrapConfig contains only the fixed settings needed to start logging,
// open the database, and expose the configuration UI.
type BootstrapConfig struct {
	HTTPAddr    string
	DatabaseDSN string
	LogLevel    string
	LogDir      string
	Environment string
}

func LoadBootstrap() BootstrapConfig {
	return BootstrapConfig{
		HTTPAddr:    getEnv("COORDINATOR_HTTP_ADDR", ":8080"),
		DatabaseDSN: getEnv("COORDINATOR_DATABASE_DSN", "file:sdlc-coordinator.db?_foreign_keys=on&_busy_timeout=5000"),
		LogLevel:    getEnv("COORDINATOR_LOG_LEVEL", "info"),
		LogDir:      getEnv("COORDINATOR_LOG_DIR", "logs"),
		Environment: getEnv("COORDINATOR_ENVIRONMENT", "development"),
	}
}
