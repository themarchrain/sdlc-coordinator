package configuration

import (
	"context"
	"testing"
	"time"
)

// TestCrossInstanceRevisionConvergence 双实例共享同一快照表（模拟 MySQL 主库）：
// 实例 A 保存 revision 2 后,实例 B 启动解析必读到 revision 2 的配置——
// 后启动成员按定义收敛到最新生效版本,不出现副本永久保留旧授权。
func TestCrossInstanceRevisionConvergence(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)

	// 实例 A：首启导入 legacy（revision 1）,随后 UI 更新（revision 2）。
	serviceA := NewService(repository, NewResolver(repository).Resolve(context.Background(), testBootstrapConfig(), completeLegacyConfig()))
	updated := EditableFromRuntime(serviceA.Runtime())
	updated.AgentRunner.Model = "model-from-instance-a"
	result, _, err := serviceA.Update(context.Background(), UpdateRequest{Config: updated, ExpectedRevision: uint64Pointer(1)})
	if err != nil {
		t.Fatalf("instance A update: %v", err)
	}
	if result.Revision != 2 {
		t.Fatalf("instance A revision = %d, want 2", result.Revision)
	}

	// 实例 B：晚于 A 启动,解析同一张表。
	resolutionB := NewResolver(repository).Resolve(context.Background(), testBootstrapConfig(), completeLegacyConfig())
	if resolutionB.Mode != ModeReady || resolutionB.Snapshot == nil {
		t.Fatalf("instance B resolution: %#v", resolutionB)
	}
	if resolutionB.Snapshot.Revision != 2 {
		t.Fatalf("instance B revision = %d, want 2 (latest wins)", resolutionB.Snapshot.Revision)
	}
	if resolutionB.EffectiveConfig.AgentRunner.Model != "model-from-instance-a" {
		t.Fatalf("instance B config = %q, want the value saved by instance A", resolutionB.EffectiveConfig.AgentRunner.Model)
	}
}

func TestWatchForNewerRevisionRequestsShutdown(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	first, err := repository.Save(context.Background(), StatusReady, SourceUI, validRuntimeConfig())
	if err != nil {
		t.Fatalf("seed configuration: %v", err)
	}
	shutdown := NewShutdownSignal()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go WatchForNewerRevision(ctx, repository, first.Revision, shutdown, 10*time.Millisecond, nil)

	updated := validRuntimeConfig()
	updated.AgentRunner.Model = "newer-model"
	if _, err := repository.Save(context.Background(), StatusRestartRequired, SourceUI, updated); err != nil {
		t.Fatalf("save newer configuration: %v", err)
	}
	select {
	case <-shutdown.Done():
	case <-time.After(time.Second):
		t.Fatal("watcher did not request shutdown for a newer revision")
	}
}

func TestWatchForNewerRevisionKeepsSameRevisionRunning(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	first, err := repository.Save(context.Background(), StatusReady, SourceUI, validRuntimeConfig())
	if err != nil {
		t.Fatalf("seed configuration: %v", err)
	}
	shutdown := NewShutdownSignal()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go WatchForNewerRevision(ctx, repository, first.Revision, shutdown, 10*time.Millisecond, nil)
	time.Sleep(50 * time.Millisecond)
	select {
	case <-shutdown.Done():
		t.Fatal("watcher requested shutdown without a newer revision")
	default:
	}
}
