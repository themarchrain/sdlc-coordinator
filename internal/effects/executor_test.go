package effects

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"sdlc-coordinator/internal/adapters/im"
	"sdlc-coordinator/internal/model"
)

func TestExecutorExecutesSendIMMessageEffect(t *testing.T) {
	backend := &fakeIMBackend{
		message: &im.Message{ID: "msg-1", Provider: "fake", RawJSON: `{"message_id":"msg-1"}`},
	}
	executor := NewExecutor(backend)

	responseJSON, err := executor.ExecuteEffect(context.Background(), &model.Effect{
		Kind:        "send_im_message",
		Target:      "team-review-group",
		RequestJSON: `{"body":"MR !12 审查完成","mentions":["author@example.com"]}`,
	})
	if err != nil {
		t.Fatalf("execute effect: %v", err)
	}

	if backend.lastRequest.Target != "team-review-group" {
		t.Fatalf("unexpected target: %s", backend.lastRequest.Target)
	}
	if backend.lastRequest.Body != "MR !12 审查完成" {
		t.Fatalf("unexpected body: %s", backend.lastRequest.Body)
	}
	if len(backend.lastRequest.Mentions) != 1 || backend.lastRequest.Mentions[0] != "author@example.com" {
		t.Fatalf("unexpected mentions: %#v", backend.lastRequest.Mentions)
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(responseJSON), &response); err != nil {
		t.Fatalf("decode response json: %v", err)
	}
	if response["message_id"] != "msg-1" {
		t.Fatalf("unexpected response message id: %#v", response["message_id"])
	}
	if response["provider"] != "fake" {
		t.Fatalf("unexpected response provider: %#v", response["provider"])
	}
}

func TestExecutorSkipsIMEffectWhenBackendNotConfigured(t *testing.T) {
	executor := NewExecutor(nil)

	responseJSON, err := executor.ExecuteEffect(context.Background(), &model.Effect{
		Kind:        "send_im_message",
		Target:      "team-review-group",
		RequestJSON: `{"body":"MR !12 审查完成"}`,
	})
	if err != nil {
		t.Fatalf("nil im backend must skip without error, got %v", err)
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(responseJSON), &response); err != nil {
		t.Fatalf("decode response json: %v", err)
	}
	if response["skipped"] != "im disabled" {
		t.Fatalf("unexpected skip response: %#v", response)
	}
}

func TestExecutorRejectsRawPayloadOverride(t *testing.T) {
	backend := &fakeIMBackend{message: &im.Message{ID: "msg-1", Provider: "fake"}}
	executor := NewExecutor(backend)

	_, err := executor.ExecuteEffect(context.Background(), &model.Effect{
		Kind:        "send_im_message",
		Target:      "team-review-group",
		RequestJSON: `{"payload":{"msgtype":"markdown","markdown":{"content":"MR !12 审查完成"}}}`,
	})
	if err == nil {
		t.Fatal("expected raw payload override to be rejected")
	}
	if !strings.Contains(err.Error(), "raw payload is not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecutorRejectsSendIMMessageWithoutBody(t *testing.T) {
	executor := NewExecutor(&fakeIMBackend{})

	_, err := executor.ExecuteEffect(context.Background(), &model.Effect{
		Kind:        "send_im_message",
		Target:      "team-review-group",
		RequestJSON: `{}`,
	})
	if err == nil {
		t.Fatal("expected missing body error")
	}
	if !strings.Contains(err.Error(), "body is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecutorRejectsUnsupportedEffectKind(t *testing.T) {
	executor := NewExecutor(&fakeIMBackend{})

	_, err := executor.ExecuteEffect(context.Background(), &model.Effect{
		Kind:        "unsupported_external_writeback",
		Target:      "external-work-item-1",
		RequestJSON: `{}`,
	})
	if err == nil {
		t.Fatal("expected unsupported kind error")
	}
	if !strings.Contains(err.Error(), `unsupported effect kind "unsupported_external_writeback"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

type fakeIMBackend struct {
	lastRequest im.SendMessageRequest
	message     *im.Message
	err         error
}

func (f *fakeIMBackend) SendMessage(ctx context.Context, req im.SendMessageRequest) (*im.Message, error) {
	f.lastRequest = req
	if f.err != nil {
		return nil, f.err
	}
	if f.message != nil {
		return f.message, nil
	}
	return &im.Message{ID: "msg-default", Provider: "fake"}, nil
}
