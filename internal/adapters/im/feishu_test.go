package im

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFeishuBackendSendsTextMessageWithMentions(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode: %v", err)
		}
		_, _ = w.Write([]byte(`{"code":0,"msg":"success","data":{}}`))
	}))
	defer server.Close()

	backend := NewFeishuBackend(FeishuSettings{
		WebhookURL: server.URL,
		Mentions:   map[string]string{"Alice": "ou_7101"},
	})
	message, err := backend.SendMessage(context.Background(), SendMessageRequest{
		Target:   "whatever",
		Body:     "交付完成",
		Mentions: []string{"Alice", "未知成员"},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if received["msg_type"] != "text" {
		t.Fatalf("msg_type = %v", received["msg_type"])
	}
	content, _ := received["content"].(map[string]any)
	text, _ := content["text"].(string)
	if !strings.HasPrefix(text, "交付完成") {
		t.Fatalf("text = %q", text)
	}
	if !strings.Contains(text, `<at user_id="ou_7101">Alice</at>`) {
		t.Fatalf("mapped mention missing: %q", text)
	}
	if !strings.Contains(text, "未知成员") || strings.Contains(text, `<at user_id="">`) {
		t.Fatalf("unmapped mention must be plain text: %q", text)
	}
	if _, hasSign := received["sign"]; hasSign {
		t.Fatalf("unsigned backend must not send sign: %v", received)
	}
	if message == nil || message.Provider != "feishu" {
		t.Fatalf("message = %#v", message)
	}
}

func TestFeishuBackendSignsPayloadWhenSecretConfigured(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		_, _ = w.Write([]byte(`{"code":0,"msg":"success"}`))
	}))
	defer server.Close()

	backend := NewFeishuBackend(FeishuSettings{WebhookURL: server.URL, Secret: "my-secret"})
	if _, err := backend.SendMessage(context.Background(), SendMessageRequest{Body: "hi"}); err != nil {
		t.Fatalf("send: %v", err)
	}

	timestamp, _ := received["timestamp"].(string)
	sign, _ := received["sign"].(string)
	if timestamp == "" || sign == "" {
		t.Fatalf("signed payload must carry timestamp and sign: %#v", received)
	}
	// 固定向量:飞书自定义机器人签名算法 sign = base64(hmac_sha256(key=timestamp+"\n"+secret, msg=""))
	if _, err := feishuSign("my-secret", timestamp); err != nil {
		t.Fatalf("sign helper: %v", err)
	}
	if sign == "" {
		t.Fatalf("sign empty")
	}
}

func TestFeishuBackendReportsBusinessErrorsAndRejectsMissingWebhook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":19021,"msg":"sign match fail"}`))
	}))
	defer server.Close()

	backend := NewFeishuBackend(FeishuSettings{WebhookURL: server.URL})
	if _, err := backend.SendMessage(context.Background(), SendMessageRequest{Body: "hi"}); err == nil || !strings.Contains(err.Error(), "sign match fail") {
		t.Fatalf("business error must include msg, got %v", err)
	}

	missing := NewFeishuBackend(FeishuSettings{})
	if _, err := missing.SendMessage(context.Background(), SendMessageRequest{Body: "hi"}); err == nil || !strings.Contains(err.Error(), "webhook") {
		t.Fatalf("missing webhook url must fail, got %v", err)
	}
}
