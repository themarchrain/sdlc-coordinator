package orchestrator

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/runner"
)

// 恢复路径的复用前提:执行投影丢失后的重建会带着派发时同一 task/run 再次
// 组装工作区请求,复用匹配按身份字段逐一比较。派发与重建两条组装路径必须
// 产出完全一致的请求,否则重建撞上工作区冲突,恢复只能靠换新 run 重跑。
func TestTesterWorkspaceRequestIsStableAcrossDispatchAndRebuild(t *testing.T) {
	task := testerVerificationTickTask("task-tester-rebuild", model.TaskStatusRunning)
	run := &model.Run{RunID: "run-tester-rebuild", TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	task.CurrentRunID = &run.RunID
	settings := ReviewerRunnerSettings{
		WorkspaceRoot:     "/app/workspaces",
		Provider:          runner.ProviderConfig{Name: "claude-cli", Revision: 4, Model: "model-a"},
		ResolveRepository: verifyCommandRepository("go test ./..."),
	}
	repoRow := &registeredrepo.RegisteredRepository{
		RepositoryID: fixtureRepositoryID, Provider: "gitlab",
		BaseURL: "https://gitlab.example", PathWithNamespace: "group/project", Token: "git-token",
	}

	dispatch, err := buildTesterWorkspaceRequest(task, run, repoRow, settings)
	if err != nil {
		t.Fatalf("build dispatch workspace request: %v", err)
	}
	rebuild, err := buildTesterWorkspaceRequestForRebuild(context.Background(), task, run, settings)
	if err != nil {
		t.Fatalf("build rebuild workspace request: %v", err)
	}

	if dispatch.TaskID != rebuild.TaskID || dispatch.RunID != rebuild.RunID ||
		dispatch.Project != rebuild.Project || dispatch.RepositoryURL != rebuild.RepositoryURL ||
		dispatch.SourceBranch != rebuild.SourceBranch || dispatch.TargetBranch != rebuild.TargetBranch ||
		dispatch.HeadSHA != rebuild.HeadSHA || dispatch.BaseSHA != rebuild.BaseSHA ||
		dispatch.RequestedReviewMode != rebuild.RequestedReviewMode ||
		dispatch.WorkerRole != rebuild.WorkerRole {
		t.Fatalf("rebuild must reproduce the dispatch workspace request identity: dispatch=%#v rebuild=%#v", dispatch, rebuild)
	}
	if rebuild.WorkerRole != "TesterWorker" || rebuild.HeadSHA != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("rebuild request lost the tester identity or frozen head: %#v", rebuild)
	}
}
