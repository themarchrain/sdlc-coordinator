package codehost

import (
	"context"
	"errors"
)

type Backend interface {
	CreateIssue(context.Context, string, CreateIssueRequest) (*Issue, error)
	CreateMergeRequest(context.Context, string, CreateMergeRequestRequest) (*MergeRequest, error)
	CreateIssueNote(context.Context, string, int, CreateNoteRequest) (*Note, error)
	CreateMergeRequestNote(context.Context, string, int, CreateNoteRequest) (*Note, error)
	SetIssueLabels(context.Context, string, int, SetLabelsRequest) (*Issue, error)
	SetMergeRequestLabels(context.Context, string, int, SetLabelsRequest) (*MergeRequest, error)
}

type Reconciler interface {
	ReconcileWriteback(context.Context, ReconcileRequest) (ReconcileResult, error)
}

type IssueState struct {
	Closed bool
}

// IssueStateReader 读取单个 issue 的状态,供需求模块做完成信号镜像。
// 不并入 Backend,避免破坏既有实现与 mock。
type IssueStateReader interface {
	GetIssueState(ctx context.Context, projectID string, iid int) (IssueState, error)
}

// MergeRequestReader 读取单个 MR 详情(分支与 HEAD);可选能力接口,与
// IssueStateReader 同模式,不并入 Backend,避免破坏既有实现与 mock。
// repo 参数按平台惯例:GitLab 传 project id 或 path-with-namespace,
// Gitea/GitHub 传 owner/repo。
type MergeRequestReader interface {
	GetMergeRequest(ctx context.Context, repo string, mrIID int) (*MergeRequest, error)
}

type MergeRequestLabelAdder interface {
	AddMergeRequestLabels(context.Context, string, int, SetLabelsRequest) (*MergeRequest, error)
}

// ErrMergeHeadMismatch 表示合并调用携带的头部 SHA 断言与平台当前头部不符
// (GitHub 409 / Gitea、GitLab 同义拒合):头部已漂移,证据过期,不可重试,
// 只能对新头部重新走证据链。
var ErrMergeHeadMismatch = errors.New("merge head mismatch")

// ErrMergeRejected 表示平台明确拒绝本次合并(保护规则未满足、冲突、无权限
// 等):原因随错误携带,由调用方回落人工,系统不绕平台规则。
var ErrMergeRejected = errors.New("merge request rejected")

// MergeRequestMerger 执行 MR/PR 合并,是写回能力之外的可选能力接口(与
// IssueStateReader/MergeRequestReader 同模式,不并入 Backend,避免破坏既有
// 实现与 mock)。ExpectedHeadSHA 是合并时序的最后一道防线:调用时声明
// 声明只合头部等于此 SHA 的 MR,头部漂移即被平台拒合。
type MergeRequestMerger interface {
	MergePullRequest(ctx context.Context, repo string, mrIID int, request MergePullRequestRequest) (*MergePullRequestResult, error)
}

type MergePullRequestRequest struct {
	ExpectedHeadSHA string
	MergeMethod     string // 空串=平台默认;github: merge|squash|rebase
	CommitTitle     string
	CommitMessage   string
}

type MergePullRequestResult struct {
	Merged          bool
	MergedCommitSHA string
}

type ReconcileState string

const (
	ReconcileStateFound   ReconcileState = "found"
	ReconcileStateAbsent  ReconcileState = "absent"
	ReconcileStateUnknown ReconcileState = "unknown"
)

type ReconcileRequest struct {
	DedupeKey       string
	OperationMarker string
	Kind            string
	ProjectID       string
	TargetType      string
	TargetIID       int
	SourceBranch    string
	TargetBranch    string
	Labels          []string
}

type ReconcileResult struct {
	State       ReconcileState
	ExternalID  string
	ExternalURL string
	Response    any
}

type CreateIssueRequest struct {
	Title       string
	Description string
	Labels      []string
}

type CreateMergeRequestRequest struct {
	Title              string
	Description        string
	SourceBranch       string
	TargetBranch       string
	RemoveSourceBranch bool
	Labels             []string
}

type CreateNoteRequest struct {
	Body string
}

type SetLabelsRequest struct {
	Labels []string
}

type Issue struct {
	IID    int    `json:"iid"`
	WebURL string `json:"web_url"`
}

type MergeRequest struct {
	IID          int    `json:"iid"`
	WebURL       string `json:"web_url"`
	SourceBranch string `json:"source_branch,omitempty"`
	TargetBranch string `json:"target_branch,omitempty"`
	HeadSHA      string `json:"sha,omitempty"`
	// Merged/MergeCommitSHA 透出平台侧真实合并状态,供自动合并门禁在拒绝
	// 或歧义之后回读真相(合并已生效则按 merged 补审计,不误记 fallback)。
	// MergeRequestMerger 的实现方必须同步透出这两项,否则拒绝回读与
	// 歧义恢复两条路径会静默失效。
	Merged         bool   `json:"merged,omitempty"`
	MergeCommitSHA string `json:"merge_commit_sha,omitempty"`
}

type Note struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
}
