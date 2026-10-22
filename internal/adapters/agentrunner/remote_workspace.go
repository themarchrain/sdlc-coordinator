package agentrunner

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"sdlc-coordinator/internal/workspace"
	"sdlc-coordinator/internal/workspaceproto"
)

// EndpointResolver 按 run 解析其归属的 runner 端点（run 行 pin 列；空串=
// 部署默认端点）。远端形态装配注入；nil = 恒用默认端点（单端点直连）。
type EndpointResolver func(ctx context.Context, runID string) (string, error)

// RemoteWorkspace 是受控工作区操作的远端实现：准备、任务包投递、commit/
// push、清理全部按 run 归属的 runner 端点寻址——独立卷下别的 runner 既没有
// 该 run 的仓库也没有 Agent 的修改，端点亲和是正确性要求而非优化。端点由
// resolver 从共享库的 run 行解析（准备前已 pin），每个端点一个客户端缓存。
type RemoteWorkspace struct {
	base       *Client
	token      string
	httpClient *http.Client
	tenantID   uint
	resolver   EndpointResolver

	mu      sync.Mutex
	clients map[string]*Client
}

// NewRemoteWorkspace 单端点形态：恒用 baseURL（既有测试与单 runner 部署）。
func NewRemoteWorkspace(baseURL, internalToken string, tenantID uint, httpClient *http.Client) *RemoteWorkspace {
	client := NewWorkspaceClient(baseURL, internalToken, httpClient)
	workspace := &RemoteWorkspace{
		base: client, token: strings.TrimSpace(internalToken), httpClient: httpClient,
		tenantID: tenantID, clients: map[string]*Client{},
	}
	if httpClient != nil {
		workspace.httpClient = httpClient
	}
	return workspace
}

// NewRoutedRemoteWorkspace 按 run 归属端点寻址的形态：resolver 返回 run 行
// 记录的端点（空=默认端点 baseURL）。多 runner 部署必须用本构造。
func NewRoutedRemoteWorkspace(baseURL, internalToken string, tenantID uint, httpClient *http.Client, resolver EndpointResolver) *RemoteWorkspace {
	workspace := NewRemoteWorkspace(baseURL, internalToken, tenantID, httpClient)
	workspace.resolver = resolver
	return workspace
}

// clientFor 解析 run 归属端点并返回对应客户端：resolver 未装配或端点为空
// 时回落默认客户端；端点客户端按 URL 缓存复用。
func (w *RemoteWorkspace) clientFor(ctx context.Context, runID string) (*Client, error) {
	if w.resolver == nil {
		return w.base, nil
	}
	endpoint, err := w.resolver(ctx, runID)
	if err != nil {
		return nil, err
	}
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return w.base, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if client, ok := w.clients[endpoint]; ok {
		return client, nil
	}
	client := NewWorkspaceClient(endpoint, w.token, w.httpClient)
	w.clients[endpoint] = client
	return client, nil
}

func (w *RemoteWorkspace) DeleteForRun(ctx context.Context, runID, executionID string) error {
	client, err := w.clientFor(ctx, runID)
	if err != nil {
		return err
	}
	return client.Delete(ctx, executionID)
}

func (w *RemoteWorkspace) Delete(ctx context.Context, executionID string) error {
	return w.base.Delete(ctx, executionID)
}

// RemoteDeveloperPreparer 适配 orchestrator.DeveloperWorkspacePreparer。
type RemoteDeveloperPreparer struct {
	workspace *RemoteWorkspace
}

func NewRemoteDeveloperPreparer(workspace *RemoteWorkspace) *RemoteDeveloperPreparer {
	return &RemoteDeveloperPreparer{workspace: workspace}
}

func (p *RemoteDeveloperPreparer) Prepare(ctx context.Context, request workspace.DeveloperRequest) (*workspace.Manifest, error) {
	client, err := p.workspace.clientFor(ctx, request.RunID)
	if err != nil {
		return nil, err
	}
	result, err := client.PrepareWorkspace(ctx, workspaceproto.PrepareRequest{
		OperationID: "prepare-" + request.RunID, Kind: workspaceproto.KindDeveloperPrepare,
		TenantID: p.workspace.tenantID, TaskID: request.TaskID, RunID: request.RunID, Role: "DeveloperWorker",
		TaskType: request.TaskType, Project: request.Project,
		RepositoryURL: request.RepositoryURL, SourceBranch: request.SourceBranch,
		TargetBranch: request.TargetBranch, DeliveryBranch: request.DeliveryBranch,
		ExpectedHeadSHA: request.ExpectedHeadSHA,
		GitAuthorName:   request.GitAuthorName, GitAuthorEmail: request.GitAuthorEmail,
		Credential: credentialPayload(request.Credential),
	})
	if err != nil {
		return nil, err
	}
	return developerManifestFromPrepare(request, result), nil
}

// RemoteReadOnlyPreparer 适配 orchestrator.ReviewerWorkspacePreparer（reviewer
// 与 tester 共用只读冻结检出）。
type RemoteReadOnlyPreparer struct {
	workspace *RemoteWorkspace
}

func NewRemoteReadOnlyPreparer(workspace *RemoteWorkspace) *RemoteReadOnlyPreparer {
	return &RemoteReadOnlyPreparer{workspace: workspace}
}

func (p *RemoteReadOnlyPreparer) Prepare(ctx context.Context, request workspace.ReviewerRequest) (*workspace.Manifest, error) {
	role := strings.TrimSpace(request.WorkerRole)
	if role == "" {
		role = "ReviewerWorker"
	}
	client, err := p.workspace.clientFor(ctx, request.RunID)
	if err != nil {
		return nil, err
	}
	result, err := client.PrepareWorkspace(ctx, workspaceproto.PrepareRequest{
		OperationID: "prepare-" + request.RunID, Kind: workspaceproto.KindWorkspacePrepare,
		TenantID: p.workspace.tenantID, TaskID: request.TaskID, RunID: request.RunID, Role: role,
		Project: request.Project, RepositoryURL: request.RepositoryURL,
		SourceBranch: request.SourceBranch, TargetBranch: request.TargetBranch,
		HeadSHA: request.HeadSHA, BaseSHA: request.BaseSHA,
		RequestedReviewMode: request.RequestedReviewMode,
		Credential:          credentialPayload(request.Credential),
	})
	if err != nil {
		return nil, err
	}
	return readOnlyManifestFromPrepare(request, role, result), nil
}

// PublishTaskBundle 适配 orchestrator.TaskBundlePublisher：把任务包三段内容
// （协调器共享渲染器生成）经受控通道投递到 run 所在 Runner 的工作区，回传
// Runner 侧工作区路径（两侧路径不共享，执行请求以该值为准）。
func (w *RemoteWorkspace) PublishTaskBundle(ctx context.Context, taskID, runID string, files map[string][]byte) (string, error) {
	client, err := w.clientFor(ctx, runID)
	if err != nil {
		return "", err
	}
	encoded := make(map[string]string, len(files))
	for name, data := range files {
		encoded[name] = string(data)
	}
	result, err := client.PublishTaskBundle(ctx, workspaceproto.TaskBundleRequest{
		OperationID: "bundle-" + runID, TenantID: w.tenantID, TaskID: taskID, RunID: runID, Files: encoded,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.WorkspacePath), nil
}

// CommitDelivery 实现 orchestrator.DeveloperGitFinalizer：受控提交（固定
// 操作 ID，重试返回原可信 SHA，不产生第二笔提交）。owner/generation 随行：
// Runner 侧授权器据此拒绝接管后旧持有者的新请求。
func (w *RemoteWorkspace) CommitDelivery(ctx context.Context, request workspace.DeveloperCommitRequest) (*workspace.DeveloperCommitResult, error) {
	client, err := w.clientFor(ctx, request.RunID)
	if err != nil {
		return nil, err
	}
	commitRequest := workspaceproto.CommitRequest{
		OperationID: "commit-" + request.RunID, TenantID: w.tenantID,
		TaskID: request.TaskID, RunID: request.RunID, TaskType: request.TaskType,
		Message: request.CommitMessage, RepositoryURL: request.RepositoryURL,
		TargetBranch: request.TargetBranch, DeliveryBranch: request.DeliveryBranch,
		BaselineSHA: request.BaselineSHA,
		AuthorName:  request.GitAuthorName, AuthorEmail: request.GitAuthorEmail,
		Owner: request.DeliveryOwner, Generation: request.DeliveryGeneration,
	}
	result, err := client.CommitWorkspace(ctx, commitRequest)
	if err != nil {
		return nil, err
	}
	return &workspace.DeveloperCommitResult{CommitSHA: result.CommitSHA}, nil
}

// PushDelivery 实现 orchestrator.DeveloperGitFinalizer：固定 SHA 推送。
func (w *RemoteWorkspace) ReadDeliveryOperation(ctx context.Context, runID, operationID string) (*workspaceproto.OperationRecord, error) {
	client, err := w.clientFor(ctx, runID)
	if err != nil {
		return nil, err
	}
	record, err := client.WorkspaceOperation(ctx, operationID)
	if err != nil {
		return nil, err
	}
	if record.TenantID != w.tenantID {
		return nil, fmt.Errorf("workspace operation belongs to another tenant")
	}
	return record, nil
}

func (w *RemoteWorkspace) PushDelivery(ctx context.Context, request workspace.DeveloperPushRequest) (*workspace.DeveloperDelivery, error) {
	client, err := w.clientFor(ctx, request.RunID)
	if err != nil {
		return nil, err
	}
	result, err := client.PushWorkspace(ctx, workspaceproto.PushRequest{
		OperationID: "push-" + request.RunID, TenantID: w.tenantID,
		TaskID: request.TaskID, RunID: request.RunID, TaskType: request.TaskType,
		RepositoryURL: request.RepositoryURL, TargetBranch: request.TargetBranch,
		DeliveryBranch: request.DeliveryBranch, CommitSHA: request.CommitSHA, BaselineSHA: request.BaselineSHA,
		Credential: credentialPayload(request.Credential),
		Owner:      request.DeliveryOwner, Generation: request.DeliveryGeneration,
	})
	if err != nil {
		return nil, err
	}
	return &workspace.DeveloperDelivery{
		CommitSHA: result.CommitSHA, ChangedFiles: result.ChangedFiles, DiffSummary: result.DiffSummary,
	}, nil
}

// RemoveRunWorkspace 实现 orchestrator.RunWorkspaceCleaner：清理在 run 归属
// 端点上进行。taskID 必须随行——Runner 侧授权器按完整身份裁决。
func (w *RemoteWorkspace) RemoveRunWorkspace(ctx context.Context, taskID, runID string) error {
	client, err := w.clientFor(ctx, runID)
	if err != nil {
		return err
	}
	_, err = client.CleanupWorkspace(ctx, workspaceproto.CleanupRequest{
		OperationID: "cleanup-" + runID, TenantID: w.tenantID, TaskID: taskID, RunID: runID,
	})
	return err
}

func credentialPayload(credential *workspace.GitCredential) workspaceproto.Credential {
	if credential == nil {
		return workspaceproto.Credential{}
	}
	return workspaceproto.Credential{Username: credential.Username, Token: credential.Token}
}

func developerManifestFromPrepare(request workspace.DeveloperRequest, result *workspaceproto.PrepareResult) *workspace.Manifest {
	return &workspace.Manifest{
		Version: 1, Status: workspace.ManifestStatusReady,
		TaskID: request.TaskID, RunID: request.RunID, Role: "DeveloperWorker", TaskType: request.TaskType,
		Repository: workspace.RepositoryManifest{
			Project: request.Project, Path: "repo", RepositoryURL: request.RepositoryURL,
			InitialHeadSHA: result.BaselineSHA, DeliveryBranch: result.DeliveryBranch,
			SourceBranch: request.SourceBranch, TargetBranch: request.TargetBranch,
			HeadSHA: result.BaselineSHA,
		},
		PreparedAt: time.Now().UTC(),
		// 独立文件系统下以 Runner 返回值为准：执行请求的 WorkspacePath 必须
		// 指向 Runner 本机路径（协调器本地路径在那边不存在）。
		RunnerWorkspacePath: strings.TrimSpace(result.WorkspacePath),
	}
}

func readOnlyManifestFromPrepare(request workspace.ReviewerRequest, role string, result *workspaceproto.PrepareResult) *workspace.Manifest {
	return &workspace.Manifest{
		Version: 1, Status: workspace.ManifestStatusReady,
		TaskID: request.TaskID, RunID: request.RunID, Role: role,
		Repository: workspace.RepositoryManifest{
			Project: request.Project, Path: "repo", RepositoryURL: request.RepositoryURL,
			InitialHeadSHA: result.BaselineSHA, SourceBranch: request.SourceBranch,
			TargetBranch: request.TargetBranch, HeadSHA: result.BaselineSHA,
			BaseSHA: request.BaseSHA, RequestedReviewMode: request.RequestedReviewMode,
		},
		PreparedAt:          time.Now().UTC(),
		RunnerWorkspacePath: strings.TrimSpace(result.WorkspacePath),
	}
}
