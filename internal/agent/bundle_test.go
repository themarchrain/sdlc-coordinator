package agent_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"sdlc-coordinator/internal/agent"
)

func TestWriteTaskBundlePublishesAuditableFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	bundle := agent.TaskBundle{
		TaskPackage: agent.TaskPackage{
			TaskID: "task-1", RunID: "run-1", Type: "review_mr", Role: "ReviewerWorker",
			Objective: "review exact head", WorkspaceStrategy: agent.WorkspaceStrategyCoordinatorPrepared,
		},
		ResultSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"verdict": map[string]any{"type": "string"}},
		},
		Prompt: "review the prepared workspace",
	}
	if err := agent.WriteTaskBundle(root, bundle); err != nil {
		t.Fatalf("write task bundle: %v", err)
	}

	taskRaw := readFile(t, filepath.Join(root, agent.TaskContextFileName))
	var task agent.TaskPackage
	if err := json.Unmarshal(taskRaw, &task); err != nil {
		t.Fatalf("decode task context: %v", err)
	}
	if task.TaskID != "task-1" || task.RunID != "run-1" || task.Role != "ReviewerWorker" {
		t.Fatalf("unexpected persisted task package: %#v", task)
	}

	schemaRaw := readFile(t, filepath.Join(root, agent.ResultSchemaFileName))
	var schema map[string]any
	if err := json.Unmarshal(schemaRaw, &schema); err != nil || schema["type"] != "object" {
		t.Fatalf("unexpected persisted schema: schema=%#v err=%v", schema, err)
	}
	if got := string(readFile(t, filepath.Join(root, agent.PromptFileName))); got != bundle.Prompt+"\n" {
		t.Fatalf("unexpected persisted prompt %q", got)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read bundle directory: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp") {
			t.Fatalf("temporary file was not removed: %s", entry.Name())
		}
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}

// TestWriteTaskBundlePublishesSharedReadableFiles 锁定任务文件的共享可读语义:
// bundle 由 coordinator(root)在已发布的 run 目录里写入,publish 后的权限整树放宽
// (0777/0666)不再覆盖这些文件,写入器必须自己落 0666,否则 runner 容器的非 root
// 用户读 prompt 依旧 permission denied。
func TestWriteTaskBundlePublishesSharedReadableFiles(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("mode bits are not portable on windows; behavior covered by posix runs")
	}
	root := t.TempDir()
	bundle := agent.TaskBundle{
		TaskPackage:  agent.TaskPackage{TaskID: "task-perm", RunID: "run-perm", Type: "develop_from_issue", Role: "DeveloperWorker"},
		ResultSchema: map[string]any{"type": "object"},
		Prompt:       "do work",
	}
	if err := agent.WriteTaskBundle(root, bundle); err != nil {
		t.Fatalf("write task bundle: %v", err)
	}
	for _, name := range []string{agent.TaskContextFileName, agent.ResultSchemaFileName, agent.PromptFileName} {
		info, err := os.Stat(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if got := info.Mode().Perm(); got != 0o666 {
			t.Fatalf("%s: expected mode 666, got %o", name, got)
		}
	}
}
