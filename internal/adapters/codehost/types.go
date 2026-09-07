package codehost

import "context"

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

type MergeRequestLabelAdder interface {
	AddMergeRequestLabels(context.Context, string, int, SetLabelsRequest) (*MergeRequest, error)
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
	IID    int    `json:"iid"`
	WebURL string `json:"web_url"`
}

type Note struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
}
