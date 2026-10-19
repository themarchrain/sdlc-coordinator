package runner_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sdlc-coordinator/internal/adapters/agentrunner"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/workspaceproto"
)

func httpTestRunGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func httpTestSeedOrigin(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	origin := filepath.Join(base, "origin.git")
	httpTestRunGit(t, base, "init", "--bare", origin)
	httpTestRunGit(t, origin, "symbolic-ref", "HEAD", "refs/heads/main")
	work := filepath.Join(base, "work")
	httpTestRunGit(t, base, "clone", origin, work)
	httpTestRunGit(t, work, "config", "user.name", "Seed")
	httpTestRunGit(t, work, "config", "user.email", "seed@example.test")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	httpTestRunGit(t, work, "add", "-A")
	httpTestRunGit(t, work, "commit", "-m", "seed")
	httpTestRunGit(t, work, "branch", "-M", "main")
	httpTestRunGit(t, work, "push", "-u", "origin", "main")
	return origin, strings.TrimSpace(httpTestRunGit(t, work, "rev-parse", "HEAD"))
}

func newWorkspaceHTTPServer(t *testing.T, token string) (*runner.WorkspaceOps, *httptest.Server) {
	t.Helper()
	service, err := runner.NewService(t.TempDir(), map[string]runner.Provider{})
	if err != nil {
		t.Fatalf("new runner service: %v", err)
	}
	ops, err := runner.NewWorkspaceOps(runner.WorkspaceOpsConfig{
		DataRoot: t.TempDir(),
		Authorize: func(context.Context, workspaceproto.AuthorizationRequest) (workspaceproto.AuthorizationDecision, error) {
			return workspaceproto.AuthorizationDecision{Authorized: true}, nil
		},
	})
	if err != nil {
		t.Fatalf("new workspace ops: %v", err)
	}
	server := httptest.NewServer(runner.NewHTTPHandler(service, runner.WithWorkspaceOperations(ops, token)))
	t.Cleanup(server.Close)
	return ops, server
}

// TestWorkspaceHTTPAuthFailClosed 受控操作面的鉴权语义：未配置令牌的通道
// fail-closed 503；错误/缺失令牌 401；合法令牌放行。
func TestWorkspaceHTTPAuthFailClosed(t *testing.T) {
	ctx := context.Background()
	request := workspaceproto.CleanupRequest{OperationID: "op-auth", TenantID: 1, TaskID: "t", RunID: "r"}

	t.Run("channel closed", func(t *testing.T) {
		_, server := newWorkspaceHTTPServer(t, "")
		client := agentrunner.NewWorkspaceClient(server.URL, "any-token", nil)
		_, err := client.CleanupWorkspace(ctx, request)
		var httpErr *runner.HTTPError
		if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("err = %v, want HTTP 503", err)
		}
		if !strings.Contains(httpErr.Body, "channel_closed") {
			t.Fatalf("body = %s", httpErr.Body)
		}
	})

	t.Run("wrong token", func(t *testing.T) {
		_, server := newWorkspaceHTTPServer(t, "deployment-secret-token")
		client := agentrunner.NewWorkspaceClient(server.URL, "wrong-token", nil)
		_, err := client.CleanupWorkspace(ctx, request)
		var httpErr *runner.HTTPError
		if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnauthorized {
			t.Fatalf("err = %v, want HTTP 401", err)
		}
	})
}

func TestWorkspaceHTTPRoundTripWithoutSharedDirectory(t *testing.T) {
	origin, seedSHA := httpTestSeedOrigin(t)
	ops, server := newWorkspaceHTTPServer(t, "deployment-secret-token")
	client := agentrunner.NewWorkspaceClient(server.URL, "deployment-secret-token", nil)
	ctx := context.Background()
	runID := "run-http"

	prepare, err := client.PrepareWorkspace(ctx, workspaceproto.PrepareRequest{
		OperationID: "op-http-prepare", Kind: workspaceproto.KindDeveloperPrepare,
		TenantID: 9, TaskID: "task-http", RunID: runID, Role: "DeveloperWorker",
		TaskType: "develop_from_issue", Project: "group/project",
		RepositoryURL: origin, TargetBranch: "main", DeliveryBranch: "agent/" + runID,
	})
	if err != nil {
		t.Fatalf("prepare over http: %v", err)
	}
	if prepare.BaselineSHA != seedSHA {
		t.Fatalf("baseline = %s, want %s", prepare.BaselineSHA, seedSHA)
	}
	if !strings.HasPrefix(prepare.WorkspacePath, ops.WorkspaceRoot()) {
		t.Fatalf("workspace path %q escaped the runner root %q", prepare.WorkspacePath, ops.WorkspaceRoot())
	}

	// 模拟 Agent 在 Runner 本机工作区写入改动（协调器侧没有该目录的任何句柄）。
	if err := os.WriteFile(filepath.Join(prepare.WorkspacePath, "repo", "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}

	commitRequest := workspaceproto.CommitRequest{
		OperationID: "op-http-commit", TenantID: 9, TaskID: "task-http", RunID: runID,
		TaskType: "develop_from_issue", Message: "feat: add feature\n\n- adds feature.txt",
		RepositoryURL: origin, TargetBranch: "main", DeliveryBranch: "agent/" + runID,
		BaselineSHA: prepare.BaselineSHA, AuthorName: "AI Developer", AuthorEmail: "ai-developer@localhost",
	}
	committed, err := client.CommitWorkspace(ctx, commitRequest)
	if err != nil {
		t.Fatalf("commit over http: %v", err)
	}
	replayed, err := client.CommitWorkspace(ctx, commitRequest)
	if err != nil || replayed.CommitSHA != committed.CommitSHA {
		t.Fatalf("commit replay over http: %+v err=%v", replayed, err)
	}

	pushed, err := client.PushWorkspace(ctx, workspaceproto.PushRequest{
		OperationID: "op-http-push", TenantID: 9, TaskID: "task-http", RunID: runID,
		TaskType: "develop_from_issue", RepositoryURL: origin, TargetBranch: "main",
		DeliveryBranch: "agent/" + runID, CommitSHA: committed.CommitSHA, BaselineSHA: prepare.BaselineSHA,
	})
	if err != nil {
		t.Fatalf("push over http: %v", err)
	}
	if pushed.RemoteHeadSHA != committed.CommitSHA {
		t.Fatalf("remote head = %s, want %s", pushed.RemoteHeadSHA, committed.CommitSHA)
	}
	remoteHead := strings.TrimSpace(httpTestRunGit(t, origin, "rev-parse", "refs/heads/agent/"+runID))
	if remoteHead != committed.CommitSHA {
		t.Fatalf("origin head = %s, want %s", remoteHead, committed.CommitSHA)
	}

	record, err := client.WorkspaceOperation(ctx, "op-http-commit")
	if err != nil {
		t.Fatalf("read operation over http: %v", err)
	}
	if record.Status != "completed" || record.RunID != runID {
		t.Fatalf("record = %+v", record)
	}
}
