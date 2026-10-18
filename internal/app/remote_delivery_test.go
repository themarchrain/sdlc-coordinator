package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"sdlc-coordinator/internal/adapters/agentrunner"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/workspace"
	"sdlc-coordinator/internal/workspaceproto"

	orchestrator "sdlc-coordinator/internal/orchestrator"
)

func deliveryGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

// newRemoteDeliveryOrigin 建裸远端与 main 种子提交，返回 file:// 形态的
// base_url 供注册仓行派生 clone URL（含 ".git" 后缀的目录名）。
func newRemoteDeliveryOrigin(t *testing.T) (baseURL string, pathWithNamespace string, originDir string, seedSHA string) {
	t.Helper()
	dir := t.TempDir()
	origin := filepath.Join(dir, "origin.git")
	deliveryGit(t, dir, "init", "--bare", origin)
	deliveryGit(t, origin, "symbolic-ref", "HEAD", "refs/heads/main")
	work := filepath.Join(dir, "work")
	deliveryGit(t, dir, "clone", origin, work)
	deliveryGit(t, work, "config", "user.name", "Seed")
	deliveryGit(t, work, "config", "user.email", "seed@example.test")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	deliveryGit(t, work, "add", "-A")
	deliveryGit(t, work, "commit", "-m", "seed")
	deliveryGit(t, work, "branch", "-M", "main")
	deliveryGit(t, work, "push", "-u", "origin", "main")
	seedSHA = strings.TrimSpace(deliveryGit(t, work, "rev-parse", "HEAD"))
	fileURL := "file:///" + strings.TrimPrefix(filepath.ToSlash(dir), "/")
	return fileURL, "origin", origin, seedSHA
}

// dropFirstCommitResponse 模拟commit 已执行但响应丢失的传输层故障注入：
// 第一次 commit 请求照常送达并消费响应体（服务端已完成操作），随后向上层报错。
type dropFirstCommitResponse struct {
	base    http.RoundTripper
	mu      sync.Mutex
	dropped bool
}

func (d *dropFirstCommitResponse) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := d.base.RoundTrip(request)
	if err != nil || response == nil {
		return response, err
	}
	if request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/internal/workspace/commit") {
		d.mu.Lock()
		first := !d.dropped
		d.dropped = true
		d.mu.Unlock()
		if first {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			return nil, errors.New("simulated response loss after the runner executed the commit")
		}
	}
	return response, nil
}

func remoteDeliveryPayload() []byte {
	return []byte(`{"taskId":"task-remote-delivery","runId":"run-remote-delivery","outcome":"change_ready","summary":"did it","tests":[{"command":"go test ./...","passed":true,"summary":"green"}],"mergeRequest":{"title":"fix","description":"desc"},"commit_message":"fix: implement the requested change"}`)
}

type remoteDeliveryWorld struct {
	task        *model.Task
	run         *model.Run
	checkpoints *repository.DeveloperDeliveryCheckpointRepository
	leases      *repository.WorkItemLeaseRepository
	settings    orchestrator.ReviewerRunnerSettings
	serverURL   string
	token       string
	originDir   string
	cloneURL    string
	seedSHA     string
	ops         *runner.WorkspaceOps
}

// newRemoteDeliveryWorld 搭协调器只经 HTTP 使用 Runner 工作区的世界：
// 注册仓行指向本地裸远端，Runner 提供受控工作区操作面（授权确认放行）。
func newRemoteDeliveryWorld(t *testing.T) *remoteDeliveryWorld {
	t.Helper()
	baseURL, pathWithNamespace, originDir, seedSHA := newRemoteDeliveryOrigin(t)

	service, err := runner.NewService(t.TempDir(), map[string]runner.Provider{})
	if err != nil {
		t.Fatalf("runner service: %v", err)
	}
	ops, err := runner.NewWorkspaceOps(runner.WorkspaceOpsConfig{
		DataRoot: t.TempDir(),
		Authorize: func(context.Context, workspaceproto.AuthorizationRequest) (workspaceproto.AuthorizationDecision, error) {
			return workspaceproto.AuthorizationDecision{Authorized: true}, nil
		},
	})
	if err != nil {
		t.Fatalf("workspace ops: %v", err)
	}
	const token = "remote-delivery-internal-token"
	server := httptest.NewServer(runner.NewHTTPHandler(service, runner.WithWorkspaceOperations(ops, token)))
	t.Cleanup(server.Close)

	db, err := store.Open("file:remote_delivery_" + model.NewID("t") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open tenant db: %v", err)
	}
	if err := store.Migrate(db, &model.DeveloperDeliveryCheckpoint{}, &model.WorkItemLease{}); err != nil {
		t.Fatalf("migrate tenant db: %v", err)
	}

	runID := "run-remote-delivery"
	task := &model.Task{
		TaskID: "task-remote-delivery", Type: "develop_from_issue", Role: "DeveloperWorker",
		TargetRepo: "group/project", SourceRepo: "group/project",
		DeliveryBranch: "agent/task-remote-delivery", Status: model.TaskStatusRunning, CurrentRunID: &runID,
		ContextJSON: `{"project":{"path":"group/project"},"repos":[{"name":"project","url":"https://gitlab.example/group/project.git","branch":"main","access":"writable"}]}`,
	}
	run := &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}
	repoRow := &registeredrepo.RegisteredRepository{
		RepositoryID: "repo-remote", Provider: "github", BaseURL: baseURL,
		PathWithNamespace: pathWithNamespace, Status: registeredrepo.StatusEnabled,
		GitAuthorName: "AI Developer", GitAuthorEmail: "ai-developer@localhost",
	}
	settings := orchestrator.ReviewerRunnerSettings{
		ResolveRepository: func(context.Context, *model.Task) (*registeredrepo.RegisteredRepository, error) {
			return repoRow, nil
		},
	}
	return &remoteDeliveryWorld{
		task: task, run: run,
		checkpoints: repository.NewDeveloperDeliveryCheckpointRepository(db),
		leases:      repository.NewWorkItemLeaseRepository(db),
		settings:    settings, serverURL: server.URL, token: token,
		originDir: originDir, cloneURL: repoRow.CloneURL(), seedSHA: seedSHA, ops: ops,
	}
}

func (w *remoteDeliveryWorld) prepareWorkspace(t *testing.T, httpClient *http.Client) {
	t.Helper()
	remote := agentrunner.NewRemoteWorkspace(w.serverURL, w.token, 7, httpClient)
	preparer := agentrunner.NewRemoteDeveloperPreparer(remote)
	manifest, err := preparer.Prepare(context.Background(), workspace.DeveloperRequest{
		TaskID: w.task.TaskID, RunID: w.run.RunID, TaskType: w.task.Type,
		Project: "group/project", RepositoryURL: w.cloneURL,
		TargetBranch: "main", DeliveryBranch: w.task.DeliveryBranch,
		GitAuthorName: "AI Developer", GitAuthorEmail: "ai-developer@localhost",
	})
	if err != nil {
		t.Fatalf("remote prepare: %v", err)
	}
	if manifest == nil || manifest.Repository.InitialHeadSHA != w.seedSHA {
		t.Fatalf("remote prepare baseline = %+v, want %s", manifest, w.seedSHA)
	}
	if err := w.checkpoints.RecordBaseline(context.Background(), &model.DeveloperDeliveryCheckpoint{
		TaskID: w.task.TaskID, RunID: w.run.RunID, TaskType: w.task.Type,
		RepositoryURL: w.cloneURL, DeliveryBranch: w.task.DeliveryBranch,
		BaselineSHA: w.seedSHA,
	}); err != nil {
		t.Fatalf("record baseline: %v", err)
	}
	// 模拟 Agent 在 Runner 本机工作区产生改动（协调器无任何该目录句柄）。
	repoRoot := filepath.Join(w.ops.WorkspaceRoot(), "runs", w.run.RunID, "repo")
	if err := os.WriteFile(filepath.Join(repoRoot, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatalf("write agent change: %v", err)
	}
}

func (w *remoteDeliveryWorld) newChannel(httpClient *http.Client, source string, submitter orchestrator.ReviewerResultSubmitter) *orchestrator.ResultChannelCoordinator {
	remote := agentrunner.NewRemoteWorkspace(w.serverURL, w.token, 7, httpClient)
	channel := orchestrator.NewResultChannelCoordinator(remote, submitter, w.settings, source)
	channel.SetDeveloperDeliveryCheckpoints(w.checkpoints)
	channel.SetDeliveryExecutionRight(w.leases)
	return channel
}

type recordingSubmitter struct {
	calls int
}

func (s *recordingSubmitter) SubmitResult(context.Context, orchestrator.SubmitResultRequest) (*orchestrator.SubmitResultResponse, error) {
	s.calls++
	return &orchestrator.SubmitResultResponse{Submission: &model.ResultSubmission{SubmissionID: "submission-remote", Status: model.ResultSubmissionStatusSucceeded}}, nil
}

func TestRemoteControlledDeveloperDeliveryOverHTTP(t *testing.T) {
	w := newRemoteDeliveryWorld(t)
	w.prepareWorkspace(t, nil)

	submitter := &recordingSubmitter{}
	channel := w.newChannel(nil, "mcp", submitter)
	outcome, err := channel.SubmitAgentResult(context.Background(), w.task, w.run, results.SubmitDeveloperResultTool, remoteDeliveryPayload())
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if outcome.Decision != orchestrator.ChannelAccepted {
		t.Fatalf("outcome = %+v", outcome)
	}
	checkpoint, err := w.checkpoints.Load(context.Background(), w.run.RunID)
	if err != nil || checkpoint == nil || checkpoint.CommitSHA == "" {
		t.Fatalf("checkpoint after delivery: %+v err=%v", checkpoint, err)
	}
	remoteHead := strings.TrimSpace(deliveryGit(t, w.originDir, "rev-parse", "refs/heads/"+w.task.DeliveryBranch))
	if remoteHead != checkpoint.CommitSHA {
		t.Fatalf("remote head = %s, want checkpointed %s", remoteHead, checkpoint.CommitSHA)
	}
	commits := strings.Fields(strings.TrimSpace(deliveryGit(t, w.originDir, "rev-list", "refs/heads/"+w.task.DeliveryBranch)))
	if len(commits) != 2 {
		t.Fatalf("delivery branch has %d commits, want 2 (seed + delivery)", len(commits))
	}
}

func TestRemoteControlledDeliverySurvivesLostCommitResponse(t *testing.T) {
	w := newRemoteDeliveryWorld(t)
	w.prepareWorkspace(t, nil)

	flakyClient := &http.Client{
		Timeout:   2 * time.Minute,
		Transport: &dropFirstCommitResponse{base: http.DefaultTransport},
	}
	flakyChannel := w.newChannel(flakyClient, "mcp", &recordingSubmitter{})
	first, err := flakyChannel.SubmitAgentResult(context.Background(), w.task, w.run, results.SubmitDeveloperResultTool, remoteDeliveryPayload())
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	if first.Decision == orchestrator.ChannelAccepted {
		t.Fatalf("lost commit response must not be accepted: %+v", first)
	}
	checkpoint, err := w.checkpoints.Load(context.Background(), w.run.RunID)
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if checkpoint != nil && checkpoint.CommitSHA != "" {
		t.Fatalf("lost response must not have recorded a sha: %q", checkpoint.CommitSHA)
	}

	// 重试（正常客户端）：同一操作 ID 回读 Runner 记录的原可信结果。
	retryChannel := w.newChannel(nil, "mcp", &recordingSubmitter{})
	retry, err := retryChannel.SubmitAgentResult(context.Background(), w.task, w.run, results.SubmitDeveloperResultTool, remoteDeliveryPayload())
	if err != nil {
		t.Fatalf("retry submit: %v", err)
	}
	if retry.Decision != orchestrator.ChannelAccepted {
		t.Fatalf("retry must converge via operation replay: %+v", retry)
	}
	checkpoint, err = w.checkpoints.Load(context.Background(), w.run.RunID)
	if err != nil || checkpoint == nil || checkpoint.CommitSHA == "" {
		t.Fatalf("checkpoint after retry: %+v err=%v", checkpoint, err)
	}
	commits := strings.Fields(strings.TrimSpace(deliveryGit(t, w.originDir, "rev-list", "refs/heads/"+w.task.DeliveryBranch)))
	if len(commits) != 2 {
		t.Fatalf("delivery branch has %d commits, want exactly 2 (no second delivery commit)", len(commits))
	}
	remoteHead := strings.TrimSpace(deliveryGit(t, w.originDir, "rev-parse", "refs/heads/"+w.task.DeliveryBranch))
	if remoteHead != checkpoint.CommitSHA {
		t.Fatalf("remote head = %s, checkpoint = %s", remoteHead, checkpoint.CommitSHA)
	}
}
