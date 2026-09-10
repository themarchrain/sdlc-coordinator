// gitea-webhook-smoke 向本机 coordinator 回放一个带 HMAC 签名的 Gitea issue
// webhook,用于联调入站链路;派发由协调器调度循环或管理台安全操作页驱动。
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
	payloadPath    string
	defaultPayload string
}

func main() {
	opts := options{}
	flag.StringVar(&opts.baseURL, "base-url", env("COORDINATOR_BASE_URL", "http://localhost:8080"), "coordinator base URL")
	flag.StringVar(&opts.secret, "secret", env("CODEHOST_WEBHOOK_SECRET", ""), "Gitea webhook secret used for HMAC signing")
	flag.StringVar(&opts.eventType, "event", "issues", "X-Gitea-Event header value")
	flag.StringVar(&opts.deliveryID, "delivery", fmt.Sprintf("smoke-%d", time.Now().UnixMilli()), "X-Gitea-Delivery header value")
	flag.StringVar(&opts.payloadPath, "payload", "", "path to a JSON payload file; omit to use the built-in issue payload")
	flag.Parse()
	opts.defaultPayload = builtInIssuePayload()

	if err := run(context.Background(), opts); err != nil {
		fmt.Fprintln(os.Stderr, "gitea smoke failed:", err)
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

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, opts.baseURL+"/api/events/gitea", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Gitea-Event", opts.eventType)
	request.Header.Set("X-Gitea-Delivery", opts.deliveryID)
	if opts.secret != "" {
		mac := hmac.New(sha256.New, []byte(opts.secret))
		mac.Write(payload)
		request.Header.Set("X-Gitea-Signature", hex.EncodeToString(mac.Sum(nil)))
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
		"number": 7,
		"issue": map[string]any{
			"id": 100, "number": 7, "title": "Gitea smoke issue", "body": "gitea-webhook-smoke",
			"state": "open", "html_url": "https://gitea.example/group/repo/issues/7",
			"updated_at": time.Now().Format(time.RFC3339),
			"labels":     []map[string]any{{"name": "developer-agent"}},
			"assignees":  []map[string]any{{"id": 1, "login": "owner"}},
		},
		"repository": map[string]any{
			"id": 42, "full_name": "group/repo",
			"clone_url":      "https://gitea.example/group/repo.git",
			"default_branch": "main",
		},
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}
