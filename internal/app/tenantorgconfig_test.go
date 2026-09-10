package app

import (
	"context"
	"path/filepath"
	"testing"

	"sdlc-coordinator/internal/configuration"
	"sdlc-coordinator/internal/orgconfig"
	"sdlc-coordinator/internal/store"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func newOrgConfigTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open("file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "orgconfig.db")) + "?_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := orgconfig.NewStore(db).Migrate(); err != nil {
		t.Fatalf("migrate org config: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestTenantEffectExecutorFollowsConfigSaves(t *testing.T) {
	db := newOrgConfigTestDB(t)
	ctx := context.Background()
	executor := newTenantEffectExecutor(orgconfig.NewProvider(db), zap.NewNop(), 1)

	if executor.EffectExecutionEnabled(ctx) {
		t.Fatalf("executor must be disabled without org config")
	}

	_, err := orgconfig.NewStore(db).Save(ctx, orgconfig.Config{
		CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com"},
		IM:       configuration.IMConfig{Provider: "feishu", Feishu: configuration.IMFeishuConfig{WebhookURL: "https://example.com/hook"}},
	}, 0)
	if err != nil {
		t.Fatalf("save org config: %v", err)
	}
	if !executor.EffectExecutionEnabled(ctx) {
		t.Fatalf("executor did not observe the save")
	}

	// 换回未配置渠道：执行器回落未启用，效果留在队列等待重放。
	_, err = orgconfig.NewStore(db).Save(ctx, orgconfig.Config{
		CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com"},
	}, 1)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if executor.EffectExecutionEnabled(ctx) {
		t.Fatalf("executor stayed enabled after the channel was removed")
	}
}

func TestTenantReviewerMentionsProviderFollowsConfigSaves(t *testing.T) {
	db := newOrgConfigTestDB(t)
	ctx := context.Background()
	provider := tenantReviewerMentionsProvider(orgconfig.NewProvider(db), zap.NewNop(), 1)

	if mentions := provider(ctx); len(mentions) != 0 {
		t.Fatalf("mentions must be empty without org config: %#v", mentions)
	}

	_, err := orgconfig.NewStore(db).Save(ctx, orgconfig.Config{
		CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com"},
		IM:       configuration.IMConfig{Provider: "telegram", Telegram: configuration.IMTelegramConfig{BotToken: "t"}, ReviewerMentions: []string{"alice"}},
	}, 0)
	if err != nil {
		t.Fatalf("save org config: %v", err)
	}
	if mentions := provider(ctx); len(mentions) != 1 || mentions[0] != "alice" {
		t.Fatalf("mentions did not observe the save: %#v", mentions)
	}
}

func TestTenantWebhookViewRejectsColdConfigReadAndDegradesWhenWarm(t *testing.T) {
	db := newOrgConfigTestDB(t)
	ctx := context.Background()
	engine := &tenantEngine{orgConfig: orgconfig.NewProvider(db), logger: zap.NewNop(), tenantID: 1}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("raw db handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	// 冷启动首读失败:没有可降级的已知配置,必须拒收让平台重试。
	if _, err := engine.tenantWebhookView(ctx); err == nil {
		t.Fatalf("cold failed read must reject the view, got nil error")
	}
}

func TestTenantWebhookViewDegradesToLastKnownConfigWhenWarm(t *testing.T) {
	db := newOrgConfigTestDB(t)
	ctx := context.Background()
	engine := &tenantEngine{orgConfig: orgconfig.NewProvider(db), logger: zap.NewNop(), tenantID: 1}

	if _, err := orgconfig.NewStore(db).Save(ctx, orgconfig.Config{
		CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com", ReworkAllowedUserIDs: []string{"7"}},
	}, 0); err != nil {
		t.Fatalf("save org config: %v", err)
	}
	if _, err := engine.orgConfig.Get(ctx); err != nil {
		t.Fatalf("warm read: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("raw db handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	// 已有已知快照:降级用旧值组装视图,而不是拒收。
	view, err := engine.tenantWebhookView(ctx)
	if err != nil || view == nil {
		t.Fatalf("warm degraded read must still build a view, got %v", err)
	}
}

func TestTenantReworkAllowlistLookupFollowsConfigSaves(t *testing.T) {
	db := newOrgConfigTestDB(t)
	lookup := tenantReworkAllowlistLookup(orgconfig.NewProvider(db), zap.NewNop(), 1)

	if ids := lookup(context.Background()); len(ids) != 0 {
		t.Fatalf("allowlist must be empty without org config: %#v", ids)
	}

	_, err := orgconfig.NewStore(db).Save(context.Background(), orgconfig.Config{
		CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com", ReworkAllowedUserIDs: []string{"7", "9"}},
	}, 0)
	if err != nil {
		t.Fatalf("save org config: %v", err)
	}
	ids := lookup(context.Background())
	if len(ids) != 2 || ids[0] != 7 || ids[1] != 9 {
		t.Fatalf("allowlist did not observe the save: %#v", ids)
	}
}
