package app

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/controlplane"

	"go.uber.org/zap"
)

// F3（审查发现）：控制面 DSN 为空时回退 file::memory:?cache=shared，
// 同进程多个实例会共享同一内存库（跨测试/跨实例状态串扰）。
// 修复后每次空 DSN 打开都得到独立内存库。
func TestNewControlPlaneEmptyDSNInstancesAreIsolated(t *testing.T) {
	settings := config.ControlPlaneSettings{}
	first, err := newControlPlane(settings, zap.NewNop())
	if err != nil {
		t.Fatalf("open first control plane: %v", err)
	}
	second, err := newControlPlane(settings, zap.NewNop())
	if err != nil {
		t.Fatalf("open second control plane: %v", err)
	}

	ctx := context.Background()
	if err := first.SeedSuperAdmin(ctx, controlplane.SeedSuperAdminInput{
		Username: "root", Password: "first-instance-pass",
	}); err != nil {
		t.Fatalf("seed first instance: %v", err)
	}

	// 第二实例不得看到第一实例的账号（共享内存库即失败）
	if _, err := second.Login(ctx, "root", "first-instance-pass", "10.0.0.1"); err == nil {
		t.Fatal("empty-DSN control plane instances must be isolated, second instance saw first instance's account")
	}
}
