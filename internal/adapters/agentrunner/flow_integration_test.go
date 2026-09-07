package agentrunner_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"net/http/httptest"

	"sdlc-coordinator/internal/adapters/agentrunner"
	"sdlc-coordinator/internal/agent"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/workers"
	"sdlc-coordinator/internal/workspace"
)

func TestReviewerFlowFromPreparedWorkspaceToCanonicalResult(t *testing.T) {
	origin, headSHA := createIntegrationOrigin(t)
	root := t.TempDir()
	task := &model.Task{
		TaskID: "task-flow", Type: "review_mr", Role: "ReviewerWorker", TargetRepo: "group/project", SourceRepo: "group/project",
		Objective: "review exact MR", AcceptanceCriteria: "return verdict",
		ContextJSON: `{"project":{"path":"group/project"},"merge_request":{"iid":21,"url":"https://gitlab.example/group/project/-/merge_requests/21","review_mode":"full","source_branch":"feature/review","target_branch":"master","head_sha":"` + headSHA + `"},"repos":[{"name":"project","url":"` + filepath.ToSlash(origin) + `","branch":"feature/review","access":"readable"}]}`,
	}
	run := &model.Run{RunID: "run-flow", TaskID: task.TaskID, WorkerRole: task.Role}
	task.CurrentRunID = &run.RunID
	manifest, err := workspace.NewReviewerPreparer().Prepare(context.Background(), workspace.ReviewerRequest{
		WorkspaceRoot: root, TaskID: task.TaskID, RunID: run.RunID, Project: task.TargetRepo,
		RepositoryURL: origin, SourceBranch: "feature/review", TargetBranch: "master", HeadSHA: headSHA, RequestedReviewMode: "full",
	})
	if err != nil || manifest.Status != workspace.ManifestStatusReady {
		t.Fatalf("prepare workspace: manifest=%#v err=%v", manifest, err)
	}
	runRoot := filepath.Join(root, "runs", run.RunID)
	bundle, err := workers.BuildReviewerRunnerTaskBundle(task, run)
	if err != nil {
		t.Fatalf("build task bundle: %v", err)
	}
	if err := agent.WriteTaskBundle(runRoot, bundle); err != nil {
		t.Fatalf("write task bundle: %v", err)
	}

	service, err := runner.NewService(t.TempDir(), map[string]runner.Provider{"fake": fileCheckingProvider{}})
	if err != nil {
		t.Fatalf("new runner service: %v", err)
	}
	server := httptest.NewServer(runner.NewHTTPHandler(service))
	defer server.Close()
	client := agentrunner.NewClient(server.URL, server.Client())
	_, err = client.Create(context.Background(), runner.CreateExecutionRequest{
		ExecutionID: run.RunID, TaskID: task.TaskID, Role: task.Role, WorkspacePath: runRoot,
		PromptFile: agent.PromptFileName, SchemaFile: agent.ResultSchemaFileName,
		Provider: runner.ProviderConfig{Name: "fake", Revision: 1},
	})
	if err != nil {
		t.Fatalf("create runner execution: %v", err)
	}
	waitForClientStatus(t, client, run.RunID, runner.StatusSucceeded)
	runnerResult, err := client.Result(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("get runner result: %v", err)
	}
	contract := results.NewReviewerContract()
	roleInput, err := contract.DecodeStrict(runnerResult.StructuredOutput)
	if err != nil {
		t.Fatalf("decode reviewer result: %v", err)
	}
	if err := contract.Validate(task, run, roleInput); err != nil {
		t.Fatalf("validate reviewer result: %v", err)
	}
	canonical, err := contract.Normalize(task, run, roleInput)
	if err != nil {
		t.Fatalf("normalize reviewer result: %v", err)
	}
	if canonical.Status != results.StatusSucceeded || len(canonical.Writebacks) != 1 || canonical.Writebacks[0].MergeRequestIID != 21 {
		t.Fatalf("unexpected canonical reviewer result: %#v", canonical)
	}
}

type fileCheckingProvider struct{}

func (fileCheckingProvider) Execute(_ context.Context, request runner.ProviderRequest, emit runner.EmitFunc) (json.RawMessage, error) {
	for _, name := range []string{workspace.ManifestFileName, request.PromptFile, request.SchemaFile, agent.TaskContextFileName} {
		if _, err := os.Stat(filepath.Join(request.WorkspacePath, name)); err != nil {
			return nil, err
		}
	}
	emit("provider_progress", map[string]any{"message": "files verified"})
	return json.RawMessage(`{"task_id":"task-flow","run_id":"run-flow","verdict":"approved","summary":"no blocking issue","findings":[],"test_summary":"not run: review only","comment_body":"AI Review 结论：建议合并。"}`), nil
}

func waitForClientStatus(t *testing.T, client *agentrunner.Client, executionID string, status runner.Status) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		execution, err := client.Get(context.Background(), executionID)
		if err != nil {
			t.Fatalf("get execution: %v", err)
		}
		if execution.Status == status {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("execution %s did not reach %s", executionID, status)
}

func createIntegrationOrigin(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	seed := filepath.Join(root, "seed")
	if err := os.MkdirAll(seed, 0o755); err != nil {
		t.Fatal(err)
	}
	integrationGit(t, seed, "init", "-b", "master")
	integrationGit(t, seed, "config", "user.name", "Integration Test")
	integrationGit(t, seed, "config", "user.email", "integration@example.com")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	integrationGit(t, seed, "add", "README.md")
	integrationGit(t, seed, "commit", "-m", "base")
	integrationGit(t, seed, "checkout", "-b", "feature/review")
	if err := os.WriteFile(filepath.Join(seed, "feature.go"), []byte("package feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	integrationGit(t, seed, "add", "feature.go")
	integrationGit(t, seed, "commit", "-m", "feature")
	head := strings.TrimSpace(integrationGit(t, seed, "rev-parse", "HEAD"))
	origin := filepath.Join(root, "origin.git")
	integrationGit(t, root, "init", "--bare", origin)
	integrationGit(t, seed, "remote", "add", "origin", origin)
	integrationGit(t, seed, "push", "origin", "master", "feature/review")
	return origin, head
}

func integrationGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
