// github-webhook-smoke 向本机 coordinator 回放一个带 HMAC 签名的 GitHub
// issue webhook,用于联调入站链路;派发由协调器调度循环或管理台安全操作页驱动。
package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

type options struct {
	baseURL        string
	secret         string
	eventType      string
	deliveryID     string
	builtin        string
	payloadPath    string
	defaultPayload string
}

func main() {
	opts := options{}
	flag.StringVar(&opts.baseURL, "base-url", env("COORDINATOR_BASE_URL", "http://localhost:8080"), "coordinator base URL")
	flag.StringVar(&opts.secret, "secret", env("CODEHOST_WEBHOOK_SECRET", ""), "GitHub webhook secret used for HMAC signing")
	flag.StringVar(&opts.eventType, "event", "issues", "X-GitHub-Event header value")
	flag.StringVar(&opts.deliveryID, "delivery", fmt.Sprintf("smoke-%d", time.Now().UnixMilli()), "X-GitHub-Delivery header value")
	flag.StringVar(&opts.builtin, "builtin", "issues", "built-in payload: issues or pr-comment")
	flag.StringVar(&opts.payloadPath, "payload", "", "path to a JSON payload file; omit to use the built-in payload")
	flag.Parse()
	switch opts.builtin {
	case "pr-comment":
		opts.defaultPayload = builtInPRCommentPayload()
		if opts.eventType == "issues" {
			opts.eventType = "issue_comment"
		}
	default:
		opts.defaultPayload = builtInIssuePayload()
	}

	if err := run(context.Background(), opts); err != nil {
		fmt.Fprintln(os.Stderr, "github smoke failed:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, opts options) error {
	payload := []byte(opts.defaultPayload)
	if opts.payloadPath != "" {
		raw, err := os.ReadFile(opts.payloadPath)
		if err != nil {
			return fmt.Errorf("read payload: %w", err)
		}
		payload = raw
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, opts.baseURL+"/api/events/github", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-GitHub-Event", opts.eventType)
	request.Header.Set("X-GitHub-Delivery", opts.deliveryID)
	if opts.secret != "" {
		mac := hmac.New(sha256.New, []byte(opts.secret))
		mac.Write(payload)
		request.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("send webhook: %w", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	fmt.Printf("status: %s\nbody: %s\n", response.Status, strings.TrimSpace(string(body)))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %s", response.Status)
	}
	return nil
}

func builtInIssuePayload() string {
	payload := map[string]any{
		"action": "opened",
		"issue": map[string]any{
			"id": 100, "number": 7, "title": "GitHub smoke issue", "body": "github-webhook-smoke",
			"state": "open", "html_url": "https://github.com/org/repo/issues/7",
			"updated_at": time.Now().Format(time.RFC3339),
			"user":       map[string]any{"id": 71, "login": "author1"},
			"labels":     []map[string]any{{"name": "developer-agent"}},
			"assignees":  []map[string]any{{"id": 1, "login": "owner"}},
		},
		"repository": map[string]any{
			"id": 72573381, "full_name": "org/repo",
			"clone_url":      "https://github.com/org/repo.git",
			"default_branch": "main",
		},
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}

// builtInPRCommentPayload 复刻 issue_comment(PR) 载荷:issue 对象带 pull_request
// 标记键且不携带 head/base(平台设计差异),用于自测外部 PR rework 入站链。
func builtInPRCommentPayload() string {
	payload := map[string]any{
		"action": "created",
		"issue": map[string]any{
			"id": 200, "number": 17, "title": "GitHub smoke PR", "body": "external pr",
			"state": "open", "html_url": "https://github.com/org/repo/pull/17",
			"pull_request": map[string]any{
				"html_url": "https://github.com/org/repo/pull/17",
				"url":      "https://api.github.com/repos/org/repo/pulls/17",
			},
			"user":      map[string]any{"id": 71, "login": "author1"},
			"labels":    []map[string]any{},
			"assignees": []map[string]any{{"id": 1, "login": "owner"}},
		},
		"comment": map[string]any{
			"id": 50001, "body": "请修正空值处理并补充对应测试。",
			"html_url": "https://github.com/org/repo/pull/17#issuecomment-50001",
			"user":     map[string]any{"id": 1, "login": "owner"},
		},
		"repository": map[string]any{
			"id": 72573381, "full_name": "org/repo",
			"clone_url": "https://github.com/org/repo.git", "default_branch": "main",
		},
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}
