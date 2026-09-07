package agentrunner_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"sdlc-coordinator/internal/adapters/agentrunner"
	"sdlc-coordinator/internal/runner"
)

func TestClientCreatesPollsEventsAndReadsResult(t *testing.T) {
	t.Parallel()

	service, err := runner.NewService(t.TempDir(), map[string]runner.Provider{"fake": immediateProvider{}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	server := httptest.NewServer(runner.NewHTTPHandler(service))
	defer server.Close()
	client := agentrunner.NewClient(server.URL, server.Client())
	request := runner.CreateExecutionRequest{
		ExecutionID: "run-client", TaskID: "task-client", Role: "ReviewerWorker",
		WorkspacePath: "/workspaces/run-client", PromptFile: "prompt.md", SchemaFile: "result-schema.json",
		Provider: runner.ProviderConfig{Name: "fake", Revision: 1, APIKey: "secret"},
	}
	created, err := client.Create(context.Background(), request)
	if err != nil || created.ExecutionID != request.ExecutionID {
		t.Fatalf("create execution: execution=%#v err=%v", created, err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		execution, getErr := client.Get(context.Background(), request.ExecutionID)
		if getErr != nil {
			t.Fatalf("get execution: %v", getErr)
		}
		if execution.Status == runner.StatusSucceeded {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("execution did not succeed: %#v", execution)
		}
		time.Sleep(10 * time.Millisecond)
	}
	events, err := client.Events(context.Background(), request.ExecutionID, 0)
	if err != nil || len(events) == 0 {
		t.Fatalf("get events: events=%#v err=%v", events, err)
	}
	result, err := client.Result(context.Background(), request.ExecutionID)
	if err != nil || !json.Valid(result.StructuredOutput) || result.ResultHash == "" {
		t.Fatalf("get result: result=%#v err=%v", result, err)
	}
}

type immediateProvider struct{}

func (immediateProvider) Execute(context.Context, runner.ProviderRequest, runner.EmitFunc) (json.RawMessage, error) {
	return json.RawMessage(`{"task_id":"task-client","run_id":"run-client","verdict":"approved","summary":"ok","findings":[],"test_summary":"not run","comment_body":"AI Review 结论：建议合并。"}`), nil
}
