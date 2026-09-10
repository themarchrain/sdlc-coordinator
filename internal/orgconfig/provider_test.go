package orgconfig

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"sdlc-coordinator/internal/configuration"
)

func TestProviderReturnsFreshConfigAfterSave(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	provider := NewProvider(db)

	config, err := provider.Get(ctx)
	if err != nil || config.CodeHost.Provider != "" {
		t.Fatalf("empty provider result: %+v err=%v", config, err)
	}

	_, err = NewStore(db).Save(ctx, Config{
		CodeHost: configuration.CodeHostConfig{Provider: "gitea", BaseURL: "https://gitea.example.com", ReworkAllowedUserIDs: []string{"5"}},
	}, 0)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	config, err = provider.Get(ctx)
	if err != nil || config.CodeHost.Provider != "gitea" {
		t.Fatalf("provider did not observe save: %+v err=%v", config, err)
	}
	if ids := config.ReworkAllowedIDs(); len(ids) != 1 || ids[0] != 5 {
		t.Fatalf("rework ids mismatch: %v", ids)
	}
}

func TestProviderReuseAcrossRevision(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	provider := NewProvider(db)

	if _, err := provider.Get(ctx); err != nil {
		t.Fatalf("first get: %v", err)
	}
	if _, err := provider.Get(ctx); err != nil {
		t.Fatalf("second get: %v", err)
	}
}

func TestProviderDegradesToLastKnownOnReadFailure(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	provider := NewProvider(db)

	if _, err := NewStore(db).Save(ctx, Config{
		CodeHost: configuration.CodeHostConfig{Provider: "gitea", BaseURL: "https://gitea.example.com"},
	}, 0); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := provider.Get(ctx); err != nil {
		t.Fatalf("warm get: %v", err)
	}
	if !provider.HasSnapshot() {
		t.Fatalf("provider must report a snapshot after a warm read")
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("raw db handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	config, err := provider.Get(ctx)
	if err == nil {
		t.Fatalf("read after close must fail")
	}
	if config.CodeHost.Provider != "gitea" || !provider.HasSnapshot() {
		t.Fatalf("degraded read must return last known config, got %+v", config)
	}
}

func TestProviderColdReadFailureReturnsZeroWithoutSnapshot(t *testing.T) {
	db := newTestDB(t)
	provider := NewProvider(db)

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("raw db handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	config, err := provider.Get(context.Background())
	if err == nil || config.CodeHost.Provider != "" {
		t.Fatalf("cold failed read must return zero config with error, got %+v err=%v", config, err)
	}
	if provider.HasSnapshot() {
		t.Fatalf("cold failed read must not produce a snapshot")
	}
}

func TestProviderRereadsWhenRowRecreatedWithSameRevision(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	provider := NewProvider(db)

	first := Config{CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com"}}
	if _, err := NewStore(db).Save(ctx, first, 0); err != nil {
		t.Fatalf("save: %v", err)
	}
	if config, err := provider.Get(ctx); err != nil || config.CodeHost.Provider != "gitlab" {
		t.Fatalf("warm get: %+v err=%v", config, err)
	}

	// 模拟租户库重建:删除行后以相同 revision=1 重新落一份不同配置,
	// updated_at 变化必须触发重新解析而不是命中缓存。
	if err := db.Exec("DELETE FROM tenant_runtime_config").Error; err != nil {
		t.Fatalf("delete row: %v", err)
	}
	rebuilt := Config{CodeHost: configuration.CodeHostConfig{Provider: "github", BaseURL: "https://github.com"}}
	payload, err := json.Marshal(rebuilt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	record := Record{ID: 1, Revision: 1, PayloadJSON: string(payload), UpdatedAt: time.Now().UTC().Add(time.Hour)}
	if err := db.Create(&record).Error; err != nil {
		t.Fatalf("recreate row: %v", err)
	}

	config, err := provider.Get(ctx)
	if err != nil || config.CodeHost.Provider != "github" {
		t.Fatalf("provider must reread recreated row, got %+v err=%v", config, err)
	}
}
