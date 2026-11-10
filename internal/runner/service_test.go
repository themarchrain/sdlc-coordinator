package runner_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"sdlc-coordinator/internal/failure"
	"sdlc-coordinator/internal/runner"
)

func TestServicePersistsExecutionEventsAndResultWithoutSecret(t *testing.T) {
	t.Parallel()

	dataRoot := t.TempDir()
	provider := &successfulProvider{result: json.RawMessage(`{"task_id":"task-1","run_id":"run-1","verdict":"approved"}`)}
	service, err := runner.NewService(dataRoot, map[string]runner.Provider{"fake": provider})
	if err != nil {
		t.Fatalf("new runner service: %v", err)
	}
	created, err := service.Create(context.Background(), runner.CreateExecutionRequest{
		ExecutionID:   "run-1",
		TaskID:        "task-1",
		Role:          "ReviewerWorker",
		WorkspacePath: "/app/workspaces/runs/run-1",
		PromptFile:    "prompt.md",
		SchemaFile:    "result-schema.json",
		Provider: runner.ProviderConfig{
			Name: "fake", Revision: 12, BaseURL: "https://gateway.example", APIKey: "secret-api-key",
			Headers: map[string]string{"X-Secret": "secret-header"}, Model: "model-a", Effort: "high",
		},
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}
	if created.Status != runner.StatusAccepted || created.ExecutionID != "run-1" || created.RequestHash == "" {
		t.Fatalf("unexpected accepted execution: %#v", created)
	}

	completed := waitForStatus(t, service, "run-1", runner.StatusSucceeded)
	if completed.ResultHash == "" || completed.FinishedAt == nil {
		t.Fatalf("successful execution missing terminal evidence: %#v", completed)
	}
	events, err := service.Events("run-1", 0)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) < 4 {
		t.Fatalf("expected lifecycle and provider events, got %#v", events)
	}
	for index, event := range events {
		if event.Seq != int64(index+1) {
			t.Fatalf("events are not contiguous: %#v", events)
		}
	}
	result, err := service.Result("run-1")
	if err != nil {
		t.Fatalf("get result: %v", err)
	}
	if string(result.StructuredOutput) != string(provider.result) || result.ResultHash != completed.ResultHash {
		t.Fatalf("unexpected structured result: %#v", result)
	}

	err = filepath.Walk(dataRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(raw), "secret-api-key") || strings.Contains(string(raw), "secret-header") {
			t.Fatalf("secret persisted in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan runner data: %v", err)
	}
}

func TestServiceCreateIsIdempotentAndRejectsDifferentRequest(t *testing.T) {
	t.Parallel()

	provider := &blockingProvider{started: make(chan struct{})}
	service, err := runner.NewService(t.TempDir(), map[string]runner.Provider{"fake": provider})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	request := sampleRequest()
	first, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}
	second, err := service.Create(context.Background(), request)
	if err != nil || second.RequestHash != first.RequestHash {
		t.Fatalf("idempotent create failed: execution=%#v err=%v", second, err)
	}
	changed := request
	changed.TaskID = "task-other"
	if _, err := service.Create(context.Background(), changed); !errors.Is(err, runner.ErrConflict) {
		t.Fatalf("expected request conflict, got %v", err)
	}
	_, _ = service.Cancel(request.ExecutionID)
	waitForStatus(t, service, request.ExecutionID, runner.StatusCancelled)
}

func TestRequestHashIgnoresSecretValuesButPinsExecutionInputs(t *testing.T) {
	request := sampleRequest()
	request.Provider.Headers = map[string]string{"Authorization": "secret-one", "X-Tenant": "tenant-one"}
	first, err := runner.RequestHash(request)
	if err != nil {
		t.Fatalf("hash request: %v", err)
	}
	secretChanged := request
	secretChanged.Provider.APIKey = "secret-two"
	secretChanged.Provider.Headers = map[string]string{"Authorization": "secret-two", "X-Tenant": "tenant-two"}
	second, err := runner.RequestHash(secretChanged)
	if err != nil {
		t.Fatalf("hash request with changed secrets: %v", err)
	}
	if first != second {
		t.Fatalf("secret values changed request identity: %s != %s", first, second)
	}

	modelChanged := request
	modelChanged.Provider.Model = "different-model"
	third, err := runner.RequestHash(modelChanged)
	if err != nil {
		t.Fatalf("hash changed request: %v", err)
	}
	if third == first {
		t.Fatal("non-secret execution input did not change request identity")
	}
}

func TestServiceCancelStopsRunningProvider(t *testing.T) {
	t.Parallel()

	provider := &blockingProvider{started: make(chan struct{})}
	service, err := runner.NewService(t.TempDir(), map[string]runner.Provider{"fake": provider})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	request := sampleRequest()
	if _, err := service.Create(context.Background(), request); err != nil {
		t.Fatalf("create execution: %v", err)
	}
	select {
	case <-provider.started:
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not start")
	}
	if _, err := service.Cancel(request.ExecutionID); err != nil {
		t.Fatalf("cancel execution: %v", err)
	}
	waitForStatus(t, service, request.ExecutionID, runner.StatusCancelled)
}

func TestServiceTimeoutStopsRunningProvider(t *testing.T) {
	t.Parallel()

	provider := &blockingProvider{started: make(chan struct{})}
	service, err := runner.NewService(t.TempDir(), map[string]runner.Provider{"fake": provider})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	request := sampleRequest()
	request.ExecutionID = "run-timeout"
	request.TaskID = "task-timeout"
	request.TimeoutSeconds = 1
	if _, err := service.Create(context.Background(), request); err != nil {
		t.Fatalf("create execution: %v", err)
	}
	select {
	case <-provider.started:
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not start")
	}
	completed := waitForStatus(t, service, request.ExecutionID, runner.StatusFailed)
	if completed.ErrorCode != "execution_timeout" || completed.FinishedAt == nil {
		t.Fatalf("unexpected timeout result: %#v", completed)
	}
	events, err := service.Events(request.ExecutionID, 0)
	if err != nil {
		t.Fatalf("list timeout events: %v", err)
	}
	last := events[len(events)-1]
	if last.Type != string(runner.StatusFailed) || last.Payload["error_code"] != "execution_timeout" {
		t.Fatalf("unexpected timeout event: %#v", last)
	}
}

func TestServicePersistsProviderFailureMessage(t *testing.T) {
	dataRoot := t.TempDir()
	provider := &failedProvider{err: errors.New("invalid request: schema rejected")}
	service, err := runner.NewService(dataRoot, map[string]runner.Provider{"fake": provider})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	request := sampleRequest()
	request.ExecutionID = "run-provider-failed"
	request.TaskID = "task-provider-failed"
	if _, err := service.Create(context.Background(), request); err != nil {
		t.Fatalf("create execution: %v", err)
	}
	completed := waitForStatus(t, service, request.ExecutionID, runner.StatusFailed)
	if completed.ErrorCode != "provider_failed" || completed.ErrorMessage != provider.err.Error() {
		t.Fatalf("provider failure detail was not preserved: %#v", completed)
	}
	if got := failure.Classify(completed.Failure); got != failure.ClassificationUnknown {
		t.Fatalf("ordinary provider failure must remain unknown, got %q", got)
	}
	raw, err := os.ReadFile(filepath.Join(dataRoot, "executions", request.ExecutionID, "execution.json"))
	if err != nil || !strings.Contains(string(raw), provider.err.Error()) {
		t.Fatalf("durable execution lost provider failure detail: raw=%s err=%v", raw, err)
	}
}

func TestServicePersistsStructuredFailure(t *testing.T) {
	const secret = "sk-live-rate-limit-secret"
	dataRoot := t.TempDir()
	provider := &failedProvider{err: &runner.ProviderFailure{
		Info: failure.Info{
			Version: failure.VersionV1,
			Stage:   failure.StageProvider,
			Source:  failure.SourceProvider,
			Code:    failure.CodeProviderRateLimited,
			Summary: "rate limited for " + secret,
		},
		Cause: errors.New("gateway returned 429 for " + secret),
	}}
	service, err := runner.NewService(dataRoot, map[string]runner.Provider{"fake": provider})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	request := sampleRequest()
	request.ExecutionID = "run-structured-failure"
	request.TaskID = "task-structured-failure"
	request.Provider.APIKey = secret
	created, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}
	completed := waitForStatus(t, service, request.ExecutionID, runner.StatusFailed)
	if completed.Failure == nil {
		t.Fatal("structured failure metadata was not persisted")
	}
	if completed.Failure.Code != failure.CodeProviderRateLimited || completed.Failure.Stage != failure.StageProvider || completed.Failure.Source != failure.SourceProvider {
		t.Fatalf("unexpected failure metadata: %#v", completed.Failure)
	}
	if got := failure.Classify(completed.Failure); got != failure.ClassificationTransient {
		t.Fatalf("Classify() = %q, want transient", got)
	}
	if strings.Contains(completed.Failure.Summary, secret) || strings.Contains(completed.ErrorMessage, secret) {
		t.Fatalf("structured failure leaked a secret: failure=%#v error=%q", completed.Failure, completed.ErrorMessage)
	}
	events, err := service.Events(request.ExecutionID, 0)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	for _, event := range events {
		raw, _ := json.Marshal(event)
		if strings.Contains(string(raw), secret) {
			t.Fatalf("structured failure leaked into event stream: %s", raw)
		}
	}
	if completed.ErrorCode != "provider_failed" {
		t.Fatalf("legacy error code changed: %q", completed.ErrorCode)
	}
	if created.RequestHash == "" {
		t.Fatal("request hash missing")
	}

	restarted, err := runner.NewService(dataRoot, nil)
	if err != nil {
		t.Fatalf("restart service: %v", err)
	}
	reloaded, err := restarted.Get(request.ExecutionID)
	if err != nil {
		t.Fatalf("reload execution: %v", err)
	}
	if reloaded.Failure == nil || reloaded.Failure.Code != failure.CodeProviderRateLimited {
		t.Fatalf("structured failure metadata was not durable: %#v", reloaded.Failure)
	}
}

func TestFailureMetadataDoesNotChangeRequestHash(t *testing.T) {
	provider := &failedProvider{err: &runner.ProviderFailure{
		Info: failure.Info{
			Version: failure.VersionV1,
			Stage:   failure.StageProvider,
			Source:  failure.SourceProvider,
			Code:    failure.CodeConnectionFailed,
		},
		Cause: errors.New("connection failed"),
	}}
	service, err := runner.NewService(t.TempDir(), map[string]runner.Provider{"fake": provider})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	request := sampleRequest()
	request.ExecutionID = "run-hash-failure"
	request.TaskID = "task-hash-failure"
	created, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}
	waitForStatus(t, service, request.ExecutionID, runner.StatusFailed)
	hashed, err := runner.RequestHash(request)
	if err != nil {
		t.Fatalf("hash request: %v", err)
	}
	if created.RequestHash != hashed {
		t.Fatalf("failure metadata changed request identity: %s != %s", created.RequestHash, hashed)
	}
}

func TestRequestHashPinsTimeout(t *testing.T) {
	request := sampleRequest()
	request.TimeoutSeconds = 60
	first, err := runner.RequestHash(request)
	if err != nil {
		t.Fatalf("hash request: %v", err)
	}
	request.TimeoutSeconds = 120
	second, err := runner.RequestHash(request)
	if err != nil {
		t.Fatalf("hash changed request: %v", err)
	}
	if first == second {
		t.Fatal("timeout did not change request identity")
	}
}

func TestServiceRestartReloadsSucceededExecutionAndResult(t *testing.T) {
	dataRoot := t.TempDir()
	provider := &successfulProvider{result: json.RawMessage(`{"task_id":"task-restart","run_id":"run-restart","verdict":"approved"}`)}
	service, err := runner.NewService(dataRoot, map[string]runner.Provider{"fake": provider})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	request := sampleRequest()
	request.ExecutionID = "run-restart"
	request.TaskID = "task-restart"
	created, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}
	waitForStatus(t, service, request.ExecutionID, runner.StatusSucceeded)

	restarted, err := runner.NewService(dataRoot, nil)
	if err != nil {
		t.Fatalf("restart service: %v", err)
	}
	loaded, err := restarted.Get(request.ExecutionID)
	if err != nil || loaded.Status != runner.StatusSucceeded || loaded.RequestHash != created.RequestHash {
		t.Fatalf("unexpected recovered execution: execution=%#v err=%v", loaded, err)
	}
	result, err := restarted.Result(request.ExecutionID)
	if err != nil || string(result.StructuredOutput) != string(provider.result) {
		t.Fatalf("unexpected recovered result: result=%#v err=%v", result, err)
	}
	events, err := restarted.Events(request.ExecutionID, 0)
	if err != nil || len(events) < 4 {
		t.Fatalf("unexpected recovered events: events=%#v err=%v", events, err)
	}
	replayed, err := restarted.Create(context.Background(), request)
	if err != nil || replayed.RequestHash != loaded.RequestHash {
		t.Fatalf("durable execution was not idempotent without provider secret replay: execution=%#v err=%v", replayed, err)
	}
	changed := request
	changed.Provider.Model = "different-model"
	if _, err := restarted.Create(context.Background(), changed); !errors.Is(err, runner.ErrConflict) {
		t.Fatalf("expected durable request conflict, got %v", err)
	}
}

func TestServiceRestartMarksAbandonedExecutionInterrupted(t *testing.T) {
	dataRoot := t.TempDir()
	executionRoot := filepath.Join(dataRoot, "executions", "run-restart-running")
	if err := os.MkdirAll(executionRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	execution := runner.Execution{
		ExecutionID: "run-restart-running", TaskID: "task-restart-running", Role: "ReviewerWorker",
		Provider: "fake", Status: runner.StatusRunning, RequestHash: strings.Repeat("a", 64), CreatedAt: time.Now().Add(-time.Minute),
	}
	writeRunnerTestJSON(t, filepath.Join(executionRoot, "execution.json"), execution)
	events := []runner.Event{
		{ExecutionID: execution.ExecutionID, Seq: 1, Type: "accepted", At: execution.CreatedAt},
		{ExecutionID: execution.ExecutionID, Seq: 2, Type: "running", At: execution.CreatedAt.Add(time.Second)},
	}
	writeRunnerTestEvents(t, filepath.Join(executionRoot, "events.jsonl"), events)

	restarted, err := runner.NewService(dataRoot, nil)
	if err != nil {
		t.Fatalf("restart service: %v", err)
	}
	loaded, err := restarted.Get(execution.ExecutionID)
	if err != nil || loaded.Status != runner.StatusInterrupted || loaded.ErrorCode != "runner_restarted" || loaded.FinishedAt == nil {
		t.Fatalf("unexpected interrupted recovery: execution=%#v err=%v", loaded, err)
	}
	recoveredEvents, err := restarted.Events(execution.ExecutionID, 0)
	if err != nil || len(recoveredEvents) != 3 || recoveredEvents[2].Seq != 3 || recoveredEvents[2].Type != "interrupted" {
		t.Fatalf("unexpected recovery events: events=%#v err=%v", recoveredEvents, err)
	}
}

func TestServiceRestartQuarantinesCorruptedExecutionWithoutBlockingOthers(t *testing.T) {
	t.Parallel()

	dataRoot := t.TempDir()
	goodRoot := filepath.Join(dataRoot, "executions", "run-restart-good")
	if err := os.MkdirAll(goodRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	failedAt := time.Now().Add(-30 * time.Second)
	writeRunnerTestJSON(t, filepath.Join(goodRoot, "execution.json"), runner.Execution{
		ExecutionID: "run-restart-good", TaskID: "task-restart-good", Role: "ReviewerWorker",
		Provider: "fake", Status: runner.StatusFailed, RequestHash: strings.Repeat("a", 64),
		CreatedAt: failedAt, FinishedAt: &failedAt,
	})
	corruptRoot := filepath.Join(dataRoot, "executions", "run-restart-corrupt")
	if err := os.MkdirAll(corruptRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corruptRoot, "execution.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataRoot, "executions", "stray.txt"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}

	restarted, err := runner.NewService(dataRoot, nil)
	if err != nil {
		t.Fatalf("corrupted durable entries must not block startup: %v", err)
	}
	if _, err := restarted.Get("run-restart-good"); err != nil {
		t.Fatalf("healthy execution must stay observable: %v", err)
	}
	if _, err := restarted.Get("run-restart-corrupt"); !errors.Is(err, runner.ErrNotFound) {
		t.Fatalf("corrupted execution must not be served: %v", err)
	}
	quarantined, err := os.ReadDir(filepath.Join(dataRoot, "executions", "quarantine"))
	if err != nil || len(quarantined) != 2 {
		t.Fatalf("corrupted directory and stray file must be quarantined: entries=%#v err=%v", quarantined, err)
	}
}

func TestServiceRestartToleratesEventSequenceGaps(t *testing.T) {
	t.Parallel()

	dataRoot := t.TempDir()
	executionRoot := filepath.Join(dataRoot, "executions", "run-restart-gaps")
	if err := os.MkdirAll(executionRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	createdAt := time.Now().Add(-time.Minute)
	writeRunnerTestJSON(t, filepath.Join(executionRoot, "execution.json"), runner.Execution{
		ExecutionID: "run-restart-gaps", TaskID: "task-restart-gaps", Role: "ReviewerWorker",
		Provider: "fake", Status: runner.StatusRunning, RequestHash: strings.Repeat("b", 64), CreatedAt: createdAt,
	})
	raw := `{"execution_id":"run-restart-gaps","seq":1,"type":"accepted","at":"2026-08-29T00:00:00Z"}
{"execution_id":"run-restart-gaps","seq":3,"type":"running","at":"2026-08-29T00:00:01Z"}
{"execution_id":"run-restart-gaps","seq":3,"type":"duplicate","at":"2026-08-29T00:00:02Z"}
{"truncated corrupt line"}
`
	if err := os.WriteFile(filepath.Join(executionRoot, "events.jsonl"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	restarted, err := runner.NewService(dataRoot, nil)
	if err != nil {
		t.Fatalf("gap-tolerant restart must succeed: %v", err)
	}
	loaded, err := restarted.Get("run-restart-gaps")
	if err != nil || loaded.Status != runner.StatusInterrupted {
		t.Fatalf("abandoned execution must converge: execution=%#v err=%v", loaded, err)
	}
	events, err := restarted.Events("run-restart-gaps", 0)
	if err != nil {
		t.Fatalf("list recovered events: %v", err)
	}
	if len(events) != 3 || events[0].Seq != 1 || events[1].Seq != 3 || events[2].Type != "interrupted" || events[2].Seq != 4 {
		t.Fatalf("events must keep gaps and continue from max seq: %#v", events)
	}
}

func TestServiceRestartPreservesStructuredOutputWithHTMLCharacters(t *testing.T) {
	dataRoot := t.TempDir()
	result := json.RawMessage(`{"summary":"依赖要求 go>=1.25.0 且 x<y & z","tests":[{"command":"go test ./...","passed":true,"summary":"a>b"}]}`)
	provider := &successfulProvider{result: result}
	service, err := runner.NewService(dataRoot, map[string]runner.Provider{"fake": provider})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	request := sampleRequest()
	request.ExecutionID = "run-restart-html"
	request.TaskID = "task-restart-html"
	if _, err := service.Create(context.Background(), request); err != nil {
		t.Fatalf("create execution: %v", err)
	}
	waitForStatus(t, service, request.ExecutionID, runner.StatusSucceeded)

	restarted, err := runner.NewService(dataRoot, nil)
	if err != nil {
		t.Fatalf("restart service with html chars: %v", err)
	}
	recovered, err := restarted.Result(request.ExecutionID)
	if err != nil {
		t.Fatalf("recover result with html chars: %v", err)
	}
	if string(recovered.StructuredOutput) != string(result) {
		t.Fatalf("structured output was not byte preserved: got %s want %s", recovered.StructuredOutput, result)
	}
}

func TestServiceRestartPromotesDurableResultBeforeInterruptedFallback(t *testing.T) {
	dataRoot := t.TempDir()
	provider := &successfulProvider{result: json.RawMessage(`{"task_id":"task-restart-result","run_id":"run-restart-result","verdict":"approved"}`)}
	service, err := runner.NewService(dataRoot, map[string]runner.Provider{"fake": provider})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	request := sampleRequest()
	request.ExecutionID = "run-restart-result"
	request.TaskID = "task-restart-result"
	if _, err := service.Create(context.Background(), request); err != nil {
		t.Fatalf("create execution: %v", err)
	}
	completed := waitForStatus(t, service, request.ExecutionID, runner.StatusSucceeded)
	completed.Status = runner.StatusRunning
	completed.ResultHash = ""
	completed.FinishedAt = nil
	writeRunnerTestJSON(t, filepath.Join(dataRoot, "executions", request.ExecutionID, "execution.json"), completed)

	restarted, err := runner.NewService(dataRoot, nil)
	if err != nil {
		t.Fatalf("restart service: %v", err)
	}
	recovered, err := restarted.Get(request.ExecutionID)
	if err != nil || recovered.Status != runner.StatusSucceeded || recovered.ResultHash == "" || recovered.FinishedAt == nil {
		t.Fatalf("durable result was not promoted: execution=%#v err=%v", recovered, err)
	}
	events, err := restarted.Events(request.ExecutionID, 0)
	if err != nil || events[len(events)-1].Type != "succeeded_recovered" {
		t.Fatalf("missing succeeded recovery event: events=%#v err=%v", events, err)
	}
}

func TestServiceRestartQuarantinesSucceededExecutionWithoutDurableResult(t *testing.T) {
	t.Parallel()

	dataRoot := t.TempDir()
	executionRoot := filepath.Join(dataRoot, "executions", "run-restart-corrupt")
	if err := os.MkdirAll(executionRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRunnerTestJSON(t, filepath.Join(executionRoot, "execution.json"), runner.Execution{
		ExecutionID: "run-restart-corrupt", TaskID: "task-restart-corrupt", Role: "ReviewerWorker",
		Provider: "fake", Status: runner.StatusSucceeded, RequestHash: strings.Repeat("b", 64), ResultHash: strings.Repeat("c", 64), CreatedAt: time.Now(),
	})

	restarted, err := runner.NewService(dataRoot, nil)
	if err != nil {
		t.Fatalf("corrupt durable success must be quarantined instead of blocking startup: %v", err)
	}
	if _, err := restarted.Get("run-restart-corrupt"); !errors.Is(err, runner.ErrNotFound) {
		t.Fatalf("corrupt execution must not be served: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "executions", "quarantine")); err != nil {
		t.Fatalf("expected quarantine directory: %v", err)
	}
}

type successfulProvider struct {
	result json.RawMessage
}

type blockingProvider struct {
	started chan struct{}
	once    sync.Once
}

type failedProvider struct{ err error }

func (p *failedProvider) Execute(context.Context, runner.ProviderRequest, runner.EmitFunc) (json.RawMessage, error) {
	return nil, p.err
}

func (p *blockingProvider) Execute(ctx context.Context, _ runner.ProviderRequest, _ runner.EmitFunc) (json.RawMessage, error) {
	p.once.Do(func() { close(p.started) })
	<-ctx.Done()
	return nil, ctx.Err()
}

func sampleRequest() runner.CreateExecutionRequest {
	return runner.CreateExecutionRequest{
		ExecutionID: "run-idempotent", TaskID: "task-idempotent", Role: "ReviewerWorker",
		WorkspacePath: "/app/workspaces/runs/run-idempotent", PromptFile: "prompt.md", SchemaFile: "result-schema.json",
		Provider: runner.ProviderConfig{Name: "fake", Revision: 1, APIKey: "secret"},
	}
}

func (p *successfulProvider) Execute(ctx context.Context, request runner.ProviderRequest, emit runner.EmitFunc) (json.RawMessage, error) {
	if request.Provider.APIKey == "" {
		return nil, context.Canceled
	}
	emit("provider_progress", map[string]any{"message": "reviewing"})
	return append(json.RawMessage(nil), p.result...), nil
}

func waitForStatus(t *testing.T, service *runner.Service, id string, status runner.Status) *runner.Execution {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		execution, err := service.Get(id)
		if err != nil {
			t.Fatalf("get execution: %v", err)
		}
		if execution.Status == status {
			return execution
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("execution %s did not reach %s", id, status)
	return nil
}

func writeRunnerTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeRunnerTestEvents(t *testing.T, path string, events []runner.Event) {
	t.Helper()
	var raw []byte
	for _, event := range events {
		line, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw, line...)
		raw = append(raw, '\n')
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestServiceCreateRollsBackWhenAcceptedEventCannotPersist(t *testing.T) {
	t.Parallel()

	dataRoot := t.TempDir()
	service, err := runner.NewService(dataRoot, map[string]runner.Provider{"fake": &successfulProvider{result: json.RawMessage(`{"task_id":"task-event-blocked","run_id":"run-event-blocked","verdict":"approved"}`)}})
	if err != nil {
		t.Fatalf("new runner service: %v", err)
	}
	// Block the events file with a directory so appending the accepted event fails.
	eventsPath := filepath.Join(dataRoot, "executions", "run-event-blocked", "events.jsonl")
	if err := os.MkdirAll(eventsPath, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(eventsPath)

	request := runner.CreateExecutionRequest{
		ExecutionID: "run-event-blocked", TaskID: "task-event-blocked", Role: "ReviewerWorker",
		WorkspacePath: "/app/workspaces/runs/run-event-blocked", PromptFile: "prompt.md", SchemaFile: "result-schema.json",
		Provider: runner.ProviderConfig{Name: "fake", Revision: 1, APIKey: "secret"},
	}
	if _, err := service.Create(context.Background(), request); err == nil {
		t.Fatal("create should fail when the accepted event cannot persist")
	}
	if _, err := service.Get("run-event-blocked"); err != runner.ErrNotFound {
		t.Fatalf("create left a partial execution behind: %v", err)
	}
	if _, err := service.Create(context.Background(), request); err != nil {
		t.Fatalf("create after rollback should succeed: %v", err)
	}
	waitForStatus(t, service, "run-event-blocked", runner.StatusSucceeded)
}

func TestServiceDeletesSucceededExecutionAndIsIdempotent(t *testing.T) {
	t.Parallel()
	dataRoot := t.TempDir()
	service, err := runner.NewService(dataRoot, map[string]runner.Provider{"fake": &successfulProvider{result: json.RawMessage(`{"ok":true}`)}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	request := sampleRequest()
	request.ExecutionID = "run-purge"
	request.TaskID = "task-purge"
	if _, err := service.Create(context.Background(), request); err != nil {
		t.Fatalf("create: %v", err)
	}
	waitForStatus(t, service, "run-purge", runner.StatusSucceeded)
	if err := service.Delete("run-purge"); err != nil {
		t.Fatalf("delete succeeded execution: %v", err)
	}
	if _, err := service.Get("run-purge"); !errors.Is(err, runner.ErrNotFound) {
		t.Fatalf("deleted execution still readable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "executions", "run-purge")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("execution directory still on disk: %v", err)
	}
	if err := service.Delete("run-purge"); err != nil {
		t.Fatalf("repeat delete must be idempotent: %v", err)
	}
}

func TestServiceRefusesToDeleteRunningExecution(t *testing.T) {
	t.Parallel()
	service, err := runner.NewService(t.TempDir(), map[string]runner.Provider{"fake": &blockingProvider{started: make(chan struct{})}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	request := sampleRequest()
	request.ExecutionID = "run-live"
	request.TaskID = "task-live"
	if _, err := service.Create(context.Background(), request); err != nil {
		t.Fatalf("create: %v", err)
	}
	waitForStatus(t, service, "run-live", runner.StatusRunning)
	if err := service.Delete("run-live"); !errors.Is(err, runner.ErrNotPurgeable) {
		t.Fatalf("running execution must not be deleted: %v", err)
	}
	if _, err := service.Get("run-live"); err != nil {
		t.Fatalf("running execution disappeared: %v", err)
	}
	if _, err := service.Cancel("run-live"); err != nil {
		t.Fatalf("cancel leftover execution: %v", err)
	}
	// 等执行 goroutine 收敛到终态后再结束，避免与 TempDir 清理竞争写 events.jsonl。
	waitForStatus(t, service, "run-live", runner.StatusCancelled)
}

type ignoreCancelProvider struct{ released chan struct{} }

func (p *ignoreCancelProvider) Execute(ctx context.Context, _ runner.ProviderRequest, _ runner.EmitFunc) (json.RawMessage, error) {
	select {
	case <-p.released:
	case <-ctx.Done():
		// 故意不随取消返回：模拟不响应取消的执行，Shutdown 必须仍能按期限返回。
		<-p.released
	}
	return nil, nil
}

func TestServiceShutdownCancelsActiveExecutionsAndWaits(t *testing.T) {
	t.Parallel()

	provider := &blockingProvider{started: make(chan struct{})}
	service, err := runner.NewService(t.TempDir(), map[string]runner.Provider{"fake": provider})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	first := sampleRequest()
	second := sampleRequest()
	second.ExecutionID = "run-shutdown-second"
	second.TaskID = "task-shutdown-second"
	for _, request := range []runner.CreateExecutionRequest{first, second} {
		if _, err := service.Create(context.Background(), request); err != nil {
			t.Fatalf("create execution: %v", err)
		}
	}
	select {
	case <-provider.started:
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not start")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	service.Shutdown(shutdownCtx)
	waitForStatus(t, service, first.ExecutionID, runner.StatusCancelled)
	waitForStatus(t, service, second.ExecutionID, runner.StatusCancelled)
}

func TestServiceShutdownReturnsWhenContextExpires(t *testing.T) {
	t.Parallel()

	released := make(chan struct{})
	provider := &ignoreCancelProvider{released: released}
	service, err := runner.NewService(t.TempDir(), map[string]runner.Provider{"fake": provider})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	request := sampleRequest()
	request.ExecutionID = "run-shutdown-stuck"
	request.TaskID = "task-shutdown-stuck"
	if _, err := service.Create(context.Background(), request); err != nil {
		t.Fatalf("create execution: %v", err)
	}

	startedAt := time.Now()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	service.Shutdown(shutdownCtx)
	if elapsed := time.Since(startedAt); elapsed > 5*time.Second {
		t.Fatalf("shutdown did not respect the context deadline: %v", elapsed)
	}
	execution, err := service.Get(request.ExecutionID)
	if err != nil || execution.Status != runner.StatusRunning {
		t.Fatalf("stuck execution must remain until its provider returns: %#v err=%v", execution, err)
	}
	// 释放 provider 并等执行收敛到终态，避免测试结束时仍有 goroutine 写
	// dataRoot 与 TempDir 清理竞争。
	close(released)
	waitForStatus(t, service, request.ExecutionID, runner.StatusFailed)
}
