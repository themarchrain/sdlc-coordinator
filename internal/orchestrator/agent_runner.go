package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
type RunTokenIssuer interface {
	IssueRunToken(taskID, runID, role string) (string, error)
}

// RunTokenIssuerFunc adapts a function into a RunTokenIssuer.
type RunTokenIssuerFunc func(taskID, runID, role string) (string, error)

func (f RunTokenIssuerFunc) IssueRunToken(taskID, runID, role string) (string, error) {
	return f(taskID, runID, role)
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

// RunnerSelection 是组织段执行选择的现取结果：CLI 与推理强度两个字段
// 独立解析，空串表示组织未设置（回落系统默认值）。
type RunnerSelection struct {
	Provider string
	Effort   string
}

// resolveRunnerProviderConfig 应用组织段执行选择（CLI 与推理强度）：pinned
// 非空（run 已钉住派发时的取值）时直接沿用，不再查询——配置热切换后恢复/
// 重建路径按钉住值重放，请求哈希（覆盖 CLI 名与推理强度）与远端既有执行
// 一致；未钉住的字段经 lookup 现取，lookup 未注入或返回空串时沿用系统
// 默认值。两个候选字段仅在非空时覆盖——BaseURL/APIKey/Model 等其余字段
// 保持系统值（组织段只租户化执行选择，不触碰模型凭据），组织只设其一时
// 不覆盖另一个。覆盖必须发生在请求哈希计算之前，幂等重放与远端校验才能
// 看到同一份请求。
func resolveRunnerProviderConfig(ctx context.Context, lookup func(context.Context) RunnerSelection, fallback runner.ProviderConfig, pinnedProvider, pinnedEffort string) runner.ProviderConfig {
	provider := strings.TrimSpace(pinnedProvider)
	effort := strings.TrimSpace(pinnedEffort)
	if (provider == "" || effort == "") && lookup != nil {
		live := lookup(ctx)
		if provider == "" {
			provider = strings.TrimSpace(live.Provider)
		}
		if effort == "" {
			effort = strings.TrimSpace(live.Effort)
		}
	}
	if provider == "" && effort == "" {
		return fallback
	}
	resolved := fallback
	if provider != "" {
		resolved.Name = provider
	}
	if effort != "" {
		resolved.Effort = effort
	}
	return resolved
}

// pinRunnerProviderOnRun 把本 run 实际使用的 CLI 与推理强度钉进 run 行
// （best-effort：回写失败不阻断派发，代价是后续重建路径回落到现取值）。
// 已钉住的值不覆盖，两列独立判定。经列级条件更新只碰各自的列——本函数
// 在分钟级的工作区准备之后执行，整行保存会把并发终态化的 run 回滚成旧
// 状态。
func pinRunnerProviderOnRun(ctx context.Context, runs RunRepository, run *model.Run, provider, effort string) {
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
}

// attemptForTask 解析派发尝试：nil 路由回退单端点直连（Endpoint 留空 =
// projection 不记录端点，观测走默认客户端，与既有行为一致）。
func (c *AgentRunnerCoordinator) attemptForTask(ctx context.Context, task *model.Task) (RunnerAttempt, error) {
	if c.router == nil {
		return RunnerAttempt{Client: c.runner}, nil
	}
	return c.router.AttemptForTask(ctx, task)
}

// createOnPool 在端点池上派发执行：每轮先 Create，失败后同端点 Get 崩溃恢复
// （既有语义）；Get 返回 404（确定性未建）才轮换下一端点，歧义失败（传输层
// 错误等）按既有失败语义终止，池耗尽返回首个 Create 错误。返回实际承接执行
// 的端点，供 projection 做观测亲和。
func (c *AgentRunnerCoordinator) createOnPool(ctx context.Context, attempt RunnerAttempt, run *model.Run, expectedHash string, request runner.CreateExecutionRequest) (*runner.Execution, string, error) {
	firstErr := error(nil)
	for {
		remote, createErr := attempt.Client.Create(ctx, request)
		if createErr == nil {
			return remote, attempt.Endpoint, nil
		}
		if firstErr == nil {
			firstErr = createErr
		}
		recovered, getErr := attempt.Client.Get(ctx, run.RunID)
		if getErr == nil {
			return recovered, attempt.Endpoint, nil
		}
		if !runnerNotFound(getErr) {
			return nil, "", fmt.Errorf("create runner execution: %w", firstErr)
		}
		next, ok := attempt.Next()
		if !ok {
			return nil, "", fmt.Errorf("create runner execution: %w", firstErr)
		}
		attempt = next
	}
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
	if found {
		return existing, nil
	}
	if c.ingressIssuer != nil && c.ingressEndpoint != "" {
		token, err := c.ingressIssuer.IssueRunToken(task.TaskID, run.RunID, task.Role)
		if err != nil {
			return nil, fmt.Errorf("issue run result token: %w", err)
		}
		request.ResultSubmission = &runner.ResultSubmissionChannel{EndpointURL: c.ingressEndpoint, BearerToken: token}
	}
	attempt, err := c.attemptForTask(ctx, task)
	if err != nil {
		return nil, err
	}
	remote, endpoint, err := c.createOnPool(ctx, attempt, run, expectedHash, request)
	if err != nil {
		return nil, err
	}
	if err := validateRemoteExecution(remote, run.RunID, expectedHash); err != nil {
		return nil, err
	}
	projection, err := c.project(remote, request.Provider.Revision, 0, request.Provider.Model, request.Provider.Effort)
	if err != nil {
		return nil, err
	}
	projection.RunnerEndpoint = endpoint
	if err := c.executions.Create(ctx, projection); err != nil {
		stored, lookupErr := c.executions.GetByRunID(ctx, run.RunID)
		if lookupErr == nil && validateStoredExecution(stored, run.RunID, expectedHash) == nil {
			return stored, nil
		}
		return nil, err
	}
	return projection, nil
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
