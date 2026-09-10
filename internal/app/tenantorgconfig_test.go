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
	executor := newTenantEffectExecutor(orgconfig.NewProvider(db), zap.NewNop())

	if executor.EffectExecutionEnabled(ctx) {
		t.Fatalf("executor must be disabled without org config")
	}

	_, err := orgconfig.NewStore(db).Save(ctx, orgconfig.Config{
		CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com"},
		IM:       configuration.IMConfig{Provider: "feishu", Feishu: configuration.IMFeishuConfig{WebhookURL: "https://example.com/hook"}},
	})
	if err != nil {
		t.Fatalf("save org config: %v", err)
	}
	if !executor.EffectExecutionEnabled(ctx) {
		t.Fatalf("executor did not observe the save")
	}

	// 换回未配置渠道：执行器回落未启用，效果留在队列等待重放。
	_, err = orgconfig.NewStore(db).Save(ctx, orgconfig.Config{
		CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com"},
	})
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
	})
	if err != nil {
		t.Fatalf("save org config: %v", err)
	}
	if mentions := provider(ctx); len(mentions) != 1 || mentions[0] != "alice" {
		t.Fatalf("mentions did not observe the save: %#v", mentions)
	}
}

func TestTenantReworkAllowlistLookupFollowsConfigSaves(t *testing.T) {
	db := newOrgConfigTestDB(t)
	lookup := tenantReworkAllowlistLookup(orgconfig.NewProvider(db), zap.NewNop(), 1)

	if ids := lookup(); len(ids) != 0 {
		t.Fatalf("allowlist must be empty without org config: %#v", ids)
	}

	_, err := orgconfig.NewStore(db).Save(context.Background(), orgconfig.Config{
		CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com", ReworkAllowedUserIDs: []string{"7", "9"}},
	})
	if err != nil {
		t.Fatalf("save org config: %v", err)
	}
	ids := lookup()
	if len(ids) != 2 || ids[0] != 7 || ids[1] != 9 {
		t.Fatalf("allowlist did not observe the save: %#v", ids)
	}
}
