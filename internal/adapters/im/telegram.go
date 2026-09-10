package im

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// TelegramSettings 驱动 Telegram Bot API 实现;Mentions 为显示名 -> 数字用户 ID。
type TelegramSettings struct {
	BotToken string
	// TODO: api_base/webhook_url 暂不限制目标网段(内网可用,自托管 bot-api
	// server 依赖此行为);未来若要拒绝私网目标以收敛出站探测面,须同时为
	// 自托管场景提供显式放行配置。
	APIBase       string // 空值用 https://api.telegram.org,自托管 bot-api server 可覆盖
	DefaultChatID string
	Mentions      map[string]string
}

type TelegramBackend struct {
	botToken      string
	apiBase       string
	defaultChatID string
	mentions      map[string]string
	httpClient    *http.Client
}

func NewTelegramBackend(settings TelegramSettings) *TelegramBackend {
	apiBase := strings.TrimRight(strings.TrimSpace(settings.APIBase), "/")
	if apiBase == "" {
		apiBase = "https://api.telegram.org"
	}
	return &TelegramBackend{
		botToken:      strings.TrimSpace(settings.BotToken),
		apiBase:       apiBase,
		defaultChatID: strings.TrimSpace(settings.DefaultChatID),
		mentions:      settings.Mentions,
		httpClient:    sendHTTPClient,
	}
}

func (backend *TelegramBackend) SendMessage(ctx context.Context, req SendMessageRequest) (*Message, error) {
	chatID := strings.TrimSpace(req.Target)
	if chatID == "" {
		chatID = backend.defaultChatID
	}
	if chatID == "" {
		return nil, fmt.Errorf("telegram chat id is required (effect target or im_telegram_default_chat_id)")
	}

	body := strings.TrimSpace(req.Body)
	text := markdownToTelegramHTML(body) + backend.mentionLines(req.Mentions, true)
	message, err := backend.postSendMessage(ctx, map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	})
	if err != nil && isTelegramEntityParseError(err) {
		// 渲染降级:转换产物被 Telegram 判为非法实体时(该请求确定未送达),
		// 用无 parse_mode 的纯文本重发原文,保证通知不因渲染失败丢失。
		return backend.postSendMessage(ctx, map[string]any{
			"chat_id":                  chatID,
			"text":                     body + backend.mentionLines(req.Mentions, false),
			"disable_web_page_preview": true,
		})
	}
	return message, err
}

// mentionLines 把通知的提及名单渲染为逐行追加的 @ 行;html 模式下已知映射
// 用户用 tg:// 链接,纯文本降级时退化为裸 @ 显示名。
func (backend *TelegramBackend) mentionLines(mentionNames []string, html bool) string {
	var out strings.Builder
	for _, name := range mentionNames {
		displayName := strings.TrimSpace(name)
		if displayName == "" {
			continue
		}
		out.WriteString("\n")
		if html {
			if userID, ok := backend.mentions[displayName]; ok && strings.TrimSpace(userID) != "" {
				fmt.Fprintf(&out, `<a href="tg://user?id=%s">@%s</a>`, strings.TrimSpace(userID), escapeTelegramHTML(displayName))
				continue
			}
			out.WriteString("@" + escapeTelegramHTML(displayName))
			continue
		}
		out.WriteString("@" + displayName)
	}
	return out.String()
}

// errTelegramEntityParse 标记「Telegram 判定消息实体非法」这一业务错误
// 类别:渲染降级的机器判据(errors.Is),哨兵文本固定,不携带远端原文。
var errTelegramEntityParse = errors.New("message entities rejected")

func isTelegramEntityParseError(err error) bool {
	return errors.Is(err, errTelegramEntityParse)
}

func (backend *TelegramBackend) postSendMessage(ctx context.Context, payload map[string]any) (*Message, error) {
	rawBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode telegram payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, backend.apiBase+"/bot"+backend.botToken+"/sendMessage", bytes.NewReader(rawBody))
	if err != nil {
		return nil, fmt.Errorf("create telegram request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := backend.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("perform telegram request: transport failed")
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	raw := strings.TrimSpace(string(data))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		// 非 2xx 只报状态码:错误消息会随效果记录外显,不携带远端响应体。
		return nil, fmt.Errorf("telegram sendMessage: %s", resp.Status)
	}

	var parsed struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"` // 仅内部错误分类,不进错误文本
		Result      struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("decode telegram response: %w", err)
	}
	if !parsed.OK {
		// 业务码失败不抄远端原文:错误消息随效果记录外显,抄录会把任意
		// 外部响应体带进管理台。渲染降级依赖的「实体非法」类别经哨兵
		// 结构化传递,判据本身不进错误文本。
		if parsed.Description != "" && strings.Contains(parsed.Description, "can't parse entities") {
			return nil, fmt.Errorf("telegram sendMessage failed: remote rejected: %w", errTelegramEntityParse)
		}
		return nil, fmt.Errorf("telegram sendMessage failed: remote rejected")
	}
	return &Message{
		ID:       fmt.Sprintf("%d", parsed.Result.MessageID),
		RawJSON:  raw,
		Provider: "telegram",
	}, nil
}

func escapeTelegramHTML(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(value)
}
