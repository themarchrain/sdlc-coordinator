package gitlabsmoke

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
)

type Options struct {
	BaseURL     string
	PayloadPath string
	RawPayload  []byte
	Secret      string
	EventName   string
	EventUUID   string
	WebhookUUID string
	InstanceURL string
	DryRun      bool
	Output      io.Writer
	HTTPClient  *http.Client
}

type Result struct {
	EventUUID           string
	WebhookStatusCode   int
	WebhookResponseBody string
	DryRun              bool
}

func Run(ctx context.Context, opts Options) (*Result, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	payload, err := loadPayload(opts)
	if err != nil {
		return nil, err
	}

	eventUUID := strings.TrimSpace(opts.EventUUID)
	if eventUUID == "" {
		eventUUID = "smoke-" + uuid.NewString()
	}

	webhookURL := baseURL + "/api/events/gitlab"
	headers := webhookHeaders(opts, eventUUID)
	result := &Result{
		EventUUID: eventUUID,
		DryRun:    opts.DryRun,
	}

	if opts.DryRun {
		if err := renderDryRun(opts.output(), webhookURL, headers, payload); err != nil {
			return nil, err
		}
		return result, nil
	}

	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	status, body, err := postJSON(ctx, client, webhookURL, payload, headers)
	result.WebhookStatusCode = status
	result.WebhookResponseBody = body
	if err != nil {
		return result, fmt.Errorf("gitlab webhook replay failed: %w", err)
	}

	return result, nil
}

func (o Options) output() io.Writer {
	if o.Output == nil {
		return io.Discard
	}
	return o.Output
}

func loadPayload(opts Options) ([]byte, error) {
	if len(opts.RawPayload) > 0 {
		return opts.RawPayload, nil
	}
	path := strings.TrimSpace(opts.PayloadPath)
	if path == "" {
		return nil, fmt.Errorf("payload path or raw payload is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read payload %s: %w", path, err)
	}
	return raw, nil
}

func webhookHeaders(opts Options, eventUUID string) map[string]string {
	headers := map[string]string{
		"Content-Type":          "application/json",
		"X-Gitlab-Event-UUID":   eventUUID,
		"X-Gitlab-Webhook-UUID": strings.TrimSpace(opts.WebhookUUID),
		"X-Gitlab-Instance":     strings.TrimSpace(opts.InstanceURL),
	}
	if eventName := strings.TrimSpace(opts.EventName); eventName != "" {
		headers["X-Gitlab-Event"] = eventName
	}
	if secret := strings.TrimSpace(opts.Secret); secret != "" {
		headers["X-Gitlab-Token"] = secret
	}
	for key, value := range headers {
		if strings.TrimSpace(value) == "" {
			delete(headers, key)
		}
	}
	return headers
}

func postJSON(ctx context.Context, client *http.Client, url string, body []byte, headers map[string]string) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, "", fmt.Errorf("build request: %w", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	responseBody := string(raw)
	if readErr != nil {
		return resp.StatusCode, responseBody, fmt.Errorf("read response body: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, responseBody, fmt.Errorf("status %d: %s", resp.StatusCode, truncate(responseBody, 2048))
	}
	return resp.StatusCode, responseBody, nil
}

func renderDryRun(out io.Writer, webhookURL string, headers map[string]string, payload []byte) error {
	redactedHeaders := make(map[string]string, len(headers))
	for key, value := range headers {
		if strings.EqualFold(key, "X-Gitlab-Token") {
			redactedHeaders[key] = "<configured>"
			continue
		}
		redactedHeaders[key] = value
	}

	doc := map[string]any{
		"dry_run": true,
		"webhook_request": map[string]any{
			"method":        http.MethodPost,
			"url":           webhookURL,
			"headers":       redactedHeaders,
			"payload_bytes": len(payload),
		},
	}

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(doc)
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "...<truncated>"
}
