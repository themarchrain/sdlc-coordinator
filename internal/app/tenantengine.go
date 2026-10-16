package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"sdlc-coordinator/internal/adapters/agentrunner"
	"sdlc-coordinator/internal/admin"
	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/events"
	handlers "sdlc-coordinator/internal/http/handlers"
	"sdlc-coordinator/internal/instance"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/orgconfig"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/requirements"
	"sdlc-coordinator/internal/resulttoken"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/runnerpool"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/tenantdb"
	"sdlc-coordinator/internal/workspace"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// engineShared 汇总跨引擎共享的平台级装配依赖：出网客户端、控制面、
// 密钥与配置——这些与具体库无关，全部引擎共用一份。
type engineShared struct {
	cfg                 config.Config
	logger              *zap.Logger
	poolSource          *runnerpool.ControlPlanePoolSource
	defaultRunner       *agentrunner.Client
	runnerClientFactory func(endpoint string) orchestrator.ReviewerRunnerClient
	resultTokenSecret   []byte
}

// tenantEngine 是一个业务库（平台主库或租户库）的完整执行引擎：
// 事件处理、任务编排、结果回传通道、需求同步全部落在本库句柄上。
// 引擎内部不知道「租户」概念——隔离完全由库句柄承担（红线：业务
// 模型无 tenant_id 列，业务代码无 tenant_id 过滤）。
type tenantEngine struct {
	tenantID     uint
	db           *gorm.DB
	logger       *zap.Logger
	orgConfig    *orgconfig.Provider
	orch         *orchestrator.Orchestrator
	processor    *events.Processor
	requirements *requirements.Service
	mcpView      handlers.MCPEngineView
}

// staticTaskTenantResolver 把派发路由的租户解析固定为本引擎的租户：
// 引擎内任务的仓行必在本库，无需再经索引反查（旧的任务→平台库仓行
// 反查在真分库下必然落空）。未指派端点的租户由池路由回退共享默认池。
type staticTaskTenantResolver struct {
	tenantID uint
}

func (r staticTaskTenantResolver) ResolveTenant(_ context.Context, _ *model.Task) (uint, error) {
	return r.tenantID, nil
}

// buildTenantEngine 对一个租户库句柄装配完整引擎。db 由调用方保证已迁移
// （migrateTenantStores/provisioner）。
func buildTenantEngine(db *gorm.DB, tenantID uint, shared engineShared) (*tenantEngine, error) {
	taskRepo := repository.NewTaskRepository(db)
	runRepo := repository.NewRunRepository(db)
	agentExecutionRepo := repository.NewAgentExecutionRepository(db)
	artifactRepo := repository.NewArtifactRepository(db)
	auditRepo := repository.NewMergeGateAuditRepository(db)
	dependencyRepo := repository.NewDependencyRepository(db)
	writebackRepo := repository.NewWritebackRepository(db)
	effectRepo := repository.NewEffectRepository(db)
	inboundEventRepo := repository.NewInboundEventRepository(db)
	resultSubmissionRepo := repository.NewResultSubmissionRepository(db)

	repoStore := registeredrepo.NewStore(db)
	opts := []orchestrator.Option{
		orchestrator.WithArtifactRepository(artifactRepo),
		orchestrator.WithDependencyRepository(dependencyRepo),
		orchestrator.WithWritebackRepository(writebackRepo),
		orchestrator.WithEffectRepository(effectRepo),
		orchestrator.WithResultSubmissionRepository(resultSubmissionRepo),
		orchestrator.WithDeveloperClarificationIMTarget(""),
	}

	// 组织段配置（租户库持有）驱动的三个动态消费点：IM 执行器、提醒人、
	// 接续开发白名单。全部经 Provider 读通解析，保存即热生效。
	orgConfig := orgconfig.NewProvider(db)
	opts = append(opts,
		orchestrator.WithEffectExecutor(newTenantEffectExecutor(orgConfig, shared.logger, tenantID)),
		orchestrator.WithReviewerIMMentionsProvider(tenantReviewerMentionsProvider(orgConfig, shared.logger, tenantID)),
	)
	// 平台身份按仓行解析:resolver 与写回探针按任务所属仓的 provider/base_url
	// 分派。组织段 codehost 配置仅作注册表单默认值(前端经 tenant-configuration
	// 读取注入);仓行身份显式必填,运行时不做全局回填。
	codeHostResolver := registeredrepo.NewCodeHostResolver(repoStore, nil)
	opts = append(opts,
		orchestrator.WithCodeHostBackendResolver(codeHostResolver),
		orchestrator.WithMergeRequestReaderResolver(codeHostResolver),
		orchestrator.WithWritebackAuthHandler(registeredrepo.NewWritebackAuthHandler(repoStore, nil)),
	)
	peopleService, _, err := newPeopleDirectory(db)
	if err != nil {
		return nil, fmt.Errorf("build people directory: %w", err)
	}
	opts = append(opts, orchestrator.WithAssigneeMentionDirectory(newPeopleMentionDirectory(peopleService, repoStore)))
	// 事件必须命中注册仓,任务必须绑定注册仓。写回凭据取该仓 Token。
	opts = append(opts, orchestrator.WithRepositoryLookup(repoStore))
	// Agent Runner 执行层：reviewer/developer 都经 agent-runner 派发 CLI 子进程。
	providerHeaders := make(map[string]string, len(shared.cfg.AgentRunner.CustomHeaders))
	for name, value := range shared.cfg.AgentRunner.CustomHeaders {
		providerHeaders[name] = value
	}
	runnerSettings := orchestrator.ReviewerRunnerSettings{
		WorkspaceRoot: shared.cfg.AgentRunner.WorkspaceRoot,
		TaskTimeout:   shared.cfg.AgentRunner.TaskTimeout,
		Provider: runner.ProviderConfig{
			Name: shared.cfg.AgentRunner.Provider, Revision: int64(shared.cfg.AgentRunner.ProviderConfigRevision),
			BaseURL: shared.cfg.AgentRunner.ProviderBaseURL, APIKey: shared.cfg.AgentRunner.APIKey,
			Headers: providerHeaders, Model: shared.cfg.AgentRunner.Model, Effort: shared.cfg.AgentRunner.Effort,
		},
		ResolveRepository: runnerRepositoryResolver(repoStore),
	}
	var orch *orchestrator.Orchestrator
	orchSubmitter := orchestrator.ReviewerResultSubmitterFunc(func(ctx context.Context, request orchestrator.SubmitResultRequest) (*orchestrator.SubmitResultResponse, error) {
		return orch.SubmitResult(ctx, request)
	})
	// 工作区形态：远端（计划 A，Runner 本机执行受控 Git）或本机（单机开发
	// 与测试）。三个角色的准备与交付必须同形态，不得混用。
	var (
		reviewerPreparer    orchestrator.ReviewerWorkspacePreparer  = workspace.NewReviewerPreparer()
		developerPreparer   orchestrator.DeveloperWorkspacePreparer = workspace.NewDeveloperPreparer()
		deliveryFinalizer   orchestrator.DeveloperGitFinalizer      = workspace.NewDeveloperFinalizer()
		runnerArtifactStore orchestrator.RunnerArtifactStore        = shared.defaultRunner
	)
	if shared.cfg.AgentRunner.RemoteWorkspace {
		// 按 run 归属端点路由：准备、任务包投递、commit/push、清理必须命中
		// 同一 runner（独立卷下别的端点既没有该 run 的仓库也没有 Agent 的
		// 修改）。端点由共享库的 run 行解析（首次派发准备前钉住）。
		endpointForRun := func(ctx context.Context, runID string) (string, error) {
			run, err := runRepo.GetByRunID(ctx, runID)
			if err != nil {
				return "", fmt.Errorf("resolve runner endpoint for run %s: %w", runID, err)
			}
			return strings.TrimRight(strings.TrimSpace(run.RunnerEndpoint), "/"), nil
		}
		remoteWorkspace := agentrunner.NewRoutedRemoteWorkspace(
			shared.cfg.AgentRunner.ServiceURL, shared.cfg.AgentRunner.InternalToken, tenantID, nil, endpointForRun,
		)
		reviewerPreparer = agentrunner.NewRemoteReadOnlyPreparer(remoteWorkspace)
		developerPreparer = agentrunner.NewRemoteDeveloperPreparer(remoteWorkspace)
		deliveryFinalizer = remoteWorkspace
		runnerArtifactStore = remoteWorkspace
		opts = append(opts, orchestrator.WithRunWorkspaceCleaner(remoteWorkspace))
	}
	reviewerRunnerCoordinator := orchestrator.NewReviewerRunnerCoordinator(
		taskRepo, runRepo, agentExecutionRepo, reviewerPreparer, shared.defaultRunner,
		orchSubmitter,
	)
	developerRunnerCoordinator := orchestrator.NewDeveloperRunnerCoordinator(
		taskRepo, runRepo, agentExecutionRepo, developerPreparer, deliveryFinalizer, shared.defaultRunner,
		orchSubmitter,
		runnerSettings,
	)
	// 远端形态：任务包必须经受控通道到达 run 所属 Runner（协调器本地路径
	// 在那边不可见）；本机形态不注入，任务包由协调器直接写本地路径。
	if bundlePublisher, ok := deliveryFinalizer.(orchestrator.TaskBundlePublisher); ok && shared.cfg.AgentRunner.RemoteWorkspace {
		reviewerRunnerCoordinator.SetTaskBundlePublisher(bundlePublisher)
		developerRunnerCoordinator.SetTaskBundlePublisher(bundlePublisher)
	}
	// 交付检查点(租户库):派发时钉基线,commit 与 push 之间写交付 SHA,
	// 是"提交由 Coordinator 创建"的唯一归属凭据;Agent 不可写租户库。
	deliveryCheckpoints := repository.NewDeveloperDeliveryCheckpointRepository(db)
	developerRunnerCoordinator.SetDeveloperDeliveryCheckpoints(deliveryCheckpoints)
	developerRunnerCoordinator.SetDeliveryExecutionRight(repository.NewWorkItemLeaseRepository(db))
	// 验证任务与 reviewer 共用同一只读工作区 preparer:两者都是按冻结头部
	// SHA 的只读检出,差别只在角色门禁与回传工具;tester pool 路由随核心
	// coordinator 注入(装配于 NewReady 前,构造后接入安全)。
	testerRunnerCoordinator := orchestrator.NewTesterRunnerCoordinator(
		taskRepo, runRepo, agentExecutionRepo, reviewerPreparer, shared.defaultRunner,
		orchSubmitter,
	)
	if bundlePublisher, ok := deliveryFinalizer.(orchestrator.TaskBundlePublisher); ok && shared.cfg.AgentRunner.RemoteWorkspace {
		testerRunnerCoordinator.SetTaskBundlePublisher(bundlePublisher)
	}
	opts = append(opts,
		orchestrator.WithReviewerRunner(reviewerRunnerCoordinator, runnerSettings),
		orchestrator.WithDeveloperRunner(developerRunnerCoordinator, runnerSettings),
		orchestrator.WithTesterRunner(testerRunnerCoordinator, runnerSettings),
		orchestrator.WithVerificationArtifactReader(artifactRepo),
		orchestrator.WithMergeGateAuditRepository(auditRepo),
		orchestrator.WithAutoMergeEnabled(tenantAutoMergeEnabledLookup(orgConfig, shared.logger, tenantID)),
		orchestrator.WithMergeGateClients(orchestrator.DefaultMergeGateClientFactory),
		orchestrator.WithAgentExecutionLookup(agentExecutionRepo),
		orchestrator.WithRunnerArtifactStore(runnerArtifactStore),
		// 写回/合并门禁的跨实例执行权:双协调器对同一写回、同一验证任务的
		// 远端副作用必须单赢家执行(与交付执行权同库同语义)。
		orchestrator.WithWorkItemLeases(repository.NewWorkItemLeaseRepository(db)),
	)
	// MCP 结果回传通道：签名密钥仅存内存且全引擎共享；token 声明携带本引擎
	// 租户，回传端按声明路由回同一引擎（无租户声明的 token 一律拒绝）。
	// TTL 跟随本次派发解析出的实际超时：组织放宽超时后 token 不能先于任务
	// 结束过期，轮询余量在 resultTokenTTLGrace 上叠加。
	ingressIssuer := orchestrator.RunTokenIssuerFunc(func(taskID, runID, role string, taskTimeout time.Duration) (string, error) {
		return resulttoken.Issue(shared.resultTokenSecret, resulttoken.Claims{
			TaskID: taskID, RunID: runID, Role: role, TenantID: tenantID,
			TTL: taskTimeout + resultTokenTTLGrace,
		}, time.Now())
	})
	mcpChannel := orchestrator.NewResultChannelCoordinator(
		deliveryFinalizer,
		orchSubmitter,
		runnerSettings, "mcp",
	)
	mcpChannel.SetDeveloperDeliveryCheckpoints(deliveryCheckpoints)
	// 交付执行权（run 级租约）：MCP 与文本两个通道的 commit/push 前都经它
	// 竞争同一处理权，保证跨实例只有一次可信交付。
	mcpChannel.SetDeliveryExecutionRight(repository.NewWorkItemLeaseRepository(db))
	opts = append(opts, orchestrator.WithResultIngress(ingressIssuer, shared.cfg.Scheduler.TickRequest.CallbackBaseURL))
	orch = orchestrator.New(taskRepo, runRepo, opts...)
	processor := events.NewProcessor(inboundEventRepo, orch)
	processor.SetGitLabReworkLookup(tenantReworkAllowlistLookup(orgConfig, shared.logger, tenantID))
	processor.SetRepositoryLookup(repoStore)
	requirementsService := requirements.NewService(
		requirements.NewStore(db), repoStore,
		requirements.NewProviderClientFactory(nil),
	)
	// 需求发布的跨实例执行权:双协调器的 syncer 不得为同一需求各建一个
	// 远端 issue(与写回执行权同库同语义)。
	requirementsService.WithWorkItemLeases(repository.NewWorkItemLeaseRepository(db), instance.ID())
	// 租户 runner 池路由：本引擎任务恒解析为本引擎租户；未指派端点的租户
	// 由池路由回退共享默认池。
	tenantRunnerRouter := runnerpool.NewRouter(
		staticTaskTenantResolver{tenantID: tenantID},
		shared.poolSource,
		shared.runnerClientFactory,
	)
	reviewerRunnerCoordinator.SetTenantRunnerRouter(tenantRunnerRouter)
	developerRunnerCoordinator.SetTenantRunnerRouter(tenantRunnerRouter)
	testerRunnerCoordinator.SetTenantRunnerRouter(tenantRunnerRouter)
	// 组织段执行选择（CLI 与推理强度）：引擎派发时现取（保存即热生效），
	// 未配置的字段回落系统默认。
	runnerProviderLookup := tenantRunnerProviderLookup(orgConfig, shared.logger, tenantID)
	reviewerRunnerCoordinator.SetRunnerProviderLookup(runnerProviderLookup)
	developerRunnerCoordinator.SetRunnerProviderLookup(runnerProviderLookup)
	testerRunnerCoordinator.SetRunnerProviderLookup(runnerProviderLookup)

	return &tenantEngine{
		tenantID:     tenantID,
		db:           db,
		logger:       shared.logger,
		orgConfig:    orgConfig,
		orch:         orch,
		processor:    processor,
		requirements: requirementsService,
		mcpView: handlers.MCPEngineView{
			Channel: mcpChannel, Tasks: taskRepo, Runs: runRepo,
			Rejections: repository.NewResultRejectionRepository(db),
		},
	}, nil
}

// enginePool 惰性构造并缓存各租户引擎；不存在平台引擎——tenantID=0
// 一律按不可用处理（无作用域会话、存量无租户 token 全部 fail-closed）。
// 租户全集每轮从控制面动态拉取（新建/停用组织自然生效），构造失败
// 记日志并跳过（不阻塞其他引擎）。
type enginePool struct {
	mu       sync.Mutex
	engines  map[uint]*tenantEngine
	resolver tenantdb.UnmanagedResolver
	control  *controlplane.Service
	index    *controlplane.Repository
	shared   engineShared
	logger   *zap.Logger
}

// newEnginePool 准备租户引擎的惰性构造；引擎随首次访问逐个建立。
func newEnginePool(resolver tenantdb.UnmanagedResolver, control *controlplane.Service, shared engineShared, logger *zap.Logger) *enginePool {
	return &enginePool{
		engines:  map[uint]*tenantEngine{},
		resolver: resolver,
		control:  control,
		index:    control.Repository(),
		shared:   shared,
		logger:   logger,
	}
}

// forTenantID 返回该租户的引擎；0（无租户）恒为 nil——平台库没有业务
// 引擎，无作用域调用方按各自语义 fail-closed。未缓存则打开租户库并构造。
// 打开或构造失败返回 nil（调用方按场景兜底或放弃）。
//
// 句柄走 OpenUnmanaged（不参与 resolver 的 LRU 淘汰）：引擎与句柄同为
// 进程生命周期，缓存通道的淘汰 Close 会截断在用连接。
//
// 构造全程持锁：引擎构造含 schema 迁移（DDL），并发构造同一租户库会在
// sqlite 写锁上互踩（实测 16 并发构造即触发 BUSY 失败）；首建仅在缓存
// miss 时发生一次，串行化代价可接受，换来「同租户恒同一实例」的无竞态
// 保证（实例级 tickMu 单飞锁因此不分裂）。持锁做 IO 是有意取舍。
func (p *enginePool) forTenantID(ctx context.Context, tenantID uint) *tenantEngine {
	if tenantID == 0 {
		return nil
	}
	// 停用（或状态异常）租户一律无引擎：webhook/管理操作/MCP 全部落空，
	// 调用方按各自语义 fail-closed。逐调用查一次控制面（本地 sqlite），
	// 缓存命中也不例外——停用必须即时生效，不等引擎重建。
	tenant, err := p.index.GetTenant(ctx, tenantID)
	if err != nil {
		p.logger.Warn("lookup tenant for engine failed", zap.Uint("tenant_id", tenantID), zap.Error(err))
		return nil
	}
	if tenant.Status != controlplane.TenantStatusActive {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if engine, ok := p.engines[tenantID]; ok {
		return engine
	}
	store, err := p.resolver.OpenUnmanaged(tenantID)
	if err != nil {
		p.logger.Warn("open tenant store for engine failed", zap.Uint("tenant_id", tenantID), zap.Error(err))
		return nil
	}
	engine, err := buildTenantEngine(store.DB, tenantID, p.shared)
	if err != nil {
		p.logger.Warn("build tenant engine failed", zap.Uint("tenant_id", tenantID), zap.Error(err))
		return nil
	}
	p.engines[tenantID] = engine
	return engine
}

// CloseTenantEngines 关闭全部租户引擎持有的独立句柄并清空缓存。
// 引擎句柄不经 resolver 缓存通道，优雅停机与测试清理需经此显式释放。
func (p *enginePool) CloseTenantEngines() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, engine := range p.engines {
		if engine == nil || engine.db == nil {
			continue
		}
		if sqlDB, err := engine.db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		delete(p.engines, id)
	}
}

// all 返回当前全部活跃租户引擎（租户清单动态拉取；拉取失败返回空集）。
func (p *enginePool) all(ctx context.Context) []*tenantEngine {
	result := []*tenantEngine{}
	tenants, err := p.control.ListTenants(ctx)
	if err != nil {
		p.logger.Warn("list tenants for engine sweep failed", zap.Error(err))
		return result
	}
	for _, tenant := range tenants {
		if tenant.ID == 0 || tenant.Status != controlplane.TenantStatusActive {
			continue
		}
		if engine := p.forTenantID(ctx, tenant.ID); engine != nil {
			result = append(result, engine)
		}
	}
	return result
}

// errTenantEngineUnavailable 表示路由索引已指认租户,但其引擎当前不可用
// (组织停用、租户库打开或构造失败)。webhook 处理器据此 fail-closed 回
// 5xx 让代码托管重试,不回落平台库。
var errTenantEngineUnavailable = errors.New("tenant engine unavailable")

// errRepoRouteMissing 表示验签通过后路由索引条目已缺失（仅剩验签与解析
// 之间条目被对账删除的微窗口）：fail-closed 回 5xx，重投后由验签按未注册
// 拒绝闭环。
var errRepoRouteMissing = errors.New("repo route missing")

// engineViewForPath 按 (provider, path) 的路由索引解析引擎视图（处理器+
// 按组织段配置构建的 Gitea/GitHub normalizer）：索引唯一行 → 该租户引擎
// 的视图；索引指认租户但引擎不可用 → 错误（fail closed，调用方 5xx 让代码
// 托管重试），故障瞬间的事件宁可被重试也不丢失。repoURL 是载荷自带完整
// 链接，仅在同路径多行命中时用于推导实例地址消歧（消歧失败同样返回
// 错误——验签层已 fail-closed，此处防御性兜底）。租户配置冷读失败同样
// 返回错误（拒收重试）。未命中同样返回错误：验签层已把未路由仓按未注册
// 401 拒收，能走到这里只剩「验签通过后条目被对账删除」的微窗口，拒收
// 重试让下一轮验签闭环。
func (p *enginePool) engineViewForPath(ctx context.Context, provider, path, repoURL string) (*handlers.WebhookEngineView, error) {
	tenantID, err := resolveTenantByPath(ctx, p.index, provider, path, repoURL)
	if err != nil {
		return nil, err
	}
	if tenantID == 0 {
		return nil, errRepoRouteMissing
	}
	engine := p.forTenantID(ctx, tenantID)
	if engine == nil {
		return nil, errTenantEngineUnavailable
	}
	return engine.tenantWebhookView(ctx)
}

// engineForSession 按请求会话的组织作用域解析引擎；无作用域会话（超管
// 直连）恒为 nil——平台库没有业务引擎，管理操作面据此 503，不回落任何
// 默认库（那会把操作写进错误的库）。租户引擎不可用同样返回 nil。
func (p *enginePool) engineForSession(c *gin.Context) *tenantEngine {
	tenantID := uint(0)
	if session := handlers.CurrentPortalSession(c); session != nil && session.EffectiveTenant != nil {
		tenantID = session.EffectiveTenant.ID
	}
	return p.forTenantID(context.Background(), tenantID)
}

// tenantEngineSweepTimeout 限制单引擎一次全租户扫描的占用：引擎内最长的
// git 交付已有 3 分钟上限（developerFinalizeTimeout），取 5 分钟既容纳合法
// 长操作，又防单个卡死引擎串行拖垮全局调度节奏。
const tenantEngineSweepTimeout = 5 * time.Minute

// tickAll 串行执行各引擎的 Tick 并合并结果；单引擎失败或超预算记日志继续。
// 串行为正确性优先：各引擎内部仍有实例级单飞锁，跨引擎并发留给后续按需放开。
func (p *enginePool) tickAll(ctx context.Context, request orchestrator.TickRequest) (*orchestrator.TickResult, error) {
	merged := &orchestrator.TickResult{}
	for _, engine := range p.all(ctx) {
		engineCtx, cancel := context.WithTimeout(ctx, tenantEngineSweepTimeout)
		result, err := engine.orch.Tick(engineCtx, request)
		timedOut := errors.Is(err, context.DeadlineExceeded) || errors.Is(engineCtx.Err(), context.DeadlineExceeded)
		cancel()
		if err != nil {
			if errors.Is(err, orchestrator.ErrTickInProgress) {
				continue
			}
			if timedOut {
				p.logger.Warn("tenant engine tick exceeded sweep budget",
					zap.Uint("tenant_id", engine.tenantID), zap.Duration("budget", tenantEngineSweepTimeout))
				continue
			}
			p.logger.Warn("tenant engine tick failed", zap.Uint("tenant_id", engine.tenantID), zap.Error(err))
			continue
		}
		if result == nil {
			continue
		}
		merged.PolledRuns = append(merged.PolledRuns, result.PolledRuns...)
		merged.ReconciledWritebacks = append(merged.ReconciledWritebacks, result.ReconciledWritebacks...)
		merged.FinalizedResults = append(merged.FinalizedResults, result.FinalizedResults...)
		merged.Recoveries = append(merged.Recoveries, result.Recoveries...)
		merged.DispatchedTasks = append(merged.DispatchedTasks, result.DispatchedTasks...)
		merged.DispatchFailures = append(merged.DispatchFailures, result.DispatchFailures...)
		merged.ExecutedEffects = append(merged.ExecutedEffects, result.ExecutedEffects...)
		merged.GateDecisions = append(merged.GateDecisions, result.GateDecisions...)
		merged.CleanedArtifacts = append(merged.CleanedArtifacts, result.CleanedArtifacts...)
	}
	return merged, nil
}

// requirementsSyncAll 串行同步各引擎的租户库需求；单引擎失败或超预算记
// 日志继续（预算与 tickAll 共用同一常量）。
func (p *enginePool) requirementsSyncAll(ctx context.Context) (requirements.SyncResult, error) {
	var merged requirements.SyncResult
	for _, engine := range p.all(ctx) {
		engineCtx, cancel := context.WithTimeout(ctx, tenantEngineSweepTimeout)
		result, err := engine.requirements.SyncOnce(engineCtx)
		timedOut := errors.Is(err, context.DeadlineExceeded) || errors.Is(engineCtx.Err(), context.DeadlineExceeded)
		cancel()
		if err != nil {
			if errors.Is(err, requirements.ErrSyncInProgress) {
				continue
			}
			if timedOut {
				p.logger.Warn("tenant requirements sync exceeded sweep budget",
					zap.Uint("tenant_id", engine.tenantID), zap.Duration("budget", tenantEngineSweepTimeout))
				continue
			}
			p.logger.Warn("tenant requirements sync failed", zap.Uint("tenant_id", engine.tenantID), zap.Error(err))
			continue
		}
		merged.Created += result.Created
		merged.Recovered += result.Recovered
		merged.Failed += result.Failed
		merged.Skipped += result.Skipped
		merged.Mirrored += result.Mirrored
		merged.MirrorErrors += result.MirrorErrors
	}
	return merged, nil
}

// MCPEngineForTenant 实现 handlers.MCPEngineSource：按 token 租户声明取
// 引擎视图；租户引擎不可用（含 tenantID=0——平台库没有业务引擎，存量
// 无租户 token 寿命早已耗尽）返回零值视图，由回传端 fail-closed（503）。
func (p *enginePool) MCPEngineForTenant(tenantID uint) handlers.MCPEngineView {
	if engine := p.forTenantID(context.Background(), tenantID); engine != nil {
		return engine.mcpView
	}
	return handlers.MCPEngineView{}
}

// OperationsFor 实现 handlers.AdminOperationEngines：按会话租户取引擎编排；
// 引擎不可用返回 nil（管理操作面 503）。
func (p *enginePool) OperationsFor(c *gin.Context) handlers.AdminOperations {
	if engine := p.engineForSession(c); engine != nil {
		return engine.orch
	}
	return nil
}

// EffectsFor 实现 handlers.AdminOperationEngines：按会话租户库建 effect 查询；
// 引擎不可用返回 nil（管理操作面 503）。
func (p *enginePool) EffectsFor(c *gin.Context) handlers.AdminEffectLookup {
	if engine := p.engineForSession(c); engine != nil {
		return admin.NewQueryService(engine.db)
	}
	return nil
}
