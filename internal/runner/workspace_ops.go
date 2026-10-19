package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/workspace"
	"sdlc-coordinator/internal/workspaceproto"
)

var (
	// ErrOperationConflict 报告同操作 ID 的载荷/归属与既有记录不一致
	//（含跨 run 复用同一 ID）。
	ErrOperationConflict = errors.New("workspace operation conflicts with an existing record")
	// ErrOperationInFlight 报告同操作 ID 正在执行中（可稍后重试）。
	ErrOperationInFlight = errors.New("workspace operation is already in flight")
	// ErrOperationNotFound 报告操作记录不存在。
	ErrOperationNotFound = errors.New("workspace operation not found")
	// ErrServiceNotAccepting 报告 Runner 已停止接纳新的工作区操作（退出中）。
	ErrServiceNotAccepting = errors.New("runner is not accepting new workspace operations")
	// ErrTooManyOperations 报告并发容量已满：本次未被接纳，可稍后重试。
	ErrTooManyOperations = errors.New("runner workspace operation capacity is exhausted")
	// ErrNotAuthorized 报告操作在执行前未取得当前授权确认（未配置确认通道
	// 或确认方明确拒绝）：状态改变类操作 fail-closed。
	ErrNotAuthorized = errors.New("workspace operation is not currently authorized")
)

// ExecutionRight 是状态改变类操作随请求携带的执行权身份（交付租约
// owner+generation）；授权确认据此拒绝接管后旧持有者的新请求。零值表示
// 请求未携带执行权身份（非交付类操作）。
type ExecutionRight struct {
	Owner      string
	Generation uint64
}

const (
	workspaceOpStatusInFlight  = "in_flight"
	workspaceOpStatusCompleted = "completed"
	workspaceOpStatusFailed    = "failed"

	defaultWorkspaceMaxInFlight = 4
	cliIsolationEnv             = "AGENT_RUNNER_CLI_LAUNCHER"
)

var operationIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

type workspaceOperationRecord struct {
	OperationID  string          `json:"operation_id"`
	Kind         string          `json:"kind"`
	TenantID     uint            `json:"tenant_id"`
	TaskID       string          `json:"task_id"`
	RunID        string          `json:"run_id"`
	PayloadHash  string          `json:"payload_hash"`
	Status       string          `json:"status"`
	ResultJSON   json.RawMessage `json:"result_json,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// WorkspaceOpsConfig 配置 Runner 侧受控工作区操作面。
type WorkspaceOpsConfig struct {
	// DataRoot 是 Runner 持久根；操作记录与工作区都落在其下（Agent 不可写）。
	DataRoot string
	// WorkspaceRoot 默认 <DataRoot>/workspaces。
	WorkspaceRoot string
	// MaxInFlight 是并发接纳上限（默认 4）；满员时新操作未被接纳。
	MaxInFlight int
	// Authorize 是当前授权确认（commit/push/cleanup 执行前调用，任何实例
	// 基于共享库裁决）；nil 表示未配置——这些操作将被 fail-closed 拒绝。
	Authorize func(context.Context, workspaceproto.AuthorizationRequest) (workspaceproto.AuthorizationDecision, error)
}

// WorkspaceOps 是 Runner 侧受控工作区操作面：窄操作（prepare/commit/push/
// cleanup）、固定操作 ID 的持久幂等记录、按 run 串行与有界并发接纳。
type WorkspaceOps struct {
	mu            sync.Mutex
	opsRoot       string
	workspaceRoot string
	maxInFlight   int
	inFlight      int
	accepting     bool
	now           func() time.Time
	runLocks      map[string]*sync.Mutex

	prepareDeveloper *workspace.DeveloperPreparer
	prepareReadOnly  *workspace.ReviewerPreparer
	finalizer        *workspace.DeveloperFinalizer

	// authorize 是当前授权确认（状态改变类操作执行前调用）；nil 表示确认
	// 通道未配置——状态改变类操作 fail-closed 拒绝。
	authorize func(context.Context, workspaceproto.AuthorizationRequest) (workspaceproto.AuthorizationDecision, error)

	// testBeforeExecute 仅测试注入（容量/停止接纳用例定点卡住执行段）。
	testBeforeExecute func()
}

func NewWorkspaceOps(config WorkspaceOpsConfig) (*WorkspaceOps, error) {
	dataRoot := strings.TrimSpace(config.DataRoot)
	if dataRoot == "" {
		return nil, errors.New("runner data root is required for workspace operations")
	}
	workspaceRoot := strings.TrimSpace(config.WorkspaceRoot)
	if workspaceRoot == "" {
		workspaceRoot = filepath.Join(dataRoot, "workspaces")
	}
	maxInFlight := config.MaxInFlight
	if maxInFlight <= 0 {
		maxInFlight = defaultWorkspaceMaxInFlight
	}
	opsRoot := filepath.Join(dataRoot, "operations")
	if err := os.MkdirAll(opsRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create runner operations directory: %w", err)
	}
	if err := secureOperationsDirectory(opsRoot); err != nil {
		return nil, fmt.Errorf("secure runner operations directory: %w", err)
	}
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create runner workspace root: %w", err)
	}
	if cliIdentityIsolationEnabled() {
		if err := secureWorkspaceRunRoots(workspaceRoot); err != nil {
			return nil, fmt.Errorf("secure runner workspaces: %w", err)
		}
	}
	ops := &WorkspaceOps{
		opsRoot:          opsRoot,
		workspaceRoot:    workspaceRoot,
		maxInFlight:      maxInFlight,
		accepting:        true,
		now:              time.Now,
		runLocks:         map[string]*sync.Mutex{},
		authorize:        config.Authorize,
		prepareDeveloper: workspace.NewDeveloperPreparer(),
		prepareReadOnly:  workspace.NewReviewerPreparer(),
		finalizer:        workspace.NewDeveloperFinalizer(),
	}
	if err := ops.recoverInterruptedRecords(); err != nil {
		return nil, err
	}
	return ops, nil
}

func cliIdentityIsolationEnabled() bool {
	return strings.TrimSpace(os.Getenv(cliIsolationEnv)) != ""
}

func (o *WorkspaceOps) recoverInterruptedRecords() error {
	entries, err := os.ReadDir(o.opsRoot)
	if err != nil {
		return fmt.Errorf("scan operation records: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		record, err := o.loadRecordLocked(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return err
		}
		if record == nil || record.Status != workspaceOpStatusInFlight {
			continue
		}
		record.Status = workspaceOpStatusFailed
		record.ErrorMessage = "runner restarted while the operation was in flight; the operation must be retried explicitly"
		record.UpdatedAt = o.now().UTC()
		if err := o.writeRecordLocked(record); err != nil {
			return err
		}
	}
	return nil
}

// StopAccepting 停止接纳新操作（退出前调用）；已受理操作继续执行并可回读。
func (o *WorkspaceOps) StopAccepting() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.accepting = false
}

// WaitIdle 等待在途操作结束（有界，由调用方给 ctx 期限）。
func (o *WorkspaceOps) WaitIdle(ctx context.Context) {
	for {
		o.mu.Lock()
		active := o.inFlight
		o.mu.Unlock()
		if active == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// InFlight 返回当前在途操作数（观测/测试用）。
func (o *WorkspaceOps) InFlight() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.inFlight
}

// WorkspaceRoot 返回 Runner 本机工作区根（协调器不依赖此路径，仅诊断用）。
func (o *WorkspaceOps) WorkspaceRoot() string { return o.workspaceRoot }

// PrepareWorkspace 准备工作区（developer 写工作区 / reviewer、tester 只读冻结
// 检出）：重复准备复用既有在途工作区（不重做检出、不覆盖在途改动）。
func (o *WorkspaceOps) PrepareWorkspace(ctx context.Context, request workspaceproto.PrepareRequest) (workspaceproto.PrepareResult, error) {
	hash, err := workspaceproto.PayloadHash(request)
	if err != nil {
		return workspaceproto.PrepareResult{}, err
	}
	kind := strings.TrimSpace(request.Kind)
	if kind == "" {
		kind = workspaceproto.KindDeveloperPrepare
	}
	result, _, err := runWorkspaceOp(ctx, o, kind, request.OperationID, request.TenantID, request.TaskID, request.RunID, hash, []string{request.Credential.Token},
		func(ctx context.Context) (workspaceproto.PrepareResult, error) {
			if kind == workspaceproto.KindDeveloperPrepare {
				return o.prepareDeveloperWorkspace(ctx, request)
			}
			return o.prepareReadOnlyWorkspace(ctx, request)
		})
	return result, err
}

func (o *WorkspaceOps) prepareDeveloperWorkspace(ctx context.Context, request workspaceproto.PrepareRequest) (workspaceproto.PrepareResult, error) {
	runRoot := filepath.Join(o.workspaceRoot, "runs", strings.TrimSpace(request.RunID))
	_, statErr := os.Stat(runRoot)
	preExisted := statErr == nil

	requested := workspace.DeveloperRequest{
		WorkspaceRoot: o.workspaceRoot, PrivateRunRoot: cliIdentityIsolationEnabled(),
		TaskID: request.TaskID, RunID: request.RunID, TaskType: request.TaskType,
		Project: request.Project, RepositoryURL: request.RepositoryURL,
		SourceBranch: request.SourceBranch, TargetBranch: request.TargetBranch,
		DeliveryBranch: request.DeliveryBranch, ExpectedHeadSHA: request.ExpectedHeadSHA,
		Credential:    credentialOrNil(request.Credential),
		GitAuthorName: request.GitAuthorName, GitAuthorEmail: request.GitAuthorEmail,
	}
	manifest, err := o.prepareDeveloper.Prepare(ctx, requested)
	if err != nil {
		return workspaceproto.PrepareResult{}, err
	}
	return workspaceproto.PrepareResult{
		OperationID: request.OperationID, RunID: request.RunID, Adopted: preExisted,
		WorkspacePath:  runRoot,
		BaselineSHA:    strings.ToLower(strings.TrimSpace(manifest.Repository.InitialHeadSHA)),
		DeliveryBranch: manifest.Repository.DeliveryBranch,
	}, nil
}

func (o *WorkspaceOps) prepareReadOnlyWorkspace(ctx context.Context, request workspaceproto.PrepareRequest) (workspaceproto.PrepareResult, error) {
	runRoot := filepath.Join(o.workspaceRoot, "runs", strings.TrimSpace(request.RunID))
	_, statErr := os.Stat(runRoot)
	preExisted := statErr == nil

	requested := workspace.ReviewerRequest{
		WorkspaceRoot: o.workspaceRoot, PrivateRunRoot: cliIdentityIsolationEnabled(),
		TaskID: request.TaskID, RunID: request.RunID,
		Project: request.Project, RepositoryURL: request.RepositoryURL,
		SourceBranch: request.SourceBranch, TargetBranch: request.TargetBranch,
		HeadSHA: request.HeadSHA, BaseSHA: request.BaseSHA,
		RequestedReviewMode: request.RequestedReviewMode,
		Credential:          credentialOrNil(request.Credential),
		WorkerRole:          request.Role,
	}
	manifest, err := o.prepareReadOnly.Prepare(ctx, requested)
	if err != nil {
		return workspaceproto.PrepareResult{}, err
	}
	baseline := strings.TrimSpace(manifest.Repository.HeadSHA)
	if baseline == "" {
		baseline = manifest.Repository.InitialHeadSHA
	}
	return workspaceproto.PrepareResult{
		OperationID: request.OperationID, RunID: request.RunID, Adopted: preExisted,
		WorkspacePath: runRoot, BaselineSHA: strings.ToLower(baseline),
	}, nil
}

// CommitWorkspace 执行受控提交：消息与身份由协调器传入，提交必须是受信
// 基线的直接子提交，且提交后回读校验消息与身份。
func (o *WorkspaceOps) CommitWorkspace(ctx context.Context, request workspaceproto.CommitRequest) (workspaceproto.CommitResult, error) {
	hash, err := workspaceproto.PayloadHash(request)
	if err != nil {
		return workspaceproto.CommitResult{}, err
	}
	result, _, err := runWorkspaceOp(ctx, o, workspaceproto.KindDeveloperCommit, request.OperationID, request.TenantID, request.TaskID, request.RunID, hash, []string{request.Credential.Token},
		func(ctx context.Context) (workspaceproto.CommitResult, error) {
			commitResult, err := o.finalizer.CommitDelivery(ctx, workspace.DeveloperCommitRequest{
				WorkspaceRoot: o.workspaceRoot, TaskID: request.TaskID, RunID: request.RunID, TaskType: request.TaskType,
				CommitMessage: request.Message, GitAuthorName: request.AuthorName, GitAuthorEmail: request.AuthorEmail,
				RepositoryURL: request.RepositoryURL, TargetBranch: request.TargetBranch,
				DeliveryBranch: request.DeliveryBranch, BaselineSHA: request.BaselineSHA,
			})
			if err != nil {
				return workspaceproto.CommitResult{}, err
			}
			return workspaceproto.CommitResult{OperationID: request.OperationID, CommitSHA: commitResult.CommitSHA}, nil
		}, ExecutionRight{Owner: request.Owner, Generation: request.Generation})
	return result, err
}

// PushWorkspace 以固定 SHA 推送（远端漂移检查、幂等已是该 SHA 即成功）。
func (o *WorkspaceOps) PushWorkspace(ctx context.Context, request workspaceproto.PushRequest) (workspaceproto.PushResult, error) {
	hash, err := workspaceproto.PayloadHash(request)
	if err != nil {
		return workspaceproto.PushResult{}, err
	}
	result, _, err := runWorkspaceOp(ctx, o, workspaceproto.KindDeveloperPush, request.OperationID, request.TenantID, request.TaskID, request.RunID, hash, []string{request.Credential.Token},
		func(ctx context.Context) (workspaceproto.PushResult, error) {
			delivery, err := o.finalizer.PushDelivery(ctx, workspace.DeveloperPushRequest{
				WorkspaceRoot: o.workspaceRoot, TaskID: request.TaskID, RunID: request.RunID, TaskType: request.TaskType,
				RepositoryURL: request.RepositoryURL, TargetBranch: request.TargetBranch,
				DeliveryBranch: request.DeliveryBranch, CommitSHA: request.CommitSHA, BaselineSHA: request.BaselineSHA,
				Credential: credentialOrNil(request.Credential),
			})
			if err != nil {
				return workspaceproto.PushResult{}, err
			}
			return workspaceproto.PushResult{
				OperationID: request.OperationID, CommitSHA: delivery.CommitSHA,
				RemoteHeadSHA: delivery.CommitSHA, ChangedFiles: delivery.ChangedFiles, DiffSummary: delivery.DiffSummary,
			}, nil
		}, ExecutionRight{Owner: request.Owner, Generation: request.Generation})
	return result, err
}

// taskBundleFileNames 是任务包允许落盘的文件名白名单（与协调器共享渲染器
// internal/agent.RenderTaskBundle 的三个常量对应）：绝不允许请求方指定
// 任意路径组件，防目录穿越。
var taskBundleFileNames = []string{"task-context.json", "result-schema.json", "prompt.md"}

// PublishTaskBundle 把任务包投递到本 Runner 的 run 工作区：协调器与 Runner
// 各自独占文件系统时，任务包不走协调器本地路径而是经受控通道落盘。内容
// 由协调器以共享渲染器生成、逐字节投递（不做二次编码）；run 工作区必须先
// 经 prepare 建立，未准备即拒绝（fail-closed，不隐式创建目录）。
func (o *WorkspaceOps) PublishTaskBundle(ctx context.Context, request workspaceproto.TaskBundleRequest) (workspaceproto.TaskBundleResult, error) {
	if missing := missingTaskBundleFiles(request.Files); len(missing) > 0 {
		return workspaceproto.TaskBundleResult{}, fmt.Errorf("task bundle is missing required files: %s", strings.Join(missing, ", "))
	}
	for name := range request.Files {
		if !taskBundleFileAllowed(name) {
			return workspaceproto.TaskBundleResult{}, fmt.Errorf("task bundle file %q is not allowed", name)
		}
	}
	hash, err := workspaceproto.PayloadHash(request)
	if err != nil {
		return workspaceproto.TaskBundleResult{}, err
	}
	runRoot := filepath.Join(o.workspaceRoot, "runs", strings.TrimSpace(request.RunID))
	result, _, err := runWorkspaceOp(ctx, o, workspaceproto.KindTaskBundle, request.OperationID, request.TenantID, request.TaskID, request.RunID, hash, nil,
		func(ctx context.Context) (workspaceproto.TaskBundleResult, error) {
			info, statErr := os.Stat(runRoot)
			if statErr != nil || !info.IsDir() {
				return workspaceproto.TaskBundleResult{}, fmt.Errorf("run workspace %s is not prepared on this runner", request.RunID)
			}
			// 白名单顺序落盘（map 迭代序不定，固定顺序便于复现与诊断）。
			for _, name := range taskBundleFileNames {
				content, ok := request.Files[name]
				if !ok {
					continue
				}
				if err := writeTaskBundleFileAtomic(filepath.Join(runRoot, name), []byte(content)); err != nil {
					return workspaceproto.TaskBundleResult{}, err
				}
			}
			return workspaceproto.TaskBundleResult{OperationID: request.OperationID, WorkspacePath: runRoot}, nil
		})
	return result, err
}

func missingTaskBundleFiles(files map[string]string) []string {
	var missing []string
	for _, name := range taskBundleFileNames {
		if _, ok := files[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

func taskBundleFileAllowed(name string) bool {
	for _, allowed := range taskBundleFileNames {
		if name == allowed {
			return true
		}
	}
	return false
}

// writeTaskBundleFileAtomic 与协调器本机形态同规：临时文件 0600 落盘后显式
// 放宽到 0666 再发布——CLI 以非特权用户运行，文件必须可读。
func writeTaskBundleFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary %s: %w", filepath.Base(path), err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary %s: %w", filepath.Base(path), err)
	}
	if err := os.Chmod(tmpPath, 0o666); err != nil {
		return fmt.Errorf("share temporary %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("publish %s: %w", filepath.Base(path), err)
	}
	return nil
}

// CleanupWorkspace 清理 run 工作区（仅协调器在 run 终态后发起）。
func (o *WorkspaceOps) CleanupWorkspace(ctx context.Context, request workspaceproto.CleanupRequest) (workspaceproto.CleanupResult, error) {
	hash, err := workspaceproto.PayloadHash(request)
	if err != nil {
		return workspaceproto.CleanupResult{}, err
	}
	result, _, err := runWorkspaceOp(ctx, o, workspaceproto.KindCleanup, request.OperationID, request.TenantID, request.TaskID, request.RunID, hash, nil,
		func(ctx context.Context) (workspaceproto.CleanupResult, error) {
			if err := workspace.RemoveRunDirectory(o.workspaceRoot, request.RunID); err != nil {
				return workspaceproto.CleanupResult{}, err
			}
			return workspaceproto.CleanupResult{OperationID: request.OperationID}, nil
		})
	return result, err
}

// GetOperation 回读操作记录（重放/对账/诊断）。
func (o *WorkspaceOps) GetOperation(operationID string) (*workspaceproto.OperationRecord, error) {
	if !operationIDPattern.MatchString(strings.TrimSpace(operationID)) {
		return nil, fmt.Errorf("%w: invalid operation id", ErrOperationConflict)
	}
	o.mu.Lock()
	record, err := o.loadRecordLocked(strings.TrimSpace(operationID))
	o.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, fmt.Errorf("%w: %s", ErrOperationNotFound, operationID)
	}
	return &workspaceproto.OperationRecord{
		OperationID: record.OperationID, Kind: record.Kind, TenantID: record.TenantID,
		TaskID: record.TaskID, RunID: record.RunID, Status: record.Status,
		PayloadHash: record.PayloadHash, Result: record.ResultJSON, ErrorMessage: record.ErrorMessage,
		CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: record.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}, nil
}

// gatedWorkspaceKind 报告该操作是否属于状态改变类（commit/push/cleanup）：
// 执行前必须取得可信的当前授权确认。
func gatedWorkspaceKind(kind string) bool {
	switch kind {
	case workspaceproto.KindDeveloperCommit, workspaceproto.KindDeveloperPush, workspaceproto.KindCleanup:
		return true
	default:
		return false
	}
}

func runWorkspaceOp[T any](ctx context.Context, o *WorkspaceOps, kind, operationID string, tenantID uint, taskID, runID, payloadHash string, secrets []string, exec func(context.Context) (T, error), executionRight ...ExecutionRight) (T, bool, error) {
	var zero T
	operationID = strings.TrimSpace(operationID)
	if !operationIDPattern.MatchString(operationID) {
		return zero, false, errors.New("workspace operation id is required and must be a safe identifier")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return zero, false, errors.New("workspace operation run id is required")
	}
	if o == nil {
		return zero, false, ErrServiceNotAccepting
	}
	authorize := func() error {
		if !gatedWorkspaceKind(kind) {
			return nil
		}
		if o.authorize == nil {
			return fmt.Errorf("%w: %s is a state changing operation and the runner has no authorization channel configured", ErrNotAuthorized, operationID)
		}
		owner, generation := "", uint64(0)
		for _, right := range executionRight {
			owner, generation = right.Owner, right.Generation
		}
		decision, authErr := o.authorize(ctx, workspaceproto.AuthorizationRequest{
			OperationID: operationID, Kind: kind, TenantID: tenantID, TaskID: taskID, RunID: runID, PayloadHash: payloadHash,
			Owner: owner, Generation: generation,
		})
		if authErr != nil {
			return fmt.Errorf("confirm workspace operation authorization: %w", authErr)
		}
		if !decision.Authorized {
			reason := strings.TrimSpace(decision.Reason)
			if reason == "" {
				reason = "the coordinator did not confirm current ownership"
			}
			return fmt.Errorf("%w: %s: %s", ErrNotAuthorized, operationID, reason)
		}
		return nil
	}
	if err := authorize(); err != nil {
		return zero, false, err
	}

	o.mu.Lock()
	existing, err := o.loadRecordLocked(operationID)
	if err != nil {
		o.mu.Unlock()
		return zero, false, err
	}
	if existing != nil {
		if existing.Kind != kind || existing.TenantID != tenantID || existing.TaskID != taskID || existing.RunID != runID || existing.PayloadHash != payloadHash {
			o.mu.Unlock()
			return zero, false, fmt.Errorf("%w: operation %s", ErrOperationConflict, operationID)
		}
		switch existing.Status {
		case workspaceOpStatusCompleted:
			var replayed T
			if err := json.Unmarshal(existing.ResultJSON, &replayed); err != nil {
				o.mu.Unlock()
				return zero, false, fmt.Errorf("decode stored operation result %s: %w", operationID, err)
			}
			o.mu.Unlock()
			return replayed, true, nil
		case workspaceOpStatusInFlight:
			o.mu.Unlock()
			return zero, false, fmt.Errorf("%w: operation %s", ErrOperationInFlight, operationID)
		}
		// failed：允许显式重试同一操作 ID（执行段自带一致性校验，重复执行
		// 不会产生第二笔提交——已提交场景以基线冲突保守拒绝）。
	}
	if !o.accepting {
		o.mu.Unlock()
		return zero, false, ErrServiceNotAccepting
	}
	if o.inFlight >= o.maxInFlight {
		o.mu.Unlock()
		return zero, false, ErrTooManyOperations
	}
	now := o.now().UTC()
	record := &workspaceOperationRecord{
		OperationID: operationID, Kind: kind, TenantID: tenantID, TaskID: taskID, RunID: runID,
		PayloadHash: payloadHash, Status: workspaceOpStatusInFlight, CreatedAt: now, UpdatedAt: now,
	}
	if err := o.writeRecordLocked(record); err != nil {
		o.mu.Unlock()
		return zero, false, err
	}
	o.inFlight++
	o.mu.Unlock()

	defer func() {
		o.mu.Lock()
		o.inFlight--
		o.mu.Unlock()
	}()
	releaseRun := o.lockRun(runID)
	defer releaseRun()
	if authErr := authorize(); authErr != nil {
		o.mu.Lock()
		record.Status = workspaceOpStatusFailed
		record.ErrorMessage = "authorization changed before acceptance"
		record.UpdatedAt = o.now().UTC()
		writeErr := o.writeRecordLocked(record)
		o.mu.Unlock()
		if writeErr != nil {
			return zero, false, writeErr
		}
		return zero, false, authErr
	}

	if o.testBeforeExecute != nil {
		o.testBeforeExecute()
	}

	result, execErr := exec(ctx)

	o.mu.Lock()
	defer o.mu.Unlock()
	record.UpdatedAt = o.now().UTC()
	if execErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			// 取消/超时不算操作失败语义之外的信息：保留原错误文本。
			_ = ctxErr
		}
		record.Status = workspaceOpStatusFailed
		record.ErrorMessage = execErr.Error()
		record.ResultJSON = nil
		writeErr := o.writeRecordLocked(record)
		if writeErr != nil {
			return zero, false, writeErr
		}
		return zero, false, store.RedactError(execErr.Error(), execErr, secrets...)
	}
	raw, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		record.Status = workspaceOpStatusFailed
		record.ErrorMessage = "encode operation result: " + marshalErr.Error()
		if writeErr := o.writeRecordLocked(record); writeErr != nil {
			return zero, false, writeErr
		}
		return zero, false, fmt.Errorf("encode operation result for %s: %w", operationID, marshalErr)
	}
	record.Status = workspaceOpStatusCompleted
	record.ResultJSON = raw
	record.ErrorMessage = ""
	if err := o.writeRecordLocked(record); err != nil {
		// 结果未持久化：返回错误让调用方按原操作 ID 重试（执行段的一致性
		// 校验兜底，不会产生第二笔提交）。
		return zero, false, err
	}
	return result, false, nil
}

func (o *WorkspaceOps) lockRun(runID string) func() {
	o.mu.Lock()
	lock, ok := o.runLocks[runID]
	if !ok {
		lock = &sync.Mutex{}
		o.runLocks[runID] = lock
	}
	o.mu.Unlock()
	lock.Lock()
	return lock.Unlock
}

func (o *WorkspaceOps) loadRecordLocked(operationID string) (*workspaceOperationRecord, error) {
	path := filepath.Join(o.opsRoot, operationID+".json")
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read operation record %s: %w", operationID, err)
	}
	var record workspaceOperationRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, fmt.Errorf("decode operation record %s: %w", operationID, err)
	}
	return &record, nil
}

func (o *WorkspaceOps) writeRecordLocked(record *workspaceOperationRecord) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode operation record %s: %w", record.OperationID, err)
	}
	tmp, err := os.CreateTemp(o.opsRoot, "."+record.OperationID+".tmp-")
	if err != nil {
		return fmt.Errorf("stage operation record %s: %w", record.OperationID, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write operation record %s: %w", record.OperationID, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close operation record %s: %w", record.OperationID, err)
	}
	if err := os.Rename(tmpName, filepath.Join(o.opsRoot, record.OperationID+".json")); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("publish operation record %s: %w", record.OperationID, err)
	}
	return nil
}

func credentialOrNil(credential workspaceproto.Credential) *workspace.GitCredential {
	if strings.TrimSpace(credential.Token) == "" {
		return nil
	}
	return &workspace.GitCredential{Username: credential.Username, Token: credential.Token}
}
