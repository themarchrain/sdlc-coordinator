package workspace

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"
)

type DeveloperRequest struct {
	WorkspaceRoot string
	// PrivateRunRoot is enabled by the Runner operation service's isolated CLI mode.
	PrivateRunRoot  bool
	TaskID          string
	RunID           string
	TaskType        string
	Project         string
	RepositoryURL   string
	SourceBranch    string
	TargetBranch    string
	DeliveryBranch  string
	ExpectedHeadSHA string
	Credential      *GitCredential
	// GitAuthorName/GitAuthorEmail 来自注册仓行的提交身份;任一为空时 prepare
	// 降级使用内置身份,并以 git config --local 写入本 run 仓库成为快照。
	GitAuthorName  string
	GitAuthorEmail string
}

type DeveloperPreparer struct {
	git gitRunner
	now func() time.Time
}

func NewDeveloperPreparer() *DeveloperPreparer {
	return &DeveloperPreparer{git: execGitRunner{}, now: time.Now}
}

func (p *DeveloperPreparer) Prepare(ctx context.Context, request DeveloperRequest) (*Manifest, error) {
	req, err := validateDeveloperRequest(request)
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
		return nil, fmt.Errorf("create developer runs directory: %w", err)
	}
	stagingRoot, err := os.MkdirTemp(runsRoot, "."+req.RunID+".preparing-")
	if err != nil {
		return nil, fmt.Errorf("create developer staging workspace: %w", err)
	}
	defer os.RemoveAll(stagingRoot)
	repoRoot := filepath.Join(stagingRoot, "repo")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create developer repository directory: %w", err)
	}
	if _, err := p.git.Run(ctx, repoRoot, nil, "init"); err != nil {
		return nil, err
	}
	// 提交身份不写进 Runner 可写的仓库本地 config(.git/config 改动不进
	// git status);唯一生效点是 Coordinator commit 命令上的受信 -c 覆盖,
	// 身份值随交付请求由编排层从注册仓行传入。
	if _, err := p.git.Run(ctx, repoRoot, nil, "remote", "add", "origin", req.RepositoryURL); err != nil {
		return nil, err
	}
	fetchArgs := []string{"fetch", "--no-tags", "--prune", "origin", "+refs/heads/" + req.TargetBranch + ":refs/remotes/origin/" + req.TargetBranch}
	if req.TaskType == model.TaskTypeReworkMergeRequest {
		fetchArgs = append(fetchArgs, "+refs/heads/"+req.SourceBranch+":refs/remotes/origin/"+req.SourceBranch)
	}
	fetchArgs, environment := credentialFetchArguments(req.Credential, fetchArgs)
	if _, err := p.git.Run(ctx, repoRoot, environment, fetchArgs...); err != nil {
		return nil, err
	}
	targetSHA, err := p.git.Run(ctx, repoRoot, nil, "rev-parse", "refs/remotes/origin/"+req.TargetBranch)
	if err != nil {
		return nil, err
	}
	targetSHA = strings.ToLower(strings.TrimSpace(targetSHA))
	initialHead := targetSHA
	if req.TaskType == model.TaskTypeReworkMergeRequest {
		sourceSHA, err := p.git.Run(ctx, repoRoot, nil, "rev-parse", "refs/remotes/origin/"+req.SourceBranch)
		if err != nil {
			return nil, fmt.Errorf("%w: source branch %s", ErrHeadUnavailable, req.SourceBranch)
		}
		initialHead = strings.ToLower(strings.TrimSpace(sourceSHA))
		if !strings.EqualFold(initialHead, req.ExpectedHeadSHA) {
			return nil, fmt.Errorf("%w: expected %s, got %s", ErrHeadUnavailable, req.ExpectedHeadSHA, initialHead)
		}
	}
	if _, err := p.git.Run(ctx, repoRoot, nil, "checkout", "-B", req.DeliveryBranch, initialHead); err != nil {
		return nil, err
	}
	if _, err := p.git.Run(ctx, repoRoot, nil, "remote", "set-url", "--push", "origin", DisabledDeveloperPushURL); err != nil {
		return nil, err
	}
	manifest := &Manifest{
		Version: 1, Status: ManifestStatusReady, TaskID: req.TaskID, RunID: req.RunID, Role: "DeveloperWorker", TaskType: req.TaskType,
		Repository: RepositoryManifest{
			Project: req.Project, Path: "repo", RepositoryURL: req.RepositoryURL,
			InitialHeadSHA: initialHead, DeliveryBranch: req.DeliveryBranch,
			SourceBranch: req.SourceBranch, TargetBranch: req.TargetBranch,
			TargetSHA: targetSHA, HeadSHA: initialHead,
		},
		PreparedAt: p.now().UTC(),
	}
	if err := writeManifestAtomic(filepath.Join(stagingRoot, ManifestFileName), manifest); err != nil {
		return nil, err
	}
	if err := prepareWorkspacePermissions(stagingRoot, req.PrivateRunRoot); err != nil {
		return nil, fmt.Errorf("set developer workspace permissions: %w", err)
	}
	if err := os.Rename(stagingRoot, runRoot); err != nil {
		existing, reuseErr := p.reuseExistingWorkspace(ctx, runRoot, req)
		if reuseErr != nil {
			return nil, reuseErr
		}
		if existing != nil {
			return existing, nil
		}
		return nil, fmt.Errorf("publish developer workspace: %w", err)
	}
	return manifest, nil
}

func (p *DeveloperPreparer) reuseExistingWorkspace(ctx context.Context, runRoot string, req DeveloperRequest) (*Manifest, error) {
	entries, err := os.ReadDir(runRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect developer workspace: %w", err)
	}
	manifest, manifestErr := readManifest(filepath.Join(runRoot, ManifestFileName))
	if manifestErr == nil {
		if !manifestMatchesDeveloperRequest(manifest, req) {
			return nil, fmt.Errorf("%w: %s", ErrWorkspaceConflict, runRoot)
		}
		repoRoot := filepath.Join(runRoot, "repo")
		if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrWorkspaceConflict, runRoot)
		}
		branch, err := p.git.Run(ctx, repoRoot, nil, "branch", "--show-current")
		if err != nil || strings.TrimSpace(branch) != req.DeliveryBranch {
			return nil, fmt.Errorf("%w: %s", ErrWorkspaceConflict, runRoot)
		}
		if _, err := p.git.Run(ctx, repoRoot, nil, "merge-base", "--is-ancestor", manifest.Repository.InitialHeadSHA, "HEAD"); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrWorkspaceConflict, runRoot)
		}
		if err := prepareWorkspacePermissions(runRoot, req.PrivateRunRoot); err != nil {
			return nil, fmt.Errorf("set developer workspace permissions: %w", err)
		}
		return manifest, nil
	}
	if !errors.Is(manifestErr, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrWorkspaceConflict, runRoot)
	}
	if len(entries) != 1 || entries[0].Name() != "repo" || !entries[0].IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrWorkspaceConflict, runRoot)
	}
	if _, err := os.Stat(filepath.Join(runRoot, "repo", ".git")); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrWorkspaceConflict, runRoot)
	}
	if err := os.RemoveAll(runRoot); err != nil {
		return nil, fmt.Errorf("remove incomplete developer workspace: %w", err)
	}
	return nil, nil
}

func validateDeveloperRequest(req DeveloperRequest) (DeveloperRequest, error) {
	req.WorkspaceRoot = strings.TrimSpace(req.WorkspaceRoot)
	req.TaskID = strings.TrimSpace(req.TaskID)
	req.RunID = strings.TrimSpace(req.RunID)
	req.TaskType = strings.TrimSpace(req.TaskType)
	req.Project = strings.TrimSpace(req.Project)
	req.RepositoryURL = strings.TrimSpace(req.RepositoryURL)
	req.SourceBranch = strings.TrimSpace(req.SourceBranch)
	req.TargetBranch = strings.TrimSpace(req.TargetBranch)
	req.DeliveryBranch = strings.TrimSpace(req.DeliveryBranch)
	req.ExpectedHeadSHA = strings.ToLower(strings.TrimSpace(req.ExpectedHeadSHA))
	for name, value := range map[string]string{
		"workspace root": req.WorkspaceRoot, "task id": req.TaskID, "run id": req.RunID,
		"task type": req.TaskType, "project": req.Project, "repository url": req.RepositoryURL,
		"target branch": req.TargetBranch, "delivery branch": req.DeliveryBranch,
	} {
		if value == "" {
			return DeveloperRequest{}, fmt.Errorf("%s is required", name)
		}
	}
	if !safeRunIDPattern.MatchString(req.RunID) {
		return DeveloperRequest{}, fmt.Errorf("run id %q is not a safe path component", req.RunID)
	}
	if req.TaskType == model.TaskTypeReworkMergeRequest {
		if req.SourceBranch == "" || !commitSHAPattern.MatchString(req.ExpectedHeadSHA) {
			return DeveloperRequest{}, errors.New("rework source branch and expected head sha are required")
		}
	}
	if strings.Contains(req.RepositoryURL, "://") {
		parsed, err := url.Parse(req.RepositoryURL)
		if err != nil {
			return DeveloperRequest{}, fmt.Errorf("parse repository url: %w", err)
		}
		if parsed.User != nil {
			return DeveloperRequest{}, errors.New("repository url must not contain credentials")
		}
	}
	return req, nil
}

func manifestMatchesDeveloperRequest(manifest *Manifest, req DeveloperRequest) bool {
	return manifest != nil && manifest.Status == ManifestStatusReady && manifest.TaskID == req.TaskID &&
		manifest.RunID == req.RunID && manifest.Role == "DeveloperWorker" && manifest.TaskType == req.TaskType &&
		manifest.Repository.Project == req.Project && manifest.Repository.RepositoryURL == req.RepositoryURL &&
		manifest.Repository.SourceBranch == req.SourceBranch && manifest.Repository.TargetBranch == req.TargetBranch &&
		manifest.Repository.DeliveryBranch == req.DeliveryBranch &&
		(req.TaskType != model.TaskTypeReworkMergeRequest || strings.EqualFold(manifest.Repository.InitialHeadSHA, req.ExpectedHeadSHA))
}
