// Package orgconfig 承载组织段运行配置（代码仓库默认值、接续开发白名单、
// IM 通知、固定提醒人）：每组织一份，落在该组织的租户库，保存即热生效。
package orgconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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
}

// ReworkAllowedIDs 把白名单字符串转为三平台 normalizer 消费的数值 id；
// 校验保证为正整数字符串，解析失败项静默跳过（防御旧数据）。
func (c Config) ReworkAllowedIDs() []int {
	ids := make([]int, 0, len(c.CodeHost.ReworkAllowedUserIDs))
	for _, raw := range c.CodeHost.ReworkAllowedUserIDs {
		value := 0
		for _, r := range raw {
			if r < '0' || r > '9' {
				value = -1
				break
			}
			value = value*10 + int(r-'0')
		}
		if value > 0 {
			ids = append(ids, value)
		}
	}
	return ids
}

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

// Save 整行覆盖保存（校验由调用方先行完成），revision 在事务内自增。
func (s *Store) Save(ctx context.Context, config Config) (int64, error) {
	payload, err := json.Marshal(config)
	if err != nil {
		return 0, fmt.Errorf("encode tenant runtime config: %w", err)
	}
	var revision int64
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current Record
		err := tx.First(&current, 1).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			record := Record{ID: 1, Revision: 1, PayloadJSON: string(payload)}
			if err := tx.Create(&record).Error; err != nil {
				return err
			}
			revision = 1
			return nil
		case err != nil:
			return err
		default:
			revision = current.Revision + 1
			return tx.Model(&Record{}).Where("id = ?", current.ID).Updates(map[string]any{
				"revision":     revision,
				"payload_json": string(payload),
				"updated_at":   time.Now().UTC(),
			}).Error
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
	Config  EditableConfig `json:"config"`
	Secrets SecretChanges  `json:"secrets"`
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
	for _, mention := range config.IM.ReviewerMentions {
		if strings.TrimSpace(mention) == "" {
			errs["im.reviewer_mentions"] = "提醒人不能为空"
			break
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
			ReworkAllowedUserIDs: append([]string(nil), request.Config.CodeHost.ReworkAllowedUserIDs...),
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
			ReviewerMentions: append([]string(nil), request.Config.IM.ReviewerMentions...),
		},
	}
	applySecretChange(&result.IM.Telegram.BotToken, request.Secrets.TelegramBotToken, request.Secrets.ClearTelegramBotToken)
	applySecretChange(&result.IM.Feishu.Secret, request.Secrets.FeishuSecret, request.Secrets.ClearFeishuSecret)
	return result
}

// Mask 产出 API 视图：密钥置空、只留「已配置」标记。
func Mask(config Config) View {
	return View{
		Config: EditableConfig{
			CodeHost: configuration.EditableCodeHostConfig{
				Provider:             config.CodeHost.Provider,
				BaseURL:              config.CodeHost.BaseURL,
				ReworkAllowedUserIDs: append([]string(nil), config.CodeHost.ReworkAllowedUserIDs...),
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
				ReviewerMentions: append([]string(nil), config.IM.ReviewerMentions...),
			},
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
