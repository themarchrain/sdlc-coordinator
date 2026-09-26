package orgconfig

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
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
	revision1, err := store.Save(ctx, Config{CodeHost: first}, 0)
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
	revision2, err := store.Save(ctx, second, revision1)
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

func TestStoreSaveRejectsStaleRevision(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	store := NewStore(db)

	config := Config{CodeHost: configuration.CodeHostConfig{Provider: "github", BaseURL: "https://github.com"}}
	if _, err := store.Save(ctx, config, 0); err != nil {
		t.Fatalf("first save: %v", err)
	}
	// 基于过期 revision 的整行覆盖必须被拒绝,而不是静默吞掉并发提交。
	if _, err := store.Save(ctx, config, 0); !errors.Is(err, ErrConfigConflict) {
		t.Fatalf("stale save must conflict, got %v", err)
	}
	if _, err := store.Save(ctx, config, 2); !errors.Is(err, ErrConfigConflict) {
		t.Fatalf("future revision must conflict, got %v", err)
	}
	if _, err := store.Save(ctx, config, 1); err != nil {
		t.Fatalf("save at head revision: %v", err)
	}
	if _, revision, _, err := store.Load(ctx); err != nil || revision != 2 {
		t.Fatalf("load after conflict-free save: rev=%d err=%v", revision, err)
	}
}

func TestReworkAllowedIDsSkipsInvalidAndOverflowEntries(t *testing.T) {
	config := Config{CodeHost: configuration.CodeHostConfig{
		ReworkAllowedUserIDs: []string{"7", "abc", "-3", "0", "99999999999999999999"},
	}}
	ids := config.ReworkAllowedIDs()
	if len(ids) != 1 || ids[0] != 7 {
		t.Fatalf("only the valid id must survive (overflow skips instead of wrapping), got %v", ids)
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

// 空集合必须编码为 [] 而非 null:前端类型契约声明为非可选数组,与系统段
// 掩码视图的历史契约一致。
func TestMaskEncodesEmptyCollectionsAsArrays(t *testing.T) {
	raw, err := json.Marshal(Mask(Config{}))
	if err != nil {
		t.Fatalf("marshal mask: %v", err)
	}
	for _, fragment := range []string{`"rework_allowed_user_ids":[]`, `"reviewer_mentions":[]`, `"configured":false`} {
		if !strings.Contains(string(raw), fragment) {
			t.Fatalf("masked view must encode %s, got %s", fragment, raw)
		}
	}
}

func TestValidateChecksIMOutboundURLs(t *testing.T) {
	base := Config{CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com"}}

	emptyAllowed := Validate(base)
	if _, ok := emptyAllowed["im.telegram.api_base"]; ok {
		t.Fatalf("empty api_base must pass, got %v", emptyAllowed)
	}

	badAPIBase := base
	badAPIBase.IM.Telegram.APIBase = "not-a-url"
	if errs := Validate(badAPIBase); errs["im.telegram.api_base"] == "" {
		t.Fatalf("non-http api_base must be rejected, got %v", errs)
	}

	badWebhook := base
	badWebhook.IM.Provider = "feishu"
	badWebhook.IM.Feishu.WebhookURL = "file:///etc/passwd"
	if errs := Validate(badWebhook); errs["im.feishu.webhook_url"] == "" {
		t.Fatalf("non-http webhook_url must be rejected, got %v", errs)
	}
}

// 执行 CLI 选择三态:空(跟随系统默认)与两个合法 CLI 通过,其他值拒绝。
func TestValidateRunnerProvider(t *testing.T) {
	base := Config{CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com"}}

	for _, provider := range []string{"", "codex-cli", "claude-cli"} {
		candidate := base
		candidate.Runner.Provider = provider
		if errs := Validate(candidate); len(errs) != 0 {
			t.Fatalf("runner provider %q must pass, got %+v", provider, errs)
		}
	}

	unknown := base
	unknown.Runner.Provider = "cursor-cli"
	if errs := Validate(unknown); !errs.Has("runner.provider") {
		t.Fatalf("unknown runner provider must be rejected on runner.provider, got %+v", errs)
	}
}

// 单任务超时,空(跟随系统默认)与 1 分钟到 4 小时区间通过,越界/坏格式拒。
func TestValidateRunnerTaskTimeout(t *testing.T) {
	base := Config{CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com"}}

	for _, raw := range []string{"", "1m", "45m", "4h"} {
		candidate := base
		candidate.Runner.TaskTimeout = raw
		if errs := Validate(candidate); len(errs) != 0 {
			t.Fatalf("runner task timeout %q must pass, got %+v", raw, errs)
		}
	}

	for _, raw := range []string{"30s", "0m", "5h", "not-a-duration"} {
		candidate := base
		candidate.Runner.TaskTimeout = raw
		if errs := Validate(candidate); !errs.Has("runner.task_timeout") {
			t.Fatalf("runner task timeout %q must be rejected on runner.task_timeout, got %+v", raw, errs)
		}
	}
}

// 推理强度档位,空(跟随系统默认)与五档通过,其他值拒绝。
func TestValidateRunnerEffort(t *testing.T) {
	base := Config{CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com"}}

	for _, effort := range []string{"", "low", "medium", "high", "xhigh", "max"} {
		candidate := base
		candidate.Runner.Effort = effort
		if errs := Validate(candidate); len(errs) != 0 {
			t.Fatalf("runner effort %q must pass, got %+v", effort, errs)
		}
	}

	unknown := base
	unknown.Runner.Effort = "ultracode"
	if errs := Validate(unknown); !errs.Has("runner.effort") {
		t.Fatalf("unknown runner effort must be rejected on runner.effort, got %+v", errs)
	}
}

func TestMergeRunnerTakesRequestValue(t *testing.T) {
	codeHost := configuration.EditableCodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com"}
	current := Config{
		CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com"},
		Runner:   RunnerConfig{Provider: "claude-cli", Effort: "low", TaskTimeout: "30m"},
	}

	merged := Merge(current, UpdateRequest{Config: EditableConfig{
		CodeHost: codeHost,
		Runner:   EditableRunnerConfig{Provider: "codex-cli", Effort: "high", TaskTimeout: "45m"},
	}})
	if merged.Runner.Provider != "codex-cli" || merged.Runner.Effort != "high" || merged.Runner.TaskTimeout != "45m" {
		t.Fatalf("merge must take request runner selection, got %+v", merged.Runner)
	}

	// 空提交 = 回到跟随系统默认,不保留现值(无密钥语义,直接覆盖)。
	merged = Merge(current, UpdateRequest{Config: EditableConfig{CodeHost: codeHost}})
	if merged.Runner.Provider != "" || merged.Runner.Effort != "" || merged.Runner.TaskTimeout != "" {
		t.Fatalf("empty request runner selection must merge to empty, got %+v", merged.Runner)
	}
}

func TestMaskCarriesRunnerSelection(t *testing.T) {
	view := Mask(Config{
		CodeHost: configuration.CodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com"},
		Runner:   RunnerConfig{Provider: "claude-cli", Effort: "low", TaskTimeout: "30m"},
	})
	if view.Config.Runner.Provider != "claude-cli" || view.Config.Runner.Effort != "low" || view.Config.Runner.TaskTimeout != "30m" {
		t.Fatalf("mask must carry the runner selection, got %+v", view.Config.Runner)
	}
}

// 自动合并开关默认关(零值),提交值经 Merge 原样落库、Mask 原样透出:
// 握有合并写权限的能力必须显式开启,观察期取证不受开关影响。
func TestAutoMergeEnabledRoundTrip(t *testing.T) {
	merged := Merge(Config{}, UpdateRequest{
		Config: EditableConfig{
			CodeHost: configuration.EditableCodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com", AutoMergeEnabled: true},
		},
	})
	if !merged.CodeHost.AutoMergeEnabled {
		t.Fatalf("submitted auto merge flag must persist: %+v", merged)
	}
	raw, err := json.Marshal(Mask(merged))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"auto_merge_enabled":true`) {
		t.Fatalf("masked view must expose the switch, got %s", raw)
	}

	rolledBack := Merge(merged, UpdateRequest{
		Config: EditableConfig{
			CodeHost: configuration.EditableCodeHostConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com"},
		},
	})
	if rolledBack.CodeHost.AutoMergeEnabled {
		t.Fatalf("unsubmitted switch must fall back to off on full-payload saves: %+v", rolledBack)
	}
}
