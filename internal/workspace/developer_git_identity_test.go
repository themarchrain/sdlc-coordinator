package workspace_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sdlc-coordinator/internal/workspace"
)

func prepareDeveloperRepo(t *testing.T, request workspace.DeveloperRequest) string {
	t.Helper()
	if _, err := workspace.NewDeveloperPreparer().Prepare(context.Background(), request); err != nil {
		t.Fatalf("prepare developer workspace: %v", err)
	}
	return filepath.Join(request.WorkspaceRoot, "runs", request.RunID, "repo")
}

func TestDeveloperPreparerLeavesGitIdentityUnset(t *testing.T) {
	origin, _, _ := createReviewOrigin(t)
	root := t.TempDir()
	repoRoot := prepareDeveloperRepo(t, workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-no-config", RunID: "run-no-config", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-no-config",
		GitAuthorName: "Release Bot", GitAuthorEmail: "bot@example.com",
	})
	// 提交身份不再落进 Runner 可写的仓库本地 config:唯一的生效点是
	// Coordinator commit 命令上的受信 -c 覆盖。
	for _, key := range []string{"user.name", "user.email"} {
		command := exec.Command("git", "config", "--local", key)
		command.Dir = repoRoot
		if err := command.Run(); err == nil {
			t.Fatalf("prepare must not write %s into the repository-local config", key)
		}
	}
}

func TestDeveloperFinalizerCommitsAIAuthoredMessageWithTrustedIdentity(t *testing.T) {
	origin, targetSHA, _ := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-ai-message", RunID: "run-ai-message", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-ai-message",
		GitAuthorName: "Release Bot", GitAuthorEmail: "bot@example.com",
	}
	repoRoot := prepareDeveloperRepo(t, request)
	if err := os.WriteFile(filepath.Join(repoRoot, "fix.go"), []byte("package fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	message := "fix: reject invalid repository state\n\n- Validate the state before delivery\n- Cover the failure path with a regression test"
	if _, err := workspace.NewDeveloperFinalizer().Finalize(context.Background(), workspace.DeveloperFinalizeRequest{
		WorkspaceRoot: root, TaskID: request.TaskID, RunID: request.RunID, CommitMessage: message,
		GitAuthorName: request.GitAuthorName, GitAuthorEmail: request.GitAuthorEmail,
		TaskType: request.TaskType, RepositoryURL: request.RepositoryURL, TargetBranch: request.TargetBranch,
		DeliveryBranch: request.DeliveryBranch, ExpectedInitialHeadSHA: targetSHA,
	}); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	expected := message + "\n\nCoordinator-Run-ID: " + request.RunID
	if got := strings.TrimSpace(runGit(t, repoRoot, "log", "-1", "--format=%B")); got != expected {
		t.Fatalf("committed message mismatch:\n got: %q\nwant: %q", got, expected)
	}
	for _, format := range []string{"%an", "%cn"} {
		if got := strings.TrimSpace(runGit(t, repoRoot, "show", "-s", "--format="+format)); got != "Release Bot" {
			t.Fatalf("identity %s = %q", format, got)
		}
	}
	for _, format := range []string{"%ae", "%ce"} {
		if got := strings.TrimSpace(runGit(t, repoRoot, "show", "-s", "--format="+format)); got != "bot@example.com" {
			t.Fatalf("identity %s = %q", format, got)
		}
	}
}

func TestDeveloperFinalizerIgnoresAmbientAuthorEnvironment(t *testing.T) {
	origin, targetSHA, _ := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-ambient", RunID: "run-ambient", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-ambient",
		GitAuthorName: "Release Bot", GitAuthorEmail: "bot@example.com",
	}
	repoRoot := prepareDeveloperRepo(t, request)
	if err := os.WriteFile(filepath.Join(repoRoot, "fix.go"), []byte("package fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_AUTHOR_NAME", "ambient-name")
	t.Setenv("GIT_AUTHOR_EMAIL", "ambient@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "ambient-name")
	t.Setenv("GIT_COMMITTER_EMAIL", "ambient@example.com")
	if _, err := workspace.NewDeveloperFinalizer().Finalize(context.Background(), workspace.DeveloperFinalizeRequest{
		WorkspaceRoot: root, TaskID: request.TaskID, RunID: request.RunID,
		CommitMessage: "fix: reject invalid repository state",
		GitAuthorName: request.GitAuthorName, GitAuthorEmail: request.GitAuthorEmail,
		TaskType:      request.TaskType, RepositoryURL: request.RepositoryURL, TargetBranch: request.TargetBranch,
		DeliveryBranch: request.DeliveryBranch, ExpectedInitialHeadSHA: targetSHA,
	}); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, repoRoot, "show", "-s", "--format=%an %ae")); got != "Release Bot bot@example.com" {
		t.Fatalf("ambient environment leaked into commit identity: %q", got)
	}
}

func TestDeveloperFinalizerOverridesRunnerTamperedIdentity(t *testing.T) {
	origin, targetSHA, _ := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-tamper", RunID: "run-tamper", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-tamper",
		GitAuthorName: "Release Bot", GitAuthorEmail: "bot@example.com",
	}
	repoRoot := prepareDeveloperRepo(t, request)
	// 模拟共享工作区上的 Runner 改写仓库本地 config:git status 不显示这种改动。
	runGit(t, repoRoot, "config", "--local", "user.name", "runner-tampered")
	runGit(t, repoRoot, "config", "--local", "user.email", "runner@example.com")
	if err := os.WriteFile(filepath.Join(repoRoot, "fix.go"), []byte("package fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.NewDeveloperFinalizer().Finalize(context.Background(), workspace.DeveloperFinalizeRequest{
		WorkspaceRoot: root, TaskID: request.TaskID, RunID: request.RunID,
		CommitMessage: "fix: reject invalid repository state",
		GitAuthorName: request.GitAuthorName, GitAuthorEmail: request.GitAuthorEmail,
		TaskType: request.TaskType, RepositoryURL: request.RepositoryURL, TargetBranch: request.TargetBranch,
		DeliveryBranch: request.DeliveryBranch, ExpectedInitialHeadSHA: targetSHA,
	}); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, repoRoot, "show", "-s", "--format=%an %ae")); got != "Release Bot bot@example.com" {
		t.Fatalf("runner-tampered config leaked into commit identity: %q", got)
	}
}

func TestDeveloperFinalizerPreservesConsecutiveBlankLines(t *testing.T) {
	origin, targetSHA, _ := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-blank", RunID: "run-blank", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-blank",
	}
	repoRoot := prepareDeveloperRepo(t, request)
	if err := os.WriteFile(filepath.Join(repoRoot, "fix.go"), []byte("package fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	message := "fix: subject\n\n- a\n\n\n- b"
	if _, err := workspace.NewDeveloperFinalizer().Finalize(context.Background(), workspace.DeveloperFinalizeRequest{
		WorkspaceRoot: root, TaskID: request.TaskID, RunID: request.RunID,
		CommitMessage: message,
		TaskType:      request.TaskType, RepositoryURL: request.RepositoryURL, TargetBranch: request.TargetBranch,
		DeliveryBranch: request.DeliveryBranch, ExpectedInitialHeadSHA: targetSHA,
	}); err != nil {
		t.Fatalf("consecutive blank lines must survive git cleanup: %v", err)
	}
	expected := message + "\n\nCoordinator-Run-ID: " + request.RunID
	if got := strings.TrimSpace(runGit(t, repoRoot, "log", "-1", "--format=%B")); got != expected {
		t.Fatalf("committed message mismatch:\n got: %q\nwant: %q", got, expected)
	}
}

func TestDeveloperFinalizerRejectsForgedTrailerBeforeCommit(t *testing.T) {
	origin, targetSHA, _ := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-forged", RunID: "run-forged", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-forged",
	}
	repoRoot := prepareDeveloperRepo(t, request)
	if err := os.WriteFile(filepath.Join(repoRoot, "fix.go"), []byte("package fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := workspace.NewDeveloperFinalizer().Finalize(context.Background(), workspace.DeveloperFinalizeRequest{
		WorkspaceRoot: root, TaskID: request.TaskID, RunID: request.RunID,
		CommitMessage: "fix: reject invalid repository state\n\nCoordinator-Run-ID: run-elsewhere",
		TaskType:      request.TaskType, RepositoryURL: request.RepositoryURL, TargetBranch: request.TargetBranch,
		DeliveryBranch: request.DeliveryBranch, ExpectedInitialHeadSHA: targetSHA,
	})
	if err == nil || !strings.Contains(err.Error(), "trailer") {
		t.Fatalf("forged trailer must be rejected, got %v", err)
	}
	if got := strings.TrimSpace(runGit(t, repoRoot, "rev-list", "--count", targetSHA+"..HEAD")); got != "0" {
		t.Fatalf("commit was created before message validation: %s commits", got)
	}
}

func TestDeveloperFinalizerRecoversPendingCommitAfterPushFailure(t *testing.T) {
	origin, targetSHA, featureHead := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-recover", RunID: "run-recover", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-recover",
	}
	repoRoot := prepareDeveloperRepo(t, request)
	if err := os.WriteFile(filepath.Join(repoRoot, "fix.go"), []byte("package fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finalizeRequest := workspace.DeveloperFinalizeRequest{
		WorkspaceRoot: root, TaskID: request.TaskID, RunID: request.RunID,
		CommitMessage: "fix: recover pending delivery",
		TaskType:      request.TaskType, RepositoryURL: request.RepositoryURL, TargetBranch: request.TargetBranch,
		DeliveryBranch: request.DeliveryBranch, ExpectedInitialHeadSHA: targetSHA,
	}
	// 第一轮:commit 落地后远端漂移被拒,进入"已提交未推送"状态。
	runGit(t, origin, "update-ref", "refs/heads/agent/task-recover", featureHead)
	if _, err := workspace.NewDeveloperFinalizer().Finalize(context.Background(), finalizeRequest); !errors.Is(err, workspace.ErrDeveloperDeliveryConflict) {
		t.Fatalf("expected remote drift conflict after commit: %v", err)
	}
	// 第二轮:远端恢复初头,同 payload 重试走恢复路径,推送既有受信提交。
	runGit(t, origin, "update-ref", "refs/heads/agent/task-recover", targetSHA)
	delivery, err := workspace.NewDeveloperFinalizer().Finalize(context.Background(), finalizeRequest)
	if err != nil {
		t.Fatalf("retry after push failure must recover the pending commit: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, origin, "rev-parse", "refs/heads/agent/task-recover")); got != delivery.CommitSHA {
		t.Fatalf("pending commit was not pushed: remote=%s delivery=%s", got, delivery.CommitSHA)
	}
}

func TestDeveloperFinalizerRejectsAgentAuthoredCommitWithForeignMessage(t *testing.T) {
	origin, targetSHA, _ := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-foreign", RunID: "run-foreign", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-foreign",
	}
	repoRoot := prepareDeveloperRepo(t, request)
	if err := os.WriteFile(filepath.Join(repoRoot, "fix.go"), []byte("package fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Agent 自提交一条不同消息的提交:尾注可自由伪造,不构成所有权证明。
	forgeMessageFile(t, repoRoot, "feat: agent self committed\n\nCoordinator-Run-ID: "+request.RunID+"\n")
	runGit(t, repoRoot, "add", "-A")
	runGit(t, repoRoot, "commit", "-F", filepath.Join(repoRoot, "forged-message.txt"))
	_, err := workspace.NewDeveloperFinalizer().Finalize(context.Background(), workspace.DeveloperFinalizeRequest{
		WorkspaceRoot: root, TaskID: request.TaskID, RunID: request.RunID,
		CommitMessage: "fix: reject invalid repository state",
		TaskType:      request.TaskType, RepositoryURL: request.RepositoryURL, TargetBranch: request.TargetBranch,
		DeliveryBranch: request.DeliveryBranch, ExpectedInitialHeadSHA: targetSHA,
	})
	if !errors.Is(err, workspace.ErrWorkspaceConflict) {
		t.Fatalf("agent-authored commit with a foreign message must be rejected: %v", err)
	}
	command := exec.Command("git", "rev-parse", "--verify", "--quiet", "refs/heads/agent/task-foreign")
	command.Dir = origin
	if err := command.Run(); err == nil {
		t.Fatal("rejected agent commit was pushed to the remote")
	}
}

func TestDeveloperFinalizerRejectsPrematureCommitWithoutCoordinatorCheckpoint(t *testing.T) {
	origin, targetSHA, _ := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-premature", RunID: "run-premature", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-premature",
	}
	repoRoot := prepareDeveloperRepo(t, request)
	if err := os.WriteFile(filepath.Join(repoRoot, "fix.go"), []byte("package fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Agent 提前提交,消息与本次提交将写出的字节完全一致:尾注与身份都可由
	// Agent 伪造,提交归属门禁不凭"长得像"认领,必须由检查点背书。
	messageFile := forgeMessageFile(t, repoRoot, "fix: reject invalid repository state\n\nCoordinator-Run-ID: "+request.RunID+"\n")
	runGit(t, repoRoot, "add", "-A")
	runGit(t, repoRoot, "commit", "-F", messageFile)
	_, err := workspace.NewDeveloperFinalizer().Finalize(context.Background(), workspace.DeveloperFinalizeRequest{
		WorkspaceRoot: root, TaskID: request.TaskID, RunID: request.RunID,
		CommitMessage: "fix: reject invalid repository state",
		TaskType:      request.TaskType, RepositoryURL: request.RepositoryURL, TargetBranch: request.TargetBranch,
		DeliveryBranch: request.DeliveryBranch, ExpectedInitialHeadSHA: targetSHA,
	})
	if !errors.Is(err, workspace.ErrWorkspaceConflict) {
		t.Fatalf("premature commit without a coordinator checkpoint must be rejected: %v", err)
	}
	command := exec.Command("git", "rev-parse", "--verify", "--quiet", "refs/heads/agent/task-premature")
	command.Dir = origin
	if err := command.Run(); err == nil {
		t.Fatal("premature commit was pushed to the remote")
	}
}

func forgeMessageFile(t *testing.T, repoRoot, message string) string {
	t.Helper()
	path := filepath.Join(repoRoot, "forged-message.txt")
	if err := os.WriteFile(path, []byte(message), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
