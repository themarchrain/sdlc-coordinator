package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	TaskContextFileName  = "task-context.json"
	ResultSchemaFileName = "result-schema.json"
	PromptFileName       = "prompt.md"
)

type TaskBundle struct {
	TaskPackage  TaskPackage
	ResultSchema map[string]any
	Prompt       string
}

func RenderTaskBundle(bundle TaskBundle) (map[string][]byte, error) {
	if strings.TrimSpace(bundle.TaskPackage.TaskID) == "" || strings.TrimSpace(bundle.TaskPackage.RunID) == "" {
		return nil, errors.New("task bundle task_id and run_id are required")
	}
	if len(bundle.ResultSchema) == 0 {
		return nil, errors.New("task bundle result schema is required")
	}
	if strings.TrimSpace(bundle.Prompt) == "" {
		return nil, errors.New("task bundle prompt is required")
	}
	taskRaw, err := json.MarshalIndent(bundle.TaskPackage, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal task context: %w", err)
	}
	schemaRaw, err := json.MarshalIndent(bundle.ResultSchema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal result schema: %w", err)
	}
	return map[string][]byte{
		TaskContextFileName:  append(taskRaw, '\n'),
		ResultSchemaFileName: append(schemaRaw, '\n'),
		PromptFileName:       []byte(strings.TrimRight(bundle.Prompt, "\r\n") + "\n"),
	}, nil
}
