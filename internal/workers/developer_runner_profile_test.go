package workers

import (
	"strings"
	"testing"

	"sdlc-coordinator/internal/agent"
	"sdlc-coordinator/internal/model"
)

func TestBuildDeveloperRunnerPromptUsesCoordinatorPreparedGit(t *testing.T) {
	pkg := agent.TaskPackage{
		TaskID: "task-dev", RunID: "run-dev", Type: "develop_from_issue", Role: "DeveloperWorker",
		Objective: "implement requested fix", WorkspaceStrategy: agent.WorkspaceStrategyCoordinatorPrepared,
		Repos:    []agent.RepoSpec{{Name: "project", URL: "https://gitlab.example/group/project.git", Branch: "main", Access: "writable"}},
		Delivery: &agent.DeliverySpec{Repo: "project", Branch: "agent/task-dev", BaseBranch: "main"},
	}
	prompt, err := BuildDeveloperRunnerPrompt(pkg)
	if err != nil {
		t.Fatalf("build developer runner prompt: %v", err)
	}
	for _, required := range []string{"Coordinator 已准备", "禁止 clone、fetch、commit、push", "result-schema.json", "最终消息只能输出一段 JSON"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("prompt missing %q: %s", required, prompt)
		}
	}
	for _, forbidden := range []string{"submit_developer_result", "RESULT_SUBMITTED_VIA_MCP"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt retained legacy transport %q: %s", forbidden, prompt)
		}
	}
}

func TestBuildDeveloperRunnerPromptPreservesReworkContext(t *testing.T) {
	pkg := agent.TaskPackage{
		TaskID: "task-rework", RunID: "run-rework", Type: model.TaskTypeReworkMergeRequest, Role: "DeveloperWorker",
		Objective: "apply requested MR changes", WorkspaceStrategy: agent.WorkspaceStrategyCoordinatorPrepared,
		Delivery: &agent.DeliverySpec{Repo: "project", Branch: "feature/existing", BaseBranch: "main"},
		Context: map[string]any{
			"merge_request":  map[string]any{"source_branch": "feature/existing", "expected_head_sha": "abc123"},
			"rework_request": map[string]any{"context": "补充边界测试"},
		},
	}
	prompt, err := BuildDeveloperRunnerPrompt(pkg)
	if err != nil {
		t.Fatalf("build rework prompt: %v", err)
	}
	for _, required := range []string{"已有 MR 的接续开发", "feature/existing", "补充边界测试", "不得创建新 MR"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("rework prompt missing %q: %s", required, prompt)
		}
	}
}
