package workers

import (
	"encoding/json"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
)

func TestBuildRunnerTaskPackagePreservesBusinessContextWithoutCallback(t *testing.T) {
	task := &model.Task{
		TaskID: "task-runner-review", Type: "review_mr", Role: "ReviewerWorker",
		Objective: "review exact MR head", TargetRepo: "group/project", SourceRepo: "group/project",
		AcceptanceCriteria: "return verdict\nwrite review comment",
		ContextJSON:        `{"workspace_strategy":"agent_managed_clone","project":{"id":"1001"},"merge_request":{"iid":21},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"feature/review","access":"readable"}]}`,
	}
	run := &model.Run{RunID: "run-runner-review", WorkerRole: "ReviewerWorker"}
	pkg, err := BuildRunnerTaskPackage(task, run)
	if err != nil {
		t.Fatalf("build runner task package: %v", err)
	}
	if pkg.TaskID != task.TaskID || pkg.RunID != run.RunID || pkg.WorkspaceStrategy != "coordinator_prepared" {
		t.Fatalf("unexpected runner identity: %#v", pkg)
	}
	if len(pkg.Repos) != 1 || pkg.Repos[0].Access != "readable" || len(pkg.SuccessCriteria) != 2 || pkg.Context["merge_request"] == nil {
		t.Fatalf("runner package lost business context: %#v", pkg)
	}
	raw, err := json.Marshal(pkg)
	if err != nil {
		t.Fatalf("marshal runner task package: %v", err)
	}
	if strings.Contains(string(raw), "callback_url") {
		t.Fatalf("runner task package leaked legacy callback: %s", raw)
	}
}

func TestBuildReviewerRunnerPromptPreservesReviewSemanticsWithoutLegacyTransport(t *testing.T) {
	task := &model.Task{
		TaskID: "task-runner-prompt", Type: "review_mr", Role: "ReviewerWorker",
		Objective: "review MR", TargetRepo: "group/project", SourceRepo: "group/project",
		AcceptanceCriteria: "return a decisive verdict",
		ContextJSON:        `{"project":{"path":"group/project"},"merge_request":{"iid":21,"review_mode":"incremental","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","base_sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"feature/review","access":"readable"}]}`,
	}
	pkg, err := BuildRunnerTaskPackage(task, &model.Run{RunID: "run-runner-prompt", WorkerRole: "ReviewerWorker"})
	if err != nil {
		t.Fatalf("build runner task package: %v", err)
	}
	prompt, err := BuildReviewerRunnerPrompt(pkg)
	if err != nil {
		t.Fatalf("build reviewer runner prompt: %v", err)
	}
	for _, required := range []string{"任务包是唯一事件事实来源", "workspace-manifest.json", "git rev-list --reverse --topo-order <base_sha>..<head_sha> --not <target_sha>", "<target_sha>...<head_sha>", "大 MR", "严格使用以下六个编号章节", `"task_id": "task-runner-prompt"`} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("prompt missing %q: %s", required, prompt)
		}
	}
	for _, forbidden := range []string{"clone", "MCP", "callback", "submit_reviewer_result", "RESULT_SUBMITTED_VIA_MCP"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("runner prompt contains legacy transport wording %q: %s", forbidden, prompt)
		}
	}
}

func TestBuildReviewerRunnerTaskBundleUsesReviewerContractSchema(t *testing.T) {
	task := &model.Task{
		TaskID: "task-runner-bundle", Type: "review_mr", Role: "ReviewerWorker",
		Objective: "review MR", TargetRepo: "group/project", SourceRepo: "group/project",
		AcceptanceCriteria: "return verdict",
		ContextJSON:        `{"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"feature/review","access":"readable"}]}`,
	}
	bundle, err := BuildReviewerRunnerTaskBundle(task, &model.Run{RunID: "run-runner-bundle", WorkerRole: "ReviewerWorker"})
	if err != nil {
		t.Fatalf("build reviewer runner bundle: %v", err)
	}
	properties, ok := bundle.ResultSchema["properties"].(map[string]any)
	if bundle.TaskPackage.TaskID != task.TaskID || !ok || properties["verdict"] == nil || properties["comment_body"] == nil {
		t.Fatalf("unexpected runner bundle: %#v", bundle)
	}
}
