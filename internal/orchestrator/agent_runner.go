package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store/repository"
)

type AgentRunnerObservation struct {
	Execution *model.AgentExecution
	Events    []runner.Event
	Result    *runner.Result
}

// RunTokenIssuer signs the per-run bearer tokens for the MCP result channel.
// taskTimeout is the dispatch-resolved effective timeout: the token must not
// expire before the run can finish, so its TTL derives from this value (the
// issuer adds the polling grace).
type RunTokenIssuer interface {
	IssueRunToken(taskID, runID, role string, taskTimeout time.Duration) (string, error)
}

// RunTokenIssuerFunc adapts a function into a RunTokenIssuer.
type RunTokenIssuerFunc func(taskID, runID, role string, taskTimeout time.Duration) (string, error)

func (f RunTokenIssuerFunc) IssueRunToken(taskID, runID, role string, taskTimeout time.Duration) (string, error) {
	return f(taskID, runID, role, taskTimeout)
}

type AgentRunnerCoordinator struct {
	tasks      TaskRepository
	runs       RunRepository
	executions AgentExecutionProjectionRepository
	runner     ReviewerRunnerClient
	// router 是租户 runner 池路由；nil = 单端点直连（既有行为）。
	router TenantRunnerRouter
	now    func() time.Time
	// ingress enables the MCP result channel on dispatch; nil keeps the
	// channel closed (text-only mode).
	ingressIssuer   RunTokenIssuer
	ingressEndpoint string
}

func NewAgentRunnerCoordinator(tasks TaskRepository, runs RunRepository, executions AgentExecutionProjectionRepository, runnerClient ReviewerRunnerClient) *AgentRunnerCoordinator {
	return &AgentRunnerCoordinator{tasks: tasks, runs: runs, executions: executions, runner: runnerClient, now: time.Now}
}

func (c *AgentRunnerCoordinator) Existing(ctx context.Context, runID string, request runner.CreateExecutionRequest) (*model.AgentExecution, bool, error) {
	if err := c.validateDependencies(); err != nil {
		return nil, false, err
	}
	expectedHash, err := runner.RequestHash(request)
	if err != nil {
		return nil, false, fmt.Errorf("build runner request identity: %w", err)
	}
	existing, err := c.executions.GetByRunID(ctx, runID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if err := validateStoredExecution(existing, runID, expectedHash); err != nil {
		return nil, false, err
	}
	return existing, true, nil
}

// setResultIngress wires the MCP result channel onto dispatch. Only real
// creates sign tokens: idempotent replays (Existing hit) keep the token the
// original dispatch carried.
func (c *AgentRunnerCoordinator) setResultIngress(issuer RunTokenIssuer, endpointURL string) {
	c.ingressIssuer = issuer
	c.ingressEndpoint = strings.TrimSpace(endpointURL)
}

// setTenantRunnerRouter 接入租户池路由；nil 保留单端点直连。
func (c *AgentRunnerCoordinator) setTenantRunnerRouter(router TenantRunnerRouter) {
	c.router = router
}

// RunnerSelection 是组织段执行选择的现取结果：CLI、推理强度与单任务
// 超时三个字段独立解析，空串/零值表示组织未设置（回落系统默认值）。
type RunnerSelection struct {
	Provider    string
	Effort      string
	TaskTimeout time.Duration
}

// resolveRunnerExecution 应用组织段执行选择（CLI、推理强度、单任务超时）：
// run 行已钉住的字段（派发时写入）直接沿用不再查询——配置热切换后恢复/
// 重建路径按钉住值重放，请求哈希（覆盖 CLI 名、推理强度与超时秒数）与
// 远端既有执行一致；未钉住的字段经 lookup 现取，lookup 未注入或零值时
// 沿用系统默认值。候选字段仅在非空/非零时覆盖——BaseURL/APIKey/Model
// 等其余字段保持系统值（组织段只租户化执行选择，不触碰模型凭据），组织
// 只设其一时不覆盖另一个。覆盖必须发生在请求哈希计算之前，幂等重放与
// 远端校验才能看到同一份请求。
func resolveRunnerExecution(ctx context.Context, lookup func(context.Context) RunnerSelection, fallback runner.ProviderConfig, fallbackTimeout time.Duration, run *model.Run) (runner.ProviderConfig, time.Duration) {
	provider := strings.TrimSpace(run.Provider)
	effort := strings.TrimSpace(run.Effort)
	timeout := time.Duration(0)
	if run.TaskTimeoutSeconds > 0 {
		timeout = time.Duration(run.TaskTimeoutSeconds) * time.Second
	}
	if (provider == "" || effort == "" || timeout == 0) && lookup != nil {
		live := lookup(ctx)
		if provider == "" {
			provider = strings.TrimSpace(live.Provider)
		}
		if effort == "" {
			effort = strings.TrimSpace(live.Effort)
		}
		if timeout == 0 {
			timeout = live.TaskTimeout
		}
	}
	resolved := fallback
	if provider != "" {
		resolved.Name = provider
	}
	if effort != "" {
		resolved.Effort = effort
	}
	if timeout == 0 {
		timeout = fallbackTimeout
	}
	return resolved, timeout
}

// pinRunnerSelectionOnRun 把本 run 实际使用的 CLI、推理强度与单任务超时
// 钉进 run 行（best-effort：回写失败不阻断派发，代价是后续重建路径回落
// 到现取值）。已钉住的值不覆盖，三列独立判定。经列级条件更新只碰各自的
// 列——本函数在分钟级的工作区准备之后执行，整行保存会把并发终态化的
// run 回滚成旧状态。
func pinRunnerSelectionOnRun(ctx context.Context, runs RunRepository, run *model.Run, provider, effort string, timeout time.Duration) {
	if run == nil {
		return
	}
	if pinned := strings.TrimSpace(provider); pinned != "" && run.Provider == "" {
		_ = runs.UpdateProviderIfEmpty(ctx, run.RunID, pinned)
		run.Provider = pinned
	}
	if pinned := strings.TrimSpace(effort); pinned != "" && run.Effort == "" {
		_ = runs.UpdateEffortIfEmpty(ctx, run.RunID, pinned)
		run.Effort = pinned
	}
	if seconds := int64(timeout / time.Second); seconds > 0 && run.TaskTimeoutSeconds == 0 {
		_ = runs.UpdateTaskTimeoutIfEmpty(ctx, run.RunID, seconds)
		run.TaskTimeoutSeconds = seconds
	}
}

// attemptForTask 解析派发尝试：nil 路由回退单端点直连（Endpoint 留空 =
// projection 不记录端点，观测走默认客户端，与既有行为一致）。
func (c *AgentRunnerCoordinator) attemptForTask(ctx context.Context, task *model.Task) (RunnerAttempt, error) {
	if c.router == nil {
		return RunnerAttempt{Client: c.runner}, nil
	}
	return c.router.AttemptForTask(ctx, task)
}

// resolveAndPinEndpoint 在准备之前解析并钉住 run 归属端点：准备、任务包
// 投递、执行、commit/push、清理全链按同一端点寻址——独立卷下别的 runner
// 既没有该 run 的仓库也没有 Agent 的修改。已有钉住值直接沿用（崩溃恢复/
// 重建路径不换 runner）；单端点直连形态返回空串（默认端点语义）。条件写
// 后重读以库内值为准（并发方可能已钉住）。
func (c *AgentRunnerCoordinator) resolveAndPinEndpoint(ctx context.Context, task *model.Task, run *model.Run) (string, error) {
	if run == nil {
		return "", nil
	}
	if pinned := strings.TrimSpace(run.RunnerEndpoint); pinned != "" {
		return pinned, nil
	}
	if c.router == nil {
		return "", nil
	}
	attempt, err := c.attemptForTask(ctx, task)
	if err != nil {
		return "", err
	}
	endpoint := strings.TrimSpace(attempt.Endpoint)
	if endpoint == "" {
		return "", nil
	}
	if err := c.runs.PinRunnerEndpointIfEmpty(ctx, run.RunID, endpoint); err != nil {
		return "", fmt.Errorf("pin runner endpoint for run %s: %w", run.RunID, err)
	}
	refreshed, err := c.runs.GetByRunID(ctx, run.RunID)
	if err != nil {
		return "", fmt.Errorf("reload run %s after pinning endpoint: %w", run.RunID, err)
	}
	run.RunnerEndpoint = strings.TrimSpace(refreshed.RunnerEndpoint)
	return run.RunnerEndpoint, nil
}

var ErrExecutionNotAccepted = errors.New("runner execution was never accepted")

// errDispatchOutcomeUnknown 报告远端派发结果不确定（Create 歧义失败且
// 无法按原端点回读）：意图行保持待对账，不得判失败重派新执行。
var errDispatchOutcomeUnknown = errors.New("dispatch outcome unknown")

// createOnPool 在端点池上派发执行：每轮先 Create，失败后同端点 Get 崩溃恢复
// （既有语义）；Get 返回 404（确定性未建）才轮换下一端点，歧义失败（传输层
// 错误等）按既有失败语义终止，池耗尽返回首个 Create 错误。返回实际承接执行
// 的端点，供 projection 做观测亲和；definite 报告失败是否为全部端点确定性
// 未接纳——只有它才允许上层按失败处理（判失败/重试），否则结果不确定。
//
// 每次轮换到新端点前先经 repin 把新目标持久化（old→new 条件写）：Create 被
// 远端接受后本进程若在投影回写前退出，接管方按库里记录的端点对账——不先落
// 新目标时接管方会回到旧端点重发同 run，两个 runner 各起一个执行（同 run
// ID 的幂等域只在各 runner 本地）。repin 失败（写库失败/条件不满足）即终止
// 轮换并保持不确定语义，不允许远端已接受而库里仍指向旧端点的窗口存在。
func (c *AgentRunnerCoordinator) createOnPool(ctx context.Context, attempt RunnerAttempt, run *model.Run, expectedHash string, request runner.CreateExecutionRequest, persistedEndpoint string, repin func(context.Context, string, string) error) (*runner.Execution, string, bool, error) {
	firstErr := error(nil)
	for {
		remote, createErr := attempt.Client.Create(ctx, request)
		if createErr == nil {
			return remote, attempt.Endpoint, true, nil
		}
		if firstErr == nil {
			firstErr = createErr
		}
		recovered, getErr := attempt.Client.Get(ctx, run.RunID)
		if getErr == nil {
			return recovered, attempt.Endpoint, true, nil
		}
		if !runnerNotFound(getErr) {
			// 结果不确定：不轮换、不判失败，交上层保持意图待对账。
			return nil, "", false, fmt.Errorf("%w: create runner execution: %v", errDispatchOutcomeUnknown, firstErr)
		}
		next, ok := attempt.Next()
		if !ok {
			return nil, "", true, fmt.Errorf("create runner execution: %w", firstErr)
		}
		if repin != nil {
			if err := repin(ctx, persistedEndpoint, next.Endpoint); err != nil {
				// 新目标未能持久化：绝不换端点，保持不确定语义让接管方
				// 按既有记录对账。
				return nil, "", false, fmt.Errorf("%w: persist fallback runner endpoint %q: %v", errDispatchOutcomeUnknown, next.Endpoint, err)
			}
		}
		persistedEndpoint = next.Endpoint
		attempt = next
	}
}

func (c *AgentRunnerCoordinator) persistIntent(ctx context.Context, run *model.Run, request runner.CreateExecutionRequest, endpoint, expectedHash string) (*model.AgentExecution, error) {
	now := c.now().UTC()
	intent := &model.AgentExecution{
		RunID: run.RunID, ExecutionID: run.RunID, Provider: request.Provider.Name,
		ProviderConfigRevision: request.Provider.Revision, ProviderModel: request.Provider.Model,
		ProviderEffort: request.Provider.Effort, RunnerEndpoint: endpoint,
		Status: model.AgentExecutionStatusDispatching, RequestHash: expectedHash, LastObservedAt: &now,
	}
	if err := c.executions.Create(ctx, intent); err != nil {
		stored, lookupErr := c.executions.GetByRunID(ctx, run.RunID)
		if lookupErr == nil {
			return stored, nil
		}
		return nil, fmt.Errorf("persist dispatch intent for run %s: %w", run.RunID, err)
	}
	return intent, nil
}

// settleIntentFailure 把确定性未接纳的意图行落为 failed（可查询原因），
// 供任务失败路径照常重试（此时确无远端执行在跑）。
func (c *AgentRunnerCoordinator) settleIntentFailure(ctx context.Context, intent *model.AgentExecution, cause error) {
	now := c.now().UTC()
	intent.Status = model.AgentExecutionStatusFailed
	intent.ErrorMessage = cause.Error()
	intent.FinishedAt = &now
	if err := c.executions.Update(ctx, intent); err != nil {
		log.Printf("orchestrator: settle dispatch intent for run %s: %v", intent.RunID, err)
	}
}

// projectRemoteIntoIntent 把远端执行投影进既有意图行（保留主键、创建时间与
// 端点亲和），再交给调用方。
func (c *AgentRunnerCoordinator) projectRemoteIntoIntent(ctx context.Context, stored *model.AgentExecution, remote *runner.Execution, lastEventSeq int64) (*model.AgentExecution, error) {
	projection, err := c.project(remote, stored.ProviderConfigRevision, lastEventSeq, stored.ProviderModel, stored.ProviderEffort)
	if err != nil {
		return nil, err
	}
	projection.ID = stored.ID
	projection.CreatedAt = stored.CreatedAt
	projection.RunnerEndpoint = stored.RunnerEndpoint
	if err := c.executions.Update(ctx, projection); err != nil {
		return nil, fmt.Errorf("persist dispatch projection for run %s: %w", stored.RunID, err)
	}
	return projection, nil
}

func (c *AgentRunnerCoordinator) reconcileIntent(ctx context.Context, stored *model.AgentExecution, request runner.CreateExecutionRequest) (*model.AgentExecution, error) {
	client := c.executionClient(stored)
	remote, getErr := client.Get(ctx, stored.ExecutionID)
	if getErr == nil {
		if err := validateRemoteExecution(remote, stored.ExecutionID, stored.RequestHash); err != nil {
			return nil, err
		}
		return c.projectRemoteIntoIntent(ctx, stored, remote, stored.LastEventSeq)
	}
	if !runnerNotFound(getErr) {
		return nil, fmt.Errorf("%w: read back execution %s on %q: %v", errDispatchOutcomeUnknown, stored.ExecutionID, stored.RunnerEndpoint, getErr)
	}
	remote, createErr := client.Create(ctx, request)
	if createErr != nil {
		return nil, fmt.Errorf("%w: resume dispatch of execution %s on %q: %v", errDispatchOutcomeUnknown, stored.ExecutionID, stored.RunnerEndpoint, createErr)
	}
	if err := validateRemoteExecution(remote, stored.ExecutionID, stored.RequestHash); err != nil {
		return nil, err
	}
	return c.projectRemoteIntoIntent(ctx, stored, remote, 0)
}

func (c *AgentRunnerCoordinator) Start(ctx context.Context, task *model.Task, run *model.Run, request runner.CreateExecutionRequest) (*model.AgentExecution, error) {
	if err := c.validateDependencies(); err != nil {
		return nil, err
	}
	if err := validateAgentRunnerTask(task, run, request); err != nil {
		return nil, err
	}
	expectedHash, err := runner.RequestHash(request)
	if err != nil {
		return nil, fmt.Errorf("build runner request identity: %w", err)
	}
	existing, found, err := c.Existing(ctx, run.RunID, request)
	if err != nil {
		return nil, err
	}
	if found && existing.Status != model.AgentExecutionStatusDispatching {
		return existing, nil
	}
	if found {
		// 意图已落库、远端结果未定：按原目标端点收敛。
		return c.reconcileIntent(ctx, existing, request)
	}
	if c.ingressIssuer != nil && c.ingressEndpoint != "" {
		token, err := c.ingressIssuer.IssueRunToken(task.TaskID, run.RunID, task.Role, time.Duration(request.TimeoutSeconds)*time.Second)
		if err != nil {
			return nil, fmt.Errorf("issue run result token: %w", err)
		}
		request.ResultSubmission = &runner.ResultSubmissionChannel{EndpointURL: c.ingressEndpoint, BearerToken: token}
	}
	attempt, err := c.attemptForTask(ctx, task)
	if err != nil {
		return nil, err
	}
	if pinned := strings.TrimSpace(run.RunnerEndpoint); pinned != "" && pinned != attempt.Endpoint {
		// 本 run 已钉住归属端点（准备/任务包投递已按它发生）：派发必须命中
		// 同一端点，绝不轮换——独立卷下别的 runner 没有这份工作区与任务包。
		attempt = c.attemptOnRecordedEndpoint(attempt, pinned)
	}
	// 先落目标再调用：意图行写入成功后才做有副作用的远端 Create；崩溃在
	// 两者之间时，接管方按该端点回读/重发，不换 Runner、不产生第二个执行。
	intent, err := c.persistIntent(ctx, run, request, attempt.Endpoint, expectedHash)
	if err != nil {
		return nil, err
	}
	if intent.Status != model.AgentExecutionStatusDispatching {
		// 并发方已收敛同一 run：按幂等回放返回既有投影。
		if err := validateStoredExecution(intent, run.RunID, expectedHash); err != nil {
			return nil, err
		}
		return intent, nil
	}
	if intent.RunnerEndpoint != attempt.Endpoint {
		// 采纳并发方记录的目标端点（不换 Runner）。
		attempt = c.attemptOnRecordedEndpoint(attempt, intent.RunnerEndpoint)
	}
	// 每次端点推进先在库里持久化新目标：Create 被接受后本进程退出时，接管
	// 方按库内端点对账；不先落目标会让接管方回到旧端点重发同 run（两个
	// runner 各起一个执行）。条件写按旧记录值防覆盖并发方的更新。
	repoPin := func(pinCtx context.Context, from, to string) error {
		return c.executions.UpdateRunnerEndpointIfUnchanged(pinCtx, run.RunID, from, to)
	}
	remote, endpoint, definite, err := c.createOnPool(ctx, attempt, run, expectedHash, request, intent.RunnerEndpoint, repoPin)
	if err != nil {
		if !definite {
			// 结果不确定：保持意图待对账（轮询按原端点收敛），本函数按
			// 已受理返回意图行，不判失败、不重派新执行。
			return intent, nil
		}
		c.settleIntentFailure(ctx, intent, err)
		return nil, err
	}
	if err := validateRemoteExecution(remote, run.RunID, expectedHash); err != nil {
		c.settleIntentFailure(ctx, intent, err)
		return nil, err
	}
	intent.RunnerEndpoint = endpoint
	projection, err := c.projectRemoteIntoIntent(ctx, intent, remote, 0)
	if err != nil {
		return nil, err
	}
	return projection, nil
}

// attemptOnRecordedEndpoint 把尝试改钉到已记录的端点：有池客户端用池客户端，
// 否则回退默认直连；不含 Next（收敛路径禁止轮换）。
func (c *AgentRunnerCoordinator) attemptOnRecordedEndpoint(attempt RunnerAttempt, endpoint string) RunnerAttempt {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" || endpoint == attempt.Endpoint {
		return attempt
	}
	if c.router != nil {
		if client, ok := c.router.ClientForEndpoint(endpoint); ok {
			return RunnerAttempt{Client: client, Endpoint: endpoint}
		}
	}
	return RunnerAttempt{Client: c.runner, Endpoint: endpoint}
}

// executionClient 返回观测执行的客户端：优先按 projection 记录的端点钉住
// （池亲和），无记录或路由未知时回退默认客户端。
func (c *AgentRunnerCoordinator) executionClient(stored *model.AgentExecution) ReviewerRunnerClient {
	if c.router != nil && stored != nil {
		if pinned, ok := c.router.ClientForEndpoint(stored.RunnerEndpoint); ok {
			return pinned
		}
	}
	return c.runner
}

func (c *AgentRunnerCoordinator) Observe(ctx context.Context, runID string) (*AgentRunnerObservation, error) {
	if err := c.validateDependencies(); err != nil {
		return nil, err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, errors.New("run id is required")
	}
	stored, err := c.executions.GetByRunID(ctx, runID)
	if err != nil {
		return nil, err
	}
	client := c.executionClient(stored)
	if stored.Status == model.AgentExecutionStatusDispatching {
		remote, getErr := client.Get(ctx, stored.ExecutionID)
		if runnerNotFound(getErr) {
			return nil, fmt.Errorf("%w: execution %s on runner %q", ErrExecutionNotAccepted, stored.ExecutionID, stored.RunnerEndpoint)
		}
		if getErr != nil {
			return nil, fmt.Errorf("read back dispatching execution %s: %w", stored.ExecutionID, getErr)
		}
		converged, err := c.projectRemoteIntoIntent(ctx, stored, remote, stored.LastEventSeq)
		if err != nil {
			return nil, err
		}
		stored = converged
	}
	events, err := client.Events(ctx, stored.ExecutionID, stored.LastEventSeq)
	if err != nil {
		return nil, fmt.Errorf("list runner execution events: %w", err)
	}
	lastEventSeq := stored.LastEventSeq
	persistedEvents := make([]model.AgentExecutionEvent, 0, len(events))
	for _, event := range events {
		if event.ExecutionID != stored.ExecutionID {
			return nil, fmt.Errorf("runner event %d belongs to execution %q", event.Seq, event.ExecutionID)
		}
		if event.Seq > lastEventSeq {
			lastEventSeq = event.Seq
		}
		payload, err := json.Marshal(event.Payload)
		if err != nil {
			return nil, fmt.Errorf("encode runner event %d payload: %w", event.Seq, err)
		}
		persistedEvents = append(persistedEvents, model.AgentExecutionEvent{
			ExecutionID: stored.ExecutionID, RunID: stored.RunID, Provider: stored.Provider,
			Seq: event.Seq, Type: event.Type, OccurredAt: event.At.UTC(), PayloadJSON: string(payload),
		})
	}
	if err := c.executions.UpsertEvents(ctx, persistedEvents); err != nil {
		return nil, err
	}
	remote, err := client.Get(ctx, stored.ExecutionID)
	if err != nil {
		return nil, fmt.Errorf("get runner execution: %w", err)
	}
	if err := validateRemoteExecution(remote, stored.ExecutionID, stored.RequestHash); err != nil {
		return nil, err
	}
	projection, err := c.project(remote, stored.ProviderConfigRevision, lastEventSeq, stored.ProviderModel, stored.ProviderEffort)
	if err != nil {
		return nil, err
	}
	projection.ID = stored.ID
	projection.CreatedAt = stored.CreatedAt
	// 重投影不覆盖端点亲和：执行仍在原 runner 上，观测继续钉住同一端点。
	projection.RunnerEndpoint = stored.RunnerEndpoint
	if err := c.executions.Update(ctx, projection); err != nil {
		return nil, err
	}
	observation := &AgentRunnerObservation{Execution: projection, Events: events}
	if remote.Status != runner.StatusSucceeded {
		return observation, nil
	}
	result, err := client.Result(ctx, stored.ExecutionID)
	if err != nil {
		return nil, fmt.Errorf("get runner structured result: %w", err)
	}
	if result == nil || result.ExecutionID != stored.ExecutionID {
		return nil, errors.New("runner structured result has unexpected execution identity")
	}
	if remote.ResultHash != "" && result.ResultHash != remote.ResultHash {
		return nil, errors.New("runner structured result hash does not match execution")
	}
	observation.Result = result
	return observation, nil
}

func (c *AgentRunnerCoordinator) validateDependencies() error {
	if c == nil || c.tasks == nil || c.runs == nil || c.executions == nil || c.runner == nil {
		return errors.New("agent runner coordinator dependencies are required")
	}
	return nil
}

func (c *AgentRunnerCoordinator) project(remote *runner.Execution, revision int64, lastEventSeq int64, providerModel, providerEffort string) (*model.AgentExecution, error) {
	status, err := projectRunnerStatus(remote.Status)
	if err != nil {
		return nil, err
	}
	now := c.now().UTC()
	return &model.AgentExecution{
		RunID: remote.ExecutionID, ExecutionID: remote.ExecutionID, Provider: remote.Provider,
		ProviderConfigRevision: revision, ProviderModel: providerModel, ProviderEffort: providerEffort,
		Status: status, RequestHash: remote.RequestHash,
		LastEventSeq: lastEventSeq, LastObservedAt: &now, ResultHash: remote.ResultHash,
		ErrorCode: remote.ErrorCode, ErrorMessage: remote.ErrorMessage, StartedAt: remote.StartedAt, FinishedAt: remote.FinishedAt,
	}, nil
}

func validateAgentRunnerTask(task *model.Task, run *model.Run, request runner.CreateExecutionRequest) error {
	if task == nil || run == nil {
		return errors.New("agent runner task and run are required")
	}
	if strings.TrimSpace(task.TaskID) == "" || strings.TrimSpace(run.RunID) == "" || run.TaskID != task.TaskID {
		return errors.New("agent runner task and run identity is inconsistent")
	}
	if task.CurrentRunID == nil || *task.CurrentRunID != run.RunID {
		return errors.New("agent runner run is not current for task")
	}
	if task.Role == "" || run.WorkerRole != task.Role {
		return errors.New("agent runner task and run role is inconsistent")
	}
	if request.ExecutionID != run.RunID || request.TaskID != task.TaskID || request.Role != task.Role {
		return errors.New("agent runner request identity does not match task and run")
	}
	return nil
}
