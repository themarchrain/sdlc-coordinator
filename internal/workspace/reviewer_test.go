package workspace_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sdlc-coordinator/internal/workspace"
)

func TestReviewerPreparerPreparesExactFullReview(t *testing.T) {
	t.Parallel()

	origin, targetSHA, featureHead := createReviewOrigin(t)
	root := t.TempDir()
	preparer := workspace.NewReviewerPreparer()

	manifest, err := preparer.Prepare(context.Background(), workspace.ReviewerRequest{
		WorkspaceRoot:       root,
		TaskID:              "task-review-1",
		RunID:               "run-review-1",
		Project:             "group/project",
		RepositoryURL:       origin,
		SourceBranch:        "feature/review",
		TargetBranch:        "master",
		HeadSHA:             featureHead,
		RequestedReviewMode: "full",
	})
	if err != nil {
		t.Fatalf("prepare reviewer workspace: %v", err)
	}
	if manifest.Status != workspace.ManifestStatusReady {
		t.Fatalf("expected ready manifest, got %#v", manifest)
	}
	if manifest.Repository.TargetSHA != targetSHA || manifest.Repository.HeadSHA != featureHead {
		t.Fatalf("unexpected fixed SHAs: %#v", manifest.Repository)
	}
	if manifest.Repository.EffectiveReviewMode != workspace.ReviewModeFull {
		t.Fatalf("expected full review, got %#v", manifest.Repository)
	}
	wantRange := targetSHA + "..." + featureHead
	if manifest.Repository.DiffRange != wantRange {
		t.Fatalf("expected diff range %q, got %q", wantRange, manifest.Repository.DiffRange)
	}

	runRoot := filepath.Join(root, "runs", "run-review-1")
	repoRoot := filepath.Join(runRoot, "repo")
	if got := strings.TrimSpace(runGit(t, repoRoot, "rev-parse", "HEAD")); got != featureHead {
		t.Fatalf("expected detached HEAD %s, got %s", featureHead, got)
	}
	cmd := exec.Command("git", "symbolic-ref", "-q", "HEAD")
	cmd.Dir = repoRoot
	if err := cmd.Run(); err == nil {
		t.Fatal("expected detached HEAD")
	}
	if got := strings.TrimSpace(runGit(t, repoRoot, "remote", "get-url", "--push", "origin")); got != workspace.DisabledReviewerPushURL {
		t.Fatalf("expected disabled push URL, got %q", got)
	}

	raw, err := os.ReadFile(filepath.Join(runRoot, workspace.ManifestFileName))
	if err != nil {
		t.Fatalf("read persisted manifest: %v", err)
	}
	var persisted workspace.Manifest
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("decode persisted manifest: %v", err)
	}
	if persisted.Repository.DiffRange != manifest.Repository.DiffRange || persisted.Status != manifest.Status {
		t.Fatalf("persisted manifest differs: returned=%#v persisted=%#v", manifest, persisted)
	}
}

func TestReviewerPreparerPreservesValidIncrementalRange(t *testing.T) {
	t.Parallel()

	origin, baseSHA, featureHead := createReviewOrigin(t)
	manifest, err := workspace.NewReviewerPreparer().Prepare(context.Background(), workspace.ReviewerRequest{
		WorkspaceRoot:       t.TempDir(),
		TaskID:              "task-review-incremental",
		RunID:               "run-review-incremental",
		Project:             "group/project",
		RepositoryURL:       origin,
		SourceBranch:        "feature/review",
		TargetBranch:        "master",
		HeadSHA:             featureHead,
		BaseSHA:             baseSHA,
		RequestedReviewMode: "incremental",
	})
	if err != nil {
		t.Fatalf("prepare incremental reviewer workspace: %v", err)
	}
	if manifest.Repository.EffectiveReviewMode != workspace.ReviewModeIncremental {
		t.Fatalf("expected incremental mode, got %#v", manifest.Repository)
	}
	if want := baseSHA + ".." + featureHead; manifest.Repository.DiffRange != want {
		t.Fatalf("expected diff range %q, got %q", want, manifest.Repository.DiffRange)
	}
	if manifest.Repository.BaseSHA != baseSHA || manifest.Repository.FallbackReason != "" {
		t.Fatalf("unexpected incremental metadata: %#v", manifest.Repository)
	}
}

func TestReviewerPreparerFallsBackToFullWhenBaseIsNotAncestor(t *testing.T) {
	t.Parallel()

	origin, _, featureHead := createReviewOrigin(t)
	nonAncestorBase := advanceTargetBranch(t, origin)
	manifest, err := workspace.NewReviewerPreparer().Prepare(context.Background(), workspace.ReviewerRequest{
		WorkspaceRoot:       t.TempDir(),
		TaskID:              "task-review-fallback",
		RunID:               "run-review-fallback",
		Project:             "group/project",
		RepositoryURL:       origin,
		SourceBranch:        "feature/review",
		TargetBranch:        "master",
		HeadSHA:             featureHead,
		BaseSHA:             nonAncestorBase,
		RequestedReviewMode: "incremental",
	})
	if err != nil {
		t.Fatalf("prepare fallback reviewer workspace: %v", err)
	}
	if manifest.Repository.EffectiveReviewMode != workspace.ReviewModeFull {
		t.Fatalf("expected full fallback, got %#v", manifest.Repository)
	}
	if manifest.Repository.FallbackReason != workspace.FallbackBaseNotAncestor {
		t.Fatalf("expected non-ancestor fallback reason, got %#v", manifest.Repository)
	}
	if want := nonAncestorBase + "..." + featureHead; manifest.Repository.DiffRange != want {
		t.Fatalf("expected full diff range %q, got %q", want, manifest.Repository.DiffRange)
	}
}

func TestReviewerPreparerRejectsUnsafeRunIDBeforeFilesystemMutation(t *testing.T) {
	t.Parallel()

	origin, _, featureHead := createReviewOrigin(t)
	root := t.TempDir()
	_, err := workspace.NewReviewerPreparer().Prepare(context.Background(), workspace.ReviewerRequest{
		WorkspaceRoot:       root,
		TaskID:              "task-unsafe-run",
		RunID:               "../escape",
		Project:             "group/project",
		RepositoryURL:       origin,
		SourceBranch:        "feature/review",
		TargetBranch:        "master",
		HeadSHA:             featureHead,
		RequestedReviewMode: "full",
	})
	if err == nil || !strings.Contains(err.Error(), "safe path component") {
		t.Fatalf("expected safe path validation error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "escape")); !os.IsNotExist(statErr) {
		t.Fatalf("unsafe run id mutated filesystem: %v", statErr)
	}
}

func TestReviewerPreparerRejectsExistingRunDirectoryWithoutDeletingIt(t *testing.T) {
	t.Parallel()

	origin, _, featureHead := createReviewOrigin(t)
	root := t.TempDir()
	runRoot := filepath.Join(root, "runs", "run-existing")
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		t.Fatalf("create existing run directory: %v", err)
	}
	sentinel := filepath.Join(runRoot, "operator-note.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	_, err := workspace.NewReviewerPreparer().Prepare(context.Background(), workspace.ReviewerRequest{
		WorkspaceRoot:       root,
		TaskID:              "task-existing",
		RunID:               "run-existing",
		Project:             "group/project",
		RepositoryURL:       origin,
		SourceBranch:        "feature/review",
		TargetBranch:        "master",
		HeadSHA:             featureHead,
		RequestedReviewMode: "full",
	})
	if !errors.Is(err, workspace.ErrWorkspaceConflict) {
		t.Fatalf("expected workspace conflict, got %v", err)
	}
	if raw, readErr := os.ReadFile(sentinel); readErr != nil || string(raw) != "keep" {
		t.Fatalf("existing workspace was modified: content=%q err=%v", raw, readErr)
	}
}

func TestReviewerPreparerRebuildsRecognizableIncompleteWorkspace(t *testing.T) {
	t.Parallel()

	origin, _, featureHead := createReviewOrigin(t)
	root := t.TempDir()
	runRoot := filepath.Join(root, "runs", "run-incomplete")
	if err := os.MkdirAll(filepath.Join(runRoot, "repo", ".git"), 0o755); err != nil {
		t.Fatalf("create incomplete reviewer workspace: %v", err)
	}

	manifest, err := workspace.NewReviewerPreparer().Prepare(context.Background(), workspace.ReviewerRequest{
		WorkspaceRoot:       root,
		TaskID:              "task-incomplete",
		RunID:               "run-incomplete",
		Project:             "group/project",
		RepositoryURL:       origin,
		SourceBranch:        "feature/review",
		TargetBranch:        "master",
		HeadSHA:             featureHead,
		RequestedReviewMode: workspace.ReviewModeFull,
	})
	if err != nil {
		t.Fatalf("rebuild incomplete workspace: %v", err)
	}
	if manifest.Status != workspace.ManifestStatusReady {
		t.Fatalf("expected rebuilt ready manifest, got %#v", manifest)
	}
	if got := strings.TrimSpace(runGit(t, filepath.Join(runRoot, "repo"), "rev-parse", "HEAD")); got != featureHead {
		t.Fatalf("expected rebuilt HEAD %s, got %s", featureHead, got)
	}
}

func TestReviewerPreparerReusesMatchingReadyWorkspace(t *testing.T) {
	t.Parallel()

	origin, _, featureHead := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.ReviewerRequest{
		WorkspaceRoot:       root,
		TaskID:              "task-reuse",
		RunID:               "run-reuse",
		Project:             "group/project",
		RepositoryURL:       origin,
		SourceBranch:        "feature/review",
		TargetBranch:        "master",
		HeadSHA:             featureHead,
		RequestedReviewMode: workspace.ReviewModeFull,
	}
	preparer := workspace.NewReviewerPreparer()
	first, err := preparer.Prepare(context.Background(), request)
	if err != nil {
		t.Fatalf("prepare initial workspace: %v", err)
	}
	second, err := preparer.Prepare(context.Background(), request)
	if err != nil {
		t.Fatalf("reuse matching ready workspace: %v", err)
	}
	if second.PreparedAt != first.PreparedAt || second.Repository.DiffRange != first.Repository.DiffRange {
		t.Fatalf("ready workspace was rebuilt instead of reused: first=%#v second=%#v", first, second)
	}
}

func TestReviewerPreparerConcurrentSameRunConvergesOnOneReadyWorkspace(t *testing.T) {
	t.Parallel()

	origin, _, featureHead := createReviewOrigin(t)
	root := t.TempDir()
	request := workspace.ReviewerRequest{
		WorkspaceRoot:       root,
		TaskID:              "task-concurrent",
		RunID:               "run-concurrent",
		Project:             "group/project",
		RepositoryURL:       origin,
		SourceBranch:        "feature/review",
		TargetBranch:        "master",
		HeadSHA:             featureHead,
		RequestedReviewMode: workspace.ReviewModeFull,
	}

	type result struct {
		manifest *workspace.Manifest
		err      error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			manifest, err := workspace.NewReviewerPreparer().Prepare(context.Background(), request)
			results <- result{manifest: manifest, err: err}
		}()
	}
	for range 2 {
		got := <-results
		if got.err != nil || got.manifest == nil || got.manifest.Status != workspace.ManifestStatusReady {
			t.Fatalf("concurrent prepare did not converge: manifest=%#v err=%v", got.manifest, got.err)
		}
	}

	runsRoot := filepath.Join(root, "runs")
	entries, err := os.ReadDir(runsRoot)
	if err != nil {
		t.Fatalf("read runs root: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != request.RunID {
		t.Fatalf("unexpected concurrent workspace entries: %#v", entries)
	}
}

func TestReviewerPreparerRejectsMissingHeadWithoutReadyManifest(t *testing.T) {
	t.Parallel()

	origin, _, _ := createReviewOrigin(t)
	root := t.TempDir()
	_, err := workspace.NewReviewerPreparer().Prepare(context.Background(), workspace.ReviewerRequest{
		WorkspaceRoot:       root,
		TaskID:              "task-missing-head",
		RunID:               "run-missing-head",
		Project:             "group/project",
		RepositoryURL:       origin,
		SourceBranch:        "feature/review",
		TargetBranch:        "master",
		HeadSHA:             strings.Repeat("f", 40),
		RequestedReviewMode: "full",
	})
	if !errors.Is(err, workspace.ErrHeadUnavailable) {
		t.Fatalf("expected unavailable head error, got %v", err)
	}
	manifestPath := filepath.Join(root, "runs", "run-missing-head", workspace.ManifestFileName)
	if _, statErr := os.Stat(manifestPath); !os.IsNotExist(statErr) {
		t.Fatalf("missing head published a manifest: %v", statErr)
	}
}

func TestReviewerPreparerRejectsHeadOutsideSourceHistory(t *testing.T) {
	t.Parallel()

	origin, _, _ := createReviewOrigin(t)
	targetOnlyHead := advanceTargetBranch(t, origin)
	_, err := workspace.NewReviewerPreparer().Prepare(context.Background(), workspace.ReviewerRequest{
		WorkspaceRoot:       t.TempDir(),
		TaskID:              "task-unrelated-head",
		RunID:               "run-unrelated-head",
		Project:             "group/project",
		RepositoryURL:       origin,
		SourceBranch:        "feature/review",
		TargetBranch:        "master",
		HeadSHA:             targetOnlyHead,
		RequestedReviewMode: "full",
	})
	if !errors.Is(err, workspace.ErrHeadUnavailable) {
		t.Fatalf("expected head/source relationship error, got %v", err)
	}
}

func TestReviewerPreparerRejectsRepositoryURLWithCredentialsBeforeFilesystemMutation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	_, err := workspace.NewReviewerPreparer().Prepare(context.Background(), workspace.ReviewerRequest{
		WorkspaceRoot:       root,
		TaskID:              "task-url-credentials",
		RunID:               "run-url-credentials",
		Project:             "group/project",
		RepositoryURL:       "https://oauth2:secret@gitlab.example/group/project.git",
		SourceBranch:        "feature/review",
		TargetBranch:        "master",
		HeadSHA:             strings.Repeat("a", 40),
		RequestedReviewMode: "full",
	})
	if err == nil || !strings.Contains(err.Error(), "must not contain credentials") {
		t.Fatalf("expected credential-bearing URL rejection, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "runs")); !os.IsNotExist(statErr) {
		t.Fatalf("credential-bearing URL mutated filesystem: %v", statErr)
	}
}

func TestReviewerPreparerDoesNotPersistCredential(t *testing.T) {
	t.Parallel()

	const token = "workspace-secret-token"
	origin, _, featureHead := createReviewOrigin(t)
	root := t.TempDir()
	_, err := workspace.NewReviewerPreparer().Prepare(context.Background(), workspace.ReviewerRequest{
		WorkspaceRoot:       root,
		TaskID:              "task-credential",
		RunID:               "run-credential",
		Project:             "group/project",
		RepositoryURL:       origin,
		SourceBranch:        "feature/review",
		TargetBranch:        "master",
		HeadSHA:             featureHead,
		RequestedReviewMode: "full",
		Credential:          &workspace.GitCredential{Username: "oauth2", Token: token},
	})
	if err != nil {
		t.Fatalf("prepare credentialed workspace: %v", err)
	}
	for _, path := range []string{
		filepath.Join(root, "runs", "run-credential", workspace.ManifestFileName),
		filepath.Join(root, "runs", "run-credential", "repo", ".git", "config"),
	} {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		if strings.Contains(string(raw), token) {
			t.Fatalf("credential persisted in %s", path)
		}
	}
}

func TestReviewerPreparerFallsBackToFullWhenBaseIsUnavailable(t *testing.T) {
	t.Parallel()

	origin, targetSHA, featureHead := createReviewOrigin(t)
	manifest, err := workspace.NewReviewerPreparer().Prepare(context.Background(), workspace.ReviewerRequest{
		WorkspaceRoot:       t.TempDir(),
		TaskID:              "task-base-unavailable",
		RunID:               "run-base-unavailable",
		Project:             "group/project",
		RepositoryURL:       origin,
		SourceBranch:        "feature/review",
		TargetBranch:        "master",
		HeadSHA:             featureHead,
		BaseSHA:             strings.Repeat("e", 40),
		RequestedReviewMode: "incremental",
	})
	if err != nil {
		t.Fatalf("prepare reviewer workspace: %v", err)
	}
	if manifest.Repository.EffectiveReviewMode != workspace.ReviewModeFull || manifest.Repository.FallbackReason != "base_unavailable" {
		t.Fatalf("expected unavailable-base full fallback, got %#v", manifest.Repository)
	}
	if want := targetSHA + "..." + featureHead; manifest.Repository.DiffRange != want {
		t.Fatalf("expected fallback range %q, got %q", want, manifest.Repository.DiffRange)
	}
}

func createReviewOrigin(t *testing.T) (origin, targetSHA, featureHead string) {
	t.Helper()

	root := t.TempDir()
	seed := filepath.Join(root, "seed")
	if err := os.MkdirAll(seed, 0o755); err != nil {
		t.Fatalf("create seed directory: %v", err)
	}
	runGit(t, seed, "init", "-b", "master")
	runGit(t, seed, "config", "user.name", "Workspace Test")
	runGit(t, seed, "config", "user.email", "workspace-test@example.com")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write base file: %v", err)
	}
	runGit(t, seed, "add", "README.md")
	runGit(t, seed, "commit", "-m", "base")
	targetSHA = strings.TrimSpace(runGit(t, seed, "rev-parse", "HEAD"))

	runGit(t, seed, "checkout", "-b", "feature/review")
	if err := os.WriteFile(filepath.Join(seed, "feature.go"), []byte("package feature\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	runGit(t, seed, "add", "feature.go")
	runGit(t, seed, "commit", "-m", "feature")
	featureHead = strings.TrimSpace(runGit(t, seed, "rev-parse", "HEAD"))

	origin = filepath.Join(root, "origin.git")
	runGit(t, root, "init", "--bare", origin)
	runGit(t, seed, "remote", "add", "origin", origin)
	runGit(t, seed, "push", "origin", "master", "feature/review")
	return origin, targetSHA, featureHead
}

func advanceTargetBranch(t *testing.T, origin string) string {
	t.Helper()

	root := t.TempDir()
	clone := filepath.Join(root, "target")
	runGit(t, root, "clone", "--branch", "master", origin, clone)
	runGit(t, clone, "config", "user.name", "Workspace Test")
	runGit(t, clone, "config", "user.email", "workspace-test@example.com")
	if err := os.WriteFile(filepath.Join(clone, "target.txt"), []byte("target moved\n"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}
	runGit(t, clone, "add", "target.txt")
	runGit(t, clone, "commit", "-m", "advance target")
	sha := strings.TrimSpace(runGit(t, clone, "rev-parse", "HEAD"))
	runGit(t, clone, "push", "origin", "master")
	return sha
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
