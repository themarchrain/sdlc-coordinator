package runner

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewServiceFallsBackToNopLogger(t *testing.T) {
	service, err := NewService(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("new service without logger: %v", err)
	}
	if service.logger == nil {
		t.Fatal("service must fall back to a non-nil logger")
	}
	if service.logger.Core().Enabled(zapcore.DebugLevel) || service.logger.Core().Enabled(zapcore.ErrorLevel) {
		t.Fatal("fallback logger must stay silent for all levels")
	}
}

func TestNewServiceWithNilLoggerOptionFallsBackToNop(t *testing.T) {
	service, err := NewService(t.TempDir(), nil, WithLogger(nil))
	if err != nil {
		t.Fatalf("new service with nil logger option: %v", err)
	}
	if service.logger == nil {
		t.Fatal("nil logger option must still fall back to a non-nil logger")
	}
}

// TestQuarantineDurableEntryLogsRenameFailure 覆盖重命名失败分支的诊断字段:
// 该分支要求 quarantine 目录创建成功而重命名失败,公开构造路径依赖平台相关的
// 文件锁行为,无法跨平台稳定触发,因此直接调用方法验证;构造期路径由
// logging_test.go 里经 NewService 的测试覆盖。
func TestQuarantineDurableEntryLogsRenameFailure(t *testing.T) {
	dataRoot := t.TempDir()
	core, logs := observer.New(zapcore.DebugLevel)
	service, err := NewService(dataRoot, nil, WithLogger(zap.New(core)))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	service.quarantineDurableEntry(filepath.Join(dataRoot, "executions"), "missing-entry", errors.New("durable entry is broken"))

	entries := logs.FilterLevelExact(zapcore.ErrorLevel).FilterMessage("quarantine entry failed").All()
	if len(entries) != 1 {
		t.Fatalf("expected one rename failure entry, got %#v", logs.All())
	}
	context := entries[0].ContextMap()
	if context["entry"] != "missing-entry" {
		t.Fatalf("unexpected entry field: %#v", context)
	}
	if cause, _ := context["cause"].(string); cause != "durable entry is broken" {
		t.Fatalf("unexpected cause field: %#v", context)
	}
	if message, _ := context["error"].(string); strings.TrimSpace(message) == "" {
		t.Fatalf("rename error must be recorded: %#v", context)
	}
}

func TestReadDurableEventsLogsOpenFailureFields(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows 把中间路径是文件的形态映射为 fs.ErrNotExist,而 os.Open 对目录
		// 会成功(Go 带 backup semantics),两种触发都会落进别的分支;该分支在
		// Windows 上无法经文件系统稳定触发,生产目标是 Linux,由 CI 覆盖。
		t.Skip("open-failure branch needs a non-ErrNotExist open error, unavailable on Windows")
	}
	dataRoot := t.TempDir()
	core, logs := observer.New(zapcore.DebugLevel)
	service, err := NewService(dataRoot, nil, WithLogger(zap.New(core)))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	// 普通文件占据中间路径:POSIX 下打开其子路径得到 ENOTDIR(非文件缺失),
	// 触发告警;打开失败仍按空前缀继续。
	blocker := filepath.Join(dataRoot, "blocker")
	if err := os.WriteFile(blocker, []byte("occupied"), 0o644); err != nil {
		t.Fatal(err)
	}
	eventsPath := filepath.Join(blocker, "events.jsonl")

	if events := service.readDurableEvents(eventsPath, "run-open"); events != nil {
		t.Fatalf("open failure must degrade to an empty prefix, got %#v", events)
	}
	entries := logs.FilterLevelExact(zapcore.WarnLevel).FilterMessage("open durable events file failed").All()
	if len(entries) != 1 {
		t.Fatalf("expected one open failure entry, got %#v", logs.All())
	}
	context := entries[0].ContextMap()
	if context["execution_id"] != "run-open" {
		t.Fatalf("unexpected execution_id field: %#v", context)
	}
	if context["path"] != eventsPath {
		t.Fatalf("unexpected path field: %#v", context)
	}
	if message, _ := context["error"].(string); strings.TrimSpace(message) == "" {
		t.Fatalf("open error must be recorded: %#v", context)
	}
}
