package im

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTelegramBackendSendsHTMLMessageWithDefaultChat(t *testing.T) {
	var received map[string]any
	var authPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode: %v", err)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42}}`))
	}))
	defer server.Close()

	backend := NewTelegramBackend(TelegramSettings{
		BotToken:      "token-1",
		APIBase:       server.URL,
		DefaultChatID: "-100123",
		Mentions:      map[string]string{"Alice": "7101"},
	})
	message, err := backend.SendMessage(context.Background(), SendMessageRequest{
		Body:     "Review 完成\n\n<a href=\"x\">点击</a>",
		Mentions: []string{"Alice", "未知成员"},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if authPath != "/bottoken-1/sendMessage" {
		t.Fatalf("path = %q", authPath)
	}
	if received["chat_id"] != "-100123" {
		t.Fatalf("chat_id = %v", received["chat_id"])
	}
	if received["parse_mode"] != "HTML" {
		t.Fatalf("parse_mode = %v", received["parse_mode"])
	}
	text, _ := received["text"].(string)
	if !strings.HasPrefix(text, "Review 完成\n\n&lt;a href=\"x\"&gt;点击&lt;/a&gt;") {
		t.Fatalf("body must be HTML-escaped: %q", text)
	}
	if !strings.Contains(text, `<a href="tg://user?id=7101">@Alice</a>`) {
		t.Fatalf("mapped mention missing: %q", text)
	}
	if !strings.Contains(text, "@未知成员") {
		t.Fatalf("unmapped mention must fall back to plain name: %q", text)
	}
	if message == nil || message.ID != "42" || message.Provider != "telegram" {
		t.Fatalf("message = %#v", message)
	}
}

func TestTelegramBackendTargetOverridesDefaultChat(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer server.Close()

	backend := NewTelegramBackend(TelegramSettings{BotToken: "t", APIBase: server.URL, DefaultChatID: "default"})
	if _, err := backend.SendMessage(context.Background(), SendMessageRequest{Target: "777", Body: "hi"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if received["chat_id"] != "777" {
		t.Fatalf("target must override default chat: %v", received["chat_id"])
	}
}

func TestTelegramBackendRejectsMissingChatAndReportsAPIErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// 业务失败走非 2xx 状态码(真实 API 契约)。
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: chat not found"}`))
	}))
	defer server.Close()

	noChat := NewTelegramBackend(TelegramSettings{BotToken: "t", APIBase: server.URL})
	if _, err := noChat.SendMessage(context.Background(), SendMessageRequest{Body: "hi"}); err == nil || !strings.Contains(err.Error(), "chat") {
		t.Fatalf("missing chat must fail with chat hint, got %v", err)
	}

	apiError := NewTelegramBackend(TelegramSettings{BotToken: "t", APIBase: server.URL, DefaultChatID: "1"})
	_, err := apiError.SendMessage(context.Background(), SendMessageRequest{Body: "hi"})
	if err == nil || err.Error() != "telegram sendMessage: 400 Bad Request" || strings.Contains(err.Error(), "chat not found") {
		t.Fatalf("api error must report the status line and omit the remote description, got %v", err)
	}
}

func TestTelegramBackendNon2xxErrorOmitsResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"internal":"secret-body"}`))
	}))
	defer server.Close()

	backend := NewTelegramBackend(TelegramSettings{BotToken: "t", APIBase: server.URL, DefaultChatID: "1"})
	_, err := backend.SendMessage(context.Background(), SendMessageRequest{Body: "hi"})
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("non-2xx must report status, got %v", err)
	}
	if strings.Contains(err.Error(), "secret-body") {
		t.Fatalf("non-2xx error must not carry remote body, got %v", err)
	}
}
