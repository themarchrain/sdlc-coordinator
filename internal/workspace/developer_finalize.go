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

// developerDeliveryMetadata 是两阶段交付共用的受信元数据:由编排层从任务与
// 租户库检查点重建,manifest 必须与之一致才允许操作仓库。
type developerDeliveryMetadata struct {
	WorkspaceRoot  string
	TaskID         string
	RunID          string
	TaskType       string
	RepositoryURL  string
	TargetBranch   string
	DeliveryBranch string
	BaselineSHA    string
}

type DeveloperCommitRequest struct {
	WorkspaceRoot string
	TaskID        string
	RunID         string
	TaskType      string
	// CommitMessage 是 AI 回传并经契约校验的提交正文;finalizer 只在末尾
	// 追加可信追溯尾注,不接受 Agent 自带的尾注。
	CommitMessage string
	// GitAuthorName/GitAuthorEmail 是注册仓行受信身份快照,以命令行 config
	// 覆盖 Runner 可写的仓库本地 config。
	GitAuthorName  string
	GitAuthorEmail string
	RepositoryURL  string
	TargetBranch   string
	DeliveryBranch string
	// BaselineSHA 是租户库检查点钉住的初始 HEAD:新建交付提交必须是它的
	// 直接子提交,当前 HEAD 偏离基线时拒绝新建。
	BaselineSHA string
	// DeliveryOwner/DeliveryGeneration 是交付执行权的当前持有身份（远端
	// 形态随请求传给 Runner 的授权确认；本机形态不消费）。
	DeliveryOwner      string
	DeliveryGeneration uint64
}

type DeveloperCommitResult struct {
	CommitSHA string
}

type DeveloperPushRequest struct {
	WorkspaceRoot  string
	TaskID         string
	RunID          string
	TaskType       string
	RepositoryURL  string
	TargetBranch   string
	DeliveryBranch string
	// CommitSHA 是检查点记录的交付提交:推送固定 SHA,不用可变 HEAD,本地
	// 后续提交不随行。空值或与本地 HEAD 不符一律拒绝。
	CommitSHA   string
	BaselineSHA string
	Credential  *GitCredential
	// DeliveryOwner/DeliveryGeneration 语义同 DeveloperCommitRequest。
	DeliveryOwner      string
	DeliveryGeneration uint64
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

// CommitDelivery 校验受信基线与 manifest 后暂存并创建本 run 的交付提交:
// 当前 HEAD 必须等于固定基线,提交必须是基线的直接子提交,消息与身份按约
// 定落库并读回核验。本方法不推送;返回的 SHA 由编排层写入检查点后才允许
// 推送。干净工作区上既有的本地提交(如 Agent 提前提交)不在此认领——
// 归属只能由 Coordinator 私有检查点背书,不凭消息、尾注或身份"长得像"。
func (f *DeveloperFinalizer) CommitDelivery(ctx context.Context, request DeveloperCommitRequest) (*DeveloperCommitResult, error) {
	meta, err := deliveryMetadata(request.WorkspaceRoot, request.TaskID, request.RunID, request.TaskType,
		request.RepositoryURL, request.TargetBranch, request.DeliveryBranch, request.BaselineSHA)
	if err != nil {
		return nil, err
	}
	// 消息校验先于任何 Git 操作(含 staging):无效消息在触碰仓库前就被拒绝。
	commitMessage, err := results.NormalizeCommitMessage(request.CommitMessage)
	if err != nil {
		return nil, fmt.Errorf("developer commit message: %w", err)
	}
	// 受信身份快照:空值降级内置身份。共享工作区里 Runner 可改写 .git/config
	// 且 git status 不显示,commit 必须用受信值覆盖,不信任仓库本地 config。
	authorName, authorEmail := resolveGitIdentity(request.GitAuthorName, request.GitAuthorEmail)
	delivery, err := f.loadDeliveryManifest(meta)
	if err != nil {
		return nil, err
	}
	repoRoot := filepath.Join(delivery.runRoot, "repo")
	status, err := f.git.Run(ctx, repoRoot, nil, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	head, err := f.git.Run(ctx, repoRoot, nil, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	head = strings.ToLower(strings.TrimSpace(head))
	// 干净或脏工作区都一样:HEAD 不在受信基线上,本 run 就没有可新建提交
	// 的起点;既有本地提交不经检查点不得认领,更不得随交付推送。
	if !strings.EqualFold(head, meta.BaselineSHA) {
		return nil, fmt.Errorf("%w: workspace HEAD diverged from the trusted delivery baseline", ErrWorkspaceConflict)
	}
	if strings.TrimSpace(status) == "" {
		return nil, ErrNoDeveloperChanges
	}
	if _, err := f.git.Run(ctx, repoRoot, nil, "add", "-A"); err != nil {
		return nil, err
	}
	changedFiles, err := f.changedFiles(ctx, repoRoot, "--cached")
	if err != nil {
		return nil, err
	}
	if len(changedFiles) == 0 {
		return nil, ErrNoDeveloperChanges
	}
	// 提交正文即 AI 回传内容,追溯尾注由 Coordinator 统一追加。身份以命令
	// 行 config 覆盖 Runner 可写的本地 config;--cleanup=verbatim 保证已校
	// 验内容原样落库(Git 默认清理会折叠正文连续空行,读回比对会误报冲突)。
	// 多段消息经临时文件交给 commit -F:Windows 的 git 会把 -m 参数在内嵌
	// 换行处截断。
	committed := commitMessage + "\n\n" + results.TrailerMarker + " " + meta.RunID
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
	committedSHA, err := f.git.Run(ctx, repoRoot, nil, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	committedSHA = strings.ToLower(strings.TrimSpace(committedSHA))
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
	// 直接父提交必须是固定基线:并发同 run 双提交的败者、或踩在未知本地
	// 提交之上的提交链,都在这里 fail-closed。
	parent, err := f.git.Run(ctx, repoRoot, nil, "rev-parse", "HEAD^")
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(parent), meta.BaselineSHA) {
		return nil, fmt.Errorf("%w: delivery commit parent differs from the trusted baseline", ErrWorkspaceConflict)
	}
	return &DeveloperCommitResult{CommitSHA: committedSHA}, nil
}

// PushDelivery 以检查点记录的固定 SHA 推送:本地 HEAD 必须仍等于该 SHA,
// 其父提交必须是受信基线;远端已是该 SHA 幂等成功,远端漂移维持冲突语义,
// 不强推、不改写远端历史。推送绝不使用可变 HEAD,Agent 在交付提交之后的
// 本地提交不随行。
func (f *DeveloperFinalizer) PushDelivery(ctx context.Context, request DeveloperPushRequest) (*DeveloperDelivery, error) {
	meta, err := deliveryMetadata(request.WorkspaceRoot, request.TaskID, request.RunID, request.TaskType,
		request.RepositoryURL, request.TargetBranch, request.DeliveryBranch, request.BaselineSHA)
	if err != nil {
		return nil, err
	}
	commitSHA := strings.ToLower(strings.TrimSpace(request.CommitSHA))
	if commitSHA == "" {
		return nil, errors.New("developer push requires the checkpointed delivery commit sha")
	}
	delivery, err := f.loadDeliveryManifest(meta)
	if err != nil {
		return nil, err
	}
	repoRoot := filepath.Join(delivery.runRoot, "repo")
	head, err := f.git.Run(ctx, repoRoot, nil, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	head = strings.ToLower(strings.TrimSpace(head))
	if !strings.EqualFold(head, commitSHA) {
		return nil, fmt.Errorf("%w: local HEAD diverged from the checkpointed delivery commit", ErrWorkspaceConflict)
	}
	parent, err := f.git.Run(ctx, repoRoot, nil, "rev-parse", "HEAD^")
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(parent), meta.BaselineSHA) {
		return nil, fmt.Errorf("%w: delivery commit parent differs from the trusted baseline", ErrWorkspaceConflict)
	}
	changedFiles, err := f.changedFiles(ctx, repoRoot, delivery.manifest.Repository.InitialHeadSHA+".."+commitSHA)
	if err != nil {
		return nil, err
	}
	diffSummary, err := f.git.Run(ctx, repoRoot, nil, "diff", "--stat", delivery.manifest.Repository.InitialHeadSHA+".."+commitSHA)
	if err != nil {
		return nil, err
	}
	remoteSHA, err := f.remoteBranchSHA(ctx, repoRoot, request.RepositoryURL, request.DeliveryBranch, request.Credential)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(remoteSHA, commitSHA) {
		return &DeveloperDelivery{CommitSHA: commitSHA, ChangedFiles: changedFiles, DiffSummary: strings.TrimSpace(diffSummary)}, nil
	}
	if remoteSHA != "" && !strings.EqualFold(remoteSHA, delivery.manifest.Repository.InitialHeadSHA) {
		return nil, fmt.Errorf("%w: branch %s points to %s", ErrDeveloperDeliveryConflict, request.DeliveryBranch, remoteSHA)
	}
	if request.TaskType == "rework_merge_request" && remoteSHA == "" {
		return nil, fmt.Errorf("%w: rework branch %s no longer exists", ErrDeveloperDeliveryConflict, request.DeliveryBranch)
	}
	pushArgs, environment := credentialFetchArguments(request.Credential, []string{
		"push", request.RepositoryURL, commitSHA + ":refs/heads/" + request.DeliveryBranch,
	})
	if _, err := f.git.Run(ctx, repoRoot, environment, pushArgs...); err != nil {
		return nil, err
	}
	verifiedSHA, err := f.remoteBranchSHA(ctx, repoRoot, request.RepositoryURL, request.DeliveryBranch, request.Credential)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(verifiedSHA, commitSHA) {
		return nil, fmt.Errorf("%w: pushed branch resolved to %s instead of %s", ErrDeveloperDeliveryConflict, verifiedSHA, commitSHA)
	}
	return &DeveloperDelivery{CommitSHA: commitSHA, ChangedFiles: changedFiles, DiffSummary: strings.TrimSpace(diffSummary)}, nil
}

func deliveryMetadata(workspaceRoot, taskID, runID, taskType, repositoryURL, targetBranch, deliveryBranch, baselineSHA string) (*developerDeliveryMetadata, error) {
	meta := &developerDeliveryMetadata{
		WorkspaceRoot: strings.TrimSpace(workspaceRoot), TaskID: strings.TrimSpace(taskID),
		RunID: strings.TrimSpace(runID), TaskType: strings.TrimSpace(taskType),
		RepositoryURL: strings.TrimSpace(repositoryURL), TargetBranch: strings.TrimSpace(targetBranch),
		DeliveryBranch: strings.TrimSpace(deliveryBranch), BaselineSHA: strings.ToLower(strings.TrimSpace(baselineSHA)),
	}
	if meta.WorkspaceRoot == "" || meta.TaskID == "" || meta.RunID == "" || meta.TaskType == "" ||
		meta.RepositoryURL == "" || meta.TargetBranch == "" || meta.DeliveryBranch == "" || meta.BaselineSHA == "" {
		return nil, errors.New("developer delivery workspace, task, run, trusted metadata and pinned baseline are required")
	}
	return meta, nil
}

// loadDeliveryManifest 读回工作区 manifest 并与受信元数据逐项核对:防篡改
// 边界与基线绑定在 commit 与 push 两个阶段都必须成立。
func (f *DeveloperFinalizer) loadDeliveryManifest(meta *developerDeliveryMetadata) (*developerDeliveryContext, error) {
	runRoot := filepath.Join(meta.WorkspaceRoot, "runs", meta.RunID)
	manifest, err := readManifest(filepath.Join(runRoot, ManifestFileName))
	if err != nil {
		return nil, fmt.Errorf("read developer workspace manifest: %w", err)
	}
	if manifest.Status != ManifestStatusReady || manifest.Role != "DeveloperWorker" || manifest.TaskID != meta.TaskID || manifest.RunID != meta.RunID {
		return nil, fmt.Errorf("%w: developer manifest identity mismatch", ErrWorkspaceConflict)
	}
	if manifest.TaskType != meta.TaskType || manifest.Repository.Path != "repo" ||
		manifest.Repository.RepositoryURL != meta.RepositoryURL || manifest.Repository.TargetBranch != meta.TargetBranch ||
		manifest.Repository.DeliveryBranch != meta.DeliveryBranch {
		return nil, fmt.Errorf("%w: developer manifest delivery metadata differs from trusted task state", ErrWorkspaceConflict)
	}
	if strings.TrimSpace(manifest.Repository.InitialHeadSHA) == "" || strings.TrimSpace(manifest.Repository.TargetSHA) == "" {
		return nil, fmt.Errorf("%w: developer manifest delivery metadata is incomplete", ErrWorkspaceConflict)
	}
	if !strings.EqualFold(manifest.Repository.InitialHeadSHA, meta.BaselineSHA) {
		return nil, fmt.Errorf("%w: developer manifest initial head differs from the trusted delivery baseline", ErrWorkspaceConflict)
	}
	if meta.TaskType != "rework_merge_request" && !strings.EqualFold(manifest.Repository.InitialHeadSHA, manifest.Repository.TargetSHA) {
		return nil, fmt.Errorf("%w: initial developer workspace is not based on its prepared target snapshot", ErrWorkspaceConflict)
	}
	return &developerDeliveryContext{runRoot: runRoot, manifest: manifest}, nil
}

type developerDeliveryContext struct {
	runRoot  string
	manifest *Manifest
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
