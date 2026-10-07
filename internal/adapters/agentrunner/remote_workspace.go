package agentrunner

import (
	"context"
	"net/http"
	"strings"
	"time"

	"sdlc-coordinator/internal/workspace"
	"sdlc-coordinator/internal/workspaceproto"
)

type RemoteWorkspace struct {
	client   *Client
	tenantID uint
}

func NewRemoteWorkspace(baseURL, internalToken string, tenantID uint, httpClient *http.Client) *RemoteWorkspace {
	return &RemoteWorkspace{client: NewWorkspaceClient(baseURL, internalToken, httpClient), tenantID: tenantID}
}

// RemoteDeveloperPreparer 适配 orchestrator.DeveloperWorkspacePreparer。
type RemoteDeveloperPreparer struct {
	workspace *RemoteWorkspace
}

func NewRemoteDeveloperPreparer(workspace *RemoteWorkspace) *RemoteDeveloperPreparer {
	return &RemoteDeveloperPreparer{workspace: workspace}
}

func (p *RemoteDeveloperPreparer) Prepare(ctx context.Context, request workspace.DeveloperRequest) (*workspace.Manifest, error) {
	result, err := p.workspace.client.PrepareWorkspace(ctx, workspaceproto.PrepareRequest{
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
	result, err := p.workspace.client.PrepareWorkspace(ctx, workspaceproto.PrepareRequest{
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

// CommitDelivery 实现 orchestrator.DeveloperGitFinalizer：受控提交（固定
// 操作 ID，重试返回原可信 SHA，不产生第二笔提交）。
func (w *RemoteWorkspace) CommitDelivery(ctx context.Context, request workspace.DeveloperCommitRequest) (*workspace.DeveloperCommitResult, error) {
	result, err := w.client.CommitWorkspace(ctx, workspaceproto.CommitRequest{
		OperationID: "commit-" + request.RunID, TenantID: w.tenantID,
		TaskID: request.TaskID, RunID: request.RunID, TaskType: request.TaskType,
		Message: request.CommitMessage, RepositoryURL: request.RepositoryURL,
		TargetBranch: request.TargetBranch, DeliveryBranch: request.DeliveryBranch,
		BaselineSHA: request.BaselineSHA,
		AuthorName:  request.GitAuthorName, AuthorEmail: request.GitAuthorEmail,
	})
	if err != nil {
		return nil, err
	}
	return &workspace.DeveloperCommitResult{CommitSHA: result.CommitSHA}, nil
}

// PushDelivery 实现 orchestrator.DeveloperGitFinalizer：固定 SHA 推送。
func (w *RemoteWorkspace) PushDelivery(ctx context.Context, request workspace.DeveloperPushRequest) (*workspace.DeveloperDelivery, error) {
	result, err := w.client.PushWorkspace(ctx, workspaceproto.PushRequest{
		OperationID: "push-" + request.RunID, TenantID: w.tenantID,
		TaskID: request.TaskID, RunID: request.RunID, TaskType: request.TaskType,
		RepositoryURL: request.RepositoryURL, TargetBranch: request.TargetBranch,
		DeliveryBranch: request.DeliveryBranch, CommitSHA: request.CommitSHA, BaselineSHA: request.BaselineSHA,
		Credential: credentialPayload(request.Credential),
	})
	if err != nil {
		return nil, err
	}
	return &workspace.DeveloperDelivery{
		CommitSHA: result.CommitSHA, ChangedFiles: result.ChangedFiles, DiffSummary: result.DiffSummary,
	}, nil
}

// RemoveRunWorkspace 实现 orchestrator.RunWorkspaceCleaner：清理在 Runner
// 本机进行（协调器不触碰 Runner 工作区目录）。
func (w *RemoteWorkspace) RemoveRunWorkspace(ctx context.Context, runID string) error {
	_, err := w.client.CleanupWorkspace(ctx, workspaceproto.CleanupRequest{
		OperationID: "cleanup-" + runID, TenantID: w.tenantID, RunID: runID,
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
		PreparedAt: time.Now().UTC(),
	}
}
