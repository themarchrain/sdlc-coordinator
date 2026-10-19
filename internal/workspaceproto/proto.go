package workspaceproto

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// 操作种类：同一 operation_id 空间内用于冲突判定。
const (
	KindDeveloperPrepare = "developer_prepare"
	KindWorkspacePrepare = "workspace_prepare"
	KindTaskBundle       = "task_bundle"
	KindDeveloperCommit  = "developer_commit"
	KindDeveloperPush    = "developer_push"
	KindCleanup          = "cleanup"
)

// Credential 是每仓 git 凭据（部署级注入，不经日志/响应回显）。
type Credential struct {
	Username string `json:"username,omitempty"`
	Token    string `json:"token,omitempty"`
}

type PrepareRequest struct {
	OperationID         string     `json:"operation_id"`
	Kind                string     `json:"kind"`
	TenantID            uint       `json:"tenant_id"`
	TaskID              string     `json:"task_id"`
	RunID               string     `json:"run_id"`
	Role                string     `json:"role"`
	TaskType            string     `json:"task_type,omitempty"`
	Project             string     `json:"project,omitempty"`
	RepositoryURL       string     `json:"repository_url"`
	SourceBranch        string     `json:"source_branch,omitempty"`
	TargetBranch        string     `json:"target_branch,omitempty"`
	DeliveryBranch      string     `json:"delivery_branch,omitempty"`
	HeadSHA             string     `json:"head_sha,omitempty"`
	BaseSHA             string     `json:"base_sha,omitempty"`
	ExpectedHeadSHA     string     `json:"expected_head_sha,omitempty"`
	RequestedReviewMode string     `json:"requested_review_mode,omitempty"`
	GitAuthorName       string     `json:"git_author_name,omitempty"`
	GitAuthorEmail      string     `json:"git_author_email,omitempty"`
	Credential          Credential `json:"credential"`
}

// PrepareResult 是工作区准备结果：BaselineSHA 是可信基线（developer=初始
// HEAD；只读=冻结头）。Adopted=true 表示复用既有在途工作区（未重做检出）。
type PrepareResult struct {
	OperationID    string `json:"operation_id"`
	RunID          string `json:"run_id"`
	Adopted        bool   `json:"adopted"`
	WorkspacePath  string `json:"workspace_path"`
	BaselineSHA    string `json:"baseline_sha"`
	DeliveryBranch string `json:"delivery_branch,omitempty"`
}

// CommitRequest 是受控提交请求：消息由协调器校验后传入，Runner 只执行。
// Owner/Generation 是协调器交付执行权的当前持有身份（developer-delivery
// 租约），随请求贯穿到 Runner 的授权确认：接管发生后旧持有者的请求被拒。
type CommitRequest struct {
	OperationID    string     `json:"operation_id"`
	TenantID       uint       `json:"tenant_id"`
	TaskID         string     `json:"task_id"`
	RunID          string     `json:"run_id"`
	TaskType       string     `json:"task_type"`
	Message        string     `json:"message"`
	RepositoryURL  string     `json:"repository_url"`
	TargetBranch   string     `json:"target_branch"`
	DeliveryBranch string     `json:"delivery_branch"`
	BaselineSHA    string     `json:"baseline_sha"`
	AuthorName     string     `json:"author_name"`
	AuthorEmail    string     `json:"author_email"`
	Credential     Credential `json:"credential"`
	Owner          string     `json:"owner,omitempty"`
	Generation     uint64     `json:"generation,omitempty"`
}

// CommitResult 是可信提交结果（固定操作 ID 回读）。
type CommitResult struct {
	OperationID string `json:"operation_id"`
	CommitSHA   string `json:"commit_sha"`
}

// PushRequest 是固定 SHA 推送请求：协调器必须先把可信 CommitSHA 条件写入
// MySQL，再发起本请求；Runner 以固定 SHA 推送并做远端漂移检查。Owner/
// Generation 语义同 CommitRequest（授权确认据其拒绝接管后的旧持有者）。
type PushRequest struct {
	OperationID    string     `json:"operation_id"`
	TenantID       uint       `json:"tenant_id"`
	TaskID         string     `json:"task_id"`
	RunID          string     `json:"run_id"`
	TaskType       string     `json:"task_type"`
	RepositoryURL  string     `json:"repository_url"`
	TargetBranch   string     `json:"target_branch"`
	DeliveryBranch string     `json:"delivery_branch"`
	CommitSHA      string     `json:"commit_sha"`
	BaselineSHA    string     `json:"baseline_sha"`
	Credential     Credential `json:"credential"`
	Owner          string     `json:"owner,omitempty"`
	Generation     uint64     `json:"generation,omitempty"`
}

// PushResult 是推送结果（远端头回读确认后的值）。
type PushResult struct {
	OperationID   string   `json:"operation_id"`
	CommitSHA     string   `json:"commit_sha"`
	RemoteHeadSHA string   `json:"remote_head_sha"`
	ChangedFiles  []string `json:"changed_files,omitempty"`
	DiffSummary   string   `json:"diff_summary,omitempty"`
}

// CleanupRequest 是 run 工作区清理请求（仅协调器在 run 终态后发起）。
type CleanupRequest struct {
	OperationID string `json:"operation_id"`
	TenantID    uint   `json:"tenant_id"`
	TaskID      string `json:"task_id"`
	RunID       string `json:"run_id"`
}

// CleanupResult 是清理结果。
type CleanupResult struct {
	OperationID string `json:"operation_id"`
}

// TaskBundleRequest 把任务包投递到运行该 run 的 Runner 工作区：协调器与
// Runner 各自独占文件系统时，任务包必须走受控通道而不是协调器本地路径。
// Files 是任务包三段内容（文件名 → 完整内容字节），由协调器以共享渲染器
// （internal/agent.RenderTaskBundle）生成，Runner 逐字节落盘——两侧不重
// 编码，内容天然一致。文件名限定渲染器的三个常量名（Runner 侧白名单校验）。
type TaskBundleRequest struct {
	OperationID string            `json:"operation_id"`
	TenantID    uint              `json:"tenant_id"`
	TaskID      string            `json:"task_id"`
	RunID       string            `json:"run_id"`
	Files       map[string]string `json:"files"`
}

// TaskBundleResult 是任务包投递结果：回报 Runner 侧工作区路径，协调器以
// 该值作为执行请求的 WorkspacePath（独立文件系统下两侧路径不共享）。
type TaskBundleResult struct {
	OperationID   string `json:"operation_id"`
	WorkspacePath string `json:"workspace_path"`
}

type AuthorizationRequest struct {
	OperationID string `json:"operation_id"`
	Kind        string `json:"kind"`
	TenantID    uint   `json:"tenant_id"`
	TaskID      string `json:"task_id"`
	RunID       string `json:"run_id"`
	PayloadHash string `json:"payload_hash"`
	// Owner/Generation 是交付执行权的持有身份（commit/push 请求随行；其他
	// 操作留空）。授权方按它拒绝接管后旧持有者的新请求。
	Owner      string `json:"owner,omitempty"`
	Generation uint64 `json:"generation,omitempty"`
}

// AuthorizationDecision 是授权确认结果（拒绝时带可查询原因）。
type AuthorizationDecision struct {
	Authorized bool   `json:"authorized"`
	Reason     string `json:"reason,omitempty"`
}

// OperationRecord 是操作受理记录的对外视图（回读/诊断）。
type OperationRecord struct {
	OperationID  string          `json:"operation_id"`
	Kind         string          `json:"kind"`
	TenantID     uint            `json:"tenant_id"`
	TaskID       string          `json:"task_id"`
	RunID        string          `json:"run_id"`
	Status       string          `json:"status"`
	PayloadHash  string          `json:"payload_hash"`
	Result       json.RawMessage `json:"result,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
}

// PayloadHash 计算请求的规范化载荷哈希；凭据字段被规范化排除（重试允许
// 轮换部署级凭据），操作 ID 本身也不参与（ID 是索引不是载荷）。
func PayloadHash(value any) (string, error) {
	var normalized any
	switch request := value.(type) {
	case PrepareRequest:
		request.OperationID = ""
		request.Credential = Credential{}
		normalized = request
	case CommitRequest:
		request.OperationID = ""
		request.Credential = Credential{}
		request.Owner = ""
		request.Generation = 0
		normalized = request
	case PushRequest:
		request.OperationID = ""
		request.Credential = Credential{}
		request.Owner = ""
		request.Generation = 0
		normalized = request
	case CleanupRequest:
		request.OperationID = ""
		normalized = request
	case TaskBundleRequest:
		request.OperationID = ""
		normalized = request
	default:
		normalized = value
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("encode workspace operation payload: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
