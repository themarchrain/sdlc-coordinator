package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ReviewerPreparer struct {
	git gitRunner
	now func() time.Time
}

func NewReviewerPreparer() *ReviewerPreparer {
	return &ReviewerPreparer{git: execGitRunner{}, now: time.Now}
}

func (p *ReviewerPreparer) Prepare(ctx context.Context, request ReviewerRequest) (*Manifest, error) {
	req, err := validateReviewerRequest(request)
	if err != nil {
		return nil, err
	}
	runRoot := filepath.Join(req.WorkspaceRoot, "runs", req.RunID)
	if manifest, err := p.reuseExistingWorkspace(ctx, runRoot, req); err != nil {
		return nil, err
	} else if manifest != nil {
		return manifest, nil
	}

	runsRoot := filepath.Dir(runRoot)
	if err := os.MkdirAll(runsRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create reviewer runs directory: %w", err)
	}
	stagingRoot, err := os.MkdirTemp(runsRoot, "."+req.RunID+".preparing-")
	if err != nil {
		return nil, fmt.Errorf("create reviewer staging workspace: %w", err)
	}
	defer os.RemoveAll(stagingRoot)

	repoRoot := filepath.Join(stagingRoot, "repo")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create reviewer repository directory: %w", err)
	}

	if _, err := p.git.Run(ctx, repoRoot, nil, "init"); err != nil {
		return nil, err
	}
	if _, err := p.git.Run(ctx, repoRoot, nil, "remote", "add", "origin", req.RepositoryURL); err != nil {
		return nil, err
	}
	fetchArgs := []string{
		"fetch", "--no-tags", "--prune", "origin",
		"+refs/heads/" + req.SourceBranch + ":refs/remotes/origin/" + req.SourceBranch,
		"+refs/heads/" + req.TargetBranch + ":refs/remotes/origin/" + req.TargetBranch,
	}
	fetchArgs, fetchEnvironment := credentialFetchArguments(req.Credential, fetchArgs)
	if _, err := p.git.Run(ctx, repoRoot, fetchEnvironment, fetchArgs...); err != nil {
		return nil, err
	}

	targetRef := "refs/remotes/origin/" + req.TargetBranch
	sourceRef := "refs/remotes/origin/" + req.SourceBranch
	targetSHA, err := p.git.Run(ctx, repoRoot, nil, "rev-parse", targetRef)
	if err != nil {
		return nil, err
	}
	targetSHA = strings.TrimSpace(targetSHA)
	if _, err := p.git.Run(ctx, repoRoot, nil, "cat-file", "-e", req.HeadSHA+"^{commit}"); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %s", ErrHeadUnavailable, req.HeadSHA)
	}
	if _, err := p.git.Run(ctx, repoRoot, nil, "merge-base", "--is-ancestor", req.HeadSHA, sourceRef); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: head %s is not in source branch %s", ErrHeadUnavailable, req.HeadSHA, req.SourceBranch)
	}
	if _, err := p.git.Run(ctx, repoRoot, nil, "checkout", "--detach", req.HeadSHA); err != nil {
		return nil, err
	}
	if _, err := p.git.Run(ctx, repoRoot, nil, "remote", "set-url", "--push", "origin", DisabledReviewerPushURL); err != nil {
		return nil, err
	}

	manifest := &Manifest{
		Version: 1,
		Status:  ManifestStatusReady,
		TaskID:  req.TaskID,
		RunID:   req.RunID,
		Role:    "ReviewerWorker",
		Repository: RepositoryManifest{
			Project:             req.Project,
			Path:                "repo",
			SourceBranch:        req.SourceBranch,
			TargetBranch:        req.TargetBranch,
			RequestedReviewMode: req.RequestedReviewMode,
			EffectiveReviewMode: ReviewModeFull,
			TargetSHA:           targetSHA,
			HeadSHA:             req.HeadSHA,
			DiffRange:           targetSHA + "..." + req.HeadSHA,
		},
		PreparedAt: p.now().UTC(),
	}
	if req.RequestedReviewMode == ReviewModeIncremental && req.BaseSHA != "" {
		if _, err := p.git.Run(ctx, repoRoot, nil, "cat-file", "-e", req.BaseSHA+"^{commit}"); err == nil {
			if _, err := p.git.Run(ctx, repoRoot, nil, "merge-base", "--is-ancestor", req.BaseSHA, req.HeadSHA); err == nil {
				manifest.Repository.EffectiveReviewMode = ReviewModeIncremental
				manifest.Repository.BaseSHA = req.BaseSHA
				manifest.Repository.DiffRange = req.BaseSHA + ".." + req.HeadSHA
			} else {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				manifest.Repository.FallbackReason = FallbackBaseNotAncestor
			}
		} else {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			manifest.Repository.FallbackReason = FallbackBaseUnavailable
		}
	}
	if err := writeManifestAtomic(filepath.Join(stagingRoot, ManifestFileName), manifest); err != nil {
		return nil, err
	}
	if err := os.Rename(stagingRoot, runRoot); err != nil {
		existing, reuseErr := p.reuseExistingWorkspace(ctx, runRoot, req)
		if reuseErr != nil {
			return nil, reuseErr
		}
		if existing != nil {
			return existing, nil
		}
		return nil, fmt.Errorf("publish reviewer workspace: %w", err)
	}
	return manifest, nil
}

func (p *ReviewerPreparer) reuseExistingWorkspace(ctx context.Context, runRoot string, req ReviewerRequest) (*Manifest, error) {
	entries, err := os.ReadDir(runRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect reviewer workspace: %w", err)
	}

	manifestPath := filepath.Join(runRoot, ManifestFileName)
	manifest, manifestErr := readManifest(manifestPath)
	if manifestErr == nil {
		if !manifestMatchesReviewerRequest(manifest, req) {
			return nil, fmt.Errorf("%w: %s", ErrWorkspaceConflict, runRoot)
		}
		repoRoot := filepath.Join(runRoot, "repo")
		if _, statErr := os.Stat(repoRoot); statErr != nil {
			return nil, fmt.Errorf("%w: %s", ErrWorkspaceConflict, runRoot)
		}
		head, headErr := p.git.Run(ctx, repoRoot, nil, "rev-parse", "HEAD")
		if headErr != nil || !strings.EqualFold(strings.TrimSpace(head), req.HeadSHA) {
			return nil, fmt.Errorf("%w: %s", ErrWorkspaceConflict, runRoot)
		}
		return manifest, nil
	}
	if !errors.Is(manifestErr, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrWorkspaceConflict, runRoot)
	}

	if len(entries) != 1 || entries[0].Name() != "repo" || !entries[0].IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrWorkspaceConflict, runRoot)
	}
	if _, statErr := os.Stat(filepath.Join(runRoot, "repo", ".git")); statErr != nil {
		return nil, fmt.Errorf("%w: %s", ErrWorkspaceConflict, runRoot)
	}
	if err := os.RemoveAll(runRoot); err != nil {
		return nil, fmt.Errorf("remove incomplete reviewer workspace: %w", err)
	}
	return nil, nil
}
