package orgconfig

import (
	"context"
	"path/filepath"
	"testing"

	"sdlc-coordinator/internal/configuration"
	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open("file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "orgconfig.db")) + "?_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := NewStore(db).Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestStoreLoadWithoutRowReturnsNotConfigured(t *testing.T) {
	db := newTestDB(t)

	config, revision, configured, err := NewStore(db).Load(context.Background())
	if err != nil || configured || revision != 0 {
		t.Fatalf("unexpected empty-load result: config=%+v rev=%d configured=%v err=%v", config, revision, configured, err)
	}
}

func TestStoreSaveRoundTripAndRevisionIncrement(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	store := NewStore(db)

	first := configuration.CodeHostConfig{Provider: "github", BaseURL: "https://github.com", ReworkAllowedUserIDs: []string{"101"}}
	revision1, err := store.Save(ctx, Config{CodeHost: first})
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	if revision1 != 1 {
		t.Fatalf("first revision = %d, want 1", revision1)
	}

	loaded, revision, configured, err := store.Load(ctx)
	if err != nil || !configured || revision != 1 {
		t.Fatalf("unexpected load result: %+v rev=%d configured=%v err=%v", loaded, revision, configured, err)
	}
	if loaded.CodeHost.Provider != "github" || len(loaded.CodeHost.ReworkAllowedUserIDs) != 1 || loaded.CodeHost.ReworkAllowedUserIDs[0] != "101" {
		t.Fatalf("round trip mismatch: %+v", loaded)
	}

	second := Config{CodeHost: first, IM: configuration.IMConfig{Provider: "feishu", Feishu: configuration.IMFeishuConfig{WebhookURL: "https://example.com/hook"}}}
	revision2, err := store.Save(ctx, second)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if revision2 != 2 {
		t.Fatalf("second revision = %d, want 2", revision2)
	}
	loaded, _, _, err = store.Load(ctx)
	if err != nil || loaded.IM.Provider != "feishu" {
		t.Fatalf("second round trip mismatch: %+v err=%v", loaded, err)
	}
}

func TestValidateRejectsBadShapes(t *testing.T) {
	cases := []struct {
		name   string
		config Config
		field  string
	}{
		{"bad provider", Config{CodeHost: configuration.CodeHostConfig{Provider: "svn", BaseURL: "https://x.example.com"}}, "codehost.provider"},
		{"missing base url", Config{CodeHost: configuration.CodeHostConfig{Provider: "gitlab"}}, "codehost.base_url"},
		{"bad rework id", Config{CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://x.example.com", ReworkAllowedUserIDs: []string{"abc"}}}, "codehost.rework_allowed_user_ids"},
		{"telegram without token", Config{CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://x.example.com"}, IM: configuration.IMConfig{Provider: "telegram"}}, "im.telegram.bot_token"},
		{"feishu without webhook", Config{CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://x.example.com"}, IM: configuration.IMConfig{Provider: "feishu"}}, "im.feishu.webhook_url"},
		{"empty mention", Config{CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://x.example.com"}, IM: configuration.IMConfig{ReviewerMentions: []string{" "}}}, "im.reviewer_mentions"},
	}
	for _, testCase := range cases {
		if errs := Validate(testCase.config); !errs.Has(testCase.field) {
			t.Fatalf("%s: expected error on %s, got %+v", testCase.name, testCase.field, errs)
		}
	}
}

func TestValidateAcceptsCompleteConfig(t *testing.T) {
	config := Config{
		CodeHost: configuration.CodeHostConfig{Provider: "github", BaseURL: "https://github.com", ReworkAllowedUserIDs: []string{"7"}},
		IM:       configuration.IMConfig{Provider: "telegram", Telegram: configuration.IMTelegramConfig{BotToken: "t"}, ReviewerMentions: []string{"Alice"}},
	}
	if errs := Validate(config); len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
}

func TestMergeKeepsUnchangedSecretsAndAppliesChanges(t *testing.T) {
	current := Config{
		CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com", ReworkAllowedUserIDs: []string{"1"}},
		IM:       configuration.IMConfig{Provider: "telegram", Telegram: configuration.IMTelegramConfig{BotToken: "keep-me", DefaultChatID: "-100"}, ReviewerMentions: []string{"Alice"}},
	}
	newToken := "rotate-me"
	merged := Merge(current, UpdateRequest{
		Config: EditableConfig{
			CodeHost: configuration.EditableCodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com", ReworkAllowedUserIDs: []string{"2"}},
			IM:       configuration.EditableIMConfig{Provider: "telegram", Telegram: configuration.EditableIMTelegram{DefaultChatID: "-200"}, ReviewerMentions: []string{"Bob"}},
		},
	})
	if merged.IM.Telegram.BotToken != "keep-me" || merged.IM.Telegram.DefaultChatID != "-200" {
		t.Fatalf("merge mismatch: %+v", merged)
	}
	if merged.CodeHost.ReworkAllowedUserIDs[0] != "2" || merged.IM.ReviewerMentions[0] != "Bob" {
		t.Fatalf("merge mismatch: %+v", merged)
	}

	merged = Merge(current, UpdateRequest{
		Config: EditableConfig{
			CodeHost: configuration.EditableCodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com"},
			IM:       configuration.EditableIMConfig{Provider: "telegram"},
		},
		Secrets: SecretChanges{TelegramBotToken: &newToken},
	})
	if merged.IM.Telegram.BotToken != "rotate-me" {
		t.Fatalf("secret replace mismatch: %+v", merged)
	}

	merged = Merge(current, UpdateRequest{
		Config: EditableConfig{
			CodeHost: configuration.EditableCodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com"},
			IM:       configuration.EditableIMConfig{Provider: "telegram"},
		},
		Secrets: SecretChanges{ClearTelegramBotToken: true},
	})
	if merged.IM.Telegram.BotToken != "" {
		t.Fatalf("secret clear mismatch: %+v", merged)
	}
}

func TestMaskViewHidesSecrets(t *testing.T) {
	view := Mask(Config{
		CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com"},
		IM:       configuration.IMConfig{Provider: "feishu", Feishu: configuration.IMFeishuConfig{WebhookURL: "https://example.com/hook", Secret: "hush"}},
	})
	if !view.Secrets.FeishuSecretConfigured {
		t.Fatalf("mask missing feishu secret flag: %+v", view)
	}
	if view.Config.IM.Feishu.WebhookURL != "https://example.com/hook" {
		t.Fatalf("mask dropped non-secret webhook: %+v", view)
	}
}
