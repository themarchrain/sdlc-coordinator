package runner_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"sdlc-coordinator/internal/runner"
)

func TestNewServiceLogsDurableEventRecoveryDiagnostics(t *testing.T) {
	dataRoot := t.TempDir()
	executionRoot := filepath.Join(dataRoot, "executions", "run-log-events")
	if err := os.MkdirAll(executionRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRunnerTestJSON(t, filepath.Join(executionRoot, "execution.json"), runner.Execution{
		ExecutionID: "run-log-events", TaskID: "task-log-events", Role: "ReviewerWorker",
		Provider: "fake", Status: runner.StatusRunning, RequestHash: strings.Repeat("a", 64), CreatedAt: time.Now().Add(-time.Minute),
	})
	raw := `{"execution_id":"run-log-events","seq":1,"type":"accepted","at":"2026-08-29T00:00:00Z"}
{"execution_id":"foreign-run","seq":2,"type":"accepted","at":"2026-08-29T00:00:01Z"}
{"execution_id":"run-log-events","seq":1,"type":"duplicate","at":"2026-08-29T00:00:02Z"}
{"truncated corrupt line"}
`
	if err := os.WriteFile(filepath.Join(executionRoot, "events.jsonl"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	logger, logs := newObservedLogger()
	service, err := runner.NewService(dataRoot, nil, runner.WithLogger(logger))
	if err != nil {
		t.Fatalf("new service with logger: %v", err)
	}

	foreign := requireSingleEntry(t, logs, zapcore.WarnLevel, "skip foreign durable event")
	requireStringField(t, foreign, "execution_id", "run-log-events")
	requireInt64Field(t, foreign, "seq", 2)

	duplicate := requireSingleEntry(t, logs, zapcore.WarnLevel, "skip duplicate or out-of-order durable event")
	requireStringField(t, duplicate, "execution_id", "run-log-events")
	requireInt64Field(t, duplicate, "seq", 1)

	undecodable := requireSingleEntry(t, logs, zapcore.WarnLevel, "stop reading durable events at undecodable line")
	requireStringField(t, undecodable, "execution_id", "run-log-events")
	requireInt64Field(t, undecodable, "last_seq", 1)
	requireNonEmptyField(t, undecodable, "error")

	if count := logs.FilterLevelExact(zapcore.ErrorLevel).Len(); count != 0 {
		t.Fatalf("recovery diagnostics must not log errors, got %#v", logs.FilterLevelExact(zapcore.ErrorLevel).All())
	}

	loaded, err := service.Get("run-log-events")
	if err != nil || loaded.Status != runner.StatusInterrupted {
		t.Fatalf("valid prefix recovery changed behavior: execution=%#v err=%v", loaded, err)
	}
	events, err := service.Events("run-log-events", 0)
	if err != nil || len(events) != 2 || events[0].Seq != 1 || events[1].Type != "interrupted" || events[1].Seq != 2 {
		t.Fatalf("valid prefix must be preserved and continued: events=%#v err=%v", events, err)
	}
}

func TestNewServiceLogsScanFailureAndKeepsExecutionUsable(t *testing.T) {
	dataRoot := t.TempDir()
	executionRoot := filepath.Join(dataRoot, "executions", "run-log-scan")
	if err := os.MkdirAll(executionRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRunnerTestJSON(t, filepath.Join(executionRoot, "execution.json"), runner.Execution{
		ExecutionID: "run-log-scan", TaskID: "task-log-scan", Role: "ReviewerWorker",
		Provider: "fake", Status: runner.StatusRunning, RequestHash: strings.Repeat("d", 64), CreatedAt: time.Now().Add(-time.Minute),
	})
	// 单行超过 16MiB 的扫描上限,触发 bufio.ErrTooLong;执行本身仍须可用。
	oversized := strings.Repeat("x", (16<<20)+1024)
	if err := os.WriteFile(filepath.Join(executionRoot, "events.jsonl"), []byte(oversized), 0o644); err != nil {
		t.Fatal(err)
	}

	logger, logs := newObservedLogger()
	service, err := runner.NewService(dataRoot, nil, runner.WithLogger(logger))
	if err != nil {
		t.Fatalf("scan failure must not block startup: %v", err)
	}

	entry := requireSingleEntry(t, logs, zapcore.WarnLevel, "scan durable events file failed")
	requireStringField(t, entry, "execution_id", "run-log-scan")
	requireNonEmptyField(t, entry, "error")

	loaded, err := service.Get("run-log-scan")
	if err != nil || loaded.Status != runner.StatusInterrupted {
		t.Fatalf("execution must still recover after scan failure: execution=%#v err=%v", loaded, err)
	}
	events, err := service.Events("run-log-scan", 0)
	if err != nil || len(events) != 1 || events[0].Type != "interrupted" || events[0].Seq != 1 {
		t.Fatalf("recovery must continue from an empty prefix: events=%#v err=%v", events, err)
	}
}

func TestNewServiceLogsQuarantinedEntries(t *testing.T) {
	dataRoot := t.TempDir()
	corruptRoot := filepath.Join(dataRoot, "executions", "run-log-corrupt")
	if err := os.MkdirAll(corruptRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corruptRoot, "execution.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataRoot, "executions", "stray.txt"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}

	logger, logs := newObservedLogger()
	if _, err := runner.NewService(dataRoot, nil, runner.WithLogger(logger)); err != nil {
		t.Fatalf("quarantine must not block startup: %v", err)
	}

	entries := logs.FilterLevelExact(zapcore.WarnLevel).FilterMessage("quarantined corrupted durable entry").All()
	if len(entries) != 2 {
		t.Fatalf("expected two quarantine warnings, got %#v", entries)
	}
	byEntry := make(map[string]observer.LoggedEntry, len(entries))
	for _, entry := range entries {
		name, _ := entry.ContextMap()["entry"].(string)
		byEntry[name] = entry
	}
	for _, name := range []string{"run-log-corrupt", "stray.txt"} {
		entry, ok := byEntry[name]
		if !ok {
			t.Fatalf("missing quarantine warning for %s: %#v", name, entries)
		}
		requireNonEmptyField(t, entry, "cause")
	}
	if count := logs.FilterLevelExact(zapcore.ErrorLevel).Len(); count != 0 {
		t.Fatalf("successful quarantine must not log errors, got %#v", logs.FilterLevelExact(zapcore.ErrorLevel).All())
	}
	quarantined, err := os.ReadDir(filepath.Join(dataRoot, "executions", "quarantine"))
	if err != nil || len(quarantined) != 2 {
		t.Fatalf("corrupted entries must still be quarantined: entries=%#v err=%v", quarantined, err)
	}
}

func TestNewServiceLogsQuarantineDirectoryFailure(t *testing.T) {
	dataRoot := t.TempDir()
	corruptRoot := filepath.Join(dataRoot, "executions", "run-log-stuck")
	if err := os.MkdirAll(corruptRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corruptRoot, "execution.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 用普通文件占据 quarantine 路径,让创建目录以跨平台稳定的方式失败。
	if err := os.WriteFile(filepath.Join(dataRoot, "executions", "quarantine"), []byte("occupied"), 0o644); err != nil {
		t.Fatal(err)
	}

	logger, logs := newObservedLogger()
	if _, err := runner.NewService(dataRoot, nil, runner.WithLogger(logger)); err != nil {
		t.Fatalf("quarantine failure must not block startup: %v", err)
	}

	entry := requireSingleEntry(t, logs, zapcore.ErrorLevel, "create quarantine directory failed")
	requireStringField(t, entry, "entry", "run-log-stuck")
	requireNonEmptyField(t, entry, "error")
	requireNonEmptyField(t, entry, "cause")
	if _, err := os.Stat(corruptRoot); err != nil {
		t.Fatalf("entry must stay in place when the quarantine directory cannot be created: %v", err)
	}
}

func TestNewServiceKeepsMissingEventsFileSilent(t *testing.T) {
	dataRoot := t.TempDir()
	executionRoot := filepath.Join(dataRoot, "executions", "run-log-no-events")
	if err := os.MkdirAll(executionRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	failedAt := time.Now().Add(-time.Minute)
	writeRunnerTestJSON(t, filepath.Join(executionRoot, "execution.json"), runner.Execution{
		ExecutionID: "run-log-no-events", TaskID: "task-log-no-events", Role: "ReviewerWorker",
		Provider: "fake", Status: runner.StatusFailed, RequestHash: strings.Repeat("c", 64),
		CreatedAt: failedAt, FinishedAt: &failedAt,
	})

	logger, logs := newObservedLogger()
	if _, err := runner.NewService(dataRoot, nil, runner.WithLogger(logger)); err != nil {
		t.Fatalf("new service: %v", err)
	}
	if logs.Len() != 0 {
		t.Fatalf("missing events file is a normal branch and must stay silent, got %#v", logs.All())
	}
}

func TestServiceLogsExecutionPersistenceFailuresWithoutSecrets(t *testing.T) {
	dataRoot := t.TempDir()
	provider := &gatedProvider{release: make(chan struct{})}
	logger, logs := newObservedLogger()
	service, err := runner.NewService(dataRoot, map[string]runner.Provider{"fake": provider}, runner.WithLogger(logger))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	request := sampleRequest()
	request.ExecutionID = "run-log-persist"
	request.TaskID = "task-log-persist"
	request.Provider.APIKey = "synth-secret-value-12345"
	if _, err := service.Create(context.Background(), request); err != nil {
		t.Fatalf("create execution: %v", err)
	}
	waitForStatus(t, service, request.ExecutionID, runner.StatusRunning)

	// 用普通文件占据执行目录:此后 provider 事件、快照与生命周期事件的写入
	// 全部失败并逐条产生诊断;执行必须仍按原路径收敛为终态。
	executionRoot := filepath.Join(dataRoot, "executions", request.ExecutionID)
	if err := os.RemoveAll(executionRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executionRoot, []byte("occupied"), 0o644); err != nil {
		t.Fatal(err)
	}
	close(provider.release)

	failed := waitForStatus(t, service, request.ExecutionID, runner.StatusFailed)
	if failed.ErrorCode != "result_persistence_failed" {
		t.Fatalf("unexpected terminal state: %#v", failed)
	}
	waitForLogCount(t, logs, "append execution event failed", 1)

	emitFailure := requireSingleEntry(t, logs, zapcore.ErrorLevel, "append provider event failed")
	requireStringField(t, emitFailure, "execution_id", request.ExecutionID)
	requireStringField(t, emitFailure, "event_type", "provider_progress")
	requireNonEmptyField(t, emitFailure, "error")

	persistFailure := requireSingleEntry(t, logs, zapcore.ErrorLevel, "persist execution snapshot failed")
	requireStringField(t, persistFailure, "execution_id", request.ExecutionID)
	requireNonEmptyField(t, persistFailure, "error")

	lifecycleFailure := requireSingleEntry(t, logs, zapcore.ErrorLevel, "append execution event failed")
	requireStringField(t, lifecycleFailure, "execution_id", request.ExecutionID)
	requireStringField(t, lifecycleFailure, "event_type", "failed")
	requireNonEmptyField(t, lifecycleFailure, "error")

	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, "synth-secret-value-12345") {
			t.Fatalf("secret leaked into log message %q", entry.Message)
		}
		for name, value := range entry.ContextMap() {
			if strings.Contains(fmt.Sprint(value), "synth-secret-value-12345") {
				t.Fatalf("secret leaked into log field %s of %q", name, entry.Message)
			}
		}
	}
}

type gatedProvider struct{ release chan struct{} }

func (p *gatedProvider) Execute(_ context.Context, _ runner.ProviderRequest, emit runner.EmitFunc) (json.RawMessage, error) {
	<-p.release
	emit("provider_progress", map[string]any{"message": "reviewing"})
	return json.RawMessage(`{"ok":true}`), nil
}

func newObservedLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)
	return zap.New(core), logs
}

func requireSingleEntry(t *testing.T, logs *observer.ObservedLogs, level zapcore.Level, message string) observer.LoggedEntry {
	t.Helper()
	entries := logs.FilterLevelExact(level).FilterMessage(message).All()
	if len(entries) != 1 {
		t.Fatalf("expected exactly one %s entry %q, got %#v", level, message, logs.All())
	}
	return entries[0]
}

func requireStringField(t *testing.T, entry observer.LoggedEntry, name, expected string) {
	t.Helper()
	if actual := entry.ContextMap()[name]; actual != expected {
		t.Fatalf("field %s: got %#v want %q (entry message %q)", name, actual, expected, entry.Message)
	}
}

func requireInt64Field(t *testing.T, entry observer.LoggedEntry, name string, expected int64) {
	t.Helper()
	if actual := entry.ContextMap()[name]; actual != expected {
		t.Fatalf("field %s: got %#v want %d (entry message %q)", name, actual, expected, entry.Message)
	}
}

func requireNonEmptyField(t *testing.T, entry observer.LoggedEntry, name string) {
	t.Helper()
	value, ok := entry.ContextMap()[name].(string)
	if !ok || strings.TrimSpace(value) == "" {
		t.Fatalf("field %s must be a non-empty string, got %#v (entry message %q)", name, entry.ContextMap()[name], entry.Message)
	}
}

func waitForLogCount(t *testing.T, logs *observer.ObservedLogs, message string, count int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(logs.FilterMessage(message).All()) >= count {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("did not observe %d entries for %q: %#v", count, message, logs.All())
}
