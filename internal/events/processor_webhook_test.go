package events

import (
	"context"
	"strings"
	"testing"
)

type fakeNormalizer struct {
	source string
	event  *NormalizedEvent
}

func (f fakeNormalizer) Source() string { return f.source }

func (f fakeNormalizer) Receipt(raw []byte, metadata WebhookMetadata) WebhookReceipt {
	if metadata.DeliveryID != "" {
		return WebhookReceipt{DedupeKey: f.source + ":webhook:event_uuid:" + metadata.DeliveryID, EventType: metadata.EventType}
	}
	return WebhookReceipt{DedupeKey: f.source + ":webhook:" + shortRawSHA256(raw), EventType: metadata.EventType}
}

func (f fakeNormalizer) Normalize(raw []byte) (*NormalizationResult, error) {
	if f.event != nil {
		return &NormalizationResult{Event: f.event}, nil
	}
	return ignoredNormalization("nothing to do")
}

func TestProcessorAcceptsArbitraryWebhookSource(t *testing.T) {
	processor, inboundRepo, taskRepo := newProcessorFixture(t)
	ctx := context.Background()

	normalizer := fakeNormalizer{source: "gitea", event: &NormalizedEvent{
		Source:        "gitea",
		Kind:          EventKindWorkItemUpdated,
		EventType:     "issues",
		EventAction:   "opened",
		ProjectID:     "42",
		ProjectPath:   "group/project",
		RepositoryURL: "https://gitea.example/group/project.git",
		ObjectIID:     7,
		ObjectURL:     "https://gitea.example/group/project/issues/7",
		Title:         "Fix widget",
		Version:       "2026-08-27T00:00:00Z",
		DedupeKey:     "gitea:issue-develop:42:7",
		Metadata:      map[string]any{},
	}}

	result, err := processor.ProcessWebhook(ctx, normalizer, []byte(`{"fake":"payload"}`), WebhookMetadata{
		EventType:  "issues",
		DeliveryID: "delivery-1",
	})
	if err != nil {
		t.Fatalf("process webhook: %v", err)
	}
	if result.Ignored || result.Task == nil {
		t.Fatalf("expected task creation, got ignored=%v task=%#v err-free", result.Ignored, result.Task)
	}
	if result.Event.Source != "gitea" {
		t.Fatalf("expected inbound source gitea, got %q", result.Event.Source)
	}
	if !strings.HasPrefix(result.Event.DedupeKey, "gitea:webhook:event_uuid:delivery-1") {
		t.Fatalf("expected delivery-scoped dedupe key, got %q", result.Event.DedupeKey)
	}

	stored, err := inboundRepo.GetByDedupeKey(ctx, result.Event.DedupeKey)
	if err != nil {
		t.Fatalf("get stored receipt: %v", err)
	}
	if stored.EventType != "issues" {
		t.Fatalf("expected event_type issues, got %q", stored.EventType)
	}
	if result.Task.Role != "DeveloperWorker" {
		t.Fatalf("unexpected task role: %s", result.Task.Role)
	}
	_ = taskRepo
}

func TestProcessorDuplicateWebhookSourceSharesReceipt(t *testing.T) {
	processor, _, _ := newProcessorFixture(t)
	ctx := context.Background()

	normalizer := fakeNormalizer{source: "gitea", event: &NormalizedEvent{
		Source: "gitea", Kind: EventKindWorkItemUpdated, ProjectID: "42", ProjectPath: "group/project",
		RepositoryURL: "https://gitea.example/group/project.git", ObjectIID: 7,
		DedupeKey: "gitea:issue-develop:42:7", Metadata: map[string]any{},
	}}
	metadata := WebhookMetadata{EventType: "issues", DeliveryID: "delivery-dup"}

	first, err := processor.ProcessWebhook(ctx, normalizer, []byte(`{"n":1}`), metadata)
	if err != nil {
		t.Fatalf("first webhook: %v", err)
	}
	second, err := processor.ProcessWebhook(ctx, normalizer, []byte(`{"n":2}`), metadata)
	if err != nil {
		t.Fatalf("second webhook: %v", err)
	}
	if second.Event == nil || first.Event == nil || first.Event.EventID != second.Event.EventID {
		t.Fatalf("expected shared receipt across deliveries, got %#v vs %#v", first.Event, second.Event)
	}
	if !second.Duplicate {
		t.Fatal("second delivery must be flagged duplicate")
	}
}
