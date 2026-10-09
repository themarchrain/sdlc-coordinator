package api

import (
	"regexp"
	"strings"

	"go.uber.org/zap"
)

var ansiEscapePattern = regexp.MustCompile("\\x1b\\[[0-9;]*m")

type zapLogWriter struct {
	logger    *zap.Logger
	component string
}

func newZapLogWriter(logger *zap.Logger, component string) zapLogWriter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return zapLogWriter{logger: logger, component: component}
}

func (w zapLogWriter) Write(p []byte) (int, error) {
	message := strings.TrimSpace(ansiEscapePattern.ReplaceAllString(string(p), ""))
	if message != "" {
		w.logger.Error(message, zap.String("component", w.component))
	}
	return len(p), nil
}
