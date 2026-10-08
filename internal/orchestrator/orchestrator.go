package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/instance"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/workspace"
)

// ErrConcurrentTaskAdvance 表示任务状态在推进过程中被并发方改变且目标状态
// 不一致：调用方不得用旧快照续写，应重新读取并按当前状态裁决。
var ErrConcurrentTaskAdvance = errors.New("task advanced concurrently")

// RunWorkspaceCleaner 是 run 工作区清理的窄端口：本地形态在协调器文件系统
// 上删除；远端形态（计划 A）经受控操作在 Runner 本机删除。
type RunWorkspaceCleaner interface {
	RemoveRunWorkspace(ctx context.Context, runID string) error
}

// WorkItemLeasePort 是跨实例执行权租约的窄端口（租户库实现）：抢占是原子
// 单语句，Verify/Release 带 owner+generation 条件。
type WorkItemLeasePort interface {
	Acquire(ctx context.Context, scope, owner, operationID string, ttl time.Duration) (model.WorkItemLease, bool, error)
	Verify(ctx context.Context, scope, owner string, generation uint64) (bool, error)
	Release(ctx context.Context, scope, owner string, generation uint64) (bool, error)
}

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
	ListByDeliveryBranch(context.Context, string) ([]model.Task, error)
	ListSucceededVerificationsWithoutAuditAfter(context.Context, uint, int) ([]model.Task, error)
	ResumeHumanRequired(context.Context, string, string) error
	ResumeRework(context.Context, string, string) error
	MarkHumanRequiredIfDispatchable(context.Context, string, string) error
	BlockRunAndRequireHuman(context.Context, string, string, time.Time) error
	ParkUnbound(context.Context, string) error
	AdvanceGuarded(context.Context, *model.Task, model.TaskStatus) (bool, error)
	UpdateLastErrorIfStatus(context.Context, string, model.TaskStatus, string) (bool, error)
	// RecoverExpiredGuarded 原子回收租约过期的任务：双实例同时回收同一
	// 过期任务时恰好一个赢家，attempt_count 在数据库侧原子 +1。
	RecoverExpiredGuarded(ctx context.Context, taskID string, next model.TaskStatus, lastError string, now time.Time) (bool, error)
	// RestoreWaitingParentGuarded 把仍在 waiting_upstream 的父任务原子恢复
	// 为 pending：双实例同时解除同一父任务的依赖链时恰好一个赢家。
	RestoreWaitingParentGuarded(ctx context.Context, taskID string) (bool, error)
}

type RunRepository interface {
	Create(context.Context, *model.Run) error
	Update(context.Context, *model.Run) error
	UpdateProviderIfEmpty(context.Context, string, string) error
	UpdateEffortIfEmpty(context.Context, string, string) error
	UpdateTaskTimeoutIfEmpty(context.Context, string, int64) error
	UpdateDispatchProjection(context.Context, string, string, string, string) error
	UpdateFailedIfInFlight(context.Context, string, string, time.Time) (bool, error)
	// MarkArtifactsPurgingIfUnclean 抢占 run 的清理归属（跨实例栅栏），
	// ClearArtifactsPurging 是清理失败的回滚通道。
	MarkArtifactsPurgingIfUnclean(ctx context.Context, runID string, now time.Time) (bool, error)
	ClearArtifactsPurging(ctx context.Context, runID string) error
	GetByRunID(context.Context, string) (*model.Run, error)
	ListRunning(context.Context, int) ([]model.Run, error)
	ListTerminalWithActiveExecution(context.Context, int) ([]model.Run, error)
	ListSucceededForArtifactCleanup(context.Context, time.Time, int) ([]model.Run, error)
}

type ArtifactRepository interface {
	UpsertByIdentity(context.Context, *model.Artifact) error
}

// MergeGateAuditRepository 留痕自动合并门禁的裁决:每验证任务至多一行,
// 审计即防重。
type MergeGateAuditRepository interface {
	Create(context.Context, *model.MergeGateAudit) error
	GetByTaskID(context.Context, string) (*model.MergeGateAudit, error)
}

// VerificationArtifactReader 是验证处方的只读窄端口:按 (task, kind, name)
// 读取 artifact,复放开发记录的测试命令。独立于写端口,最小装配可不提供。
type VerificationArtifactReader interface {
	ListByTaskAndName(context.Context, string, string, string) ([]model.Artifact, error)
}

type EffectRepository interface {
	Update(context.Context, *model.Effect) error
	GetByDedupeKey(context.Context, string) (*model.Effect, error)
	ListPending(context.Context, int) ([]model.Effect, error)
	ListStaleRunning(context.Context, interface{}, int) ([]model.Effect, error)
	ListByRunID(context.Context, string) ([]model.Effect, error)
	UpsertByDedupeKey(context.Context, *model.Effect) error
	// Claim 原子抢占执行权：pending 恒可抢，running 超 staleWindow 可接管；
	// won=false 表示执行权在别处。Complete/RequeueIfRunning 带 attempt 代次
	// 栅栏，被接管的旧持有方回写 0 行。FailStaleRunning/SkipIfPending 是恢复
	// 与作废路径的条件终态回写（不改 attempt 计数）。
	Claim(ctx context.Context, dedupeKey string, staleWindow time.Duration) (*model.Effect, bool, error)
	Complete(ctx context.Context, dedupeKey string, attempt int, status model.EffectStatus, responseJSON, errorMessage string) (bool, error)
	RequeueIfRunning(ctx context.Context, dedupeKey string, attempt int) (bool, error)
	FailStaleRunning(ctx context.Context, dedupeKey string, staleWindow time.Duration, errorMessage string) (bool, error)
	SkipIfPending(ctx context.Context, dedupeKey string, errorMessage string) (bool, error)
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
	tickMu                         sync.Mutex
	mergeGateMu                    sync.Mutex
	mergeGateCursor                uint
	workItemLeases                 WorkItemLeasePort
	workItemLeaseOwner             string
	tasks                          TaskRepository
	runs                           RunRepository
	artifacts                      ArtifactRepository
	effects                        EffectRepository
	effectExecutor                 EffectExecutor
	writebacks                     WritebackRepository
	dependencies                   DependencyRepository
	resultSubmissions              ResultSubmissionRepository
	codeHostResolver               CodeHostBackendResolver
	mergeRequestReaderResolver     MergeRequestReaderResolver
	reworkMergeRequestFetchTimeout time.Duration
	writebackAuth                  WritebackAuthHandler
	repositoryLookup               RepositoryLookup
	assigneeMentionDirectory       AssigneeMentionDirectory
	developerIMTarget              string
	reviewerIMMentionsProvider     func(context.Context) []string
	reviewerRunner                 ReviewerRunnerExecutionCoordinator
	reviewerRunnerSettings         ReviewerRunnerSettings
	developerRunner                DeveloperRunnerExecutionCoordinator
	developerRunnerSettings        ReviewerRunnerSettings
	testerRunner                   TesterRunnerExecutionCoordinator
	testerRunnerSettings           ReviewerRunnerSettings
	verificationArtifacts          VerificationArtifactReader
	mergeAudits                    MergeGateAuditRepository
	autoMergeEnabled               func(context.Context) bool
	mergeGateClients               func(*registeredrepo.RegisteredRepository) (codehost.MergeRequestReader, codehost.MergeRequestMerger, error)
	agentExecutions                AgentExecutionLookup
	runnerArtifacts                RunnerArtifactStore
	runWorkspaceCleaner            RunWorkspaceCleaner
	stateMachine                   *StateMachine
	policy                         Policy
	now                            func() time.Time
}

type ReviewerRunnerExecutionCoordinator interface {
	Start(context.Context, ReviewerRunnerStartRequest) (*model.AgentExecution, error)
	Reconcile(context.Context, string) (*ReviewerRunnerReconcileResult, error)
}

type DeveloperRunnerExecutionCoordinator interface {
	Start(context.Context, DeveloperRunnerStartRequest) (*model.AgentExecution, error)
	Reconcile(context.Context, string) (*DeveloperRunnerReconcileResult, error)
}

type TesterRunnerExecutionCoordinator interface {
	Start(context.Context, TesterRunnerStartRequest) (*model.AgentExecution, error)
	Reconcile(context.Context, string) (*ReviewerRunnerReconcileResult, error)
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
	// ResolveRepository 按任务解析注册仓行:workspace clone/push URL 与 git 凭据的
	// 唯一权威源(URL 由仓行 CloneURL 派生、凭据取同一行的每仓 Token,单次查询双消费);
	// 任务未绑定注册仓时返回 ErrRepositoryNotBound。
	ResolveRepository func(ctx context.Context, task *model.Task) (*registeredrepo.RegisteredRepository, error)
	TaskTimeout       time.Duration
}

// RegisteredRepositoryFor 是 ResolveRepository 的空值安全入口,装配遗漏或
// resolver 返回空行时给出明确错误而非 panic。
func (s ReviewerRunnerSettings) RegisteredRepositoryFor(ctx context.Context, task *model.Task) (*registeredrepo.RegisteredRepository, error) {
	if s.ResolveRepository == nil {
		return nil, errors.New("runner repository resolver is required")
	}
	repo, err := s.ResolveRepository(ctx, task)
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return nil, errors.New("runner repository resolver returned no repository")
	}
	return repo, nil
}

// gitCredentialFrom 用注册仓行的每仓 Token 构造 git 凭据,与 CloneURL 同行同查。
func gitCredentialFrom(repo *registeredrepo.RegisteredRepository) *workspace.GitCredential {
	if repo == nil {
		return nil
	}
	return &workspace.GitCredential{Username: "oauth2", Token: repo.Token}
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

// WithReviewerIMMentions 固定提醒人名单(静态装配与测试用)。
func WithReviewerIMMentions(mentions []string) Option {
	return WithReviewerIMMentionsProvider(func(context.Context) []string { return mentions })
}

// WithReviewerIMMentionsProvider 动态提醒人来源:引擎侧接组织段配置,
// 每次审查收尾现取,配置保存即生效。
func WithReviewerIMMentionsProvider(provider func(context.Context) []string) Option {
	return func(o *Orchestrator) {
		o.reviewerIMMentionsProvider = provider
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

func WithTesterRunner(coordinator TesterRunnerExecutionCoordinator, settings ReviewerRunnerSettings) Option {
	return func(o *Orchestrator) {
		o.testerRunner = coordinator
		o.testerRunnerSettings = settings
	}
}

func WithVerificationArtifactReader(reader VerificationArtifactReader) Option {
	return func(o *Orchestrator) {
		o.verificationArtifacts = reader
	}
}

func WithMergeGateAuditRepository(audits MergeGateAuditRepository) Option {
	return func(o *Orchestrator) {
		o.mergeAudits = audits
	}
}

func WithAutoMergeEnabled(lookup func(context.Context) bool) Option {
	return func(o *Orchestrator) {
		o.autoMergeEnabled = lookup
	}
}

func WithMergeGateClients(factory func(*registeredrepo.RegisteredRepository) (codehost.MergeRequestReader, codehost.MergeRequestMerger, error)) Option {
	return func(o *Orchestrator) {
		o.mergeGateClients = factory
	}
}

func WithAgentExecutionLookup(executions AgentExecutionLookup) Option {
	return func(o *Orchestrator) {
		o.agentExecutions = executions
	}
}

// WithResultIngress enables the MCP result submission channel on installed runner
// coordinators. issuer 为 nil（或 endpoint 为空）时通道保持关闭（纯文本模式）。
// 必须排在已安装的 WithReviewerRunner/WithDeveloperRunner/WithTesterRunner 之后调用。
// reviewer/developer 缺失是装配失效，必须 fail-fast；tester 注入是可选能力：
// 仅内建 *TesterRunnerCoordinator 生效，自定义实现静默跳过（JSON 文本回退
// 兜底），不参与 fail-fast。
func WithResultIngress(issuer RunTokenIssuer, endpointURL string) Option {
	return func(o *Orchestrator) {
		reviewerCoordinator, reviewerOK := o.reviewerRunner.(*ReviewerRunnerCoordinator)
		developerCoordinator, developerOK := o.developerRunner.(*DeveloperRunnerCoordinator)
		testerCoordinator, testerOK := o.testerRunner.(*TesterRunnerCoordinator)
		// 装配失效必须 fail-fast：静默 no-op 意味着 dispatch 永远不带 token，
		// agent 全部落文本降级通道且无从排查（仅指 reviewer/developer，
		// tester 的可选语义见函数注释）。
		if issuer != nil && endpointURL != "" && (!reviewerOK || !developerOK) {
			panic("orchestrator: WithResultIngress requires WithReviewerRunner/WithDeveloperRunner to be applied first")
		}
		if reviewerOK {
			reviewerCoordinator.core.setResultIngress(issuer, endpointURL)
		}
		if developerOK {
			developerCoordinator.core.setResultIngress(issuer, endpointURL)
		}
		if testerOK {
			testerCoordinator.core.setResultIngress(issuer, endpointURL)
		}
	}
}

// WithRunWorkspaceCleaner 接入 run 工作区清理端口（远端工作区形态注入
// Runner 受控清理；缺省保留本机文件系统删除）。
func WithRunWorkspaceCleaner(cleaner RunWorkspaceCleaner) Option {
	return func(o *Orchestrator) { o.runWorkspaceCleaner = cleaner }
}

func WithRunnerArtifactStore(store RunnerArtifactStore) Option {
	return func(o *Orchestrator) {
		o.runnerArtifacts = store
	}
}

// WithWorkItemLeases 装配跨实例工作项租约端口（租户库 work_item_leases 实现）。
// 未装配时 writeback/merge gate 保持单实例语义；多实例部署必须装配，否则
// 两台协调器会对同一写回/同一验证任务各自执行远端副作用。同一进程内装配
// 多个编排器（单机多租户引擎）时用 WithWorkItemLeaseOwner 区分租约身份。
func WithWorkItemLeases(leases WorkItemLeasePort) Option {
	return func(o *Orchestrator) {
		o.workItemLeases = leases
	}
}

// WithWorkItemLeaseOwner 覆盖租约 owner：默认 instance.ID()（进程身份）。
// 同一进程并发多个引擎/编排器实例时,各实例必须持不同 owner,否则同 owner
// 的第二次 Acquire 会被当作重入续期直接获胜,失去互斥。
func WithWorkItemLeaseOwner(owner string) Option {
	return func(o *Orchestrator) {
		o.workItemLeaseOwner = owner
	}
}

// workItemLeaseIdentity 返回本编排器的租约 owner 身份。
func (o *Orchestrator) workItemLeaseIdentity() string {
	if o.workItemLeaseOwner != "" {
		return o.workItemLeaseOwner
	}
	return instance.ID()
}

// acquireWorkItemLease 按工作项抢跨实例执行权；端口未装配时恒获胜（单实例
// 部署语义不变）。owner 绑定实例身份+链路，失败留待租期到期自然回收。
func (o *Orchestrator) acquireWorkItemLease(ctx context.Context, scope, operation string, ttl time.Duration) (model.WorkItemLease, bool, error) {
	if o.workItemLeases == nil {
		return model.WorkItemLease{}, true, nil
	}
	return o.workItemLeases.Acquire(ctx, scope, o.workItemLeaseIdentity()+":"+operation, operation, ttl)
}

// releaseWorkItemLease 尽力释放执行权（失败留待租期到期自然回收）。
func (o *Orchestrator) releaseWorkItemLease(lease model.WorkItemLease) {
	if o.workItemLeases == nil || lease.Scope == "" {
		return
	}
	releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = o.workItemLeases.Release(releaseCtx, lease.Scope, lease.Owner, lease.Generation)
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

// MergeRequestReaderResolver 按 repository_id 解析平台客户端的 MR 读取能力;
// 供 rework 分支补链出网(当前仅 GitHub 载荷缺失分支时消费)。
type MergeRequestReaderResolver interface {
	ResolveMergeRequestReader(ctx context.Context, repositoryID string) (codehost.MergeRequestReader, error)
}

func WithMergeRequestReaderResolver(resolver MergeRequestReaderResolver) Option {
	return func(o *Orchestrator) {
		o.mergeRequestReaderResolver = resolver
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
	if orch.reworkMergeRequestFetchTimeout == 0 {
		orch.reworkMergeRequestFetchTimeout = reworkMergeRequestFetchTimeout
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
	// 列级条件更新只碰状态、错误信息与完成时间：本函数持有的是派发前的
	// 旧快照，分钟级工作区准备窗口内 run 可能已被结果回传通道终态化，
	// 整行保存会把库中的终态回滚成旧值。
	completedAt := o.now()
	applied, err := o.runs.UpdateFailedIfInFlight(ctx, run.RunID, cause.Error(), completedAt)
	if err != nil {
		log.Printf("orchestrator: record dispatch failure for run %s (task %s): update failed: %v; dispatch cause: %v", run.RunID, task.TaskID, err, cause)
		return
	}
	if !applied {
		// 未生效说明并发方已把 run 终态化且任务已随结果推进：失败回写到此
		// 为止，不推进任务也不回写 LastError，防止把已推进的任务回拉。
		log.Printf("orchestrator: skip dispatch failure writeback for run %s: run already finalized concurrently", run.RunID)
		return
	}

	advanced, err := o.AdvanceTask(ctx, task.TaskID, nextTaskStatus)
	if err == nil {
		advanced.LastError = cause.Error()
		if _, updateErr := o.tasks.UpdateLastErrorIfStatus(ctx, advanced.TaskID, advanced.Status, cause.Error()); updateErr != nil {
			log.Printf("orchestrator: record dispatch failure for task %s: last error update failed: %v", advanced.TaskID, updateErr)
		}
		o.materializeTesterStopIMEffect(ctx, advanced, cause.Error())
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

	applied, err := o.tasks.AdvanceGuarded(ctx, task, previous)
	if err != nil {
		return nil, fmt.Errorf("advance task %s: %w", taskID, err)
	}
	if !applied {
		// 并发推进（另一实例或人工操作先改了状态）：按库内当前状态裁决。
		// 目标状态已达 → 幂等成功；否则显式失败，调用方不得用旧快照续写。
		current, reloadErr := o.tasks.GetByTaskID(ctx, taskID)
		if reloadErr != nil {
			return nil, fmt.Errorf("advance task %s: reload after concurrent advance: %w", taskID, reloadErr)
		}
		if current.Status == next {
			return current, nil
		}
		return nil, fmt.Errorf("%w: task %s moved %s -> %s while advancing to %s",
			ErrConcurrentTaskAdvance, taskID, previous, current.Status, next)
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
	// expandedNextTasks 现在只统计阻塞式子任务(fire-and-forget 不计):
	// 只有父任务必须等子任务收口的阻塞式派生才把父任务送进 waiting_upstream。
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

	// 验证链的人工停靠必须有触点:tester 任务经结果应用落到 human_required
	// (上下文不足)或 blocked(重试预算耗尽)时物化 IM 通知,否则无证据出口
	// 对人工侧不可见。本轮只收窄到 tester 角色控制风险半径;developer/
	// reviewer 同类停靠的统一通知收口挂账。
	if task.Role == "TesterWorker" &&
		(nextTaskStatus == model.TaskStatusHumanRequired || nextTaskStatus == model.TaskStatusBlocked) {
		cause := ""
		if payload.BlockerReason != nil {
			cause = *payload.BlockerReason
		}
		o.materializeHumanRequiredIMEffect(ctx, task, cause)
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

	// 恢复通知绑定刚失败的这次尝试（恢复前的 attempt 值与过期 run），与
	// 既有 dedupe key 语义一致；先留恢复前快照再原子回收。
	notify := *task
	notify.Status = next

	// 原子回收:条件 UPDATE 带"仍在 claimed/running 且租约已过期"栅栏,
	// attempt_count 在数据库侧 +1。双实例同时回收同一过期任务时恰好一个
	// 赢家;败者按库内最新状态返回,不重复物化恢复通知,不回滚胜者写入。
	applied, err := o.tasks.RecoverExpiredGuarded(ctx, taskID, next, "lease expired; task recovered by coordinator", o.now())
	if err != nil {
		return nil, fmt.Errorf("recover task %s: %w", taskID, err)
	}
	if !applied {
		return o.tasks.GetByTaskID(ctx, taskID)
	}

	task, err = o.tasks.GetByTaskID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("recover task %s: reload after guarded update: %w", taskID, err)
	}
	if notify.Status == model.TaskStatusRetryPending {
		o.materializeLeaseRecoveryIMEffect(ctx, &notify)
	} else if notify.Status == model.TaskStatusHumanRequired {
		o.materializeLeaseHumanRequiredIMEffect(ctx, &notify)
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
	var reviewerIMMentions []string
	if o.reviewerIMMentionsProvider != nil {
		reviewerIMMentions = normalizeIMMentions(o.reviewerIMMentionsProvider(ctx))
	}
	platform := strings.Join(reviewerIMMentions, ",")
	resolved := registeredrepo.ResolvePeople(o.lookupRegisteredRepo(ctx, task), registeredrepo.PeopleSettings{
		ReviewerMentions: platform,
	})
	if strings.TrimSpace(resolved.ReviewerMentions) == "" || resolved.ReviewerMentions == platform {
		return reviewerIMMentions
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
