package workspace_test

import (
	"context"
	"os"
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

func TestDeveloperPreparerWritesRepositoryLocalIdentity(t *testing.T) {
	origin, _, _ := createReviewOrigin(t)
	root := t.TempDir()
	repoRoot := prepareDeveloperRepo(t, workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-identity", RunID: "run-identity", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-identity",
		GitAuthorName: "Release Bot", GitAuthorEmail: "bot@example.com",
	})
	if got := strings.TrimSpace(runGit(t, repoRoot, "config", "--local", "user.name")); got != "Release Bot" {
		t.Fatalf("repository local user.name = %q", got)
	}
	if got := strings.TrimSpace(runGit(t, repoRoot, "config", "--local", "user.email")); got != "bot@example.com" {
		t.Fatalf("repository local user.email = %q", got)
	}
}

func TestDeveloperPreparerFallsBackToBuiltInIdentity(t *testing.T) {
	origin, _, _ := createReviewOrigin(t)
	root := t.TempDir()
	repoRoot := prepareDeveloperRepo(t, workspace.DeveloperRequest{
		WorkspaceRoot: root, TaskID: "task-fallback", RunID: "run-fallback", TaskType: "develop_from_issue",
		Project: "group/project", RepositoryURL: origin, TargetBranch: "master", DeliveryBranch: "agent/task-fallback",
	})
	if got := strings.TrimSpace(runGit(t, repoRoot, "config", "--local", "user.name")); got != workspace.FallbackGitAuthorName {
		t.Fatalf("fallback user.name = %q, want %q", got, workspace.FallbackGitAuthorName)
	}
	if got := strings.TrimSpace(runGit(t, repoRoot, "config", "--local", "user.email")); got != workspace.FallbackGitAuthorEmail {
		t.Fatalf("fallback user.email = %q, want %q", got, workspace.FallbackGitAuthorEmail)
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
