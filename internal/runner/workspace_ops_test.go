package runner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/workspaceproto"
)

func runGitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

// createSeedOrigin 建一个裸远端与一条 main 分支种子提交（local path 远端，
// 无凭据要求）。
func createSeedOrigin(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	origin := filepath.Join(base, "origin.git")
	runGitIn(t, base, "init", "--bare", origin)
	runGitIn(t, origin, "symbolic-ref", "HEAD", "refs/heads/main")
	work := filepath.Join(base, "work")
	runGitIn(t, base, "clone", origin, work)
	runGitIn(t, work, "config", "user.name", "Seed")
	runGitIn(t, work, "config", "user.email", "seed@example.test")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	runGitIn(t, work, "add", "-A")
	runGitIn(t, work, "commit", "-m", "seed")
	runGitIn(t, work, "branch", "-M", "main")
	runGitIn(t, work, "push", "-u", "origin", "main")
	return origin, strings.TrimSpace(runGitIn(t, work, "rev-parse", "HEAD"))
}

// allowAllAuthorizer 是测试用授权确认桩（状态改变类操作放行）。
func allowAllAuthorizer(context.Context, workspaceproto.AuthorizationRequest) (workspaceproto.AuthorizationDecision, error) {
	return workspaceproto.AuthorizationDecision{Authorized: true}, nil
}

func newWorkspaceOpsForTest(t *testing.T, maxInFlight int) (*WorkspaceOps, string) {
	t.Helper()
	dataRoot := t.TempDir()
	ops, err := NewWorkspaceOps(WorkspaceOpsConfig{DataRoot: dataRoot, MaxInFlight: maxInFlight, Authorize: allowAllAuthorizer})
	if err != nil {
		t.Fatalf("new workspace ops: %v", err)
	}
	return ops, dataRoot
}

func developerPrepareRequest(origin, runID, operationID string) workspaceproto.PrepareRequest {
	return workspaceproto.PrepareRequest{
		OperationID: operationID, Kind: workspaceproto.KindDeveloperPrepare,
		TenantID: 1, TaskID: "task-" + runID, RunID: runID, Role: "DeveloperWorker",
		TaskType: "develop_from_issue", Project: "group/project",
		RepositoryURL: origin, TargetBranch: "main", DeliveryBranch: "agent/" + runID,
	}
}

func developerCommitRequest(origin, runID, operationID, message, baseline string) workspaceproto.CommitRequest {
	return workspaceproto.CommitRequest{
		OperationID: operationID, TenantID: 1, TaskID: "task-" + runID, RunID: runID,
		TaskType: "develop_from_issue", Message: message,
		RepositoryURL: origin, TargetBranch: "main", DeliveryBranch: "agent/" + runID,
		BaselineSHA: baseline, AuthorName: "AI Developer", AuthorEmail: "ai-developer@localhost",
	}
}

func developerPushRequest(origin, runID, operationID, commitSHA, baseline string) workspaceproto.PushRequest {
	return workspaceproto.PushRequest{
		OperationID: operationID, TenantID: 1, TaskID: "task-" + runID, RunID: runID,
		TaskType: "develop_from_issue", RepositoryURL: origin, TargetBranch: "main",
		DeliveryBranch: "agent/" + runID, CommitSHA: commitSHA, BaselineSHA: baseline,
	}
}

func TestWorkspaceOpsPrepareIdempotentAndNoOverwrite(t *testing.T) {
	origin, seedSHA := createSeedOrigin(t)
	ops, dataRoot := newWorkspaceOpsForTest(t, 0)
	ctx := context.Background()

	first, err := ops.PrepareWorkspace(ctx, developerPrepareRequest(origin, "run-a", "op-prepare-1"))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if first.BaselineSHA != seedSHA {
		t.Fatalf("baseline = %s, want seed %s", first.BaselineSHA, seedSHA)
	}
	if first.Adopted {
		t.Fatal("first prepare must not report adoption")
	}
	repoRoot := filepath.Join(dataRoot, "workspaces", "runs", "run-a", "repo")
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err != nil {
		t.Fatalf("workspace repo missing: %v", err)
	}

	// 模拟 Agent 在途改动。
	agentFile := filepath.Join(repoRoot, "agent-work.txt")
	if err := os.WriteFile(agentFile, []byte("in progress\n"), 0o644); err != nil {
		t.Fatalf("write agent file: %v", err)
	}

	replayed, err := ops.PrepareWorkspace(ctx, developerPrepareRequest(origin, "run-a", "op-prepare-1"))
	if err != nil {
		t.Fatalf("prepare replay: %v", err)
	}
	if replayed.BaselineSHA != first.BaselineSHA || replayed.WorkspacePath != first.WorkspacePath {
		t.Fatalf("replay diverged: %+v vs %+v", replayed, first)
	}

	adopted, err := ops.PrepareWorkspace(ctx, developerPrepareRequest(origin, "run-a", "op-prepare-2"))
	if err != nil {
		t.Fatalf("second prepare with new operation id: %v", err)
	}
	if !adopted.Adopted {
		t.Fatal("second prepare must adopt the existing in-flight workspace")
	}
	if adopted.BaselineSHA != first.BaselineSHA {
		t.Fatalf("adopted baseline = %s, want %s", adopted.BaselineSHA, first.BaselineSHA)
	}
	if _, err := os.Stat(agentFile); err != nil {
		t.Fatalf("in-flight workspace changes were overwritten: %v", err)
	}
}

func TestWorkspaceOpsCommitReplayAndConflict(t *testing.T) {
	origin, _ := createSeedOrigin(t)
	ops, dataRoot := newWorkspaceOpsForTest(t, 0)
	ctx := context.Background()
	runID := "run-commit"
	prepare, err := ops.PrepareWorkspace(ctx, developerPrepareRequest(origin, runID, "op-prepare"))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	repoRoot := filepath.Join(dataRoot, "workspaces", "runs", runID, "repo")
	if err := os.WriteFile(filepath.Join(repoRoot, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}

	commitRequest := developerCommitRequest(origin, runID, "op-commit-1", "feat: add feature file\n\n- adds feature.txt", prepare.BaselineSHA)
	commitRequest.Owner = "coordinator-a"
	commitRequest.Generation = 1
	committed, err := ops.CommitWorkspace(ctx, commitRequest)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if len(committed.CommitSHA) != 40 {
		t.Fatalf("commit sha = %q", committed.CommitSHA)
	}

	replayRequest := commitRequest
	replayRequest.Owner = "coordinator-b"
	replayRequest.Generation = 2
	replayed, err := ops.CommitWorkspace(ctx, replayRequest)
	if err != nil {
		t.Fatalf("commit replay: %v", err)
	}
	if replayed.CommitSHA != committed.CommitSHA {
		t.Fatalf("replay sha = %s, want %s", replayed.CommitSHA, committed.CommitSHA)
	}
	commits := strings.Fields(strings.TrimSpace(runGitIn(t, repoRoot, "rev-list", "HEAD")))
	if len(commits) != 2 {
		t.Fatalf("workspace has %d commits, want 2 (seed + delivery; replay must not create another)", len(commits))
	}

	conflict := commitRequest
	conflict.Message = "feat: different message"
	if _, err := ops.CommitWorkspace(ctx, conflict); !errors.Is(err, ErrOperationConflict) {
		t.Fatalf("same operation id with different payload: err = %v, want ErrOperationConflict", err)
	}

	second := developerCommitRequest(origin, runID, "op-commit-2", "feat: add feature file\n\n- adds feature.txt", prepare.BaselineSHA)
	if _, err := ops.CommitWorkspace(ctx, second); err == nil {
		t.Fatal("second delivery commit on the same run must be refused")
	}
}

// TestWorkspaceOpsPushFixedSHAAndReplay 锁定固定 SHA 推送与重放：远端头在推送
// 后等于该 SHA；同操作 ID 重放返回同一结果。
func TestWorkspaceOpsPushFixedSHAAndReplay(t *testing.T) {
	origin, _ := createSeedOrigin(t)
	ops, dataRoot := newWorkspaceOpsForTest(t, 0)
	ctx := context.Background()
	runID := "run-push"
	prepare, err := ops.PrepareWorkspace(ctx, developerPrepareRequest(origin, runID, "op-prepare"))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	repoRoot := filepath.Join(dataRoot, "workspaces", "runs", runID, "repo")
	if err := os.WriteFile(filepath.Join(repoRoot, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	committed, err := ops.CommitWorkspace(ctx, developerCommitRequest(origin, runID, "op-commit", "feat: add feature\n\n- adds feature.txt", prepare.BaselineSHA))
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	pushRequest := developerPushRequest(origin, runID, "op-push-1", committed.CommitSHA, prepare.BaselineSHA)
	pushRequest.Owner = "coordinator-a"
	pushRequest.Generation = 1
	pushed, err := ops.PushWorkspace(ctx, pushRequest)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if pushed.RemoteHeadSHA != committed.CommitSHA || pushed.CommitSHA != committed.CommitSHA {
		t.Fatalf("push result = %+v", pushed)
	}
	remoteHead := strings.TrimSpace(runGitIn(t, origin, "rev-parse", "refs/heads/agent/"+runID))
	if remoteHead != committed.CommitSHA {
		t.Fatalf("remote head = %s, want %s", remoteHead, committed.CommitSHA)
	}

	replayRequest := pushRequest
	replayRequest.Owner = "coordinator-b"
	replayRequest.Generation = 2
	replayed, err := ops.PushWorkspace(ctx, replayRequest)
	if err != nil {
		t.Fatalf("push replay: %v", err)
	}
	if replayed.RemoteHeadSHA != committed.CommitSHA {
		t.Fatalf("push replay = %+v", replayed)
	}
}

// TestWorkspaceOpsRestartReadBackAndInterruptedRecovery 锁定 Runner 重启语义：
// 已完成操作可回读；在途记录落为 failed 并允许显式重试（清理操作幂等）。
func TestWorkspaceOpsRestartReadBackAndInterruptedRecovery(t *testing.T) {
	origin, _ := createSeedOrigin(t)
	ops, dataRoot := newWorkspaceOpsForTest(t, 0)
	ctx := context.Background()
	prepare, err := ops.PrepareWorkspace(ctx, developerPrepareRequest(origin, "run-restart", "op-prepare"))
	if err != nil || prepare.BaselineSHA == "" {
		t.Fatalf("prepare: %v", err)
	}

	// 重启：同一数据根上的新实例必须能读回已完成操作。
	restarted, err := NewWorkspaceOps(WorkspaceOpsConfig{DataRoot: dataRoot, Authorize: allowAllAuthorizer})
	if err != nil {
		t.Fatalf("restart ops: %v", err)
	}
	record, err := restarted.GetOperation("op-prepare")
	if err != nil {
		t.Fatalf("read back after restart: %v", err)
	}
	if record.Status != "completed" {
		t.Fatalf("record status = %s", record.Status)
	}
	replayed, err := restarted.PrepareWorkspace(ctx, developerPrepareRequest(origin, "run-restart", "op-prepare"))
	if err != nil || replayed.BaselineSHA != prepare.BaselineSHA {
		t.Fatalf("prepare replay after restart: %+v err=%v", replayed, err)
	}

	// 模拟上次进程中断遗留的在途记录：新实例恢复时落为 failed，可显式重试。
	cleanup := workspaceproto.CleanupRequest{OperationID: "op-interrupted", TenantID: 1, TaskID: "task-run-restart", RunID: "run-restart"}
	cleanupHash, err := workspaceproto.PayloadHash(cleanup)
	if err != nil {
		t.Fatalf("hash cleanup payload: %v", err)
	}
	stale := &workspaceOperationRecord{
		OperationID: "op-interrupted", Kind: workspaceproto.KindCleanup, TenantID: 1,
		TaskID: "task-run-restart", RunID: "run-restart", PayloadHash: cleanupHash,
		Status: workspaceOpStatusInFlight, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := restarted.writeRecordLocked(stale); err != nil {
		t.Fatalf("seed stale record: %v", err)
	}
	recovered, err := NewWorkspaceOps(WorkspaceOpsConfig{DataRoot: dataRoot, Authorize: allowAllAuthorizer})
	if err != nil {
		t.Fatalf("second restart: %v", err)
	}
	record, err = recovered.GetOperation("op-interrupted")
	if err != nil {
		t.Fatalf("read recovered record: %v", err)
	}
	if record.Status != "failed" || !strings.Contains(record.ErrorMessage, "restarted") {
		t.Fatalf("interrupted record not settled: %+v", record)
	}
	if _, err := recovered.CleanupWorkspace(ctx, cleanup); err != nil {
		t.Fatalf("explicit retry after interruption: %v", err)
	}
}

func TestWorkspaceOpsAuthorizationGate(t *testing.T) {
	origin, _ := createSeedOrigin(t)
	ctx := context.Background()

	t.Run("no channel is fail closed", func(t *testing.T) {
		dataRoot := t.TempDir()
		ops, err := NewWorkspaceOps(WorkspaceOpsConfig{DataRoot: dataRoot})
		if err != nil {
			t.Fatalf("new ops: %v", err)
		}
		// 准备（非状态改变）不受影响。
		if _, err := ops.PrepareWorkspace(ctx, developerPrepareRequest(origin, "run-gate", "op-gate-prepare")); err != nil {
			t.Fatalf("prepare must not require the authorization channel: %v", err)
		}
		_, err = ops.CleanupWorkspace(ctx, workspaceproto.CleanupRequest{OperationID: "op-gate-cleanup", TenantID: 1, TaskID: "t", RunID: "run-gate"})
		if !errors.Is(err, ErrNotAuthorized) {
			t.Fatalf("cleanup without authorization channel: err = %v, want ErrNotAuthorized", err)
		}
		if _, err := ops.GetOperation("op-gate-cleanup"); !errors.Is(err, ErrOperationNotFound) {
			t.Fatalf("rejected operation must not be recorded: %v", err)
		}
	})

	t.Run("denied decision rejects the operation", func(t *testing.T) {
		dataRoot := t.TempDir()
		ops, err := NewWorkspaceOps(WorkspaceOpsConfig{
			DataRoot: dataRoot,
			Authorize: func(context.Context, workspaceproto.AuthorizationRequest) (workspaceproto.AuthorizationDecision, error) {
				return workspaceproto.AuthorizationDecision{Authorized: false, Reason: "task is owned by another coordinator"}, nil
			},
		})
		if err != nil {
			t.Fatalf("new ops: %v", err)
		}
		_, err = ops.CleanupWorkspace(ctx, workspaceproto.CleanupRequest{OperationID: "op-gate-denied", TenantID: 1, TaskID: "t", RunID: "run-gate"})
		if !errors.Is(err, ErrNotAuthorized) || !strings.Contains(err.Error(), "another coordinator") {
			t.Fatalf("denied authorization: err = %v", err)
		}
		if _, err := ops.GetOperation("op-gate-denied"); !errors.Is(err, ErrOperationNotFound) {
			t.Fatalf("denied operation must not be recorded: %v", err)
		}
	})

	t.Run("takeover between first check and durable acceptance rejects execution", func(t *testing.T) {
		checks := 0
		ops, err := NewWorkspaceOps(WorkspaceOpsConfig{
			DataRoot: t.TempDir(),
			Authorize: func(context.Context, workspaceproto.AuthorizationRequest) (workspaceproto.AuthorizationDecision, error) {
				checks++
				return workspaceproto.AuthorizationDecision{Authorized: checks == 1}, nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ops.PrepareWorkspace(ctx, developerPrepareRequest(origin, "run-takeover", "op-takeover-prepare")); err != nil {
			t.Fatal(err)
		}
		_, err = ops.CleanupWorkspace(ctx, workspaceproto.CleanupRequest{OperationID: "op-takeover-cleanup", TenantID: 1, TaskID: "task", RunID: "run-takeover"})
		if !errors.Is(err, ErrNotAuthorized) {
			t.Fatalf("stale owner operation must be denied before execution: checks=%d err=%v", checks, err)
		}
		if checks != 2 {
			t.Fatalf("authorization checks = %d, want 2", checks)
		}
	})
}

// TestWorkspaceOpsCapacityAndStopAccepting 锁定有界并发与退出语义：容量满时
// 新操作未被接纳（可重试）；停止接纳后新操作拒绝，已完成操作仍可重放。
func TestWorkspaceOpsCapacityAndStopAccepting(t *testing.T) {
	origin, _ := createSeedOrigin(t)
	ops, dataRoot := newWorkspaceOpsForTest(t, 1)
	ctx := context.Background()
	if _, err := ops.PrepareWorkspace(ctx, developerPrepareRequest(origin, "run-cap", "op-cap-prepare")); err != nil {
		t.Fatalf("prepare: %v", err)
	}

	// 卡住执行段，占用唯一容量槽。
	entered := make(chan struct{})
	release := make(chan struct{})
	ops.testBeforeExecute = func() {
		close(entered)
		<-release
	}
	done := make(chan error, 1)
	go func() {
		_, err := ops.CleanupWorkspace(ctx, workspaceproto.CleanupRequest{OperationID: "op-cap-1", TenantID: 1, TaskID: "t", RunID: "run-cap"})
		done <- err
	}()
	<-entered

	_, err := ops.CleanupWorkspace(ctx, workspaceproto.CleanupRequest{OperationID: "op-cap-2", TenantID: 1, TaskID: "t", RunID: "run-cap"})
	if !errors.Is(err, ErrTooManyOperations) {
		t.Fatalf("capacity error = %v, want ErrTooManyOperations", err)
	}

	ops.StopAccepting()
	ops.testBeforeExecute = nil
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("in-flight operation must finish after stop accepting: %v", err)
	}
	if _, err := ops.CleanupWorkspace(ctx, workspaceproto.CleanupRequest{OperationID: "op-cap-3", TenantID: 1, TaskID: "t", RunID: "run-cap"}); !errors.Is(err, ErrServiceNotAccepting) {
		t.Fatalf("stop-accepting error = %v, want ErrServiceNotAccepting", err)
	}
	// 已完成操作的显式重放不受停止接纳影响。
	if _, err := ops.CleanupWorkspace(ctx, workspaceproto.CleanupRequest{OperationID: "op-cap-1", TenantID: 1, TaskID: "t", RunID: "run-cap"}); err != nil {
		t.Fatalf("replay of completed operation must not be blocked: %v", err)
	}
	_ = dataRoot
}
