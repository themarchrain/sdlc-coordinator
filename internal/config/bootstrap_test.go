package config

import "testing"

func TestLoadBootstrapUsesOnlyFixedStartupSettings(t *testing.T) {
	t.Setenv("COORDINATOR_HTTP_ADDR", ":9090")
	t.Setenv("COORDINATOR_DATABASE_DSN", "file:test.db")
	t.Setenv("COORDINATOR_LOG_LEVEL", "debug")
	t.Setenv("COORDINATOR_LOG_DIR", "var/log/coordinator")
	t.Setenv("COORDINATOR_ENVIRONMENT", "production")

	got := LoadBootstrap()

	if got.HTTPAddr != ":9090" || got.DatabaseDSN != "file:test.db" {
		t.Fatalf("unexpected bootstrap endpoints: %#v", got)
	}
	if got.LogLevel != "debug" || got.LogDir != "var/log/coordinator" || got.Environment != "production" {
		t.Fatalf("unexpected bootstrap runtime: %#v", got)
	}
}

func TestLoadBootstrapUsesSafeDefaults(t *testing.T) {
	for _, key := range []string{
		"COORDINATOR_HTTP_ADDR",
		"COORDINATOR_DATABASE_DSN",
		"COORDINATOR_LOG_LEVEL",
		"COORDINATOR_LOG_DIR",
		"COORDINATOR_ENVIRONMENT",
	} {
		t.Setenv(key, "")
	}

	got := LoadBootstrap()

	if got.HTTPAddr != ":8080" {
		t.Fatalf("expected default HTTP address, got %q", got.HTTPAddr)
	}
	if got.DatabaseDSN != "file:sdlc-coordinator.db?_foreign_keys=on&_busy_timeout=5000" {
		t.Fatalf("unexpected default DSN %q", got.DatabaseDSN)
	}
	if got.LogLevel != "info" || got.LogDir != "logs" || got.Environment != "development" {
		t.Fatalf("unexpected defaults: %#v", got)
	}
}
