package app

import (
	"strings"
	"testing"

	"sdlc-coordinator/internal/config"

	"go.uber.org/zap"
)

func TestNewControlPlaneRejectsEmptyDSN(t *testing.T) {
	_, err := newControlPlane(config.ControlPlaneSettings{}, zap.NewNop())
	if err == nil {
		t.Fatal("expected empty CONTROL_PLANE_DSN to fail assembly")
	}
	if !strings.Contains(err.Error(), "CONTROL_PLANE_DSN") {
		t.Fatalf("err = %v, want CONTROL_PLANE_DSN in message", err)
	}
}
