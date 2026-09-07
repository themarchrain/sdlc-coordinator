package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/workspace"
)

type TaskRepository interface {
	Create(context.Context, *model.Task) error
	Update(context.Context, *model.Task) error
	GetByTaskID(context.Context, string) (*model.Task, error)
	GetByDedupeKey(context.Context, string) (*model.Task, error)
	ListExpiredLeases(context.Context, time.Time, int) ([]model.Task, error)
	ListDispatchable(context.Context, int) ([]model.Task, error)
	Claim(context.Context, string, string, time.Time) error
	RenewActiveLease(context.Context, string, string, time.Time) error
	HasActiveReviewerTaskForDedupePrefix(context.Context, string, string) (bool, error)
	ResumeHumanRequired(context.Context, string, string) error
	ResumeRework(context.Context, string, string) error
	MarkHumanRequiredIfDispatchable(context.Context, string, string) error
	BlockRunAndRequireHuman(context.Context, string, string, time.Time) error
	ParkUnbound(context.Context, string) error
}

type RunRepository interface {
	Create(context.Context, *model.Run) error
	Update(context.Context, *model.Run) error
	GetByRunID(context.Context, string) (*model.Run, error)
	ListRunning(context.Context, int) ([]model.Run, error)
	ListSucceededForArtifactCleanup(context.Context, time.Time, int) ([]model.Run, error)
}

type ArtifactRepository interface {
	UpsertByIdentity(context.Context, *model.Artifact) error
}

type EffectRepository interface {
	Update(context.Context, *model.Effect) error
	GetByDedupeKey(context.Context, string) (*model.Effect, error)
	ListPending(context.Context, int) ([]model.Effect, error)
	ListStaleRunning(context.Context, interface{}, int) ([]model.Effect, error)
	ListByRunID(context.Context, string) ([]model.Effect, error)
	UpsertByDedupeKey(context.Context, *model.Effect) error
}

type EffectExecutor interface {
	ExecuteEffect(context.Context, *model.Effect) (string, error)
}

type WritebackRepository interface {
	Update(context.Context, *model.Writeback) error
	GetByDedupeKey(context.Context, string) (*model.Writeback, error)
	GetSucceededMergeRequest(context.Context, string, int) (*model.Writeback, error)
	GetLatestSucceededReviewerComment(context.Context, string, int) (*model.Writeback, error)
	ListUnknown(context.Context, int) ([]model.Writeback, error)
	ListByRunID(context.Context, string) ([]model.Writeback, error)
	UpsertByDedupeKey(context.Context, *model.Writeback) error
}

type DependencyRepository interface {
	Update(context.Context, *model.Dependency) error
	ListByParentTaskID(context.Context, string) ([]model.Dependency, error)
	ListByChildTaskID(context.Context, string) ([]model.Dependency, error)
	UpsertByIdentity(context.Context, *model.Dependency) error
}

type ResultSubmissionRepository interface {
	Accept(context.Context, *model.ResultSubmission, time.Time) error
	GetByRunID(context.Context, string) (*model.ResultSubmission, error)
	GetBySubmissionID(context.Context, string) (*model.ResultSubmission, error)
	Update(context.Context, *model.ResultSubmission) error
	ClaimRecoverable(context.Context, string, time.Time, time.Time, int) (*model.ResultSubmission, error)
	ListRecoverable(context.Context, time.Time, int, int) ([]model.ResultSubmission, error)
	FailFinalization(context.Context, string, string, time.Time, int) (*model.ResultSubmission, bool, error)
	DeferReconciliationWait(context.Context, string, string) (*model.ResultSubmission, error)
}

type Orchestrator struct {
	tickMu                   sync.Mutex
	tasks                    TaskRepository
	runs                     RunRepository
	artifacts                ArtifactRepository
	effects                  EffectRepository
	effectExecutor           EffectExecutor
	writebacks               WritebackRepository
	dependencies             DependencyRepository
	resultSubmissions        ResultSubmissionRepository
	codeHostResolver         CodeHostBackendResolver
	writebackAuth            WritebackAuthHandler
	repositoryLookup         RepositoryLookup
	assigneeMentionDirectory AssigneeMentionDirectory
	developerIMTarget        string
	reviewerIMMentions       []string
	reviewerRunner           ReviewerRunnerExecutionCoordinator
	reviewerRunnerSettings   ReviewerRunnerSettings
	developerRunner          DeveloperRunnerExecutionCoordinator
	developerRunnerSettings  ReviewerRunnerSettings
	agentExecutions          AgentExecutionLookup
	runnerArtifacts          RunnerArtifactStore
	stateMachine             *StateMachine
	policy                   Policy
	now                      func() time.Time
}

type ReviewerRunnerExecutionCoordinator interface {
	Start(context.Context, ReviewerRunnerStartRequest) (*model.AgentExecution, error)
	Reconcile(context.Context, string) (*ReviewerRunnerReconcileResult, error)
}

type DeveloperRunnerExecutionCoordinator interface {
	Start(context.Context, DeveloperRunnerStartRequest) (*model.AgentExecution, error)
	Reconcile(context.Context, string) (*DeveloperRunnerReconcileResult, error)
}

type AgentExecutionLookup interface {
	GetByRunID(context.Context, string) (*model.AgentExecution, error)
}

type RunnerArtifactStore interface {
	Delete(context.Context, string) error
}

type ReviewerRunnerSettings struct {
	WorkspaceRoot string
	Provider      runner.ProviderConfig
	// ResolveCredential 按任务解析 git 克隆/推送凭据:取任务所属注册仓的 Token。
	// 任务未绑定注册仓时返回 ErrRepositoryNotBound。
	ResolveCredential func(ctx context.Context, task *model.Task) (*workspace.GitCredential, error)
	TaskTimeout       time.Duration
}

// CredentialFor 是 ResolveCredential 的空值安全入口,装配遗漏时给出明确错误而非 panic。
func (s ReviewerRunnerSettings) CredentialFor(ctx context.Context, task *model.Task) (*workspace.GitCredential, error) {
	if s.ResolveCredential == nil {
		return nil, errors.New("runner credential resolver is required")
	}
	return s.ResolveCredential(ctx, task)
}

type Option func(*Orchestrator)

func WithPolicy(policy Policy) Option {
	return func(o *Orchestrator) {
		o.policy = policy
	}
}

func WithClock(clock func() time.Time) Option {
	return func(o *Orchestrator) {
		o.now = clock
	}
}

func WithDeveloperClarificationIMTarget(target string) Option {
	return func(o *Orchestrator) {
		o.developerIMTarget = strings.TrimSpace(target)
	}
}

func WithReviewerIMMentions(mentions []string) Option {
	return func(o *Orchestrator) {
		o.reviewerIMMentions = normalizeIMMentions(mentions)
	}
}

func WithReviewerRunner(coordinator ReviewerRunnerExecutionCoordinator, settings ReviewerRunnerSettings) Option {
	return func(o *Orchestrator) {
		o.reviewerRunner = coordinator
		o.reviewerRunnerSettings = settings
	}
}

func WithDeveloperRunner(coordinator DeveloperRunnerExecutionCoordinator, settings ReviewerRunnerSettings) Option {
	return func(o *Orchestrator) {
		o.developerRunner = coordinator
		o.developerRunnerSettings = settings
	}
}

func WithAgentExecutionLookup(executions AgentExecutionLookup) Option {
	return func(o *Orchestrator) {
		o.agentExecutions = executions
	}
}

// WithResultIngress enables the MCP result submission channel on both runner
// coordinators. issuer 为 nil（或 endpoint 为空）时通道保持关闭（纯文本模式）。
// 必须排在 WithReviewerRunner/WithDeveloperRunner 之后调用。
func WithResultIngress(issuer RunTokenIssuer, endpointURL string) Option {
	return func(o *Orchestrator) {
		reviewerCoordinator, reviewerOK := o.reviewerRunner.(*ReviewerRunnerCoordinator)
		developerCoordinator, developerOK := o.developerRunner.(*DeveloperRunnerCoordinator)
		// 装配失效必须 fail-fast：静默 no-op 意味着 dispatch 永远不带 token，
		// agent 全部落文本降级通道且无从排查。
		if issuer != nil && endpointURL != "" && (!reviewerOK || !developerOK) {
			panic("orchestrator: WithResultIngress requires WithReviewerRunner/WithDeveloperRunner to be applied first")
		}
		if reviewerOK {
			reviewerCoordinator.core.setResultIngress(issuer, endpointURL)
		}
		if developerOK {
			developerCoordinator.core.setResultIngress(issuer, endpointURL)
		}
	}
}

func WithRunnerArtifactStore(store RunnerArtifactStore) Option {
	return func(o *Orchestrator) {
		o.runnerArtifacts = store
	}
}

func WithArtifactRepository(artifacts ArtifactRepository) Option {
	return func(o *Orchestrator) {
		o.artifacts = artifacts
	}
}

func WithEffectRepository(effects EffectRepository) Option {
	return func(o *Orchestrator) {
		o.effects = effects
	}
}

func WithEffectExecutor(executor EffectExecutor) Option {
	return func(o *Orchestrator) {
		o.effectExecutor = executor
	}
}

func WithWritebackRepository(writebacks WritebackRepository) Option {
	return func(o *Orchestrator) {
		o.writebacks = writebacks
	}
}

func WithDependencyRepository(dependencies DependencyRepository) Option {
	return func(o *Orchestrator) {
		o.dependencies = dependencies
	}
}

func WithResultSubmissionRepository(submissions ResultSubmissionRepository) Option {
	return func(o *Orchestrator) {
		o.resultSubmissions = submissions
	}
}

type CodeHostBackendResolver interface {
	Resolve(ctx context.Context, task *model.Task, writeback *model.Writeback) (codehost.Backend, error)
}

func WithCodeHostBackendResolver(resolver CodeHostBackendResolver) Option {
	return func(o *Orchestrator) {
		o.codeHostResolver = resolver
	}
}

type WritebackAuthHandler interface {
	HandleWritebackFailure(ctx context.Context, task *model.Task, writebackErr error) bool
}

func WithWritebackAuthHandler(handler WritebackAuthHandler) Option {
	return func(o *Orchestrator) {
		o.writebackAuth = handler
	}
}

type RepositoryLookup interface {
	Get(ctx context.Context, repositoryID string) (*registeredrepo.RegisteredRepository, error)
}

func WithRepositoryLookup(lookup RepositoryLookup) Option {
	return func(o *Orchestrator) {
		o.repositoryLookup = lookup
	}
}

var (
	ErrRepositoryNotEnabled = errors.New("repository_not_enabled")
	ErrRepositoryNotBound   = errors.New("repository_not_bound")
)

func New(tasks TaskRepository, runs RunRepository, opts ...Option) *Orchestrator {
	orch := &Orchestrator{
		tasks:        tasks,
		runs:         runs,
		stateMachine: NewStateMachine(),
		policy:       DefaultPolicy(),
		now:          time.Now,
	}

	for _, opt := range opts {
		opt(orch)
	}

	if orch.now == nil {
		orch.now = time.Now
	}
	if orch.developerIMTarget == "" {
		orch.developerIMTarget = "default"
	}
	return orch
}

func (o *Orchestrator) ClaimTask(ctx context.Context, taskID, runID string) (*model.Task, error) {
	leaseUntil := o.policy.LeaseUntil(o.now())

	if err := o.tasks.Claim(ctx, taskID, runID, leaseUntil); err != nil {
		return nil, err
	}

	return o.tasks.GetByTaskID(ctx, taskID)
}

func (o *Orchestrator) CreateTask(ctx context.Context, task *model.Task) (*model.Task, error) {
	if task == nil {
		return nil, errors.New("task is required")
	}
	if task.DedupeKey == "" {
		return nil, errors.New("task dedupe key is required")
	}
	if task.TaskID == "" {
		task.TaskID = model.NewID("task")
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = o.now()
	}
	if err := o.prepareDeliveryBranch(task); err != nil {
		return nil, err
	}

	existing, err := o.tasks.GetByDedupeKey(ctx, task.DedupeKey)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	if err := o.tasks.Create(ctx, task); err != nil {
		// Another receipt may have created the same business Task between the
		// lookup and insert. Re-read by the unique business identity before
		// surfacing the insert error.
		existing, lookupErr := o.tasks.GetByDedupeKey(ctx, task.DedupeKey)
		if lookupErr == nil {
			return existing, nil
		}
		return nil, fmt.Errorf("create task: %w", err)
	}
	return task, nil
}

func (o *Orchestrator) GetTask(ctx context.Context, taskID string) (*model.Task, error) {
	return o.tasks.GetByTaskID(ctx, taskID)
}

// RebindTask 给未绑定任务补记 repository_id 并恢复可调度状态，是 L2-003 park
// 机制的配套恢复入口：park 为 blocked 的任务经此恢复到 pending。
func (o *Orchestrator) RebindTask(ctx context.Context, taskID, repositoryID string) (*model.Task, error) {
	if strings.TrimSpace(repositoryID) == "" {
		return nil, errors.New("repository id is required")
	}
	if o.repositoryLookup == nil {
		return nil, errors.New("repository lookup is required")
	}
	repo, err := o.repositoryLookup.Get(ctx, repositoryID)
	if err != nil {
		return nil, fmt.Errorf("lookup repository %s: %w", repositoryID, err)
	}
	if repo.Status != registeredrepo.StatusEnabled {
		return nil, errors.New("repository is not enabled")
	}
	task, err := o.tasks.GetByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	switch task.Status {
	case model.TaskStatusSucceeded, model.TaskStatusRunning, model.TaskStatusClaimed, model.TaskStatusFinalizing, model.TaskStatusWaitingMerge:
		return nil, fmt.Errorf("task %s cannot be rebound in status %s", taskID, task.Status)
	}
	if task.Status == model.TaskStatusBlocked || task.Status == model.TaskStatusHumanRequired {
		task.Status = model.TaskStatusPending
	}
	task.RepositoryID = &repositoryID
	task.LastError = ""
	task.CurrentRunID = nil
	task.LeaseUntil = nil
	if err := o.tasks.Update(ctx, task); err != nil {
		return nil, fmt.Errorf("rebind task %s: %w", taskID, err)
	}
	return task, nil
}

func (o *Orchestrator) parkUnboundTask(ctx context.Context, taskID string) error {
	if err := o.tasks.ParkUnbound(ctx, taskID); err != nil {
		return fmt.Errorf("park unbound task %s: %w", taskID, err)
	}
	return nil
}

func (o *Orchestrator) GetRun(ctx context.Context, runID string) (*model.Run, error) {
	return o.runs.GetByRunID(ctx, runID)
}

type DispatchResult struct {
	Task *model.Task `json:"task"`
	Run  *model.Run  `json:"run"`
}

type RecoveryResult struct {
	TaskID         string           `json:"task_id"`
	PreviousStatus model.TaskStatus `json:"previous_status"`
	Status         model.TaskStatus `json:"status"`
	Recovered      bool             `json:"recovered"`
	Reason         string           `json:"reason,omitempty"`
}

func (o *Orchestrator) recordDispatchFailure(ctx context.Context, task *model.Task, run *model.Run, cause error) {
	nextTaskStatus := o.policy.NextFailureStatus(task)
	run.Status = model.RunStatusFailed
	run.ErrorMessage = cause.Error()
	completedAt := o.now()
	run.CompletedAt = &completedAt
	_ = o.runs.Update(ctx, run)

	advanced, err := o.AdvanceTask(ctx, task.TaskID, nextTaskStatus)
	if err == nil {
		advanced.LastError = cause.Error()
		_ = o.tasks.Update(ctx, advanced)
	}
}

func (o *Orchestrator) AdvanceTask(ctx context.Context, taskID string, next model.TaskStatus) (*model.Task, error) {
	task, err := o.tasks.GetByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}

	previous := task.Status
	if err := o.stateMachine.Transition(task, next); err != nil {
		return nil, err
	}

	o.policy.Apply(task, previous, next, o.now())

	if err := o.tasks.Update(ctx, task); err != nil {
		return nil, fmt.Errorf("advance task %s: %w", taskID, err)
	}

	return task, nil
}

func (o *Orchestrator) ApplyCanonicalResult(ctx context.Context, payload results.CanonicalResult) (*model.Task, *model.Run, error) {
	if payload.TaskID == "" {
		return nil, nil, errors.New("result task_id is required")
	}
	if payload.RunID == "" {
		return nil, nil, errors.New("result run_id is required")
	}

	task, err := o.tasks.GetByTaskID(ctx, payload.TaskID)
	if err != nil {
		return nil, nil, err
	}
	run, err := o.runs.GetByRunID(ctx, payload.RunID)
	if err != nil {
		return nil, nil, err
	}
	if isStaleRunForTask(task, run.RunID) {
		return o.applyStaleRunResult(ctx, task, run, payload)
	}
	payload, err = o.withDeveloperReworkFollowups(ctx, task, payload)
	if err != nil {
		return nil, nil, err
	}
	payload, err = o.withDeveloperClarificationFollowups(task, payload)
	if err != nil {
		return nil, nil, err
	}
	if err := validateCodeHostWritebacksForTask(task, payload); err != nil {
		return nil, nil, err
	}
	if isTerminalRunStatus(run.Status) {
		if err := o.materializeResultArtifacts(ctx, payload); err != nil {
			return nil, nil, err
		}
		if err := o.applyCodeHostWritebacks(ctx, payload); err != nil {
			return nil, nil, err
		}
		payload, err = o.withDeveloperDeliveryFollowups(ctx, task, payload)
		if err != nil {
			return nil, nil, err
		}
		payload, err = o.withReviewerFollowups(ctx, task, payload)
		if err != nil {
			return nil, nil, err
		}
		if err := o.materializeResultEffects(ctx, payload); err != nil {
			return nil, nil, err
		}
		return task, run, nil
	}

	expandedNextTasks, err := o.expandResultNextTasks(ctx, task, run, payload)
	if err != nil {
		return nil, nil, err
	}

	if err := o.materializeResultArtifacts(ctx, payload); err != nil {
		return nil, nil, err
	}
	if err := o.applyCodeHostWritebacks(ctx, payload); err != nil {
		return nil, nil, err
	}
	payload, err = o.withDeveloperDeliveryFollowups(ctx, task, payload)
	if err != nil {
		return nil, nil, err
	}
	payload, err = o.withReviewerFollowups(ctx, task, payload)
	if err != nil {
		return nil, nil, err
	}
	if err := o.materializeResultEffects(ctx, payload); err != nil {
		return nil, nil, err
	}

	nextTaskStatus := resultTaskStatus(task, payload, o.policy)
	if payload.Status == results.StatusSucceeded && expandedNextTasks > 0 {
		nextTaskStatus = model.TaskStatusWaitingUpstream
	}
	previousTaskStatus := task.Status
	if err := o.stateMachine.Transition(task, nextTaskStatus); err != nil {
		return nil, nil, err
	}
	o.policy.Apply(task, previousTaskStatus, nextTaskStatus, o.now())

	if payload.Status == results.StatusSucceeded {
		task.LastError = ""
	} else if payload.BlockerReason != nil && *payload.BlockerReason != "" {
		task.LastError = *payload.BlockerReason
	} else if payload.Summary != "" {
		task.LastError = payload.Summary
	}

	if task.CurrentRunID == nil {
		task.CurrentRunID = &run.RunID
	}

	run.Status = resultRunStatus(payload.Status)
	completedAt := o.now()
	run.CompletedAt = &completedAt
	if raw, marshalErr := json.Marshal(payload); marshalErr == nil {
		run.OutputJSON = string(raw)
	}
	if payload.BlockerReason != nil {
		run.ErrorMessage = *payload.BlockerReason
	} else if payload.Status != results.StatusSucceeded && payload.Summary != "" {
		run.ErrorMessage = payload.Summary
	}

	if err := o.tasks.Update(ctx, task); err != nil {
		return nil, nil, fmt.Errorf("update result task %s: %w", payload.TaskID, err)
	}
	if err := o.runs.Update(ctx, run); err != nil {
		return nil, nil, fmt.Errorf("update result run %s: %w", payload.RunID, err)
	}
	if err := o.resolveDependenciesForResult(ctx, task, payload); err != nil {
		return nil, nil, err
	}

	return task, run, nil
}

func (o *Orchestrator) applyStaleRunResult(ctx context.Context, task *model.Task, run *model.Run, payload results.CanonicalResult) (*model.Task, *model.Run, error) {
	if isTerminalRunStatus(run.Status) {
		return task, run, nil
	}

	run.Status = resultRunStatus(payload.Status)
	completedAt := o.now()
	run.CompletedAt = &completedAt
	if raw, marshalErr := json.Marshal(payload); marshalErr == nil {
		run.OutputJSON = string(raw)
	}
	run.ErrorMessage = "stale run result ignored for task state"
	if err := o.runs.Update(ctx, run); err != nil {
		return nil, nil, fmt.Errorf("update stale result run %s: %w", run.RunID, err)
	}
	return task, run, nil
}

func isStaleRunForTask(task *model.Task, runID string) bool {
	if task == nil || task.CurrentRunID == nil || *task.CurrentRunID == "" {
		return false
	}
	return *task.CurrentRunID != runID
}

func (o *Orchestrator) RecoverTask(ctx context.Context, taskID string) (*model.Task, error) {
	task, err := o.tasks.GetByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}

	next, shouldRecover := o.policy.RecoveryStatus(task, o.now())
	if !shouldRecover {
		return task, nil
	}

	previous := task.Status
	if err := o.stateMachine.Transition(task, next); err != nil {
		return nil, err
	}
	o.policy.Apply(task, previous, next, o.now())
	task.LastError = "lease expired; task recovered by coordinator"

	if err := o.tasks.Update(ctx, task); err != nil {
		return nil, fmt.Errorf("recover task %s: %w", taskID, err)
	}
	if task.Status == model.TaskStatusRetryPending {
		o.materializeLeaseRecoveryIMEffect(ctx, task)
	} else if task.Status == model.TaskStatusHumanRequired {
		o.materializeLeaseHumanRequiredIMEffect(ctx, task)
	}

	return task, nil
}

func (o *Orchestrator) RecoverExpiredTasks(ctx context.Context, limit int) ([]RecoveryResult, error) {
	tasks, err := o.tasks.ListExpiredLeases(ctx, o.now(), limit)
	if err != nil {
		return nil, err
	}

	results := make([]RecoveryResult, 0, len(tasks))
	for _, task := range tasks {
		previous := task.Status
		recovered, err := o.RecoverTask(ctx, task.TaskID)
		if err != nil {
			return nil, err
		}

		result := RecoveryResult{
			TaskID:         task.TaskID,
			PreviousStatus: previous,
			Status:         recovered.Status,
			Recovered:      recovered.Status != previous,
		}
		if result.Recovered {
			result.Reason = "lease_expired"
		}
		results = append(results, result)
	}

	return results, nil
}

func (o *Orchestrator) StartRun(ctx context.Context, taskID string, run *model.Run) (*model.Run, *model.Task, error) {
	if run == nil {
		return nil, nil, errors.New("run is required")
	}

	if run.RunID == "" {
		run.RunID = model.NewID("run")
	}
	run.TaskID = taskID

	task, err := o.ClaimTask(ctx, taskID, run.RunID)
	if err != nil {
		return nil, nil, err
	}

	if err := o.runs.Create(ctx, run); err != nil {
		return nil, nil, fmt.Errorf("create run %s: %w", run.RunID, err)
	}

	task, err = o.AdvanceTask(ctx, taskID, model.TaskStatusRunning)
	if err != nil {
		return nil, nil, err
	}
	task.CurrentRunID = &run.RunID
	if err := o.tasks.Update(ctx, task); err != nil {
		return nil, nil, fmt.Errorf("attach run to task %s: %w", taskID, err)
	}

	run.Status = model.RunStatusRunning
	startedAt := o.now()
	run.StartedAt = &startedAt
	if err := o.runs.Update(ctx, run); err != nil {
		return nil, nil, fmt.Errorf("update run %s: %w", run.RunID, err)
	}

	return run, task, nil
}

func resultTaskStatus(task *model.Task, payload results.CanonicalResult, policy Policy) model.TaskStatus {
	switch payload.Status {
	case results.StatusSucceeded:
		if task != nil && task.Type == model.TaskTypeReworkMergeRequest {
			return model.TaskStatusWaitingMerge
		}
		return model.TaskStatusSucceeded
	case results.StatusBlocked:
		return model.TaskStatusBlocked
	case results.StatusHumanRequired:
		return model.TaskStatusHumanRequired
	case results.StatusRetryPending:
		return policy.NextFailureStatus(task)
	case results.StatusFailed:
		return policy.NextFailureStatus(task)
	default:
		return model.TaskStatusBlocked
	}
}

func resultRunStatus(status results.ResultStatus) model.RunStatus {
	switch status {
	case results.StatusSucceeded:
		return model.RunStatusSucceeded
	case results.StatusFailed,
		results.StatusRetryPending:
		return model.RunStatusFailed
	case results.StatusBlocked,
		results.StatusHumanRequired:
		return model.RunStatusBlocked
	default:
		return model.RunStatusFailed
	}
}

func (o *Orchestrator) lookupRegisteredRepo(ctx context.Context, task *model.Task) *registeredrepo.RegisteredRepository {
	if o.repositoryLookup == nil || task == nil || task.RepositoryID == nil || strings.TrimSpace(*task.RepositoryID) == "" {
		return nil
	}
	repo, err := o.repositoryLookup.Get(ctx, *task.RepositoryID)
	if err != nil {
		return nil
	}
	return repo
}

func (o *Orchestrator) reviewerMentionsFor(ctx context.Context, task *model.Task) []string {
	platform := strings.Join(o.reviewerIMMentions, ",")
	resolved := registeredrepo.ResolvePeople(o.lookupRegisteredRepo(ctx, task), registeredrepo.PeopleSettings{
		ReviewerMentions: platform,
	})
	if strings.TrimSpace(resolved.ReviewerMentions) == "" || resolved.ReviewerMentions == platform {
		return o.reviewerIMMentions
	}
	raw := strings.TrimSpace(resolved.ReviewerMentions)
	if strings.HasPrefix(raw, "[") {
		var items []string
		if err := json.Unmarshal([]byte(raw), &items); err == nil {
			return normalizeIMMentions(items)
		}
	}
	return normalizeIMMentions(strings.Split(raw, ","))
}

// assertRepositoryDispatchable 是派发前置门禁:任务必须绑定已启用的注册仓。
func (o *Orchestrator) assertRepositoryDispatchable(ctx context.Context, task *model.Task) error {
	empty := task == nil || task.RepositoryID == nil || strings.TrimSpace(*task.RepositoryID) == ""
	if empty {
		return ErrRepositoryNotBound
	}
	if o.repositoryLookup == nil {
		return nil
	}
	repo, err := o.repositoryLookup.Get(ctx, *task.RepositoryID)
	if err != nil {
		return err
	}
	if repo.Status != registeredrepo.StatusEnabled {
		return ErrRepositoryNotEnabled
	}
	return nil
}

func isTerminalRunStatus(status model.RunStatus) bool {
	switch status {
	case model.RunStatusSucceeded,
		model.RunStatusFailed,
		model.RunStatusBlocked:
		return true
	default:
		return false
	}
}
