package im

import (
	"net/http"
	"strings"
	"time"
)

// sendHTTPClient 是 IM 出站发送共用的 HTTP 客户端:整体超时上限防止
// 挂起的远端(接受连接后不响应)把效果执行循环无限拖住。
var sendHTTPClient = &http.Client{Timeout: 20 * time.Second}

// Settings 是 IM 通知层的总配置;Provider 为空、none 或未知值时 NewBackend 返回 nil,
// 表示本实例不启用 IM 通知。
type Settings struct {
	Provider string // "" | "none" | "telegram" | "feishu"
	Telegram TelegramSettings
	Feishu   FeishuSettings
}

// NewBackend 按已配置的 provider 构造 Backend;未配置/配置不完整返回 nil。
func NewBackend(settings Settings) Backend {
	switch settings.Provider {
	case "telegram":
		if strings.TrimSpace(settings.Telegram.BotToken) == "" {
			return nil
		}
		return NewTelegramBackend(settings.Telegram)
	case "feishu":
		if strings.TrimSpace(settings.Feishu.WebhookURL) == "" {
			return nil
		}
		return NewFeishuBackend(settings.Feishu)
	default:
		return nil
	}
}
