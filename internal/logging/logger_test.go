package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
