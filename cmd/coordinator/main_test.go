package main

import (
	"testing"

	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/configuration"
	"sdlc-coordinator/internal/store"

	"go.uber.org/zap"
)

func TestBuildApplicationSelectsBootstrapModeWithoutRuntimeWorkers(t *testing.T) {
	db, err := store.Open("file:main-bootstrap?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repository := configuration.NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate config: %v", err)
	}
	resolution := configuration.Resolution{Mode: configuration.ModeSetupPending}
	service := configuration.NewService(repository, resolution)

	application, err := buildApplication(config.BootstrapConfig{HTTPAddr: ":0"}, resolution, service, configuration.NewShutdownSignal(), zap.NewNop(), db)

	if err != nil {
		t.Fatalf("build application: %v", err)
	}
	if application.Mode != configuration.ModeSetupPending || application.Orch != nil || application.Scheduler != nil {
		t.Fatalf("unexpected bootstrap application: %#v", application)
	}
}
