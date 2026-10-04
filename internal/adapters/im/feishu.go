package im

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// FeishuSettings 驱动飞书群自定义机器人 webhook;Mentions 为显示名 -> open_id。
type FeishuSettings struct {
	WebhookURL string
	Secret     string // 可选;配置后启用签名校验
	Mentions   map[string]string
}

type FeishuBackend struct {
	webhookURL string
	secret     string
	mentions   map[string]string
	httpClient *http.Client
}

func NewFeishuBackend(settings FeishuSettings) *FeishuBackend {
	return &FeishuBackend{
		webhookURL: strings.TrimSpace(settings.WebhookURL),
		secret:     strings.TrimSpace(settings.Secret),
		mentions:   settings.Mentions,
		httpClient: sendHTTPClient,
	}
}

func (backend *FeishuBackend) SendMessage(ctx context.Context, req SendMessageRequest) (*Message, error) {
	if backend.webhookURL == "" {
		return nil, fmt.Errorf("feishu webhook url is required")
	}

	text := strings.TrimSpace(req.Body)
	for _, name := range req.Mentions {
		displayName := strings.TrimSpace(name)
		if displayName == "" {
			continue
		}
		if openID, ok := backend.mentions[displayName]; ok && strings.TrimSpace(openID) != "" {
			text += "\n" + fmt.Sprintf(`<at user_id="%s">%s</at>`, strings.TrimSpace(openID), displayName)
			continue
		}
		text += "\n" + displayName
	}

	payload := map[string]any{
		"msg_type": "text",
		"content":  map[string]any{"text": text},
	}
	if backend.secret != "" {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		sign, err := feishuSign(backend.secret, timestamp)
		if err != nil {
			return nil, fmt.Errorf("compute feishu sign: %w", err)
		}
		payload["timestamp"] = timestamp
		payload["sign"] = sign
	}

	rawBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode feishu payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, backend.webhookURL, bytes.NewReader(rawBody))
	if err != nil {
		return nil, fmt.Errorf("create feishu request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := backend.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("perform feishu request: transport failed")
	}
	defer resp.Body.Close()

	// 回执按全量读取,不设字节上限:截断读取会把已送达的消息误标为
	// 失败;请求整体超时由 sendHTTPClient 兜底。
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read feishu response: %w", err)
	}
	raw := strings.TrimSpace(string(data))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		// 非 2xx 只报状态码:错误消息会随效果记录外显,不携带远端响应体。
		return nil, fmt.Errorf("feishu webhook: %s", resp.Status)
	}

	var parsed struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("decode feishu response: %w", err)
	}
	if parsed.Code != 0 {
		// 业务码失败只报码不抄远端原文:错误消息随效果记录外显,抄录会
		// 把任意外部响应体带进管理台。
		return nil, fmt.Errorf("feishu webhook failed: code %d", parsed.Code)
	}
	return &Message{RawJSON: raw, Provider: "feishu"}, nil
}

// feishuSign 实现飞书自定义机器人签名算法:
// sign = base64(hmac_sha256(key = timestamp + "\n" + secret, message = ""))
func feishuSign(secret, timestamp string) (string, error) {
	mac := hmac.New(sha256.New, []byte(timestamp+"\n"+secret))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}
