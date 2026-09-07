package im

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// TelegramSettings 驱动 Telegram Bot API 实现;Mentions 为显示名 -> 数字用户 ID。
type TelegramSettings struct {
	BotToken      string
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
		httpClient:    http.DefaultClient,
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

	text := escapeTelegramHTML(strings.TrimSpace(req.Body))
	for _, name := range req.Mentions {
		displayName := strings.TrimSpace(name)
		if displayName == "" {
			continue
		}
		if userID, ok := backend.mentions[displayName]; ok && strings.TrimSpace(userID) != "" {
			text += "\n" + fmt.Sprintf(`<a href="tg://user?id=%s">@%s</a>`, strings.TrimSpace(userID), escapeTelegramHTML(displayName))
			continue
		}
		text += "\n@" + escapeTelegramHTML(displayName)
	}

	payload := map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}
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
		return nil, fmt.Errorf("telegram sendMessage: %s: %s", resp.Status, raw)
	}

	var parsed struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("decode telegram response: %w", err)
	}
	if !parsed.OK {
		return nil, fmt.Errorf("telegram sendMessage failed: %s", strings.TrimSpace(parsed.Description))
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
