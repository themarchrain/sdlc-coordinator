package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	defaultLogDir        = "logs"
	defaultRetentionDays = 30
	logFilePrefix        = "coordinator-"
	logFileSuffix        = ".log"
)

func New(level, directory string) (*zap.Logger, error) {
	lvl, err := zapcore.ParseLevel(strings.ToLower(level))
	if err != nil {
		lvl = zapcore.InfoLevel
	}
	if strings.TrimSpace(directory) == "" {
		directory = defaultLogDir
	}

	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(lvl)

	consoleEncoderConfig := cfg.EncoderConfig
	consoleEncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	consoleEncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

	fileWriter := newDailyWriter(directory, defaultRetentionDays, time.Now)
	if err := fileWriter.prepare(time.Now()); err != nil {
		return nil, fmt.Errorf("initialize daily log writer: %w", err)
	}
	core := zapcore.NewTee(
		zapcore.NewCore(zapcore.NewConsoleEncoder(consoleEncoderConfig), zapcore.Lock(os.Stderr), lvl),
		zapcore.NewCore(zapcore.NewJSONEncoder(cfg.EncoderConfig), fileWriter, lvl),
	)
	return zap.New(
		core,
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
		zap.ErrorOutput(zapcore.Lock(os.Stderr)),
	), nil
}

type dailyWriter struct {
	mu            sync.Mutex
	directory     string
	retentionDays int
	now           func() time.Time
	cleanedDate   string
}

func newDailyWriter(directory string, retentionDays int, now func() time.Time) *dailyWriter {
	return &dailyWriter{
		directory:     directory,
		retentionDays: retentionDays,
		now:           now,
	}
}

func (w *dailyWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := w.now()
	date := now.Format("2006-01-02")
	if w.cleanedDate != date {
		if err := w.prepare(now); err != nil {
			return 0, err
		}
	}

	path := filepath.Join(w.directory, logFilePrefix+date+logFileSuffix)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open daily log: %w", err)
	}
	written, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return written, fmt.Errorf("write daily log: %w", writeErr)
	}
	if closeErr != nil {
		return written, fmt.Errorf("close daily log: %w", closeErr)
	}
	return written, nil
}

func (w *dailyWriter) Sync() error {
	return nil
}

func (w *dailyWriter) prepare(now time.Time) error {
	if err := os.MkdirAll(w.directory, 0o755); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}
	if err := w.removeExpired(now); err != nil {
		return err
	}
	w.cleanedDate = now.Format("2006-01-02")
	return nil
}

func (w *dailyWriter) removeExpired(now time.Time) error {
	entries, err := os.ReadDir(w.directory)
	if err != nil {
		return fmt.Errorf("read log directory: %w", err)
	}
	cutoff := startOfDay(now).AddDate(0, 0, -w.retentionDays)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		archiveDate, ok := parseArchiveDate(entry.Name(), now.Location())
		if !ok || !archiveDate.Before(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(w.directory, entry.Name())); err != nil {
			return fmt.Errorf("remove expired log %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func parseArchiveDate(name string, location *time.Location) (time.Time, bool) {
	if !strings.HasPrefix(name, logFilePrefix) || !strings.HasSuffix(name, logFileSuffix) {
		return time.Time{}, false
	}
	rawDate := strings.TrimSuffix(strings.TrimPrefix(name, logFilePrefix), logFileSuffix)
	parsed, err := time.ParseInLocation("2006-01-02", rawDate, location)
	return parsed, err == nil
}

func startOfDay(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}
