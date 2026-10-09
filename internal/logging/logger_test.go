package logging

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestDailyWriterRotatesWhenDateChanges(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, time.July, 27, 23, 59, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	writer := newDailyWriter(dir, 30, func() time.Time { return now })

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

	assertFileContains(t, filepath.Join(dir, "coordinator-2026-07-27.log"), "day one")
	assertFileContains(t, filepath.Join(dir, "coordinator-2026-07-28.log"), "day two")
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
	writer := newDailyWriter(dir, 30, func() time.Time { return now })
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
