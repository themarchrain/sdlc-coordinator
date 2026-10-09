package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewWritesConfiguredLevelToDailyFile(t *testing.T) {
	dir := t.TempDir()
	logger, err := New("info", dir)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	logger.Debug("hidden debug message")
	logger.Info("persisted info message")
	// stderr 在 Linux 上是字符设备,fsync 返回 EINVAL;与生产一致,Sync 错误不作为
	// 失败条件(main.go 退出清理同样忽略该错误),文件写入本身即写即关、无需刷盘保证。
	_ = logger.Sync()

	files, err := filepath.Glob(filepath.Join(dir, "coordinator-*.log"))
	if err != nil {
		t.Fatalf("glob log files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one daily log file, got %v", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read daily log: %v", err)
	}
	contents := string(data)
	if !strings.Contains(contents, "persisted info message") {
		t.Fatalf("expected info message in daily log, got %q", contents)
	}
	if strings.Contains(contents, "hidden debug message") {
		t.Fatalf("debug message must be filtered at info level, got %q", contents)
	}
}

func TestNewRejectsUnavailableLogDirectory(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "not-a-directory")
	if err := os.WriteFile(path, []byte("occupied"), 0o600); err != nil {
		t.Fatalf("prepare occupied path: %v", err)
	}

	if _, err := New("info", path); err == nil {
		t.Fatal("expected unavailable log directory to fail logger initialization")
	}
}

func TestNewRunnerWritesOwnArchiveAndFiltersTerminalIndependently(t *testing.T) {
	dir := t.TempDir()
	logger, console := newRunnerWithCapturedStderr(t, "warn", dir)

	logger.Debug("debug file only")
	logger.Info("info file only")
	logger.Warn("warn both channels")
	_ = logger.Sync()

	runnerLog := singleFile(t, dir, "agent-runner-*.log")
	data, err := os.ReadFile(runnerLog)
	if err != nil {
		t.Fatalf("read runner daily log: %v", err)
	}
	for _, expected := range []string{"debug file only", "info file only", "warn both channels"} {
		if !strings.Contains(string(data), expected) {
			t.Fatalf("runner daily log must keep %q regardless of terminal level, got %q", expected, string(data))
		}
	}
	coordinatorFiles, err := filepath.Glob(filepath.Join(dir, "coordinator-*.log"))
	if err != nil || len(coordinatorFiles) != 0 {
		t.Fatalf("runner logger must not write coordinator archives: files=%v err=%v", coordinatorFiles, err)
	}

	terminal := console()
	if !strings.Contains(terminal, "warn both channels") {
		t.Fatalf("terminal must keep warn output, got %q", terminal)
	}
	for _, hidden := range []string{"debug file only", "info file only"} {
		if strings.Contains(terminal, hidden) {
			t.Fatalf("terminal must filter %q at warn level, got %q", hidden, terminal)
		}
	}
}

func TestNewRunnerFallsBackToInfoLevelForInvalidInput(t *testing.T) {
	dir := t.TempDir()
	logger, console := newRunnerWithCapturedStderr(t, "bogus", dir)

	logger.Debug("debug filtered output")
	logger.Info("info fallback output")
	_ = logger.Sync()

	terminal := console()
	if !strings.Contains(terminal, "info fallback output") {
		t.Fatalf("invalid level must fall back to info, got %q", terminal)
	}
	if strings.Contains(terminal, "debug filtered output") {
		t.Fatalf("debug must stay filtered at fallback info level, got %q", terminal)
	}
	assertFileContains(t, singleFile(t, dir, "agent-runner-*.log"), "debug filtered output")
}

func TestNewRunnerRejectsUnavailableLogDirectory(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "not-a-directory")
	if err := os.WriteFile(path, []byte("occupied"), 0o600); err != nil {
		t.Fatalf("prepare occupied path: %v", err)
	}

	if _, err := NewRunner("info", path); err == nil {
		t.Fatal("expected unavailable log directory to fail runner logger initialization")
	}
}

func TestDailyWriterRotatesWhenDateChanges(t *testing.T) {
	for _, prefix := range []string{logFilePrefix, runnerLogFilePrefix} {
		t.Run(prefix, func(t *testing.T) {
			dir := t.TempDir()
			now := time.Date(2026, time.July, 27, 23, 59, 0, 0, time.FixedZone("UTC+8", 8*60*60))
			writer := newDailyWriter(dir, prefix, 30, func() time.Time { return now })

			if _, err := writer.Write([]byte("day one\n")); err != nil {
				t.Fatalf("write first day: %v", err)
			}
			now = now.Add(2 * time.Minute)
			if _, err := writer.Write([]byte("day two\n")); err != nil {
				t.Fatalf("write second day: %v", err)
			}
			if err := writer.Sync(); err != nil {
				t.Fatalf("sync writer: %v", err)
			}

			assertFileContains(t, filepath.Join(dir, prefix+"2026-07-27.log"), "day one")
			assertFileContains(t, filepath.Join(dir, prefix+"2026-07-28.log"), "day two")
		})
	}
}

func TestDailyWriterDeletesArchivesOlderThanRetention(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "coordinator-2026-06-27.log")
	boundaryPath := filepath.Join(dir, "coordinator-2026-06-28.log")
	unrelatedPath := filepath.Join(dir, "keep-me.log")
	for _, path := range []string{oldPath, boundaryPath, unrelatedPath} {
		if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil {
			t.Fatalf("prepare %s: %v", path, err)
		}
	}

	now := time.Date(2026, time.July, 28, 8, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	writer := newDailyWriter(dir, logFilePrefix, 30, func() time.Time { return now })
	if _, err := writer.Write([]byte("today\n")); err != nil {
		t.Fatalf("write current log: %v", err)
	}
	if err := writer.Sync(); err != nil {
		t.Fatalf("sync writer: %v", err)
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old archive to be deleted, stat error=%v", err)
	}
	for _, path := range []string{boundaryPath, unrelatedPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to remain: %v", path, err)
		}
	}
}

func TestDailyWriterCleanupNeverCrossesArchivePrefixes(t *testing.T) {
	dir := t.TempDir()
	coordinatorOld := filepath.Join(dir, "coordinator-2026-06-27.log")
	runnerOld := filepath.Join(dir, "agent-runner-2026-06-27.log")
	runnerBoundary := filepath.Join(dir, "agent-runner-2026-06-28.log")
	for _, path := range []string{coordinatorOld, runnerOld, runnerBoundary} {
		if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil {
			t.Fatalf("prepare %s: %v", path, err)
		}
	}
	now := time.Date(2026, time.July, 28, 8, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))

	coordinatorWriter := newDailyWriter(dir, logFilePrefix, 30, func() time.Time { return now })
	if _, err := coordinatorWriter.Write([]byte("coordinator today\n")); err != nil {
		t.Fatalf("write coordinator log: %v", err)
	}
	if _, err := os.Stat(coordinatorOld); !os.IsNotExist(err) {
		t.Fatalf("coordinator writer must delete its own expired archive, stat error=%v", err)
	}
	for _, path := range []string{runnerOld, runnerBoundary} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("coordinator cleanup must keep runner archive %s: %v", path, err)
		}
	}

	runnerWriter := newDailyWriter(dir, runnerLogFilePrefix, 30, func() time.Time { return now })
	if _, err := runnerWriter.Write([]byte("runner today\n")); err != nil {
		t.Fatalf("write runner log: %v", err)
	}
	if _, err := os.Stat(runnerOld); !os.IsNotExist(err) {
		t.Fatalf("runner writer must delete its own expired archive, stat error=%v", err)
	}
	for _, path := range []string{runnerBoundary, filepath.Join(dir, "coordinator-2026-07-28.log")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("runner cleanup must keep %s: %v", path, err)
		}
	}
}

// newRunnerWithCapturedStderr 构造 runner logger 并把终端输出重定向到测试文件:
// 双路输出里 stderr 一侧在 core 构造时捕获 os.Stderr,因此必须在 NewRunner 前完成替换。
func newRunnerWithCapturedStderr(t *testing.T, level, directory string) (*zap.Logger, func() string) {
	t.Helper()
	consolePath := filepath.Join(t.TempDir(), "console.log")
	consoleFile, err := os.Create(consolePath)
	if err != nil {
		t.Fatalf("create console capture: %v", err)
	}
	original := os.Stderr
	os.Stderr = consoleFile
	t.Cleanup(func() {
		os.Stderr = original
		_ = consoleFile.Close()
	})
	logger, err := NewRunner(level, directory)
	if err != nil {
		t.Fatalf("new runner logger: %v", err)
	}
	return logger, func() string {
		data, err := os.ReadFile(consolePath)
		if err != nil {
			t.Fatalf("read console capture: %v", err)
		}
		return string(data)
	}
}

func singleFile(t *testing.T, dir, pattern string) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected exactly one %s file, got %v err=%v", pattern, files, err)
	}
	return files[0]
}

func assertFileContains(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), expected) {
		t.Fatalf("expected %s to contain %q, got %q", path, expected, string(data))
	}
}
