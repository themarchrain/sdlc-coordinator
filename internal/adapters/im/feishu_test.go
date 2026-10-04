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
	if _, err := backend.SendMessage(context.Background(), SendMessageRequest{Body: "hi"}); err == nil || err.Error() != "feishu webhook failed: code 19021" || strings.Contains(err.Error(), "sign match fail") {
		t.Fatalf("business error must carry the code but omit the remote msg, got %v", err)
	}

	missing := NewFeishuBackend(FeishuSettings{})
	if _, err := missing.SendMessage(context.Background(), SendMessageRequest{Body: "hi"}); err == nil || !strings.Contains(err.Error(), "webhook") {
		t.Fatalf("missing webhook url must fail, got %v", err)
	}
}

func TestFeishuBackendNon2xxErrorOmitsResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"internal":"secret-body"}`))
	}))
	defer server.Close()

	backend := NewFeishuBackend(FeishuSettings{WebhookURL: server.URL})
	_, err := backend.SendMessage(context.Background(), SendMessageRequest{Body: "hi"})
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("non-2xx must report status, got %v", err)
	}
	if strings.Contains(err.Error(), "secret-body") {
		t.Fatalf("non-2xx error must not carry remote body, got %v", err)
	}
}

func TestFeishuBackendReadsFullReceiptBeyond4096(t *testing.T) {
	// 飞书自定义机器人回执体量小,但按字节上限截断读取是同款隐患:
	// 响应变大会以同样方式把已送达的消息误标为失败。
	receipt := `{"code":0,"msg":"success","data":{"note":"` + strings.Repeat("回执", 2000) + `END-OF-RECEIPT"}}`
	if len(receipt) <= 4096 {
		t.Fatalf("receipt fixture must exceed the old 4096 cap, got %d bytes", len(receipt))
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(receipt))
	}))
	defer server.Close()

	backend := NewFeishuBackend(FeishuSettings{WebhookURL: server.URL})
	message, err := backend.SendMessage(context.Background(), SendMessageRequest{Body: "hi"})
	if err != nil {
		t.Fatalf("long receipt must not fail the delivery: %v", err)
	}
	if !strings.Contains(message.RawJSON, "END-OF-RECEIPT") {
		t.Fatalf("receipt must be read in full, got %d bytes", len(message.RawJSON))
	}
}

func TestFeishuBackendMarksOversizedReceiptUnverified(t *testing.T) {
	// 与 Telegram 相同的读取上限:超限即停,记投递未核验,错误只含有界
	// 元数据,不携带远端响应正文。
	receipt := `{"code":0,"msg":"success","data":"` + strings.Repeat("回执", 200000) + `"}`
	if len(receipt) <= receiptLimit {
		t.Fatalf("receipt fixture must exceed the limit, got %d bytes", len(receipt))
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(receipt))
	}))
	defer server.Close()

	backend := NewFeishuBackend(FeishuSettings{WebhookURL: server.URL})
	_, err := backend.SendMessage(context.Background(), SendMessageRequest{Body: "hi"})
	var unverified *DeliveryUnverifiedError
	if !errors.As(err, &unverified) {
		t.Fatalf("oversized receipt must be delivery unverified, got %v", err)
	}
	if unverified.Reason != ReasonReceiptExceedsLimit || unverified.Read != receiptLimit+1 {
		t.Fatalf("unexpected unverified error: %+v", unverified)
	}
	if text := err.Error(); strings.Contains(text, "回执") || len(text) > 200 {
		t.Fatalf("unverified error must stay bounded and omit the remote body: %q", text)
	}
}

func TestFeishuBackendMarksInterruptedReceiptUnverified(t *testing.T) {
	// 2xx 已收到但回执传输中断:可能已送达,记投递未核验,不当普通失败。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "4096")
		_, _ = w.Write([]byte(`{"code":0,`))
	}))
	defer server.Close()

	backend := NewFeishuBackend(FeishuSettings{WebhookURL: server.URL})
	_, err := backend.SendMessage(context.Background(), SendMessageRequest{Body: "hi"})
	var unverified *DeliveryUnverifiedError
	if !errors.As(err, &unverified) || unverified.Reason != ReasonReceiptReadFailed {
		t.Fatalf("interrupted receipt must be delivery unverified, got %v", err)
	}
}

func TestSendHTTPClientHasTimeout(t *testing.T) {
	if sendHTTPClient == nil || sendHTTPClient.Timeout <= 0 {
		t.Fatalf("shared send client must carry a timeout, got %+v", sendHTTPClient)
	}
}
