package api

import (
	"fmt"
	"os"
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
	// logger 缺席时保留 nil:Write 回退 stderr 直写——框架诊断(gin panic
	// 现场、http.Server 连接错误)迁移前恒写 stderr,漏注入不得静默消失。
	return zapLogWriter{logger: logger, component: component}
}

func (w zapLogWriter) Write(p []byte) (int, error) {
	message := strings.TrimSpace(ansiEscapePattern.ReplaceAllString(string(p), ""))
	if message == "" {
		return len(p), nil
	}
	if w.logger == nil {
		_, _ = fmt.Fprintln(os.Stderr, message)
		return len(p), nil
	}
	w.logger.Error(message, zap.String("component", w.component))
	return len(p), nil
}
