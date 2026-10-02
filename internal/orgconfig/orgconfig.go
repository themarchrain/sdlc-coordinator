// Package orgconfig 承载组织段运行配置（代码仓库默认值、接续开发白名单、
// IM 通知、固定提醒人）：每组织一份，落在该组织的租户库，保存即热生效。
package orgconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"sdlc-coordinator/internal/configuration"

	"gorm.io/gorm"
)

// Record 是组织段配置在租户库中的存储形态：每库至多一行（固定主键 1），
// revision 单调递增，是热生效消费点的版本锚点。密钥内联在载荷里，
// 与平台快照的持有方式一致。
type Record struct {
	ID          uint   `gorm:"primaryKey"`
	Revision    int64  `gorm:"not null"`
	PayloadJSON string `gorm:"type:text;not null"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Record) TableName() string { return "tenant_runtime_config" }

// Config 复用系统段既有类型，不发明新形状。
type Config struct {
	CodeHost configuration.CodeHostConfig `json:"codehost"`
	IM       configuration.IMConfig       `json:"im"`
	Runner   RunnerConfig                 `json:"runner"`
}

// RunnerConfig 是组织段的执行选择：provider/effort 空串与 task_timeout
// 空串均表示跟随系统默认；合法取值与系统段 agent_runner 同名字段一致
// （CLI 为 codex-cli/claude-cli，推理强度为 low/medium/high/xhigh/max
// 五档，单任务超时为 1 分钟到 4 小时的时长）。字段独立解析：组织只设
// 其一时，派发不覆盖另一个。
type RunnerConfig struct {
	Provider    string `json:"provider"`
	Effort      string `json:"effort"`
	TaskTimeout string `json:"task_timeout"`
}

// ReworkAllowedIDs 把白名单字符串转为三平台 normalizer 消费的数值 id；
// 校验保证为正整数字符串，解析失败项（含超长溢出）静默跳过（防御旧数据）。
func (c Config) ReworkAllowedIDs() []int {
	ids := make([]int, 0, len(c.CodeHost.ReworkAllowedUserIDs))
	for _, raw := range c.CodeHost.ReworkAllowedUserIDs {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			continue
		}
		ids = append(ids, int(value))
	}
	return ids
}

// ErrConfigConflict 表示提交所基于的 revision 已过期(保存窗口内已被
// 其他提交覆盖);调用方应提示刷新后重试,而不是静默整行覆盖。
var ErrConfigConflict = errors.New("tenant runtime config revision conflict")

// Store 提供组织段配置在单个库句柄上的读写；无状态，可按请求构造。
type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

func (s *Store) Migrate() error { return s.db.AutoMigrate(&Record{}) }

// Load 返回当前配置；无行时 found=false（组织段未配置是合法状态）。
func (s *Store) Load(ctx context.Context) (Config, int64, bool, error) {
	var record Record
	err := s.db.WithContext(ctx).First(&record, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Config{}, 0, false, nil
	}
	if err != nil {
		return Config{}, 0, false, fmt.Errorf("load tenant runtime config: %w", err)
	}
	var config Config
	if err := json.Unmarshal([]byte(record.PayloadJSON), &config); err != nil {
		return Config{}, 0, false, fmt.Errorf("decode tenant runtime config: %w", err)
	}
	return config, record.Revision, true, nil
}

// Save 整行覆盖保存（校验由调用方先行完成）。仅当当前行 revision 等于
// expectedRevision 时写入，revision 在 SQL 侧自增；无行且 expectedRevision
// 为 0 视为首次保存。并发覆盖的提交以 ErrConfigConflict 拒绝。
func (s *Store) Save(ctx context.Context, config Config, expectedRevision int64) (int64, error) {
	payload, err := json.Marshal(config)
	if err != nil {
		return 0, fmt.Errorf("encode tenant runtime config: %w", err)
	}
	var revision int64
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Record{}).Where("id = ? AND revision = ?", 1, expectedRevision).Updates(map[string]any{
			"revision":     gorm.Expr("revision + 1"),
			"payload_json": string(payload),
			"updated_at":   time.Now().UTC(),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			revision = expectedRevision + 1
			return nil
		}
		var current Record
		err := tx.First(&current, 1).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			if expectedRevision != 0 {
				return ErrConfigConflict
			}
			record := Record{ID: 1, Revision: 1, PayloadJSON: string(payload)}
			if err := tx.Create(&record).Error; err != nil {
				// 并发首次保存双双落 Create 时,输家收到主键唯一约束冲突:
				// 语义是「行已被别人抢先建好」的保存冲突,映射为同一哨兵,
				// 调用方统一 409 提示刷新重试(SQLite 文本稳定,方言固定)。
				if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					return ErrConfigConflict
				}
				return fmt.Errorf("create tenant runtime config: %w", err)
			}
			revision = 1
			return nil
		case err != nil:
			return err
		default:
			return ErrConfigConflict
		}
	})
	if err != nil {
		return 0, fmt.Errorf("save tenant runtime config: %w", err)
	}
	return revision, nil
}

type EditableConfig struct {
	CodeHost configuration.EditableCodeHostConfig `json:"codehost"`
	IM       configuration.EditableIMConfig       `json:"im"`
	Runner   EditableRunnerConfig                 `json:"runner"`
}

// EditableRunnerConfig 与 RunnerConfig 同形状（执行选择无密钥语义，
// 编辑面直接读写原值）。
type EditableRunnerConfig struct {
	Provider    string `json:"provider"`
	Effort      string `json:"effort"`
	TaskTimeout string `json:"task_timeout"`
}

type SecretChanges struct {
	TelegramBotToken      *string `json:"telegram_bot_token,omitempty"`
	FeishuSecret          *string `json:"feishu_secret,omitempty"`
	ClearTelegramBotToken bool    `json:"clear_telegram_bot_token,omitempty"`
	ClearFeishuSecret     bool    `json:"clear_feishu_secret,omitempty"`
}

type SecretStatus struct {
	TelegramBotTokenConfigured bool `json:"telegram_bot_token_configured"`
	FeishuSecretConfigured     bool `json:"feishu_secret_configured"`
}

type View struct {
	Config     EditableConfig `json:"config"`
	Secrets    SecretStatus   `json:"secrets"`
	Revision   int64          `json:"revision"`
	Configured bool           `json:"configured"`
}

type UpdateRequest struct {
	Config           EditableConfig `json:"config"`
	Secrets          SecretChanges  `json:"secrets"`
	ExpectedRevision int64          `json:"expected_revision"`
}

// Validate 校验规则与系统段历史规则一致（组织段 codehost 保持必填：
// 一组织一平台默认，匹配注册表单 UX）。
func Validate(config Config) configuration.ValidationErrors {
	errs := configuration.ValidationErrors{}
	provider := strings.ToLower(strings.TrimSpace(config.CodeHost.Provider))
	if provider != "gitlab" && provider != "gitea" && provider != "github" {
		errs["codehost.provider"] = "仅支持 gitlab、gitea 或 github"
	}
	requireAbsoluteHTTPURL(errs, "codehost.base_url", config.CodeHost.BaseURL)
	for _, id := range config.CodeHost.ReworkAllowedUserIDs {
		if !configuration.IsValidPlatformUserID(id) {
			errs["codehost.rework_allowed_user_ids"] = "用户 ID 必须为正整数字符串"
			break
		}
	}
	switch strings.ToLower(strings.TrimSpace(config.IM.Provider)) {
	case "", "none", "telegram", "feishu":
	default:
		errs["im.provider"] = "IM 通知渠道仅支持 telegram 或 feishu"
	}
	if strings.EqualFold(strings.TrimSpace(config.IM.Provider), "telegram") && strings.TrimSpace(config.IM.Telegram.BotToken) == "" {
		errs["im.telegram.bot_token"] = "启用 Telegram 通知时必须填写 Bot Token"
	}
	if strings.EqualFold(strings.TrimSpace(config.IM.Provider), "feishu") && strings.TrimSpace(config.IM.Feishu.WebhookURL) == "" {
		errs["im.feishu.webhook_url"] = "启用飞书通知时必须填写群机器人 Webhook 地址"
	}
	// 出网地址可空(空=渠道官方默认/必填由上面分支管),非空时必须是完整
	// http(s) 地址:配置面通过校验的值,运行期才能直接消费。
	optionalAbsoluteHTTPURL(errs, "im.telegram.api_base", config.IM.Telegram.APIBase)
	optionalAbsoluteHTTPURL(errs, "im.feishu.webhook_url", config.IM.Feishu.WebhookURL)
	for _, mention := range config.IM.ReviewerMentions {
		if strings.TrimSpace(mention) == "" {
			errs["im.reviewer_mentions"] = "提醒人不能为空"
			break
		}
	}
	// 执行 CLI 选择可空(空=跟随系统默认),非空时合法值与系统段
	// agent_runner.provider 规则一致。
	runnerProvider := strings.TrimSpace(config.Runner.Provider)
	if runnerProvider != "" && runnerProvider != "codex-cli" && runnerProvider != "claude-cli" {
		errs["runner.provider"] = "执行 CLI 仅支持 codex-cli 或 claude-cli"
	}
	// 推理强度可空(空=跟随系统默认),非空时合法值与系统段
	// agent_runner.effort 规则一致(五档)。
	switch strings.TrimSpace(config.Runner.Effort) {
	case "", "low", "medium", "high", "xhigh", "max":
	default:
		errs["runner.effort"] = "推理强度仅支持 low、medium、high、xhigh 或 max"
	}
	// 单任务超时可空(空=跟随系统默认),非空时须为 1 分钟到 4 小时的时长。
	if raw := strings.TrimSpace(config.Runner.TaskTimeout); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil || value < time.Minute || value > 4*time.Hour {
			errs["runner.task_timeout"] = "必须是 1 分钟到 4 小时之间的有效时长"
		}
	}
	return errs
}

// Merge 以提交的编辑面覆盖现值，密钥按「留空保留/填新替换/勾选清除」合并。
func Merge(current Config, request UpdateRequest) Config {
	result := Config{
		CodeHost: configuration.CodeHostConfig{
			Provider:             request.Config.CodeHost.Provider,
			BaseURL:              request.Config.CodeHost.BaseURL,
			ReworkAllowedUserIDs: cloneStringSlice(request.Config.CodeHost.ReworkAllowedUserIDs),
			AutoMergeEnabled:     request.Config.CodeHost.AutoMergeEnabled,
		},
		IM: configuration.IMConfig{
			Provider: request.Config.IM.Provider,
			Telegram: configuration.IMTelegramConfig{
				BotToken:      current.IM.Telegram.BotToken,
				APIBase:       request.Config.IM.Telegram.APIBase,
				DefaultChatID: request.Config.IM.Telegram.DefaultChatID,
				MentionMap:    cloneStringMap(request.Config.IM.Telegram.MentionMap),
			},
			Feishu: configuration.IMFeishuConfig{
				WebhookURL: request.Config.IM.Feishu.WebhookURL,
				Secret:     current.IM.Feishu.Secret,
				MentionMap: cloneStringMap(request.Config.IM.Feishu.MentionMap),
			},
			ReviewerMentions: cloneStringSlice(request.Config.IM.ReviewerMentions),
		},
		// 执行选择无密钥语义,直接取提交值(空=跟随系统默认)。
		Runner: RunnerConfig{Provider: request.Config.Runner.Provider, Effort: request.Config.Runner.Effort, TaskTimeout: request.Config.Runner.TaskTimeout},
	}
	applySecretChange(&result.IM.Telegram.BotToken, request.Secrets.TelegramBotToken, request.Secrets.ClearTelegramBotToken)
	applySecretChange(&result.IM.Feishu.Secret, request.Secrets.FeishuSecret, request.Secrets.ClearFeishuSecret)
	return result
}

// cloneStringSlice 空输入返回空切片(非 nil):集合字段编码为 [] 而非
// null,与系统段 API 契约一致(前端类型声明为非可选数组)。
func cloneStringSlice(input []string) []string {
	result := make([]string, len(input))
	copy(result, input)
	return result
}

// Mask 产出 API 视图：密钥置空、只留「已配置」标记。
func Mask(config Config) View {
	return View{
		Config: EditableConfig{
			CodeHost: configuration.EditableCodeHostConfig{
				Provider:             config.CodeHost.Provider,
				BaseURL:              config.CodeHost.BaseURL,
				ReworkAllowedUserIDs: cloneStringSlice(config.CodeHost.ReworkAllowedUserIDs),
				AutoMergeEnabled:     config.CodeHost.AutoMergeEnabled,
			},
			IM: configuration.EditableIMConfig{
				Provider: config.IM.Provider,
				Telegram: configuration.EditableIMTelegram{
					APIBase:       config.IM.Telegram.APIBase,
					DefaultChatID: config.IM.Telegram.DefaultChatID,
					MentionMap:    cloneStringMap(config.IM.Telegram.MentionMap),
				},
				Feishu: configuration.EditableIMFeishu{
					WebhookURL: config.IM.Feishu.WebhookURL,
					MentionMap: cloneStringMap(config.IM.Feishu.MentionMap),
				},
				ReviewerMentions: cloneStringSlice(config.IM.ReviewerMentions),
			},
			Runner: EditableRunnerConfig{Provider: config.Runner.Provider, Effort: config.Runner.Effort, TaskTimeout: config.Runner.TaskTimeout},
		},
		Secrets: SecretStatus{
			TelegramBotTokenConfigured: strings.TrimSpace(config.IM.Telegram.BotToken) != "",
			FeishuSecretConfigured:     strings.TrimSpace(config.IM.Feishu.Secret) != "",
		},
	}
}

func applySecretChange(target *string, replacement *string, clear bool) {
	if clear {
		*target = ""
		return
	}
	if replacement != nil && strings.TrimSpace(*replacement) != "" {
		*target = *replacement
	}
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func requireAbsoluteHTTPURL(errs configuration.ValidationErrors, field, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		errs[field] = "不能为空"
		return
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		errs[field] = "必须是完整的 http 或 https 地址"
	}
}

// optionalAbsoluteHTTPURL 空值合法(字段可选),非空时须为完整 http(s) 地址。
func optionalAbsoluteHTTPURL(errs configuration.ValidationErrors, field, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	requireAbsoluteHTTPURL(errs, field, value)
}
