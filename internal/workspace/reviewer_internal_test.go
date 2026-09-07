package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReviewerPreparerDoesNotExposeRunDirectoryBeforeReady(t *testing.T) {
	t.Parallel()

	const (
		headSHA   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		targetSHA = "cccccccccccccccccccccccccccccccccccccccc"
	)
	root := t.TempDir()
	started := make(chan struct{})
	release := make(chan struct{})
	preparer := &ReviewerPreparer{
		git: &blockingGitRunner{started: started, release: release, target: targetSHA},
		now: time.Now,
	}

	type result struct {
		manifest *Manifest
		err      error
	}
	done := make(chan result, 1)
	go func() {
		manifest, err := preparer.Prepare(context.Background(), ReviewerRequest{
			WorkspaceRoot: root, TaskID: "task-staging", RunID: "run-staging",
			Project: "group/project", RepositoryURL: "https://gitlab.example/group/project.git",
			SourceBranch: "feature/review", TargetBranch: "master",
			HeadSHA: headSHA, RequestedReviewMode: ReviewModeFull,
		})
		done <- result{manifest: manifest, err: err}
	}()

	<-started
	runRoot := filepath.Join(root, "runs", "run-staging")
	if _, err := os.Stat(runRoot); !os.IsNotExist(err) {
		close(release)
		<-done
		t.Fatalf("formal run directory became visible before ready: %v", err)
	}
	close(release)
	got := <-done
	if got.err != nil || got.manifest == nil || got.manifest.Status != ManifestStatusReady {
		t.Fatalf("prepare staged workspace: manifest=%#v err=%v", got.manifest, got.err)
	}
	if _, err := os.Stat(filepath.Join(runRoot, ManifestFileName)); err != nil {
		t.Fatalf("ready workspace was not published: %v", err)
	}
}

func TestReviewerPreparerDoesNotPublishReadyManifestAfterContextCancellation(t *testing.T) {
	t.Parallel()

	const (
		headSHA   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		baseSHA   = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		targetSHA = "cccccccccccccccccccccccccccccccccccccccc"
	)
	tests := []struct {
		name       string
		cancelWhen func([]string) bool
		baseSHA    string
	}{
		{
			name: "head source ancestry check",
			cancelWhen: func(args []string) bool {
				return len(args) >= 4 && args[0] == "merge-base" && args[2] == headSHA
			},
		},
		{
			name: "incremental base lookup",
			cancelWhen: func(args []string) bool {
				return len(args) >= 3 && args[0] == "cat-file" && strings.HasPrefix(args[2], baseSHA)
			},
			baseSHA: baseSHA,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			root := t.TempDir()
			preparer := &ReviewerPreparer{
				git: &cancellingGitRunner{
					cancel: cancel,
					match:  tt.cancelWhen,
					target: targetSHA,
				},
				now: time.Now,
			}
			_, err := preparer.Prepare(ctx, ReviewerRequest{
				WorkspaceRoot:       root,
				TaskID:              "task-cancelled",
				RunID:               "run-cancelled",
				Project:             "group/project",
				RepositoryURL:       "https://gitlab.example/group/project.git",
				SourceBranch:        "feature/review",
				TargetBranch:        "master",
				HeadSHA:             headSHA,
				BaseSHA:             tt.baseSHA,
				RequestedReviewMode: map[bool]string{true: ReviewModeIncremental, false: ReviewModeFull}[tt.baseSHA != ""],
			})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("expected context cancellation, got %v", err)
			}
			manifestPath := filepath.Join(root, "runs", "run-cancelled", ManifestFileName)
			if _, statErr := os.Stat(manifestPath); !os.IsNotExist(statErr) {
				t.Fatalf("cancelled preparation published manifest: %v", statErr)
			}
		})
	}
}

type cancellingGitRunner struct {
	cancel context.CancelFunc
	match  func([]string) bool
	target string
}

type blockingGitRunner struct {
	started chan struct{}
	release chan struct{}
	target  string
}

func (r *blockingGitRunner) Run(_ context.Context, _ string, _ []string, args ...string) (string, error) {
	if len(args) > 0 && args[0] == "fetch" {
		close(r.started)
		<-r.release
	}
	if len(args) >= 2 && args[0] == "rev-parse" {
		return r.target + "\n", nil
	}
	return "", nil
}

func (r *cancellingGitRunner) Run(_ context.Context, _ string, _ []string, args ...string) (string, error) {
	if r.match(args) {
		r.cancel()
		return "", context.Canceled
	}
	if len(args) >= 2 && args[0] == "rev-parse" {
		return r.target + "\n", nil
	}
	return "", nil
}
