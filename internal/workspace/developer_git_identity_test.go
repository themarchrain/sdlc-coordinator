package workspace_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
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

func TestDeveloperDeliveryCommitsAIAuthoredMessageWithTrustedIdentity(t *testing.T) {
	origin, targetSHA, _ := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-ai-message", RunID: "run-ai-message", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-ai-message",
	}
	repoRoot := prepareDeveloperRepo(t, request)
	if err := os.WriteFile(filepath.Join(repoRoot, "fix.go"), []byte("package fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	message := "fix: reject invalid repository state\n\n- Validate the state before delivery\n- Cover the failure path with a regression test"
	commitRequest := commitRequestFor(request, message, targetSHA)
	commitRequest.GitAuthorName = "Release Bot"
	commitRequest.GitAuthorEmail = "bot@example.com"
	finalizer := workspace.NewDeveloperFinalizer()
	commit, err := finalizer.CommitDelivery(context.Background(), commitRequest)
	if err != nil {
		t.Fatalf("commit delivery: %v", err)
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
	if _, err := finalizer.PushDelivery(context.Background(), pushRequestFor(request, commit.CommitSHA, targetSHA)); err != nil {
		t.Fatalf("push delivery: %v", err)
	}
}

func TestDeveloperDeliveryIgnoresAmbientAuthorEnvironment(t *testing.T) {
	origin, targetSHA, _ := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-ambient", RunID: "run-ambient", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-ambient",
	}
	repoRoot := prepareDeveloperRepo(t, request)
	if err := os.WriteFile(filepath.Join(repoRoot, "fix.go"), []byte("package fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_AUTHOR_NAME", "ambient-name")
	t.Setenv("GIT_AUTHOR_EMAIL", "ambient@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "ambient-name")
	t.Setenv("GIT_COMMITTER_EMAIL", "ambient@example.com")
	commitRequest := commitRequestFor(request, "fix: reject invalid repository state", targetSHA)
	commitRequest.GitAuthorName = "Release Bot"
	commitRequest.GitAuthorEmail = "bot@example.com"
	if _, err := workspace.NewDeveloperFinalizer().CommitDelivery(context.Background(), commitRequest); err != nil {
		t.Fatalf("commit delivery: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, repoRoot, "show", "-s", "--format=%an %ae")); got != "Release Bot bot@example.com" {
		t.Fatalf("ambient environment leaked into commit identity: %q", got)
	}
}

func TestDeveloperDeliveryOverridesRunnerTamperedIdentity(t *testing.T) {
	origin, targetSHA, _ := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-tamper", RunID: "run-tamper", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-tamper",
	}
	repoRoot := prepareDeveloperRepo(t, request)
	// 模拟共享工作区上的 Runner 改写仓库本地 config:git status 不显示这种改动。
	runGit(t, repoRoot, "config", "--local", "user.name", "runner-tampered")
	runGit(t, repoRoot, "config", "--local", "user.email", "runner@example.com")
	if err := os.WriteFile(filepath.Join(repoRoot, "fix.go"), []byte("package fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitRequest := commitRequestFor(request, "fix: reject invalid repository state", targetSHA)
	commitRequest.GitAuthorName = "Release Bot"
	commitRequest.GitAuthorEmail = "bot@example.com"
	if _, err := workspace.NewDeveloperFinalizer().CommitDelivery(context.Background(), commitRequest); err != nil {
		t.Fatalf("commit delivery: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, repoRoot, "show", "-s", "--format=%an %ae")); got != "Release Bot bot@example.com" {
		t.Fatalf("runner-tampered config leaked into commit identity: %q", got)
	}
}

func TestDeveloperDeliveryPreservesConsecutiveBlankLines(t *testing.T) {
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
	commit, err := workspace.NewDeveloperFinalizer().CommitDelivery(context.Background(), commitRequestFor(request, message, targetSHA))
	if err != nil {
		t.Fatalf("consecutive blank lines must survive git cleanup: %v", err)
	}
	expected := message + "\n\nCoordinator-Run-ID: " + request.RunID
	if got := strings.TrimSpace(runGit(t, repoRoot, "log", "-1", "--format=%B")); got != expected {
		t.Fatalf("committed message mismatch:\n got: %q\nwant: %q", got, expected)
	}
	if _, err := workspace.NewDeveloperFinalizer().PushDelivery(context.Background(), pushRequestFor(request, commit.CommitSHA, targetSHA)); err != nil {
		t.Fatalf("push delivery: %v", err)
	}
}

func TestDeveloperDeliveryRecoversPendingCommitAfterPushFailure(t *testing.T) {
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
	finalizer := workspace.NewDeveloperFinalizer()
	// 第一轮:commit 落地后远端漂移被拒,进入"已提交未推送"状态;检查点
	// 由编排层持有,这里以测试变量模拟。
	runGit(t, origin, "update-ref", "refs/heads/agent/task-recover", featureHead)
	commit, err := finalizer.CommitDelivery(context.Background(), commitRequestFor(request, "fix: recover pending delivery", targetSHA))
	if err != nil {
		t.Fatalf("commit delivery: %v", err)
	}
	if _, err := finalizer.PushDelivery(context.Background(), pushRequestFor(request, commit.CommitSHA, targetSHA)); !errors.Is(err, workspace.ErrDeveloperDeliveryConflict) {
		t.Fatalf("expected remote drift conflict after commit: %v", err)
	}
	// 第二轮:远端恢复基线,同检查点 SHA 重试推送。
	runGit(t, origin, "update-ref", "refs/heads/agent/task-recover", targetSHA)
	delivery, err := finalizer.PushDelivery(context.Background(), pushRequestFor(request, commit.CommitSHA, targetSHA))
	if err != nil {
		t.Fatalf("retry after push failure must recover the pending commit: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, origin, "rev-parse", "refs/heads/agent/task-recover")); got != delivery.CommitSHA {
		t.Fatalf("pending commit was not pushed: remote=%s delivery=%s", got, delivery.CommitSHA)
	}
}

func TestDeveloperDeliveryRejectsAgentAuthoredCommitWithForeignMessage(t *testing.T) {
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
	// Agent 自提交一条不同消息的提交:尾注可自由伪造,不构成所有权证明;
	// 没有检查点背书的本地提交一律拒绝,无论消息长什么样。
	forgeAgentCommit(t, repoRoot, "feat: agent self committed\n\nCoordinator-Run-ID: "+request.RunID+"\n")
	_, err := workspace.NewDeveloperFinalizer().CommitDelivery(context.Background(), commitRequestFor(request, "fix: reject invalid repository state", targetSHA))
	if !errors.Is(err, workspace.ErrWorkspaceConflict) {
		t.Fatalf("agent-authored commit with a foreign message must be rejected: %v", err)
	}
	assertRemoteBranchAbsent(t, origin, "refs/heads/agent/task-foreign")
}

func TestDeveloperDeliveryRejectsPrematureCommitWithoutCoordinatorCheckpoint(t *testing.T) {
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
	// 提前提交且消息与本次提交将写出的字节完全一致:逐字节同质也不认领,
	// 归属只能由检查点背书。
	forgeAgentCommit(t, repoRoot, "fix: reject invalid repository state\n\nCoordinator-Run-ID: "+request.RunID+"\n")
	_, err := workspace.NewDeveloperFinalizer().CommitDelivery(context.Background(), commitRequestFor(request, "fix: reject invalid repository state", targetSHA))
	if !errors.Is(err, workspace.ErrWorkspaceConflict) {
		t.Fatalf("premature commit without a coordinator checkpoint must be rejected: %v", err)
	}
	assertRemoteBranchAbsent(t, origin, "refs/heads/agent/task-premature")
}

func TestDeveloperDeliveryRejectsSecondPrematureCommit(t *testing.T) {
	origin, targetSHA, _ := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-second", RunID: "run-second", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-second",
	}
	repoRoot := prepareDeveloperRepo(t, request)
	if err := os.WriteFile(filepath.Join(repoRoot, "fix.go"), []byte("package fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	forgeAgentCommit(t, repoRoot, "feat: agent commit one\n")
	forgeAgentCommit(t, repoRoot, "feat: agent commit two\n")
	if _, err := workspace.NewDeveloperFinalizer().CommitDelivery(context.Background(), commitRequestFor(request, "fix: reject invalid repository state", targetSHA)); !errors.Is(err, workspace.ErrWorkspaceConflict) {
		t.Fatalf("a run with two premature commits must be rejected: %v", err)
	}
	assertRemoteBranchAbsent(t, origin, "refs/heads/agent/task-second")
}

func TestDeveloperDeliveryRejectsDirtyWorktreeOnUnknownHead(t *testing.T) {
	origin, targetSHA, _ := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-dirty", RunID: "run-dirty", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-dirty",
	}
	repoRoot := prepareDeveloperRepo(t, request)
	forgeAgentCommit(t, repoRoot, "feat: agent self committed\n")
	if err := os.WriteFile(filepath.Join(repoRoot, "extra.go"), []byte("package extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 脏工作区+未知 HEAD:必须在 staging 之前拒绝,不得把未知链提交进来。
	if _, err := workspace.NewDeveloperFinalizer().CommitDelivery(context.Background(), commitRequestFor(request, "fix: reject invalid repository state", targetSHA)); !errors.Is(err, workspace.ErrWorkspaceConflict) {
		t.Fatalf("dirty worktree on an unknown head must be rejected: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, repoRoot, "status", "--porcelain")); got == "" {
		t.Fatal("rejection must not stage the worktree")
	}
	if count := strings.TrimSpace(runGit(t, repoRoot, "rev-list", "--count", targetSHA+"..HEAD")); count != "1" {
		t.Fatalf("rejection must not create commits: %s", count)
	}
	assertRemoteBranchAbsent(t, origin, "refs/heads/agent/task-dirty")
}

func TestDeveloperDeliveryPushRejectsLocalHeadDivergedFromCheckpoint(t *testing.T) {
	origin, targetSHA, _ := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-diverge", RunID: "run-diverge", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-diverge",
	}
	repoRoot := prepareDeveloperRepo(t, request)
	if err := os.WriteFile(filepath.Join(repoRoot, "fix.go"), []byte("package fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finalizer := workspace.NewDeveloperFinalizer()
	commit, err := finalizer.CommitDelivery(context.Background(), commitRequestFor(request, "fix: reject invalid repository state", targetSHA))
	if err != nil {
		t.Fatalf("commit delivery: %v", err)
	}
	// Agent 在受信交付提交之后又追加本地提交:推送只认检查点 SHA,本地
	// HEAD 偏离即拒绝,追加提交绝不随行。
	forgeAgentCommit(t, repoRoot, "feat: agent commit after delivery\n")
	if _, err := finalizer.PushDelivery(context.Background(), pushRequestFor(request, commit.CommitSHA, targetSHA)); !errors.Is(err, workspace.ErrWorkspaceConflict) {
		t.Fatalf("push with a diverged local head must be rejected: %v", err)
	}
	assertRemoteBranchAbsent(t, origin, "refs/heads/agent/task-diverge")
}

func TestDeveloperDeliveryRejectsPrematureReworkCommit(t *testing.T) {
	origin, _, featureHead := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-rework-premature", RunID: "run-rework-premature", TaskType: model.TaskTypeReworkMergeRequest,
		Project: "group/project", RepositoryURL: origin, SourceBranch: "feature/review", TargetBranch: "master",
		DeliveryBranch: "feature/review", ExpectedHeadSHA: featureHead,
	}
	repoRoot := prepareDeveloperRepo(t, request)
	if err := os.WriteFile(filepath.Join(repoRoot, "rework.txt"), []byte("reworked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// rework 任务的提前提交同样不认领:检查点缺失即拒绝,原 source branch 不动。
	forgeAgentCommit(t, repoRoot, "feat: agent self committed rework\n")
	if _, err := workspace.NewDeveloperFinalizer().CommitDelivery(context.Background(), commitRequestFor(request, "fix: apply review feedback", featureHead)); !errors.Is(err, workspace.ErrWorkspaceConflict) {
		t.Fatalf("premature rework commit must be rejected: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, origin, "rev-parse", "refs/heads/feature/review")); got != featureHead {
		t.Fatalf("premature rework commit reached the source branch: %s", got)
	}
}

func TestDeveloperDeliveryRejectsForgedTrailerBeforeCommit(t *testing.T) {
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
	_, err := workspace.NewDeveloperFinalizer().CommitDelivery(context.Background(), commitRequestFor(request, "fix: reject invalid repository state\n\nCoordinator-Run-ID: run-elsewhere", targetSHA))
	if err == nil || !strings.Contains(err.Error(), "trailer") {
		t.Fatalf("forged trailer must be rejected, got %v", err)
	}
	if got := strings.TrimSpace(runGit(t, repoRoot, "rev-list", "--count", targetSHA+"..HEAD")); got != "0" {
		t.Fatalf("commit was created before message validation: %s commits", got)
	}
}

// forgeAgentCommit 以 Agent 视角在共享仓库里创建本地提交(身份来自测试环境,
// 与 Coordinator 无关)。
func forgeAgentCommit(t *testing.T, repoRoot, message string) string {
	t.Helper()
	path := filepath.Join(repoRoot, "forged-message.txt")
	if err := os.WriteFile(path, []byte(message), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoRoot, "add", "-A")
	runGit(t, repoRoot, "-c", "user.name=agent", "-c", "user.email=agent@example.com", "commit", "-F", path)
	return path
}

func assertRemoteBranchAbsent(t *testing.T, origin, ref string) {
	t.Helper()
	command := exec.Command("git", "rev-parse", "--verify", "--quiet", ref)
	command.Dir = origin
	if err := command.Run(); err == nil {
		t.Fatalf("remote ref %s must not exist after rejection", ref)
	}
}
