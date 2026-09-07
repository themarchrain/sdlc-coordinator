package im

import (
	"context"
	"testing"
)

func TestNewBackendSelectsProvider(t *testing.T) {
	settings := Settings{
		Provider: "telegram",
		Telegram: TelegramSettings{BotToken: "t", DefaultChatID: "1"},
	}
	if _, ok := NewBackend(settings).(Backend); !ok {
		t.Fatal("telegram provider must return a backend")
	}

	settings.Provider = "feishu"
	settings.Feishu = FeishuSettings{WebhookURL: "https://open.feishu.cn/hook/x"}
	if _, ok := NewBackend(settings).(Backend); !ok {
		t.Fatal("feishu provider must return a backend")
	}
}

func TestNewBackendReturnsNilWithoutProvider(t *testing.T) {
	if backend := NewBackend(Settings{}); backend != nil {
		t.Fatalf("empty provider must disable im, got %#v", backend)
	}
	if backend := NewBackend(Settings{Provider: "none"}); backend != nil {
		t.Fatalf("provider none must disable im, got %#v", backend)
	}
	if backend := NewBackend(Settings{Provider: "carrier-pigeon"}); backend != nil {
		t.Fatalf("unknown provider must disable im, got %#v", backend)
	}
	if backend := NewBackend(Settings{Provider: "telegram"}); backend != nil {
		t.Fatalf("telegram without bot token must not wire, got %#v", backend)
	}
	if backend := NewBackend(Settings{Provider: "feishu"}); backend != nil {
		t.Fatalf("feishu without webhook url must not wire, got %#v", backend)
	}
}

func TestNilBackendSkipsWithoutError(t *testing.T) {
	var backend Backend = NewBackend(Settings{})
	if backend != nil {
		t.Fatal("precondition: nil backend")
	}
	// nil interface 消费方(executor)负责跳过;这里锁住接口形态。
	_ = backend
	_ = context.Background()
}
