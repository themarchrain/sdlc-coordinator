package im

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 回归背景:TG 通知以 parse_mode=HTML 发送,正文却是 Markdown,记号原样
// 显示不渲染。这组测试锁定「发送前把 Markdown 子集转换为 Telegram HTML,
// 字面 HTML 仍必须转义,转换失败降级纯文本」。

func TestMarkdownToTelegramHTML(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text passthrough", "hello world", "hello world"},
		{"literal html is escaped", "a <b> & c", "a &lt;b&gt; &amp; c"},
		{"heading level 1", "# 标题", "<b>标题</b>"},
		{"heading level 2", "## Reviewer Agent 完成", "<b>Reviewer Agent 完成</b>"},
		{"heading level 3", "### 小节", "<b>小节</b>"},
		{"heading without space stays literal", "#nospace", "#nospace"},
		{"list item", "- 类型：新建 MR", "• 类型：新建 MR"},
		{"dash without space stays literal", "-x", "-x"},
		{"blockquote line", "> 引用", "<blockquote>引用</blockquote>"},
		{"blockquote with list and link", "> - Issue: [#18](http://h/18)", "<blockquote>• Issue: <a href=\"http://h/18\">#18</a></blockquote>"},
		{"code span", "`run_123`", "<code>run_123</code>"},
		{"code span escapes inner html", "`<b>x</b>`", "<code>&lt;b&gt;x&lt;/b&gt;</code>"},
		{"unclosed code span stays literal", "a ` b", "a ` b"},
		{"bold", "**重要**", "<b>重要</b>"},
		{"bold with inner code", "**`sha`**", "<b><code>sha</code></b>"},
		{"unclosed bold stays literal", "a ** b", "a ** b"},
		{"markdown link", "[#18](http://h/18)", "<a href=\"http://h/18\">#18</a>"},
		{"https link", "[x](https://h/p)", "<a href=\"https://h/p\">x</a>"},
		{"link url ampersand escaped", "[x](http://h/?a=1&b=2)", "<a href=\"http://h/?a=1&amp;b=2\">x</a>"},
		{"link label escapes html", "[<x>](http://h/)", "<a href=\"http://h/\">&lt;x&gt;</a>"},
		{"non-http link stays literal", "[x](javascript:alert(1))", "[x](javascript:alert(1))"},
		{"unclosed link stays literal", "[x](http://h/", "[x](http://h/"},
		{"numbered list passes through", "1. 第一项", "1. 第一项"},
		{"italic marker passes through", "_强调_", "_强调_"},
		{"strikethrough passes through", "~~删除线~~", "~~删除线~~"},
		{"multiline reviewer body",
			"## 审查完成 — MR !19\n\n- MR：[标题](http://g/19)\n- 提交：`406f3ad3`\n\n请进入 MR 查看。",
			"<b>审查完成 — MR !19</b>\n\n• MR：<a href=\"http://g/19\">标题</a>\n• 提交：<code>406f3ad3</code>\n\n请进入 MR 查看。"},
	}
	for _, tc := range cases {
		if got := markdownToTelegramHTML(tc.in); got != tc.want {
			t.Errorf("%s: markdownToTelegramHTML(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestTelegramBackendConvertsMarkdownBodyToHTML(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7}}`))
	}))
	defer server.Close()

	backend := NewTelegramBackend(TelegramSettings{BotToken: "t", APIBase: server.URL, DefaultChatID: "1"})
	if _, err := backend.SendMessage(context.Background(), SendMessageRequest{
		Body: "## Reviewer Agent 代码审查完成 — MR !19\n\n- MR：[标题](http://g/19)\n- 当前审查提交：`406f3ad3`",
	}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if received["parse_mode"] != "HTML" {
		t.Fatalf("parse_mode = %v", received["parse_mode"])
	}
	text, _ := received["text"].(string)
	for _, expected := range []string{"<b>Reviewer Agent 代码审查完成 — MR !19</b>", `<a href="http://g/19">标题</a>`, "• 当前审查提交：<code>406f3ad3</code>"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("rendered text missing %q, got %q", expected, text)
		}
	}
	if strings.Contains(text, "##") || strings.Contains(text, "](http") {
		t.Fatalf("raw markdown leaked into text: %q", text)
	}
}

func TestTelegramBackendFallsBackToPlainTextWhenEntitiesRejected(t *testing.T) {
	var payloads []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		payloads = append(payloads, payload)
		if len(payloads) == 1 {
			_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: can't parse entities: unclosed tag at byte 5"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":9}}`))
	}))
	defer server.Close()

	backend := NewTelegramBackend(TelegramSettings{BotToken: "t", APIBase: server.URL, DefaultChatID: "1"})
	message, err := backend.SendMessage(context.Background(), SendMessageRequest{
		Body:     "**加粗** [链接](http://h/)",
		Mentions: []string{"alice"},
	})
	if err != nil {
		t.Fatalf("fallback send must succeed: %v", err)
	}
	if message == nil || message.ID != "9" {
		t.Fatalf("message = %#v", message)
	}
	if len(payloads) != 2 {
		t.Fatalf("expected html attempt plus plain fallback, got %d requests", len(payloads))
	}
	if payloads[0]["parse_mode"] != "HTML" {
		t.Fatalf("first attempt parse_mode = %v", payloads[0]["parse_mode"])
	}
	if _, has := payloads[1]["parse_mode"]; has {
		t.Fatalf("fallback must drop parse_mode, got %v", payloads[1]["parse_mode"])
	}
	plain, _ := payloads[1]["text"].(string)
	if plain != "**加粗** [链接](http://h/)\n@alice" {
		t.Fatalf("fallback text must be the original body with plain mentions, got %q", plain)
	}
}

func TestTelegramBackendDoesNotFallbackOnNonParseErrors(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: chat not found"}`))
	}))
	defer server.Close()

	backend := NewTelegramBackend(TelegramSettings{BotToken: "t", APIBase: server.URL, DefaultChatID: "1"})
	if _, err := backend.SendMessage(context.Background(), SendMessageRequest{Body: "**x**"}); err == nil || strings.Contains(err.Error(), "chat not found") {
		t.Fatalf("api error must omit the remote description, got %v", err)
	}
	if requests != 1 {
		t.Fatalf("non-parse error must not trigger fallback, got %d requests", requests)
	}
}
