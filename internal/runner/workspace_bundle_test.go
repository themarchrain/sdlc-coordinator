package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sdlc-coordinator/internal/agent"
	"sdlc-coordinator/internal/workspaceproto"
)

// TestWorkspaceOpsPublishTaskBundleCrossesFilesystemBoundary 任务包投递的全链
// 回归：协调器不写本地文件（独立文件系统），三段内容经协议落到 Runner 本机
// run 工作区，与本地形态共享渲染器的产物逐字节一致；重放幂等、缺文件与
// 非法文件名被拒、未准备的工作区被拒。
func TestWorkspaceOpsPublishTaskBundleCrossesFilesystemBoundary(t *testing.T) {
	origin, _ := createSeedOrigin(t)
	ops, dataRoot := newWorkspaceOpsForTest(t, 4)
	ctx := context.Background()
	runID := "run-bundle-fs"
	if _, err := ops.PrepareWorkspace(ctx, developerPrepareRequest(origin, runID, "prepare-"+runID)); err != nil {
		t.Fatalf("prepare workspace: %v", err)
	}

	bundle := agent.TaskBundle{
		TaskPackage: agent.TaskPackage{
			TaskID: "task-" + runID, RunID: runID, Type: "develop_from_issue", Role: "DeveloperWorker",
			Objective: "implement the feature", WorkspaceStrategy: agent.WorkspaceStrategyCoordinatorPrepared,
		},
		ResultSchema: map[string]any{"type": "object", "required": []any{"task_id"}},
		Prompt:       "do the work\n",
	}
	rendered, err := agent.RenderTaskBundle(bundle)
	if err != nil {
		t.Fatalf("render task bundle: %v", err)
	}
	files := map[string]string{}
	for name, data := range rendered {
		files[name] = string(data)
	}

	request := workspaceproto.TaskBundleRequest{
		OperationID: "bundle-" + runID, TenantID: 1, TaskID: "task-" + runID, RunID: runID, Files: files,
	}
	result, err := ops.PublishTaskBundle(ctx, request)
	if err != nil {
		t.Fatalf("publish task bundle: %v", err)
	}

	// Runner 侧路径必须是本机 dataRoot 下的工作区（与协调器路径无关）。
	wantRoot := filepath.Join(dataRoot, "workspaces", "runs", runID)
	if result.WorkspacePath != wantRoot {
		t.Fatalf("workspace path = %q, want %q", result.WorkspacePath, wantRoot)
	}
	for name, want := range rendered {
		got, readErr := os.ReadFile(filepath.Join(wantRoot, name))
		if readErr != nil {
			t.Fatalf("read delivered %s: %v", name, readErr)
		}
		if string(got) != string(want) {
			t.Fatalf("delivered %s differs from rendered content:\n got %q\nwant %q", name, got, want)
		}
	}
	// 部署令牌层之外的路径白名单：投递不会写出三段之外的文件。
	entries, err := os.ReadDir(wantRoot)
	if err != nil {
		t.Fatalf("read run root: %v", err)
	}
	allowed := map[string]bool{agent.TaskContextFileName: true, agent.ResultSchemaFileName: true, agent.PromptFileName: true, "repo": true, "workspace-manifest.json": true}
	for _, entry := range entries {
		if !allowed[entry.Name()] {
			t.Fatalf("unexpected entry in run workspace: %s", entry.Name())
		}
	}

	// 重放同操作 ID 同载荷：返回原结果，不重复落盘。
	replayed, err := ops.PublishTaskBundle(ctx, request)
	if err != nil {
		t.Fatalf("replay task bundle: %v", err)
	}
	if replayed.WorkspacePath != result.WorkspacePath {
		t.Fatalf("replay returned %q, want %q", replayed.WorkspacePath, result.WorkspacePath)
	}

	// 缺必需文件被拒（fail-closed，不静默落部分内容）。
	missing := workspaceproto.TaskBundleRequest{
		OperationID: "bundle-missing-" + runID, TenantID: 1, TaskID: "task-" + runID, RunID: runID,
		Files: map[string]string{agent.PromptFileName: "x\n"},
	}
	if _, err := ops.PublishTaskBundle(ctx, missing); err == nil {
		t.Fatal("task bundle missing required files must be rejected")
	}
	// 非法文件名（目录穿越尝试）被拒。
	traversal := workspaceproto.TaskBundleRequest{
		OperationID: "bundle-traversal-" + runID, TenantID: 1, TaskID: "task-" + runID, RunID: runID,
		Files: map[string]string{
			agent.TaskContextFileName: "{}", agent.ResultSchemaFileName: "{}", agent.PromptFileName: "x\n",
			"../escape.txt": "nope",
		},
	}
	if _, err := ops.PublishTaskBundle(ctx, traversal); err == nil {
		t.Fatal("task bundle with a path-escaping file name must be rejected")
	}
}

// TestWorkspaceOpsPublishTaskBundleRequiresPreparedWorkspace 未准备的工作区
// 拒绝投递（fail-closed，不隐式创建目录）：prepare 是工作区存在的唯一来源。
func TestWorkspaceOpsPublishTaskBundleRequiresPreparedWorkspace(t *testing.T) {
	ops, dataRoot := newWorkspaceOpsForTest(t, 4)
	ctx := context.Background()
	runID := "run-bundle-unprepared"
	_, err := ops.PublishTaskBundle(ctx, workspaceproto.TaskBundleRequest{
		OperationID: "bundle-" + runID, TenantID: 1, TaskID: "task-" + runID, RunID: runID,
		Files: map[string]string{
			agent.TaskContextFileName: "{}", agent.ResultSchemaFileName: "{}", agent.PromptFileName: "x\n",
		},
	})
	if err == nil {
		t.Fatal("publishing a task bundle into an unprepared workspace must be rejected")
	}
	if !strings.Contains(err.Error(), "not prepared") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dataRoot, "workspaces", "runs", runID)); !os.IsNotExist(statErr) {
		t.Fatalf("unprepared run workspace must not be created: %v", statErr)
	}
}
