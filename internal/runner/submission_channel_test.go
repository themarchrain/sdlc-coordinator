package runner_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"sdlc-coordinator/internal/runner"
)

type capturingProvider struct {
	request runner.ProviderRequest
	err     error
}

func (p *capturingProvider) Execute(_ context.Context, request runner.ProviderRequest, emit runner.EmitFunc) (json.RawMessage, error) {
	p.request = request
	if p.err != nil {
		return nil, p.err
	}
	emit("provider_progress", map[string]any{"message": "working"})
	return json.RawMessage(`{"verdict":"approved"}`), nil
}

func submissionChannel() *runner.ResultSubmissionChannel {
	return &runner.ResultSubmissionChannel{
		EndpointURL: "http://coordinator:8080/mcp",
		BearerToken: "run-token-secret-value",
	}
}

func TestRequestHashExcludesSubmissionChannel(t *testing.T) {
	request := sampleRequest()
	first, err := runner.RequestHash(request)
	if err != nil {
		t.Fatalf("hash request: %v", err)
	}
	withChannel := request
	withChannel.ResultSubmission = submissionChannel()
	second, err := runner.RequestHash(withChannel)
	if err != nil {
		t.Fatalf("hash request with channel: %v", err)
	}
	if first != second {
		t.Fatalf("submission channel must not change request identity: %s != %s", first, second)
	}
}

func TestServicePassesSubmissionChannelToProvider(t *testing.T) {
	provider := &capturingProvider{}
	service, err := runner.NewService(t.TempDir(), map[string]runner.Provider{"fake": provider})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	request := sampleRequest()
	request.ResultSubmission = submissionChannel()
	if _, err := service.Create(context.Background(), request); err != nil {
		t.Fatalf("create execution: %v", err)
	}
	execution := waitForStatus(t, service, request.ExecutionID, runner.StatusSucceeded)
	if execution.Status != runner.StatusSucceeded {
		t.Fatalf("unexpected status: %#v", execution)
	}
	if provider.request.ResultSubmission == nil || provider.request.ResultSubmission.BearerToken != "run-token-secret-value" {
		t.Fatalf("submission channel was not passed through: %#v", provider.request.ResultSubmission)
	}
	if provider.request.ResultSubmission.EndpointURL != "http://coordinator:8080/mcp" {
		t.Fatalf("unexpected endpoint: %q", provider.request.ResultSubmission.EndpointURL)
	}
}

func TestServiceRedactsRunTokenFromFailureMessage(t *testing.T) {
	provider := &capturingProvider{err: errors.New("gateway rejected Authorization: Bearer run-token-secret-value")}
	service, err := runner.NewService(t.TempDir(), map[string]runner.Provider{"fake": provider})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	request := sampleRequest()
	request.ResultSubmission = submissionChannel()
	if _, err := service.Create(context.Background(), request); err != nil {
		t.Fatalf("create execution: %v", err)
	}
	waitForStatus(t, service, request.ExecutionID, runner.StatusFailed)
	execution, _ := service.Get(request.ExecutionID)
	if strings.Contains(execution.ErrorMessage, "run-token-secret-value") {
		t.Fatalf("run token leaked into failure message: %s", execution.ErrorMessage)
	}
	if !strings.Contains(execution.ErrorMessage, "[REDACTED]") {
		t.Fatalf("failure message was not redacted: %s", execution.ErrorMessage)
	}
	events, err := service.Events(request.ExecutionID, 0)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	for _, event := range events {
		raw, _ := json.Marshal(event)
		if strings.Contains(string(raw), "run-token-secret-value") {
			t.Fatalf("run token leaked into event stream: %s", raw)
		}
	}
}

func TestServiceRejectsSubmissionChannelWithoutEndpoint(t *testing.T) {
	provider := &capturingProvider{}
	service, err := runner.NewService(t.TempDir(), map[string]runner.Provider{"fake": provider})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	request := sampleRequest()
	request.ResultSubmission = &runner.ResultSubmissionChannel{BearerToken: "token-only"}
	if _, err := service.Create(context.Background(), request); err == nil {
		t.Fatal("channel without endpoint url must be rejected")
	}
}
