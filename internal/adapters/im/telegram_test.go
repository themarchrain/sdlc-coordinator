package im

import (
	"context"
	"encoding/json"
	"errors"
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

func TestTelegramBackendReadsFullReceiptBeyond4096(t *testing.T) {
	// 真实 Bot API 的成功回执会把整条消息文本回显在 result.text;长中文通知
	// 的回执超过 4096 字节,按字节上限截断读取会把已送达的消息误标为失败。
	receiptText := strings.Repeat("审查结果正常,交付通知已送达。", 300) + "END-OF-RECEIPT"
	receipt, err := json.Marshal(map[string]any{
		"ok":     true,
		"result": map[string]any{"message_id": 42, "text": receiptText},
	})
	if err != nil {
		t.Fatalf("build receipt: %v", err)
	}
	if len(receipt) <= 4096 {
		t.Fatalf("receipt fixture must exceed the old 4096 cap, got %d bytes", len(receipt))
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(receipt)
	}))
	defer server.Close()

	backend := NewTelegramBackend(TelegramSettings{BotToken: "t", APIBase: server.URL, DefaultChatID: "1"})
	message, err := backend.SendMessage(context.Background(), SendMessageRequest{Body: "hi"})
	if err != nil {
		t.Fatalf("long receipt must not fail the delivery: %v", err)
	}
	if message == nil || message.ID != "42" {
		t.Fatalf("message = %#v", message)
	}
	if !strings.Contains(message.RawJSON, "END-OF-RECEIPT") {
		t.Fatalf("receipt must be read in full, got %d bytes", len(message.RawJSON))
	}
}

func TestTelegramBackendMarksInterruptedReceiptUnverified(t *testing.T) {
	// 声明 4096 字节却只写半截:2xx 已收到但回执传输中断,消息可能已送达,
	// 不得把半截响应当普通失败(会误导重发),应记为投递未核验。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "4096")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"mess`))
	}))
	defer server.Close()

	backend := NewTelegramBackend(TelegramSettings{BotToken: "t", APIBase: server.URL, DefaultChatID: "1"})
	_, err := backend.SendMessage(context.Background(), SendMessageRequest{Body: "hi"})
	var unverified *DeliveryUnverifiedError
	if !errors.As(err, &unverified) {
		t.Fatalf("interrupted receipt must be delivery unverified, got %v", err)
	}
	if unverified.Reason != ReasonReceiptReadFailed {
		t.Fatalf("reason = %q", unverified.Reason)
	}
}

func TestTelegramBackendMarksOversizedReceiptUnverified(t *testing.T) {
	// 中转端返回异常大内容:读到上限+1 字节即停,记投递未核验;
	// 错误只含有界元数据,不携带远端响应正文。
	receipt := `{"ok":true,"result":{"message_id":42,"text":"` + strings.Repeat("回执", 200000) + `"}}`
	if len(receipt) <= receiptLimit {
		t.Fatalf("receipt fixture must exceed the limit, got %d bytes", len(receipt))
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(receipt))
	}))
	defer server.Close()

	backend := NewTelegramBackend(TelegramSettings{BotToken: "t", APIBase: server.URL, DefaultChatID: "1"})
	_, err := backend.SendMessage(context.Background(), SendMessageRequest{Body: "hi"})
	var unverified *DeliveryUnverifiedError
	if !errors.As(err, &unverified) {
		t.Fatalf("oversized receipt must be delivery unverified, got %v", err)
	}
	if unverified.Reason != ReasonReceiptExceedsLimit {
		t.Fatalf("reason = %q", unverified.Reason)
	}
	if unverified.Read != receiptLimit+1 {
		t.Fatalf("must stop reading at limit+1, read %d", unverified.Read)
	}
	if text := err.Error(); strings.Contains(text, "回执") || len(text) > 200 {
		t.Fatalf("unverified error must stay bounded and omit the remote body: %q", text)
	}
}

func TestTelegramBackendStopsReadingUnboundedReceipt(t *testing.T) {
	// 无限流响应:读到上限+1 即停并关闭连接;若实现退回全量读取,会一直
	// 读到客户端超时,错误形态随之不同,本测试即为红灯。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := []byte(strings.Repeat("a", 4096))
		for {
			select {
			case <-r.Context().Done():
				return
			default:
			}
			if _, err := w.Write(chunk); err != nil {
				return
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}))
	defer server.Close()

	backend := NewTelegramBackend(TelegramSettings{BotToken: "t", APIBase: server.URL, DefaultChatID: "1"})
	_, err := backend.SendMessage(context.Background(), SendMessageRequest{Body: "hi"})
	var unverified *DeliveryUnverifiedError
	if !errors.As(err, &unverified) || unverified.Reason != ReasonReceiptExceedsLimit {
		t.Fatalf("unbounded receipt must stop at the limit and be unverified, got %v", err)
	}
}

func TestTelegramBackendAcceptsReceiptAtExactLimit(t *testing.T) {
	base := `{"ok":true,"result":{"message_id":42,"text":"`
	tail := `"}}`
	receipt := base + strings.Repeat("a", receiptLimit-len(base)-len(tail)) + tail
	if len(receipt) != receiptLimit {
		t.Fatalf("fixture must be exactly the limit, got %d bytes", len(receipt))
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(receipt))
	}))
	defer server.Close()

	backend := NewTelegramBackend(TelegramSettings{BotToken: "t", APIBase: server.URL, DefaultChatID: "1"})
	message, err := backend.SendMessage(context.Background(), SendMessageRequest{Body: "hi"})
	if err != nil || message == nil || message.ID != "42" {
		t.Fatalf("receipt at the exact limit must succeed, got %v", err)
	}
}

func TestTelegramReceiptLimitCoversWorstLegalReceipt(t *testing.T) {
	// sendMessage 正文上限 4096 字符,成功回执回显全文且非 ASCII 全量
	// \uXXXX 转义(每字符 6 字节),再加实体数组与 chat/from 元数据;
	// 上限必须覆盖该最坏常见编码并留有余量,否则合法长通知会被误记未核验。
	var text strings.Builder
	for i := 0; i < 4096; i++ {
		text.WriteString(`\u5ba1`)
	}
	entities := strings.TrimRight(strings.Repeat(`{"offset":0,"length":1,"type":"bold"},`, 200), ",")
	receipt := `{"ok":true,"result":{"message_id":42,"date":1700000000,"text":"` + text.String() +
		`","entities":[` + entities + `],"chat":{"id":-100123,"title":"review","type":"group"},"from":{"id":7101,"is_bot":true,"first_name":"bot"}}}`
	if len(receipt) >= receiptLimit {
		t.Fatalf("limit %d does not cover the worst legal receipt (%d bytes)", receiptLimit, len(receipt))
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(receipt))
	}))
	defer server.Close()

	backend := NewTelegramBackend(TelegramSettings{BotToken: "t", APIBase: server.URL, DefaultChatID: "1"})
	message, err := backend.SendMessage(context.Background(), SendMessageRequest{Body: "hi"})
	if err != nil || message == nil || message.ID != "42" {
		t.Fatalf("worst legal receipt must parse successfully, got %v", err)
	}
}

func TestTelegramBackendNon2xxOversizedBodyStillReportsStatus(t *testing.T) {
	// 非 2xx 是渠道的明确拒绝:即使错误体超限,结论仍由状态码给出,
	// 不记投递未核验。
	receipt := `{"description":"` + strings.Repeat("x", 100000) + `"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(receipt))
	}))
	defer server.Close()

	backend := NewTelegramBackend(TelegramSettings{BotToken: "t", APIBase: server.URL, DefaultChatID: "1"})
	_, err := backend.SendMessage(context.Background(), SendMessageRequest{Body: "hi"})
	var unverified *DeliveryUnverifiedError
	if err == nil || errors.As(err, &unverified) || !strings.Contains(err.Error(), "502") {
		t.Fatalf("non-2xx must keep reporting the status, got %v", err)
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
