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

// RenderTaskBundle 把任务包渲染为与 WriteTaskBundle 落盘产物逐字节一致的
// 三段内容（文件名 → 内容）。远端工作区形态经受控通道投递同一份字节，
// 两条路径共享本渲染器保证一致。
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

func WriteTaskBundle(root string, bundle TaskBundle) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return errors.New("task bundle root is required")
	}
	files, err := RenderTaskBundle(bundle)
	if err != nil {
		return err
	}
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("inspect task bundle root: %w", err)
	}
	if !info.IsDir() {
		return errors.New("task bundle root must be a directory")
	}

	for name, data := range files {
		if err := writeFileAtomic(filepath.Join(root, name), data); err != nil {
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
	// os.CreateTemp 固定 0600;bundle 文件写入发生在 run 目录发布与权限放宽之后,
	// 必须自己落 0666,否则跨容器 bind mount 下 runner 的非 root 用户不可读。
	if err := os.Chmod(tmpPath, 0o666); err != nil {
		return fmt.Errorf("share temporary %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("publish %s: %w", filepath.Base(path), err)
	}
	return nil
}
