package workspace_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"sdlc-coordinator/internal/workspace"
)

// TestDeveloperPreparerPublishesSharedRuntimePermissions 锁定共享 bind mount 拓扑的
// 发布语义:run 目录树必须对 agent-runner 容器的非 root 用户可读写,否则跨容器
// 读取 prompt / 写入 repo 将以 permission denied 失败。
func TestDeveloperPreparerPublishesSharedRuntimePermissions(t *testing.T) {
	origin, _, _ := createReviewOrigin(t)
	root := t.TempDir()
	_, err := workspace.NewDeveloperPreparer().Prepare(context.Background(), workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-perm-1", RunID: "run-perm-1",
		TaskType: "develop_from_issue", Project: "group/project", RepositoryURL: origin,
		TargetBranch: "master", DeliveryBranch: "agent/task-perm-1",
	})
	if err != nil {
		t.Fatalf("prepare developer workspace: %v", err)
	}
	assertSharedRuntimeTree(t, filepath.Join(root, "runs", "run-perm-1"))
}

func TestReviewerPreparerPublishesSharedRuntimePermissions(t *testing.T) {
	origin, _, featureHead := createReviewOrigin(t)
	root := t.TempDir()
	_, err := workspace.NewReviewerPreparer().Prepare(context.Background(), workspace.ReviewerRequest{
		WorkspaceRoot: root, TaskID: "task-perm-r", RunID: "run-perm-r",
		Project: "group/project", RepositoryURL: origin,
		SourceBranch: "feature/review", TargetBranch: "master", HeadSHA: featureHead,
		RequestedReviewMode: "full",
	})
	if err != nil {
		t.Fatalf("prepare reviewer workspace: %v", err)
	}
	assertSharedRuntimeTree(t, filepath.Join(root, "runs", "run-perm-r"))
}

// assertSharedRuntimeTree 检查发布的 run 树:run 根与 repo 目录人人可读写、
// manifest 与 checkout 出的工作区文件人人可读写;Windows 上 mode 位不可移植,仅
// 断言结构存在。
func assertSharedRuntimeTree(t *testing.T, runRoot string) {
	t.Helper()
	manifestPath := filepath.Join(runRoot, workspace.ManifestFileName)
	worktreeFile := filepath.Join(runRoot, "repo", "README.md")
	for _, path := range []string{runRoot, filepath.Join(runRoot, "repo"), manifestPath, worktreeFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected published path %s: %v", path, err)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	assertPosixMode(t, runRoot, 0o777)
	assertPosixMode(t, filepath.Join(runRoot, "repo"), 0o777)
	assertPosixMode(t, manifestPath, 0o666)
	assertPosixMode(t, worktreeFile, 0o666)
}

func assertPosixMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s: expected mode %o, got %o", path, want, got)
	}
}
