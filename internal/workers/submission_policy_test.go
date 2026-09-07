package workers

import (
	"strings"
	"testing"

	"sdlc-coordinator/internal/agent"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
)

func submissionPolicyPackage(role string) agent.TaskPackage {
	runID := "run-policy"
	return agent.TaskPackage{
		TaskID: "task-policy", RunID: runID, Type: "develop_from_issue", Role: role,
		Objective: "do it", WorkspaceStrategy: agent.WorkspaceStrategyCoordinatorPrepared,
	}
}

func TestDeveloperPromptCarriesMCPCallPolicy(t *testing.T) {
	prompt, err := BuildDeveloperRunnerPrompt(submissionPolicyPackage("DeveloperWorker"))
	if err != nil {
		t.Fatalf("build prompt: %v", err)
	}
	for _, required := range []string{
		results.SubmitDeveloperResultTool,
		"task_id 与 run_id",   // 身份取值指令
		"accepted",           // 成功即停
		"rejected_final",     // 耗尽交接
		"result-schema.json", // 文本降级前提（契约描述保留）
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("developer prompt missing %q", required)
		}
	}
	if strings.Contains(prompt, "不要主动回调、调用 MCP") {
		t.Fatal("developer prompt still forbids MCP")
	}
}

func TestDeveloperReworkPromptCarriesMCPCallPolicy(t *testing.T) {
	pkg := submissionPolicyPackage("DeveloperWorker")
	pkg.Type = model.TaskTypeReworkMergeRequest
	prompt, err := BuildDeveloperRunnerPrompt(pkg)
	if err != nil {
		t.Fatalf("build prompt: %v", err)
	}
	if !strings.Contains(prompt, results.SubmitDeveloperResultTool) {
		t.Fatal("rework prompt missing submit tool policy")
	}
	if strings.Contains(prompt, "不要主动回调、调用 MCP") {
		t.Fatal("rework prompt still forbids MCP")
	}
}

func TestReviewerPromptCarriesMCPCallPolicy(t *testing.T) {
	prompt, err := BuildReviewerRunnerPrompt(submissionPolicyPackage("ReviewerWorker"))
	if err != nil {
		t.Fatalf("build prompt: %v", err)
	}
	for _, required := range []string{
		results.SubmitReviewerResultTool,
		"rejected_final",
		"result-schema.json",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("reviewer prompt missing %q", required)
		}
	}
	if strings.Contains(prompt, "不要尝试主动提交或回调结果") {
		t.Fatal("reviewer prompt still forbids callbacks")
	}
}

func TestPromptsNeverMentionRunToken(t *testing.T) {
	developer, err := BuildDeveloperRunnerPrompt(submissionPolicyPackage("DeveloperWorker"))
	if err != nil {
		t.Fatalf("build developer prompt: %v", err)
	}
	reviewer, err := BuildReviewerRunnerPrompt(submissionPolicyPackage("ReviewerWorker"))
	if err != nil {
		t.Fatalf("build reviewer prompt: %v", err)
	}
	for _, prompt := range []string{developer, reviewer} {
		if strings.Contains(prompt, "COORDINATOR_RUN_TOKEN") || strings.Contains(prompt, "Bearer") {
			t.Fatal("prompt must not reference the run token channel")
		}
	}
}
