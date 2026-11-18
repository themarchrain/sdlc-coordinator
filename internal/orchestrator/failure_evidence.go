package orchestrator

import (
	"encoding/json"
	"strings"

	"sdlc-coordinator/internal/failure"
	"sdlc-coordinator/internal/model"
)

func executionFailureInfo(execution *model.AgentExecution) *failure.Info {
	if execution == nil {
		return nil
	}
	if raw := strings.TrimSpace(execution.FailureJSON); raw != "" {
		var info failure.Info
		if err := json.Unmarshal([]byte(raw), &info); err == nil {
			return &info
		}
	}
	if strings.TrimSpace(execution.ErrorCode) == "" && strings.TrimSpace(execution.ErrorMessage) == "" {
		return nil
	}
	return failure.FromLegacy(execution.ErrorCode, execution.ErrorMessage)
}

func executionFailureMessage(execution *model.AgentExecution) string {
	if execution == nil {
		return "agent runner execution failed"
	}
	if message := strings.TrimSpace(execution.ErrorMessage); message != "" {
		return message
	}
	if code := strings.TrimSpace(execution.ErrorCode); code != "" {
		return code
	}
	return "agent runner execution failed"
}
