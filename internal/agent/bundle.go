package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

func WriteTaskBundle(root string, bundle TaskBundle) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return errors.New("task bundle root is required")
	}
	if strings.TrimSpace(bundle.TaskPackage.TaskID) == "" || strings.TrimSpace(bundle.TaskPackage.RunID) == "" {
		return errors.New("task bundle task_id and run_id are required")
	}
	if len(bundle.ResultSchema) == 0 {
		return errors.New("task bundle result schema is required")
	}
	if strings.TrimSpace(bundle.Prompt) == "" {
		return errors.New("task bundle prompt is required")
	}
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("inspect task bundle root: %w", err)
	}
	if !info.IsDir() {
		return errors.New("task bundle root must be a directory")
	}

	taskRaw, err := json.MarshalIndent(bundle.TaskPackage, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal task context: %w", err)
	}
	schemaRaw, err := json.MarshalIndent(bundle.ResultSchema, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result schema: %w", err)
	}
	files := []struct {
		name string
		data []byte
	}{
		{name: TaskContextFileName, data: append(taskRaw, '\n')},
		{name: ResultSchemaFileName, data: append(schemaRaw, '\n')},
		{name: PromptFileName, data: []byte(strings.TrimRight(bundle.Prompt, "\r\n") + "\n")},
	}
	for _, file := range files {
		if err := writeFileAtomic(filepath.Join(root, file.name), file.data); err != nil {
			return err
		}
	}
	return nil
}

func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary %s: %w", filepath.Base(path), err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temporary %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("publish %s: %w", filepath.Base(path), err)
	}
	return nil
}
