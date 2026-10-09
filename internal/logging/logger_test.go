package logging

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

// 文件与终端分级筛选:文件恒 Debug 起(各级别应用日志都持久化),终端按
// 配置级别筛选。终端 Info 时 Debug/Info 都必须落文件。
func TestNewPersistsDebugAndAboveInDailyFile(t *testing.T) {
	dir := t.TempDir()
	logger, err := New("info", dir)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	logger.Debug("persisted debug message")
	logger.Info("persisted info message")
	logger.Warn("persisted warn message")
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
	for _, expected := range []string{"persisted debug message", "persisted info message", "persisted warn message"} {
		if !strings.Contains(contents, expected) {
			t.Fatalf("expected daily log to contain %q even with terminal level info, got %q", expected, contents)
		}
	}
	if !strings.Contains(contents, `"level":"debug"`) || !strings.Contains(contents, `"level":"info"`) {
		t.Fatalf("expected JSON-encoded levels in daily log, got %q", contents)
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

func TestNewRunnerRejectsDirectoryThatCannotOpenDailyFile(t *testing.T) {
	dir := t.TempDir()
	// 当日文件路径被目录占据:目录本身可写,但文件通道不可用,构造期必须失败,
	// 不能带着失效的文件通道继续服务。
	occupied := filepath.Join(dir, runnerLogFilePrefix+time.Now().Format("2006-01-02")+logFileSuffix)
	if err := os.MkdirAll(occupied, 0o755); err != nil {
		t.Fatalf("occupy daily log path: %v", err)
	}

	if _, err := NewRunner("info", dir); err == nil {
		t.Fatal("expected an unusable daily log path to fail runner logger initialization")
	}
}

func TestNewRunnerFallsBackToDefaultDirectory(t *testing.T) {
	workingDir := t.TempDir()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(workingDir); err != nil {
		t.Fatalf("enter working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })

	logger, err := NewRunner("info", "  ")
	if err != nil {
		t.Fatalf("new runner logger with blank directory: %v", err)
	}
	logger.Info("default directory message")
	_ = logger.Sync()

	defaultDir := filepath.Join(workingDir, filepath.FromSlash(DefaultRunnerLogDir))
	assertFileContains(t, singleFile(t, defaultDir, "agent-runner-*.log"), "default directory message")
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

	// runner 侧同样不得删除协调器的过期归档:重新放一份过期文件,再让 runner 清理。
	coordinatorExpired := filepath.Join(dir, "coordinator-2026-06-27.log")
	if err := os.WriteFile(coordinatorExpired, []byte("existing"), 0o600); err != nil {
		t.Fatalf("prepare %s: %v", coordinatorExpired, err)
	}
	runnerWriter := newDailyWriter(dir, runnerLogFilePrefix, 30, func() time.Time { return now })
	if _, err := runnerWriter.Write([]byte("runner today\n")); err != nil {
		t.Fatalf("write runner log: %v", err)
	}
	if _, err := os.Stat(runnerOld); !os.IsNotExist(err) {
		t.Fatalf("runner writer must delete its own expired archive, stat error=%v", err)
	}
	for _, path := range []string{runnerBoundary, coordinatorExpired, filepath.Join(dir, "coordinator-2026-07-28.log")} {
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

// 终端阈值 Warn 时:Debug/Info 只进文件不出终端,Warn/Error 两路都出。
// 终端输出用 os.Pipe 捕获进程 stderr——生产 core 直接锁 os.Stderr,
// 捕获后恢复,避免污染测试输出。
func TestNewSplitsTerminalAndFileLevels(t *testing.T) {
	dir := t.TempDir()

	// zapcore.Lock 在 New() 构造时快照 os.Stderr,必须先替换再构造,
	// 否则 core 锁住的是原始 stderr,捕获不到输出。
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStderr := os.Stderr
	os.Stderr = writer
	defer func() {
		os.Stderr = origStderr
	}()

	logger, err := New("warn", dir)
	if err != nil {
		os.Stderr = origStderr
		writer.Close()
		t.Fatalf("new logger: %v", err)
	}

	logger.Debug("file-only debug")
	logger.Info("file-only info")
	logger.Warn("both-paths warn")
	logger.Error("both-paths error")
	// 不调 Sync:两个 core 均无缓冲直写,而 Sync 会对替换后的 stderr 管道
	// 做 Fsync,Windows 上永久阻塞(Linux 字符设备则返回 EINVAL,均无意义)。
	writer.Close()
	os.Stderr = origStderr

	captured, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}
	reader.Close()
	terminal := string(captured)

	files, err := filepath.Glob(filepath.Join(dir, "coordinator-*.log"))
	if err != nil {
		t.Fatalf("glob log files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one daily log file, got %v", files)
	}
	fileData, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read daily log: %v", err)
	}
	fileContents := string(fileData)

	// 文件:Debug 及以上全部在。
	for _, expected := range []string{"file-only debug", "file-only info", "both-paths warn", "both-paths error"} {
		if !strings.Contains(fileContents, expected) {
			t.Fatalf("expected file to contain %q at terminal level warn, got %q", expected, fileContents)
		}
	}
	// 终端:Warn/Error 在,Debug/Info 不在。
	for _, expected := range []string{"both-paths warn", "both-paths error"} {
		if !strings.Contains(terminal, expected) {
			t.Fatalf("expected terminal output to contain %q, got %q", expected, terminal)
		}
	}
	for _, unexpected := range []string{"file-only debug", "file-only info"} {
		if strings.Contains(terminal, unexpected) {
			t.Fatalf("terminal output must not contain %q at terminal level warn, got %q", unexpected, terminal)
		}
	}
}

// 无效终端级别回落 Info:行为与原实现一致,且 Info 在该回落级别下仍落文件。
func TestNewFallsBackToInfoOnInvalidLevel(t *testing.T) {
	dir := t.TempDir()
	logger, err := New("not-a-level", dir)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	logger.Debug("debug at fallback")
	logger.Info("info at fallback")
	_ = logger.Sync()

	assertFileContains(t, filepath.Join(dir, "coordinator-"+time.Now().Format("2006-01-02")+".log"), "info at fallback")
	assertFileContains(t, filepath.Join(dir, "coordinator-"+time.Now().Format("2006-01-02")+".log"), "debug at fallback")
}
