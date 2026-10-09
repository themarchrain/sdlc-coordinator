package runner

import (
	"errors"
	"path/filepath"
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
