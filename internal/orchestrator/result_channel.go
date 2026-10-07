package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"sdlc-coordinator/internal/instance"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/workspace"
)

// The result submission core is transport agnostic: both the agent-runner
// reconcile path (text channel) and the /mcp tools/call path (MCP channel)
// funnel through SubmitAgentResult so validation, git delivery, normalization
// and idempotent acceptance behave identically regardless of how the payload
// arrived.
type ChannelDecision string

const (
	ChannelAccepted ChannelDecision = "accepted"
	ChannelRejected ChannelDecision = "rejected"
	ChannelFailed   ChannelDecision = "failed"
)

// Stages distinguish where a rejection happened; the reconcile wrapper maps
// them onto its invalid/delivery error taxonomy without changing behavior.
type ChannelStage string

const (
	ChannelStageNormalize ChannelStage = "normalize"
	ChannelStageDecode    ChannelStage = "decode"
	ChannelStageValidate  ChannelStage = "validate"
	ChannelStageFinalize  ChannelStage = "finalize"
	ChannelStageSubmit    ChannelStage = "submit"
)

type ValidationProblem struct {
	Path    string `json:"path,omitempty"`
	Problem string `json:"problem"`
	Hint    string `json:"hint,omitempty"`
}

type ChannelOutcome struct {
	Decision   ChannelDecision
	Stage      ChannelStage
	Response   *SubmitResultResponse
	Problems   []ValidationProblem
	Normalized bool
	Err        error
}

// developerFinalizeTimeout 约束 MCP tools/call 内同步执行的 git
// commit/push：无此上限，代码托管黑洞会耗尽整个任务时间预算。
const developerFinalizeTimeout = 3 * time.Minute

type ResultChannelCoordinator struct {
	finalizer     DeveloperGitFinalizer
	checkpoints   DeveloperDeliveryCheckpoints
	submitter     ReviewerResultSubmitter
	settings      ReviewerRunnerSettings
	source        string
	deliveryRight WorkItemLeasePort
}

func NewResultChannelCoordinator(
	finalizer DeveloperGitFinalizer,
	submitter ReviewerResultSubmitter,
	settings ReviewerRunnerSettings,
	source string,
) *ResultChannelCoordinator {
	if strings.TrimSpace(source) == "" {
		source = "unknown"
	}
	return &ResultChannelCoordinator{finalizer: finalizer, submitter: submitter, settings: settings, source: source}
}

// SetDeveloperDeliveryCheckpoints 在装配期接入交付检查点端口(租户库实现);
// 未接入时 Developer ready 结果 fail-closed 拒绝交付。
func (c *ResultChannelCoordinator) SetDeveloperDeliveryCheckpoints(checkpoints DeveloperDeliveryCheckpoints) {
	c.checkpoints = checkpoints
}

// SetDeliveryExecutionRight 在装配期接入交付执行权端口（租户库租约实现）：
// 任何 Git 副作用（commit/push）之前必须持有该权利。
func (c *ResultChannelCoordinator) SetDeliveryExecutionRight(leases WorkItemLeasePort) {
	c.deliveryRight = leases
}

// developerDeliveryLeaseScope / TTL：交付执行权按 run 粒度，租期覆盖单次
// 交付预算（finalize 上限）并留出对账余量。
const developerDeliveryLeaseScope = "developer-delivery:"

const developerDeliveryLeaseTTL = 5 * time.Minute

func (c *ResultChannelCoordinator) deliveryOwner() string {
	return instance.ID() + ":" + c.source
}

func (c *ResultChannelCoordinator) acquireDeliveryRight(ctx context.Context, runID string) (model.WorkItemLease, bool, error) {
	if c.deliveryRight == nil {
		return model.WorkItemLease{}, true, nil
	}
	return c.deliveryRight.Acquire(ctx, developerDeliveryLeaseScope+runID, c.deliveryOwner(), "developer-delivery", developerDeliveryLeaseTTL)
}

// verifyDeliveryRight 在 push 前核验权利仍在自己手上（被接管则不得推送）。
func (c *ResultChannelCoordinator) verifyDeliveryRight(ctx context.Context, lease model.WorkItemLease) bool {
	if c.deliveryRight == nil || lease.Scope == "" {
		return true
	}
	ok, err := c.deliveryRight.Verify(ctx, lease.Scope, lease.Owner, lease.Generation)
	if err != nil {
		return false
	}
	return ok
}

// releaseDeliveryRight 尽力释放（失败留待租期到期自然回收）。
func (c *ResultChannelCoordinator) releaseDeliveryRight(lease model.WorkItemLease) {
	if c.deliveryRight == nil || lease.Scope == "" {
		return
	}
	releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = c.deliveryRight.Release(releaseCtx, lease.Scope, lease.Owner, lease.Generation)
}

// SubmitAgentResult runs raw agent payload through thin normalization, strict
// decoding, contract validation, coordinator-side git delivery (developer) and
// canonical normalization, then submits. Validation failures are returned as
// structured problems for the correction protocol; transport-internal errors
// (conflict, processing, repository failures) come back as ChannelFailed.
func (c *ResultChannelCoordinator) SubmitAgentResult(
	ctx context.Context,
	task *model.Task,
	run *model.Run,
	toolName string,
	rawPayload []byte,
) (ChannelOutcome, error) {
	if c == nil || c.submitter == nil {
		return ChannelOutcome{}, errors.New("result channel submitter is required")
	}
	contract, isDeveloper, err := contractForRole(run)
	if err != nil {
		return ChannelOutcome{}, err
	}
	if contract.ToolName() != toolName {
		return ChannelOutcome{}, fmt.Errorf("tool %q does not match %s role contract tool %q", toolName, run.WorkerRole, contract.ToolName())
	}

	normalization, normErr := results.NormalizeInput(inputSchemaForRole(contract, isDeveloper), rawPayload)
	if normErr != nil {
		return rejectedOutcome(ChannelStageNormalize, []ValidationProblem{{Problem: normErr.Error(), Hint: hintFor(normErr.Error())}}), nil
	}

	if isDeveloper {
		return c.submitDeveloperResult(ctx, task, run, contract, normalization, rawPayload)
	}
	return c.submitRoleResult(ctx, task, run, contract, normalization, rawPayload)
}

func (c *ResultChannelCoordinator) submitDeveloperResult(
	ctx context.Context,
	task *model.Task,
	run *model.Run,
	contract results.RoleResultContract,
	normalization results.NormalizationResult,
	rawPayload []byte,
) (ChannelOutcome, error) {
	agentContract := results.NewDeveloperAgentContract()
	agentInput, err := agentContract.DecodeStrict(normalization.Payload)
	if err != nil {
		return rejectedOutcome(ChannelStageDecode, problemsFromError(err)), nil
	}
	if err := agentContract.Validate(task, run, agentInput); err != nil {
		return rejectedOutcome(ChannelStageValidate, problemsFromError(err)), nil
	}
	inputValue, ok := agentInput.(*results.DeveloperAgentResultInput)
	if !ok {
		return rejectedOutcome(ChannelStageDecode, problemsFromError(errors.New("developer agent result input type is invalid"))), nil
	}
	var developerInput *results.DeveloperResultInput
	if inputValue.Outcome == "change_ready" || inputValue.Outcome == "rework_ready" {
		if c.finalizer == nil {
			// 构造时未装配 finalizer（reviewer 轻量通道复用同一类型）：
			// fail-closed 而不是 nil panic。
			return rejectedOutcome(ChannelStageFinalize, problemsFromError(errors.New("developer git finalizer is not configured"))), nil
		}
		if c.checkpoints == nil {
			return ChannelOutcome{Decision: ChannelFailed, Stage: ChannelStageFinalize,
				Err: errors.New("developer delivery checkpoints are not configured")}, nil
		}
		workspaceRequest, err := buildDeveloperWorkspaceRequest(ctx, task, run, c.settings)
		if err != nil {
			return rejectedOutcome(ChannelStageFinalize, problemsFromError(fmt.Errorf("rebuild trusted developer workspace metadata: %w", err))), nil
		}
		finalizeCtx, cancel := context.WithTimeout(ctx, developerFinalizeTimeout)
		defer cancel()
		// 处理权必须在 commit/push 之前生效：两通道（MCP 与文本）可能同时
		// 到达不同实例，这里是 Git 副作用前的唯一线性化边界。
		deliveryLease, won, leaseErr := c.acquireDeliveryRight(finalizeCtx, run.RunID)
		if leaseErr != nil {
			return ChannelOutcome{Decision: ChannelFailed, Stage: ChannelStageFinalize,
				Err: fmt.Errorf("acquire developer delivery right: %w", leaseErr)}, nil
		}
		if !won {
			return ChannelOutcome{Decision: ChannelFailed, Stage: ChannelStageFinalize,
				Err: errors.New("developer delivery is currently owned by another coordinator instance")}, nil
		}
		defer c.releaseDeliveryRight(deliveryLease)
		// 交付检查点是提交归属的唯一受信凭据:基线缺失(部署前在途 run)或
		// 与当前任务/仓/分支不符一律 fail-closed,属基础设施/配置问题,不
		// 进 AI 可矫正通道。必须在取得执行权之后读取：并发胜者可能刚写入
		// 交付 SHA，按旧视图再提交会破坏只有一次可信提交。
		checkpoint, checkpointErr := c.checkpoints.Load(finalizeCtx, run.RunID)
		if checkpointErr != nil {
			return ChannelOutcome{Decision: ChannelFailed, Stage: ChannelStageFinalize,
				Err: fmt.Errorf("load developer delivery checkpoint: %w", checkpointErr)}, nil
		}
		if checkpoint == nil {
			return ChannelOutcome{Decision: ChannelFailed, Stage: ChannelStageFinalize,
				Err: errors.New("developer delivery baseline is not recorded for the run; redispatch the task")}, nil
		}
		if checkpoint.TaskID != task.TaskID || checkpoint.TaskType != task.Type ||
			checkpoint.RepositoryURL != workspaceRequest.RepositoryURL || checkpoint.DeliveryBranch != workspaceRequest.DeliveryBranch {
			return ChannelOutcome{Decision: ChannelFailed, Stage: ChannelStageFinalize,
				Err: errors.New("developer delivery baseline does not match the current task and run")}, nil
		}
		var delivery *workspace.DeveloperDelivery
		if checkpoint.CommitSHA != "" {
			// 重试/恢复路径:只信任检查点记录的固定 SHA,推送前核对本地
			// HEAD 与父提交仍与其相符。
			if !c.verifyDeliveryRight(finalizeCtx, deliveryLease) {
				return ChannelOutcome{Decision: ChannelFailed, Stage: ChannelStageFinalize,
					Err: errors.New("developer delivery right was lost before push")}, nil
			}
			delivery, err = c.finalizer.PushDelivery(finalizeCtx, workspace.DeveloperPushRequest{
				WorkspaceRoot: c.settings.WorkspaceRoot, TaskID: task.TaskID, RunID: run.RunID,
				TaskType: task.Type, RepositoryURL: workspaceRequest.RepositoryURL,
				TargetBranch: workspaceRequest.TargetBranch, DeliveryBranch: workspaceRequest.DeliveryBranch,
				CommitSHA: checkpoint.CommitSHA, BaselineSHA: checkpoint.BaselineSHA,
				Credential: workspaceRequest.Credential,
			})
		} else {
			// commit 与下方 push 共用外层 err:推送错误必须走 217 行的
			// 结构化错误出口,不能在 else 块内被遮蔽吞掉。
			var commit *workspace.DeveloperCommitResult
			commit, err = c.finalizer.CommitDelivery(finalizeCtx, workspace.DeveloperCommitRequest{
				WorkspaceRoot: c.settings.WorkspaceRoot, TaskID: task.TaskID, RunID: run.RunID,
				CommitMessage: inputValue.CommitMessage, GitAuthorName: workspaceRequest.GitAuthorName,
				GitAuthorEmail: workspaceRequest.GitAuthorEmail,
				TaskType:       task.Type, RepositoryURL: workspaceRequest.RepositoryURL,
				TargetBranch: workspaceRequest.TargetBranch, DeliveryBranch: workspaceRequest.DeliveryBranch,
				BaselineSHA: checkpoint.BaselineSHA,
			})
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
					// git 黑洞是基础设施故障而非参数可矫正问题：归传输桶，
					// 不消耗矫正次数。
					return ChannelOutcome{Decision: ChannelFailed, Stage: ChannelStageFinalize, Err: fmt.Errorf("create developer git delivery: %w", err)}, nil
				}
				return rejectedOutcome(ChannelStageFinalize, problemsFromError(fmt.Errorf("create developer git delivery: %w", err))), nil
			}
			// 检查点写入必须在 push 之前:写入失败则已创建的提交不推送,
			// 由重试路径 fail-closed 处理,绝不凭消息/尾注"猜测恢复"。
			if err := c.checkpoints.RecordCommit(finalizeCtx, run.RunID, commit.CommitSHA); err != nil {
				return ChannelOutcome{Decision: ChannelFailed, Stage: ChannelStageFinalize,
					Err: fmt.Errorf("record developer delivery checkpoint: %w (the created commit will not be pushed)", err)}, nil
			}
			if !c.verifyDeliveryRight(finalizeCtx, deliveryLease) {
				return ChannelOutcome{Decision: ChannelFailed, Stage: ChannelStageFinalize,
					Err: errors.New("developer delivery right was lost before push")}, nil
			}
			delivery, err = c.finalizer.PushDelivery(finalizeCtx, workspace.DeveloperPushRequest{
				WorkspaceRoot: c.settings.WorkspaceRoot, TaskID: task.TaskID, RunID: run.RunID,
				TaskType: task.Type, RepositoryURL: workspaceRequest.RepositoryURL,
				TargetBranch: workspaceRequest.TargetBranch, DeliveryBranch: workspaceRequest.DeliveryBranch,
				CommitSHA: commit.CommitSHA, BaselineSHA: checkpoint.BaselineSHA,
				Credential: workspaceRequest.Credential,
			})
		}
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				// git 黑洞是基础设施故障而非参数可矫正问题：归传输桶，
				// 不消耗矫正次数。
				return ChannelOutcome{Decision: ChannelFailed, Stage: ChannelStageFinalize, Err: fmt.Errorf("push developer git delivery: %w", err)}, nil
			}
			return rejectedOutcome(ChannelStageFinalize, problemsFromError(fmt.Errorf("push developer git delivery: %w", err))), nil
		}
		developerInput, err = agentContract.WithDelivery(agentInput, results.DeliveryEvidence{
			CommitSHA: delivery.CommitSHA, ChangedFiles: delivery.ChangedFiles, DiffSummary: delivery.DiffSummary,
		})
		if err != nil {
			return rejectedOutcome(ChannelStageValidate, problemsFromError(err)), nil
		}
	} else {
		developerInput, err = agentContract.WithoutDelivery(agentInput)
		if err != nil {
			return rejectedOutcome(ChannelStageValidate, problemsFromError(err)), nil
		}
	}
	if err := contract.Validate(task, run, developerInput); err != nil {
		return rejectedOutcome(ChannelStageValidate, problemsFromError(err)), nil
	}
	canonical, err := contract.Normalize(task, run, developerInput)
	if err != nil {
		return rejectedOutcome(ChannelStageValidate, problemsFromError(err)), nil
	}
	return c.submit(ctx, task, run, contract, canonical, normalization, rawPayload)
}

func (c *ResultChannelCoordinator) submitRoleResult(
	ctx context.Context,
	task *model.Task,
	run *model.Run,
	contract results.RoleResultContract,
	normalization results.NormalizationResult,
	rawPayload []byte,
) (ChannelOutcome, error) {
	roleInput, err := contract.DecodeStrict(normalization.Payload)
	if err != nil {
		return rejectedOutcome(ChannelStageDecode, problemsFromError(err)), nil
	}
	if err := contract.Validate(task, run, roleInput); err != nil {
		return rejectedOutcome(ChannelStageValidate, problemsFromError(err)), nil
	}
	canonical, err := contract.Normalize(task, run, roleInput)
	if err != nil {
		return rejectedOutcome(ChannelStageValidate, problemsFromError(err)), nil
	}
	return c.submit(ctx, task, run, contract, canonical, normalization, rawPayload)
}

func (c *ResultChannelCoordinator) submit(
	ctx context.Context,
	task *model.Task,
	run *model.Run,
	contract results.RoleResultContract,
	canonical results.CanonicalResult,
	normalization results.NormalizationResult,
	rawPayload []byte,
) (ChannelOutcome, error) {
	response, err := c.submitter.SubmitResult(ctx, SubmitResultRequest{
		Source:             c.source,
		ToolName:           contract.ToolName(),
		RawPayload:         append([]byte(nil), rawPayload...),
		Canonical:          canonical,
		NormalizationTrail: normalizationTrailJSON(normalization),
	})
	if err != nil {
		// Conflict/processing/repository failures are not validation problems:
		// the caller distinguishes them via errors.Is on outcome.Err.
		return ChannelOutcome{Decision: ChannelFailed, Stage: ChannelStageSubmit, Err: err}, nil
	}
	return ChannelOutcome{
		Decision: ChannelAccepted, Stage: ChannelStageSubmit, Response: response,
		Normalized: !normalization.Clean(),
	}, nil
}

func contractForRole(run *model.Run) (results.RoleResultContract, bool, error) {
	switch run.WorkerRole {
	case "DeveloperWorker":
		return results.NewDeveloperContract(), true, nil
	case "ReviewerWorker":
		return results.NewReviewerContract(), false, nil
	case "TesterWorker":
		return results.NewTesterContract(), false, nil
	default:
		return nil, false, fmt.Errorf("run role %q has no result contract", run.WorkerRole)
	}
}

func inputSchemaForRole(contract results.RoleResultContract, isDeveloper bool) map[string]any {
	if isDeveloper {
		return results.NewDeveloperAgentContract().InputSchema()
	}
	return contract.InputSchema()
}

func normalizationTrailJSON(normalization results.NormalizationResult) string {
	if normalization.Clean() {
		return ""
	}
	encoded, err := json.Marshal(normalization.Repairs)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func rejectedOutcome(stage ChannelStage, problems []ValidationProblem) ChannelOutcome {
	return ChannelOutcome{Decision: ChannelRejected, Stage: stage, Problems: problems}
}

// problemsFromError keeps the contract error verbatim as the problem text and
// decorates it with a best-effort parameter path plus a closed-domain hint.
// The L3 contract checks are untouched.
func problemsFromError(err error) []ValidationProblem {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "result payload was rejected"
	}
	return []ValidationProblem{{Path: extractProblemPath(message), Problem: message, Hint: hintFor(message)}}
}

var (
	indexPathPattern   = regexp.MustCompile(`\b(?:tests|findings|changed_files|next_tasks)\[\d+\](?:\.[a-z_]+)?`)
	quotedFieldPattern = regexp.MustCompile(`field "([\w]+)"`)
)

func extractProblemPath(message string) string {
	if match := indexPathPattern.FindString(message); match != "" {
		return match
	}
	if match := quotedFieldPattern.FindStringSubmatch(message); match != nil {
		return match[1]
	}
	return ""
}

// hintFor maps error message fragments onto closed-domain correction hints.
// Unmatched messages simply carry no hint — the verbatim problem stays the
// authority for the agent's correction loop.
func hintFor(message string) string {
	switch {
	case strings.Contains(message, "tests["):
		return "tests 必须包含真实执行的验证命令（command）、结果（passed）与摘要（summary）；交付类 outcome 要求全部 passed=true"
	case strings.Contains(message, "task_id") || strings.Contains(message, "run_id") || strings.Contains(message, "not match") || strings.Contains(message, "not current"):
		return "task_id 与 run_id 必须与任务包中的值完全一致，不要改写或猜测"
	case strings.Contains(message, "blocker_reason"):
		return "该 outcome 需要在 blocker_reason 中说明阻塞原因与已尝试的处置"
	case strings.Contains(message, "merge_request"):
		return "change_ready 需要 merge_request（至少 title 与 description）"
	case strings.Contains(message, "unknown field"):
		return "存在结果契约之外的字段；请对照任务提示词中的结果契约，仅提交契约字段"
	case strings.Contains(message, "summary"):
		return "summary 不能为空；请概述本次执行做了什么与结论"
	case strings.Contains(message, "outcome"):
		return "outcome 取值必须来自契约枚举，见任务提示词中的 outcome 说明"
	case strings.Contains(message, "writable repository"):
		return "任务上下文要求恰好一个可写仓库；无法修正时改用 insufficient_context 并说明"
	case strings.Contains(message, "finalize developer git delivery") || strings.Contains(message, "workspace"):
		return "协调器提交 git 交付失败；若没有可交付的代码变更，请改用 fix_failed 并在 blocker_reason 说明，而不是坚持 change_ready"
	default:
		return ""
	}
}
