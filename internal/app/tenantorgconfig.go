package app

import (
	"context"
	"errors"
	"strings"

	"sdlc-coordinator/internal/adapters/im"
	effectexec "sdlc-coordinator/internal/effects"
	"sdlc-coordinator/internal/events"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orgconfig"

	handlers "sdlc-coordinator/internal/http/handlers"

	"go.uber.org/zap"
)

// tenantIMSettings 把组织段 IM 配置换算为 IM 后端构造入参。
func tenantIMSettings(config orgconfig.Config) im.Settings {
	return im.Settings{
		Provider: strings.ToLower(strings.TrimSpace(config.IM.Provider)),
		Telegram: im.TelegramSettings{
			BotToken:      config.IM.Telegram.BotToken,
			APIBase:       config.IM.Telegram.APIBase,
			DefaultChatID: config.IM.Telegram.DefaultChatID,
			Mentions:      config.IM.Telegram.MentionMap,
		},
		Feishu: im.FeishuSettings{
			WebhookURL: config.IM.Feishu.WebhookURL,
			Secret:     config.IM.Feishu.Secret,
			Mentions:   config.IM.Feishu.MentionMap,
		},
	}
}

// tenantEffectExecutor 把 IM 发送按组织段配置动态化：每次执行前经
// Provider 读当前配置（revision 缓存）；未配置渠道时申报未启用，效果留
// 在队列等待配置后重放（与全局无执行器语义一致）。后端构造轻量，按需
// 重建不做缓存。
type tenantEffectExecutor struct {
	provider *orgconfig.Provider
	logger   *zap.Logger
}

func newTenantEffectExecutor(provider *orgconfig.Provider, logger *zap.Logger) *tenantEffectExecutor {
	return &tenantEffectExecutor{provider: provider, logger: logger}
}

func (e *tenantEffectExecutor) EffectExecutionEnabled(ctx context.Context) bool {
	_, enabled := e.current(ctx)
	return enabled
}

func (e *tenantEffectExecutor) ExecuteEffect(ctx context.Context, effect *model.Effect) (string, error) {
	executor, enabled := e.current(ctx)
	if !enabled {
		return "", errors.New("IM 通知渠道未配置")
	}
	return executor.ExecuteEffect(ctx, effect)
}

func (e *tenantEffectExecutor) current(ctx context.Context) (*effectexec.Executor, bool) {
	config, err := e.provider.Get(ctx)
	if err != nil && e.logger != nil {
		e.logger.Warn("load tenant runtime config for IM failed", zap.Error(err))
	}
	backend := im.NewBackend(tenantIMSettings(config))
	if backend == nil {
		return nil, false
	}
	return effectexec.NewExecutor(backend), true
}

// tenantReviewerMentionsProvider 产出提醒人名单的动态来源（审查收尾现取）。
func tenantReviewerMentionsProvider(provider *orgconfig.Provider, logger *zap.Logger, tenantID uint) func(context.Context) []string {
	return func(ctx context.Context) []string {
		config, err := provider.Get(ctx)
		if err != nil && logger != nil {
			logger.Warn("load tenant runtime config for reviewer mentions failed", zap.Uint("tenant_id", tenantID), zap.Error(err))
		}
		return config.IM.ReviewerMentions
	}
}

// tenantReworkAllowlistLookup 产出接续开发白名单的动态来源（GitLab 侧经
// 处理器构造期选项挂入，每个 webhook 现取）。
func tenantReworkAllowlistLookup(provider *orgconfig.Provider, logger *zap.Logger, tenantID uint) func() []int {
	return func() []int {
		config, err := provider.Get(context.Background())
		if err != nil && logger != nil {
			logger.Warn("load tenant runtime config for allowlist failed", zap.Uint("tenant_id", tenantID), zap.Error(err))
		}
		return config.ReworkAllowedIDs()
	}
}

// tenantWebhookView 为一次 webhook 请求组装处理器与 Gitea/GitHub
// normalizer；normalizer 按当前组织段配置现建，白名单保存即生效。
// GitLab 侧由处理器自身按构造期 lookup 选项现建，不在本视图内。
func (e *tenantEngine) tenantWebhookView(ctx context.Context) *handlers.WebhookEngineView {
	config, err := e.orgConfig.Get(ctx)
	if err != nil && e.logger != nil {
		e.logger.Warn("load tenant runtime config for webhook failed", zap.Uint("tenant_id", e.tenantID), zap.Error(err))
	}
	ids := config.ReworkAllowedIDs()
	return &handlers.WebhookEngineView{
		Processor:        e.processor,
		GiteaNormalizer:  events.NewGiteaNormalizer(events.WithGiteaReworkAllowedUserIDs(ids)),
		GitHubNormalizer: events.NewGitHubNormalizer(events.WithGitHubReworkAllowedUserIDs(ids)),
	}
}
