package gitlabsmoke

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunPostsGitLabWebhookWithHeaders(t *testing.T) {
	const payload = `{"object_kind":"merge_request"}`
	var receivedBody string
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/events/gitlab" {
			t.Fatalf("expected webhook path, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		receivedBody = string(raw)
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"event_id":"evt-1","event_status":"task_created"}`))
	}))
	defer server.Close()

	result, err := Run(context.Background(), Options{
		BaseURL:     server.URL,
		RawPayload:  []byte(payload),
		Secret:      "secret-1",
		EventName:   "Merge Request Hook",
		EventUUID:   "event-1",
		WebhookUUID: "delivery-1",
		InstanceURL: "https://gitlab.example",
		HTTPClient:  server.Client(),
	})
	if err != nil {
		t.Fatalf("run smoke replay: %v", err)
	}

	if receivedBody != payload {
		t.Fatalf("expected raw payload preserved, got %s", receivedBody)
	}
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Fatalf("expected content type, got %s", receivedHeaders.Get("Content-Type"))
	}
	if receivedHeaders.Get("X-Gitlab-Token") != "secret-1" {
		t.Fatalf("expected token header")
	}
	if receivedHeaders.Get("X-Gitlab-Event") != "Merge Request Hook" {
		t.Fatalf("expected event header, got %s", receivedHeaders.Get("X-Gitlab-Event"))
	}
	if receivedHeaders.Get("X-Gitlab-Event-UUID") != "event-1" {
		t.Fatalf("expected event uuid header, got %s", receivedHeaders.Get("X-Gitlab-Event-UUID"))
	}
	if receivedHeaders.Get("X-Gitlab-Webhook-UUID") != "delivery-1" {
		t.Fatalf("expected webhook uuid header")
	}
	if receivedHeaders.Get("X-Gitlab-Instance") != "https://gitlab.example" {
		t.Fatalf("expected instance header")
	}
	if result.WebhookStatusCode != http.StatusAccepted {
		t.Fatalf("expected webhook status %d, got %d", http.StatusAccepted, result.WebhookStatusCode)
	}
	if result.EventUUID != "event-1" {
		t.Fatalf("expected event uuid in result, got %s", result.EventUUID)
	}
}

func TestRunGeneratesSmokeEventUUIDWhenMissing(t *testing.T) {
	var eventUUID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		eventUUID = r.Header.Get("X-Gitlab-Event-UUID")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	result, err := Run(context.Background(), Options{
		BaseURL:    server.URL,
		RawPayload: []byte(`{"object_kind":"merge_request"}`),
		EventName:  "Merge Request Hook",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("run smoke replay: %v", err)
	}

	if !strings.HasPrefix(eventUUID, "smoke-") {
		t.Fatalf("expected generated smoke event uuid, got %s", eventUUID)
	}
	if result.EventUUID != eventUUID {
		t.Fatalf("expected result event uuid %s, got %s", eventUUID, result.EventUUID)
	}
}

func TestRunDryRunDoesNotSendRequestAndRedactsSecret(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	var out bytes.Buffer
	result, err := Run(context.Background(), Options{
		BaseURL:    server.URL,
		RawPayload: []byte(`{"object_kind":"merge_request"}`),
		Secret:     "secret-1",
		EventName:  "Merge Request Hook",
		DryRun:     true,
		Output:     &out,
	})
	if err != nil {
		t.Fatalf("run dry-run replay: %v", err)
	}
	if called {
		t.Fatal("expected dry-run not to call server")
	}
	if !result.DryRun {
		t.Fatal("expected dry-run result")
	}
	text := out.String()
	if !strings.Contains(text, "/api/events/gitlab") {
		t.Fatalf("expected webhook URL in dry-run output, got %s", text)
	}
	if strings.Contains(text, "secret-1") {
		t.Fatalf("expected secret redacted, got %s", text)
	}
	if !strings.Contains(text, "<configured>") {
		t.Fatalf("expected redacted configured marker, got %s", text)
	}
}

func TestRunReturnsErrorForNonSuccessWebhookResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized gitlab webhook"}`))
	}))
	defer server.Close()

	_, err := Run(context.Background(), Options{
		BaseURL:    server.URL,
		RawPayload: []byte(`{"object_kind":"merge_request"}`),
		EventName:  "Merge Request Hook",
		HTTPClient: server.Client(),
	})
	if err == nil {
		t.Fatal("expected webhook error")
	}
	if !strings.Contains(err.Error(), "gitlab webhook replay failed") {
		t.Fatalf("expected webhook replay failure, got %v", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected status code in error, got %v", err)
	}
}
