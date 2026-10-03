package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sdlc-coordinator/internal/results"
)

var (
	ErrNoDeveloperChanges        = errors.New("developer result has no workspace changes")
	ErrDeveloperDeliveryConflict = errors.New("developer delivery branch has conflicting remote state")
)

type DeveloperFinalizeRequest struct {
	WorkspaceRoot string
	TaskID        string
	RunID         string
	// CommitMessage 是 AI 回传并经契约校验的提交正文;finalizer 只在末尾追加
	// 可信追溯尾注,不再从 summary 派生标题。
	CommitMessage string
	// GitAuthorName/GitAuthorEmail 是任务绑定的注册仓行受信身份快照。共享工作
	// 区里 Runner 可改写 .git/config(status 不显示),commit 时以命令行 config
	// 覆盖本地 config,保证落库身份恒为受信值。
	GitAuthorName          string
	GitAuthorEmail         string
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
	request.TaskType = strings.TrimSpace(request.TaskType)
	request.RepositoryURL = strings.TrimSpace(request.RepositoryURL)
	request.TargetBranch = strings.TrimSpace(request.TargetBranch)
	request.DeliveryBranch = strings.TrimSpace(request.DeliveryBranch)
	request.ExpectedInitialHeadSHA = strings.ToLower(strings.TrimSpace(request.ExpectedInitialHeadSHA))
	if request.WorkspaceRoot == "" || request.TaskID == "" || request.RunID == "" ||
		request.TaskType == "" || request.RepositoryURL == "" || request.TargetBranch == "" || request.DeliveryBranch == "" {
		return nil, errors.New("developer finalization workspace, task, run and trusted delivery metadata are required")
	}
	// 消息校验先于任何 Git 操作(含 staging):无效消息在触碰仓库前就被拒绝。
	commitMessage, err := results.NormalizeCommitMessage(request.CommitMessage)
	if err != nil {
		return nil, fmt.Errorf("developer commit message: %w", err)
	}
	// 受信身份快照:空值降级内置身份。共享工作区里 Runner 可改写 .git/config
	// 且 git status 不显示,commit 必须用受信值覆盖,不信任仓库本地 config。
	authorName, authorEmail := resolveGitIdentity(request.GitAuthorName, request.GitAuthorEmail)
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
		// 提交正文即 AI 回传内容,追溯尾注由 Coordinator 统一追加。身份以
		// 命令行 config 覆盖 Runner 可写的本地 config;--cleanup=verbatim
		// 保证已校验内容原样落库(Git 默认清理会折叠正文连续空行,读回比对
		// 会误报冲突)。多段消息经临时文件交给 commit -F:Windows 的 git 会
		// 把 -m 参数在内嵌换行处截断。
		committed := commitMessage + "\n\n" + results.TrailerMarker + " " + request.RunID
		messageFile, err := os.CreateTemp("", "developer-commit-message-*.txt")
		if err != nil {
			return nil, err
		}
		defer os.Remove(messageFile.Name())
		if _, err := messageFile.WriteString(committed); err != nil {
			messageFile.Close()
			return nil, err
		}
		if err := messageFile.Close(); err != nil {
			return nil, err
		}
		if _, err := f.git.Run(ctx, repoRoot, nil,
			"-c", "user.name="+authorName, "-c", "user.email="+authorEmail,
			"commit", "--cleanup=verbatim", "-F", messageFile.Name(),
		); err != nil {
			return nil, err
		}
		written, err := f.git.Run(ctx, repoRoot, nil, "log", "-1", "--format=%B")
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(written) != committed {
			return nil, fmt.Errorf("%w: committed message differs from requested content", ErrWorkspaceConflict)
		}
		if err := f.verifyCommitIdentity(ctx, repoRoot, authorName, authorEmail); err != nil {
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
		// 恢复路径只接受本 run 内 Coordinator 已落的提交(如 push 失败后的
		// 重试):消息必须与本次提交将写出的字节完全一致,作者/提交者必须等
		// 于受信身份。尾注行可被 Agent 自由伪造,仅凭它不构成所有权证明;
		// 逐字节同质的提交与协调器所写在消息、身份与尾注上无差异。
		expected := commitMessage + "\n\n" + results.TrailerMarker + " " + request.RunID
		message, err := f.git.Run(ctx, repoRoot, nil, "log", "-1", "--format=%B")
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(message) != expected {
			return nil, fmt.Errorf("%w: local delivery commit does not match the submitted commit message", ErrWorkspaceConflict)
		}
		if err := f.verifyCommitIdentity(ctx, repoRoot, authorName, authorEmail); err != nil {
			return nil, err
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

// verifyCommitIdentity 读回 HEAD 的作者与提交者,必须逐项等于受信身份快照。
// 它是命令行 config 覆盖的可观测兜底:环境残留或未来引入的身份旁路在这里
// 显式报错,而不是静默落库错身份。
func (f *DeveloperFinalizer) verifyCommitIdentity(ctx context.Context, repoRoot, authorName, authorEmail string) error {
	raw, err := f.git.Run(ctx, repoRoot, nil, "show", "-s", "--format=%an%n%ae%n%cn%n%ce", "HEAD")
	if err != nil {
		return err
	}
	fields := make([]string, 0, 4)
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		if value := strings.TrimSpace(line); value != "" {
			fields = append(fields, value)
		}
	}
	if len(fields) != 4 || fields[0] != authorName || fields[1] != authorEmail || fields[2] != authorName || fields[3] != authorEmail {
		return fmt.Errorf("%w: committed identity differs from trusted repository configuration", ErrWorkspaceConflict)
	}
	return nil
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