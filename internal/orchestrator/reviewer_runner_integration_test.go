package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"net/http/httptest"

	"sdlc-coordinator/internal/adapters/agentrunner"
	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/agent"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/runner/claudecli"
	"sdlc-coordinator/internal/runner/codexcli"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/workspace"
)

func TestReviewerRunnerPersistedFlow(t *testing.T) {
	testReviewerRunnerPersistedFlow(t, "fake", persistedReviewerProvider{}, 5*time.Second)
}

func TestReviewerRunnerPersistedFlowThroughCLIProcess(t *testing.T) {
	testReviewerRunnerPersistedFlow(t, "claude-cli", claudecli.New(createFakeClaudeExecutable(t)), 5*time.Second)
}

func TestReviewerRunnerPersistedFlowThroughCodexCLIProcess(t *testing.T) {
	testReviewerRunnerPersistedFlow(t, "codex-cli", codexcli.New(createFakeCodexExecutable(t)), 5*time.Second)
}

func TestReviewerRunnerPersistedFlowThroughRealClaudeCLI(t *testing.T) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("RUN_REAL_CLAUDE_SMOKE")), "true") {
		t.Skip("set RUN_REAL_CLAUDE_SMOKE=true to execute a real Claude CLI review")
	}
	testReviewerRunnerPersistedFlow(t, "claude-cli", claudecli.New("claude"), 2*time.Minute)
}

func testReviewerRunnerPersistedFlow(t *testing.T, providerName string, provider runner.Provider, timeout time.Duration) {
	t.Helper()
	ctx := context.Background()
	origin, headSHA := createReviewerRunnerOrigin(t)
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(
		db,
		&model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.AgentExecutionEvent{}, &model.Artifact{},
		&model.Writeback{}, &model.Effect{}, &model.ResultSubmission{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	tasks := repository.NewTaskRepository(db)
	runs := repository.NewRunRepository(db)
	executions := repository.NewAgentExecutionRepository(db)
	artifacts := repository.NewArtifactRepository(db)
	writebacks := repository.NewWritebackRepository(db)
	effects := repository.NewEffectRepository(db)
	submissions := repository.NewResultSubmissionRepository(db)
	runID := "run-persisted-reviewer"
	repositoryID := fixtureRepositoryID
	task := &model.Task{
		TaskID: "task-persisted-reviewer", DedupeKey: "persisted-reviewer", Type: "review_mr", Role: "ReviewerWorker",
		SourceRepo: "group/project", TargetRepo: "group/project", RepositoryID: &repositoryID, Objective: "review exact MR", AcceptanceCriteria: "return verdict",
		Status: model.TaskStatusRunning, CurrentRunID: &runID,
		ContextJSON: `{"project":{"path":"group/project"},"merge_request":{"iid":21,"url":"https://gitlab.example/group/project/-/merge_requests/21","title":"feat: reviewer runner","review_mode":"full","source_branch":"feature/review","target_branch":"master","head_sha":"` + headSHA + `","author":{"name":"Reviewer Owner","email":"owner@example.com"}},"repos":[{"name":"project","url":"` + filepath.ToSlash(origin) + `","branch":"feature/review","access":"readable"}]}`,
	}
	runRecord := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runs.Create(ctx, runRecord); err != nil {
		t.Fatalf("create run: %v", err)
	}

	backend := &reviewerRunnerCodeHostBackend{}
	orch := New(
		tasks,
		runs,
		WithArtifactRepository(artifacts),
		WithWritebackRepository(writebacks),
		WithEffectRepository(effects),
		WithResultSubmissionRepository(submissions),
		WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, backend)),
	)
	runnerService, err := runner.NewService(t.TempDir(), map[string]runner.Provider{providerName: provider})
	if err != nil {
		t.Fatalf("new runner service: %v", err)
	}
	server := httptest.NewServer(runner.NewHTTPHandler(runnerService))
	defer server.Close()
	client := agentrunner.NewClient(server.URL, server.Client())
	workspaceRoot := t.TempDir()
	coordinator := NewReviewerRunnerCoordinator(tasks, runs, executions, workspace.NewReviewerPreparer(), client, orch)
	started, err := coordinator.Start(ctx, ReviewerRunnerStartRequest{
		Task: task,
		Run:  runRecord,
		Workspace: workspace.ReviewerRequest{
			WorkspaceRoot: workspaceRoot, Project: task.TargetRepo, RepositoryURL: origin,
			SourceBranch: "feature/review", TargetBranch: "master", HeadSHA: headSHA, RequestedReviewMode: workspace.ReviewModeFull,
		},
		Provider: runner.ProviderConfig{Name: providerName, Revision: 1},
	})
	if err != nil {
		t.Fatalf("start persisted reviewer flow: %v", err)
	}
	if started.ExecutionID != runID || started.RequestHash == "" {
		t.Fatalf("unexpected started projection: %#v", started)
	}

	var reconciled *ReviewerRunnerReconcileResult
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		reconciled, err = coordinator.Reconcile(ctx, runID)
		if err != nil {
			t.Fatalf("reconcile persisted reviewer flow: %v", err)
		}
		if reconciled.Submission != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if reconciled == nil || reconciled.Submission == nil {
		if reconciled == nil {
			t.Fatal("reviewer flow did not produce a reconcile result")
		}
		t.Fatalf("reviewer flow did not submit result: execution=%#v events=%#v", reconciled.Execution, reconciled.Events)
	}
	if reconciled.Execution.Status != model.AgentExecutionStatusSucceeded || reconciled.Execution.LastEventSeq == 0 || reconciled.Execution.ResultHash == "" {
		t.Fatalf("unexpected terminal projection: %#v", reconciled.Execution)
	}

	storedTask, err := tasks.GetByTaskID(ctx, task.TaskID)
	if err != nil || storedTask.Status != model.TaskStatusSucceeded {
		t.Fatalf("unexpected terminal task: task=%#v err=%v", storedTask, err)
	}
	storedRun, err := runs.GetByRunID(ctx, runID)
	if err != nil || storedRun.Status != model.RunStatusSucceeded {
		t.Fatalf("unexpected terminal run: run=%#v err=%v", storedRun, err)
	}
	storedSubmission, err := submissions.GetByRunID(ctx, runID)
	if err != nil || storedSubmission.Source != "agent_runner" || storedSubmission.Status != model.ResultSubmissionStatusSucceeded {
		t.Fatalf("unexpected result submission: submission=%#v err=%v", storedSubmission, err)
	}
	var writeback model.Writeback
	if err := db.Where("run_id = ?", runID).First(&writeback).Error; err != nil || writeback.Status != model.WritebackStatusSucceeded {
		t.Fatalf("unexpected reviewer writeback: writeback=%#v err=%v", writeback, err)
	}
	var effect model.Effect
	if err := db.Where("run_id = ?", runID).First(&effect).Error; err != nil || effect.Status != model.EffectStatusPending {
		t.Fatalf("unexpected reviewer effect: effect=%#v err=%v", effect, err)
	}
	if backend.mergeRequestNoteCalls != 1 {
		t.Fatalf("expected one MR review comment, got %d", backend.mergeRequestNoteCalls)
	}

	replayed, err := coordinator.Reconcile(ctx, runID)
	if err != nil || replayed.Submission == nil || replayed.Submission.Submission.SubmissionID != storedSubmission.SubmissionID {
		t.Fatalf("reconcile immutable result replay: result=%#v err=%v", replayed, err)
	}
	if backend.mergeRequestNoteCalls != 1 {
		t.Fatalf("immutable replay duplicated MR comment: %d", backend.mergeRequestNoteCalls)
	}
}

func createFakeClaudeExecutable(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	result := `{"type":"result","subtype":"success","session_id":"fake-session","is_error":false,"duration_ms":25,"ttft_ms":5,"num_turns":1,"usage":{"input_tokens":10,"output_tokens":5},"structured_output":{"task_id":"task-persisted-reviewer","run_id":"run-persisted-reviewer","verdict":"approved","summary":"no blocking issue","findings":[],"test_summary":"not run: fake CLI","comment_body":"AI Review 结论：建议合并。"}}`
	source := filepath.Join(root, "main.go")
	content := "package main\nimport \"fmt\"\nfunc main() { fmt.Println(" + strconv.Quote(result) + ") }\n"
	if err := os.WriteFile(source, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	name := "fake-claude"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(root, name)
	command := exec.Command("go", "build", "-o", path, source)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build fake Claude CLI: %v\n%s", err, output)
	}
	return path
}

func createFakeCodexExecutable(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "main.go")
	content := `package main
import (
  "fmt"
  "os"
)
func main() {
  output := ""
  for index := 1; index+1 < len(os.Args); index++ {
    if os.Args[index] == "--output-last-message" { output = os.Args[index+1] }
  }
  if output == "" { os.Exit(2) }
  result := []byte("{\"task_id\":\"task-persisted-reviewer\",\"run_id\":\"run-persisted-reviewer\",\"verdict\":\"approved\",\"summary\":\"no blocking issue\",\"findings\":[],\"test_summary\":\"not run: fake CLI\",\"comment_body\":\"AI Review 结论：建议合并。\"}")
  if err := os.WriteFile(output, result, 0600); err != nil { panic(err) }
  fmt.Println("{\"type\":\"thread.started\",\"thread_id\":\"fake-thread\"}")
  fmt.Println("{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":10,\"cached_input_tokens\":2,\"output_tokens\":5}}")
}`
	if err := os.WriteFile(source, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	name := "fake-codex"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(root, name)
	command := exec.Command("go", "build", "-o", path, source)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build fake Codex CLI: %v\n%s", err, output)
	}
	return path
}

type persistedReviewerProvider struct{}

func (persistedReviewerProvider) Execute(_ context.Context, request runner.ProviderRequest, emit runner.EmitFunc) (json.RawMessage, error) {
	for _, name := range []string{workspace.ManifestFileName, request.PromptFile, request.SchemaFile, agent.TaskContextFileName} {
		if _, err := os.Stat(filepath.Join(request.WorkspacePath, name)); err != nil {
			return nil, err
		}
	}
	emit("provider_progress", map[string]any{"message": "review complete"})
	return json.RawMessage(`{"task_id":"task-persisted-reviewer","run_id":"run-persisted-reviewer","verdict":"approved","summary":"no blocking issue","findings":[],"test_summary":"not run: review only","comment_body":"AI Review 结论：建议合并。"}`), nil
}

type reviewerRunnerCodeHostBackend struct{ mergeRequestNoteCalls int }

func (*reviewerRunnerCodeHostBackend) CreateIssue(context.Context, string, codehost.CreateIssueRequest) (*codehost.Issue, error) {
	return &codehost.Issue{IID: 1}, nil
}

func (*reviewerRunnerCodeHostBackend) CreateMergeRequest(context.Context, string, codehost.CreateMergeRequestRequest) (*codehost.MergeRequest, error) {
	return &codehost.MergeRequest{IID: 1}, nil
}

func (*reviewerRunnerCodeHostBackend) CreateIssueNote(context.Context, string, int, codehost.CreateNoteRequest) (*codehost.Note, error) {
	return &codehost.Note{ID: 1}, nil
}

func (b *reviewerRunnerCodeHostBackend) CreateMergeRequestNote(_ context.Context, _ string, _ int, request codehost.CreateNoteRequest) (*codehost.Note, error) {
	b.mergeRequestNoteCalls++
	return &codehost.Note{ID: 202, Body: request.Body}, nil
}

func (*reviewerRunnerCodeHostBackend) SetIssueLabels(context.Context, string, int, codehost.SetLabelsRequest) (*codehost.Issue, error) {
	return &codehost.Issue{IID: 1}, nil
}

func (*reviewerRunnerCodeHostBackend) SetMergeRequestLabels(context.Context, string, int, codehost.SetLabelsRequest) (*codehost.MergeRequest, error) {
	return &codehost.MergeRequest{IID: 1}, nil
}

func createReviewerRunnerOrigin(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	seed := filepath.Join(root, "seed")
	if err := os.MkdirAll(seed, 0o755); err != nil {
		t.Fatal(err)
	}
	reviewerRunnerGit(t, seed, "init", "-b", "master")
	reviewerRunnerGit(t, seed, "config", "user.name", "Integration Test")
	reviewerRunnerGit(t, seed, "config", "user.email", "integration@example.com")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reviewerRunnerGit(t, seed, "add", "README.md")
	reviewerRunnerGit(t, seed, "commit", "-m", "base")
	reviewerRunnerGit(t, seed, "checkout", "-b", "feature/review")
	if err := os.WriteFile(filepath.Join(seed, "feature.go"), []byte("package feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reviewerRunnerGit(t, seed, "add", "feature.go")
	reviewerRunnerGit(t, seed, "commit", "-m", "feature")
	head := strings.TrimSpace(reviewerRunnerGit(t, seed, "rev-parse", "HEAD"))
	origin := filepath.Join(root, "origin.git")
	reviewerRunnerGit(t, root, "init", "--bare", origin)
	reviewerRunnerGit(t, seed, "remote", "add", "origin", origin)
	reviewerRunnerGit(t, seed, "push", "origin", "master", "feature/review")
	return origin, head
}

func reviewerRunnerGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}
