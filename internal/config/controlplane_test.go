package config

import "testing"

func TestLoadControlPlaneSettings(t *testing.T) {
	t.Setenv("CONTROL_PLANE_DSN", "file:control-plane-test.db")
	t.Setenv("CONTROL_PLANE_SUPERADMIN_USERNAME", "Root")
	t.Setenv("CONTROL_PLANE_SUPERADMIN_PASSWORD", "test-seed-password")
	t.Setenv("CONTROL_PLANE_SUPERADMIN_RESET", "true")

	cfg := Load()
	settings := cfg.ControlPlane
	if settings.DSN != "file:control-plane-test.db" {
		t.Fatalf("unexpected control plane dsn %q", settings.DSN)
	}
	if settings.SuperAdminUsername != "root" {
		t.Fatalf("expected normalized username root, got %q", settings.SuperAdminUsername)
	}
	if settings.SuperAdminPassword != "test-seed-password" {
		t.Fatalf("unexpected seed password %q", settings.SuperAdminPassword)
	}
	if !settings.SuperAdminReset {
		t.Fatal("expected reset flag enabled")
	}
}

func TestLoadControlPlaneSettingsDefaultToDisabled(t *testing.T) {
	cfg := Load()
	if cfg.ControlPlane.DSN != "" {
		t.Fatalf("expected empty dsn default, got %q", cfg.ControlPlane.DSN)
	}
	if cfg.ControlPlane.SuperAdminUsername != "" || cfg.ControlPlane.SuperAdminPassword != "" {
		t.Fatalf("expected no seed defaults, got %+v", cfg.ControlPlane)
	}
	if cfg.ControlPlane.SuperAdminReset {
		t.Fatal("expected reset disabled by default")
	}
}
