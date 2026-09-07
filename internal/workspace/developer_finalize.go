package workspace

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var (
	ErrNoDeveloperChanges        = errors.New("developer result has no workspace changes")
	ErrDeveloperDeliveryConflict = errors.New("developer delivery branch has conflicting remote state")
)

type DeveloperFinalizeRequest struct {
	WorkspaceRoot          string
	TaskID                 string
	RunID                  string
	Summary                string
	TaskType               string
	RepositoryURL          string
	TargetBranch           string
	DeliveryBranch         string
	ExpectedInitialHeadSHA string
	Credential             *GitCredential
}

type DeveloperDelivery struct {
	CommitSHA    string
	ChangedFiles []string
	DiffSummary  string
}

type DeveloperFinalizer struct {
	git gitRunner
}

func NewDeveloperFinalizer() *DeveloperFinalizer {
	return &DeveloperFinalizer{git: execGitRunner{}}
}

func (f *DeveloperFinalizer) Finalize(ctx context.Context, request DeveloperFinalizeRequest) (*DeveloperDelivery, error) {
	request.WorkspaceRoot = strings.TrimSpace(request.WorkspaceRoot)
	request.TaskID = strings.TrimSpace(request.TaskID)
	request.RunID = strings.TrimSpace(request.RunID)
	request.Summary = strings.TrimSpace(request.Summary)
	request.TaskType = strings.TrimSpace(request.TaskType)
	request.RepositoryURL = strings.TrimSpace(request.RepositoryURL)
	request.TargetBranch = strings.TrimSpace(request.TargetBranch)
	request.DeliveryBranch = strings.TrimSpace(request.DeliveryBranch)
	request.ExpectedInitialHeadSHA = strings.ToLower(strings.TrimSpace(request.ExpectedInitialHeadSHA))
	if request.WorkspaceRoot == "" || request.TaskID == "" || request.RunID == "" || request.Summary == "" ||
		request.TaskType == "" || request.RepositoryURL == "" || request.TargetBranch == "" || request.DeliveryBranch == "" {
		return nil, errors.New("developer finalization workspace, task, run, summary and trusted delivery metadata are required")
	}
	runRoot := filepath.Join(request.WorkspaceRoot, "runs", request.RunID)
	manifest, err := readManifest(filepath.Join(runRoot, ManifestFileName))
	if err != nil {
		return nil, fmt.Errorf("read developer workspace manifest: %w", err)
	}
	if manifest.Status != ManifestStatusReady || manifest.Role != "DeveloperWorker" || manifest.TaskID != request.TaskID || manifest.RunID != request.RunID {
		return nil, fmt.Errorf("%w: developer manifest identity mismatch", ErrWorkspaceConflict)
	}
	if manifest.TaskType != request.TaskType || manifest.Repository.Path != "repo" ||
		manifest.Repository.RepositoryURL != request.RepositoryURL || manifest.Repository.TargetBranch != request.TargetBranch ||
		manifest.Repository.DeliveryBranch != request.DeliveryBranch {
		return nil, fmt.Errorf("%w: developer manifest delivery metadata differs from trusted task state", ErrWorkspaceConflict)
	}
	if strings.TrimSpace(manifest.Repository.InitialHeadSHA) == "" || strings.TrimSpace(manifest.Repository.TargetSHA) == "" {
		return nil, fmt.Errorf("%w: developer manifest delivery metadata is incomplete", ErrWorkspaceConflict)
	}
	if request.ExpectedInitialHeadSHA != "" && !strings.EqualFold(manifest.Repository.InitialHeadSHA, request.ExpectedInitialHeadSHA) {
		return nil, fmt.Errorf("%w: developer manifest initial head differs from trusted task state", ErrWorkspaceConflict)
	}
	if request.TaskType != "rework_merge_request" && !strings.EqualFold(manifest.Repository.InitialHeadSHA, manifest.Repository.TargetSHA) {
		return nil, fmt.Errorf("%w: initial developer workspace is not based on its prepared target snapshot", ErrWorkspaceConflict)
	}
	repoRoot := filepath.Join(runRoot, "repo")
	status, err := f.git.Run(ctx, repoRoot, nil, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	head, err := f.git.Run(ctx, repoRoot, nil, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	head = strings.ToLower(strings.TrimSpace(head))

	var changedFiles []string
	var diffSummary string
	if strings.TrimSpace(status) != "" {
		if _, err := f.git.Run(ctx, repoRoot, nil, "add", "-A"); err != nil {
			return nil, err
		}
		changedFiles, err = f.changedFiles(ctx, repoRoot, "--cached")
		if err != nil {
			return nil, err
		}
		if len(changedFiles) == 0 {
			return nil, ErrNoDeveloperChanges
		}
		diffSummary, err = f.git.Run(ctx, repoRoot, nil, "diff", "--cached", "--stat")
		if err != nil {
			return nil, err
		}
		subject := developerCommitSubject(request.Summary)
		if _, err := f.git.Run(ctx, repoRoot, nil,
			"-c", "user.name=AI Developer", "-c", "user.email=ai-developer@localhost",
			"commit", "-m", subject, "-m", "Coordinator-Run-ID: "+request.RunID,
		); err != nil {
			return nil, err
		}
		head, err = f.git.Run(ctx, repoRoot, nil, "rev-parse", "HEAD")
		if err != nil {
			return nil, err
		}
		head = strings.ToLower(strings.TrimSpace(head))
	} else {
		if strings.EqualFold(head, manifest.Repository.InitialHeadSHA) {
			return nil, ErrNoDeveloperChanges
		}
		if _, err := f.git.Run(ctx, repoRoot, nil, "merge-base", "--is-ancestor", manifest.Repository.InitialHeadSHA, head); err != nil {
			return nil, fmt.Errorf("%w: local delivery commit is not based on initial head", ErrWorkspaceConflict)
		}
		message, err := f.git.Run(ctx, repoRoot, nil, "log", "-1", "--format=%B")
		if err != nil {
			return nil, err
		}
		if !containsRunTrailer(message, request.RunID) {
			return nil, fmt.Errorf("%w: local commit is not owned by current run", ErrWorkspaceConflict)
		}
		changedFiles, err = f.changedFiles(ctx, repoRoot, manifest.Repository.InitialHeadSHA+".."+head)
		if err != nil {
			return nil, err
		}
		diffSummary, err = f.git.Run(ctx, repoRoot, nil, "diff", "--stat", manifest.Repository.InitialHeadSHA+".."+head)
		if err != nil {
			return nil, err
		}
	}

	remoteSHA, err := f.remoteBranchSHA(ctx, repoRoot, request.RepositoryURL, request.DeliveryBranch, request.Credential)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(remoteSHA, head) {
		return &DeveloperDelivery{CommitSHA: head, ChangedFiles: changedFiles, DiffSummary: strings.TrimSpace(diffSummary)}, nil
	}
	if remoteSHA != "" && !strings.EqualFold(remoteSHA, manifest.Repository.InitialHeadSHA) {
		return nil, fmt.Errorf("%w: branch %s points to %s", ErrDeveloperDeliveryConflict, request.DeliveryBranch, remoteSHA)
	}
	if request.TaskType == "rework_merge_request" && remoteSHA == "" {
		return nil, fmt.Errorf("%w: rework branch %s no longer exists", ErrDeveloperDeliveryConflict, request.DeliveryBranch)
	}
	pushArgs, environment := credentialFetchArguments(request.Credential, []string{
		"push", request.RepositoryURL, "HEAD:refs/heads/" + request.DeliveryBranch,
	})
	if _, err := f.git.Run(ctx, repoRoot, environment, pushArgs...); err != nil {
		return nil, err
	}
	verifiedSHA, err := f.remoteBranchSHA(ctx, repoRoot, request.RepositoryURL, request.DeliveryBranch, request.Credential)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(verifiedSHA, head) {
		return nil, fmt.Errorf("%w: pushed branch resolved to %s instead of %s", ErrDeveloperDeliveryConflict, verifiedSHA, head)
	}
	return &DeveloperDelivery{CommitSHA: head, ChangedFiles: changedFiles, DiffSummary: strings.TrimSpace(diffSummary)}, nil
}

func (f *DeveloperFinalizer) changedFiles(ctx context.Context, repoRoot string, diffArgs ...string) ([]string, error) {
	args := append([]string{"diff"}, diffArgs...)
	args = append(args, "--name-only")
	raw, err := f.git.Run(ctx, repoRoot, nil, args...)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	files := make([]string, 0, len(lines))
	for _, line := range lines {
		if value := strings.TrimSpace(line); value != "" {
			files = append(files, value)
		}
	}
	return files, nil
}

func (f *DeveloperFinalizer) remoteBranchSHA(ctx context.Context, repoRoot, repositoryURL, branch string, credential *GitCredential) (string, error) {
	args, environment := credentialFetchArguments(credential, []string{"ls-remote", repositoryURL, "refs/heads/" + branch})
	raw, err := f.git.Run(ctx, repoRoot, environment, args...)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return "", nil
	}
	return strings.ToLower(fields[0]), nil
}

func developerCommitSubject(summary string) string {
	line := strings.TrimSpace(strings.Split(strings.ReplaceAll(summary, "\r\n", "\n"), "\n")[0])
	if len(line) > 68 {
		line = strings.TrimSpace(line[:68])
	}
	if line == "" {
		line = "apply requested change"
	}
	return "ai: " + line
}

func containsRunTrailer(message, runID string) bool {
	for _, line := range strings.Split(strings.ReplaceAll(message, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "Coordinator-Run-ID: "+runID {
			return true
		}
	}
	return false
}
