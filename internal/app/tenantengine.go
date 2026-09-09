package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"sdlc-coordinator/internal/adapters/agentrunner"
	"sdlc-coordinator/internal/admin"
	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/controlplane"
	effectexec "sdlc-coordinator/internal/effects"
	"sdlc-coordinator/internal/events"
	handlers "sdlc-coordinator/internal/http/handlers"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
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
	cfg                  config.Config
	logger               *zap.Logger
	reworkAllowedUserIDs []int
	effectExecutor       *effectexec.Executor // nil = 通知链关闭
	poolSource           *runnerpool.ControlPlanePoolSource
	defaultRunner        *agentrunner.Client
	runnerClientFactory  func(endpoint string) orchestrator.ReviewerRunnerClient
	resultTokenSecret    []byte
}

// tenantEngine 是一个业务库（平台主库或租户库）的完整执行引擎：
// 事件处理、任务编排、结果回传通道、需求同步全部落在本库句柄上。
// 引擎内部不知道「租户」概念——隔离完全由库句柄承担（红线：业务
// 模型无 tenant_id 列，业务代码无 tenant_id 过滤）。
type tenantEngine struct {
	tenantID     uint
	db           *gorm.DB
	orch         *orchestrator.Orchestrator
	processor    *events.Processor
	requirements *requirements.Service
	mcpView      handlers.MCPEngineView
}

// staticTaskTenantResolver 把派发路由的租户解析固定为本引擎的租户：
// 引擎内任务的仓行必在本库，无需再经索引反查（旧的任务→平台库仓行
// 反查在真分库下必然落空）。平台引擎（tenantID=0）解析为 0，池路由
// 自然回退共享默认池。
type staticTaskTenantResolver struct {
	tenantID uint
}

func (r staticTaskTenantResolver) ResolveTenant(_ context.Context, _ *model.Task) (uint, error) {
	return r.tenantID, nil
}

// buildTenantEngine 对一个库句柄装配完整引擎。db 由调用方保证已迁移
// （平台库 migrateBusinessStore、租户库 migrateTenantStores/provisioner）。
func buildTenantEngine(db *gorm.DB, tenantID uint, shared engineShared) (*tenantEngine, error) {
	taskRepo := repository.NewTaskRepository(db)
	runRepo := repository.NewRunRepository(db)
	agentExecutionRepo := repository.NewAgentExecutionRepository(db)
	artifactRepo := repository.NewArtifactRepository(db)
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
		orchestrator.WithReviewerIMMentions(shared.cfg.ReviewerIMMentions),
	}
	if shared.effectExecutor != nil {
		opts = append(opts, orchestrator.WithEffectExecutor(shared.effectExecutor))
	}
	// 平台身份按仓行解析:resolver 与写回探针按任务所属仓的 provider/base_url
	// 分派。全局 codehost 配置只作为注册表单默认值与空列回填来源,运行时不读取。
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
	reviewerRunnerCoordinator := orchestrator.NewReviewerRunnerCoordinator(
		taskRepo, runRepo, agentExecutionRepo, workspace.NewReviewerPreparer(), shared.defaultRunner,
		orchSubmitter,
	)
	developerRunnerCoordinator := orchestrator.NewDeveloperRunnerCoordinator(
		taskRepo, runRepo, agentExecutionRepo, workspace.NewDeveloperPreparer(), workspace.NewDeveloperFinalizer(), shared.defaultRunner,
		orchSubmitter,
		runnerSettings,
	)
	opts = append(opts,
		orchestrator.WithReviewerRunner(reviewerRunnerCoordinator, runnerSettings),
		orchestrator.WithDeveloperRunner(developerRunnerCoordinator, runnerSettings),
		orchestrator.WithAgentExecutionLookup(agentExecutionRepo),
		orchestrator.WithRunnerArtifactStore(shared.defaultRunner),
	)
	// MCP 结果回传通道：签名密钥仅存内存且全引擎共享；token 声明携带本引擎
	// 租户，回传端按声明路由回同一引擎（存量无租户 token 回落平台）。
	ingressIssuer := orchestrator.RunTokenIssuerFunc(func(taskID, runID, role string) (string, error) {
		return resulttoken.Issue(shared.resultTokenSecret, resulttoken.Claims{
			TaskID: taskID, RunID: runID, Role: role, TenantID: tenantID,
			TTL: shared.cfg.AgentRunner.TaskTimeout + resultTokenTTLGrace,
		}, time.Now())
	})
	mcpChannel := orchestrator.NewResultChannelCoordinator(
		workspace.NewDeveloperFinalizer(),
		orchSubmitter,
		runnerSettings, "mcp",
	)
	opts = append(opts, orchestrator.WithResultIngress(ingressIssuer, shared.cfg.Scheduler.TickRequest.CallbackBaseURL))
	orch = orchestrator.New(taskRepo, runRepo, opts...)
	processor := events.NewProcessor(inboundEventRepo, orch, events.WithCodeHostReworkAllowedUserIDs(shared.reworkAllowedUserIDs))
	processor.SetRepositoryLookup(repoStore)
	requirementsService := requirements.NewService(
		requirements.NewStore(db), repoStore,
		requirements.NewProviderClientFactory(nil),
	)
	// 租户 runner 池路由：本引擎任务恒解析为本引擎租户；未指派端点的租户
	// 由池路由回退共享默认池。
	tenantRunnerRouter := runnerpool.NewRouter(
		staticTaskTenantResolver{tenantID: tenantID},
		shared.poolSource,
		shared.runnerClientFactory,
	)
	reviewerRunnerCoordinator.SetTenantRunnerRouter(tenantRunnerRouter)
	developerRunnerCoordinator.SetTenantRunnerRouter(tenantRunnerRouter)

	return &tenantEngine{
		tenantID:     tenantID,
		db:           db,
		orch:         orch,
		processor:    processor,
		requirements: requirementsService,
		mcpView: handlers.MCPEngineView{
			Channel: mcpChannel, Tasks: taskRepo, Runs: runRepo,
			Rejections: repository.NewResultRejectionRepository(db),
		},
	}, nil
}

// enginePool 惰性构造并缓存各租户引擎；tenantID=0 固定为平台引擎。
// 租户全集每轮从控制面动态拉取（新建/停用组织自然生效），构造失败
// 记日志并跳过（不阻塞其他引擎）。
type enginePool struct {
	mu       sync.Mutex
	engines  map[uint]*tenantEngine
	resolver *tenantdb.SqliteFileResolver
	control  *controlplane.Service
	index    *controlplane.Repository
	shared   engineShared
	platform *tenantEngine
	logger   *zap.Logger
}

// newEnginePool 固定平台引擎（构造失败即装配失败）并准备租户惰性构造。
func newEnginePool(platform *tenantEngine, resolver *tenantdb.SqliteFileResolver, control *controlplane.Service, shared engineShared, logger *zap.Logger) *enginePool {
	return &enginePool{
		engines:  map[uint]*tenantEngine{0: platform},
		resolver: resolver,
		control:  control,
		index:    control.Repository(),
		shared:   shared,
		platform: platform,
		logger:   logger,
	}
}

// forTenantID 返回该租户的引擎（0=平台）；未缓存则打开租户库并构造。
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
		return p.platform
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

// CloseTenantEngines 关闭全部租户引擎持有的独立句柄并清空缓存（平台
// 引擎的库句柄归装配层，不在范围）。引擎句柄不经 resolver 缓存通道，
// 优雅停机与测试清理需经此显式释放。
func (p *enginePool) CloseTenantEngines() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, engine := range p.engines {
		if id == 0 || engine == nil || engine.db == nil {
			continue
		}
		if sqlDB, err := engine.db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		delete(p.engines, id)
	}
}

// all 返回平台引擎与当前全部租户引擎（租户清单动态拉取；拉取失败仅平台）。
func (p *enginePool) all(ctx context.Context) []*tenantEngine {
	result := []*tenantEngine{p.platform}
	tenants, err := p.control.ListTenants(ctx)
	if err != nil {
		p.logger.Warn("list tenants for engine sweep failed", zap.Error(err))
		return result
	}
	for _, tenant := range tenants {
		if tenant.ID == 0 {
			continue
		}
		if engine := p.forTenantID(ctx, tenant.ID); engine != nil {
			result = append(result, engine)
		}
	}
	return result
}

// engineForPath 按 (provider, path) 的路由索引解析事件处理器：索引唯一
// 行 → 该租户引擎的处理器；未命中或引擎不可用 → 平台引擎处理器（与
// webhook 验签的平台兜底对称）；索引歧义 → nil（验签层已 fail-closed，
// 此处防御性返回 nil 交由 handler 报未配置）。
func (p *enginePool) engineForPath(ctx context.Context, provider, path string) *events.Processor {
	tenantID, err := resolveTenantByPath(ctx, p.index, provider, path)
	if err != nil {
		return nil
	}
	if engine := p.forTenantID(ctx, tenantID); engine != nil {
		return engine.processor
	}
	return p.platform.processor
}

// engineForSession 按请求会话的组织作用域解析引擎；无作用域会话用平台引擎。
func (p *enginePool) engineForSession(c *gin.Context) *tenantEngine {
	tenantID := uint(0)
	if session := handlers.CurrentPortalSession(c); session != nil && session.EffectiveTenant != nil {
		tenantID = session.EffectiveTenant.ID
	}
	if engine := p.forTenantID(context.Background(), tenantID); engine != nil {
		return engine
	}
	return p.platform
}

// tickAll 串行执行各引擎的 Tick 并合并结果；单引擎失败记日志继续。
// 串行为正确性优先：各引擎内部仍有实例级单飞锁，跨引擎并发留给后续按需放开。
func (p *enginePool) tickAll(ctx context.Context, request orchestrator.TickRequest) (*orchestrator.TickResult, error) {
	merged := &orchestrator.TickResult{}
	for _, engine := range p.all(ctx) {
		result, err := engine.orch.Tick(ctx, request)
		if err != nil {
			if errors.Is(err, orchestrator.ErrTickInProgress) {
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
		merged.CleanedArtifacts = append(merged.CleanedArtifacts, result.CleanedArtifacts...)
	}
	return merged, nil
}

// requirementsSyncAll 串行同步各引擎的租户库需求；单引擎失败记日志继续。
func (p *enginePool) requirementsSyncAll(ctx context.Context) (requirements.SyncResult, error) {
	var merged requirements.SyncResult
	for _, engine := range p.all(ctx) {
		result, err := engine.requirements.SyncOnce(ctx)
		if err != nil {
			if errors.Is(err, requirements.ErrSyncInProgress) {
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
// 引擎视图；未命中回落平台视图（存量 token 兼容窗口）。
func (p *enginePool) MCPEngineForTenant(tenantID uint) handlers.MCPEngineView {
	if engine := p.forTenantID(context.Background(), tenantID); engine != nil {
		return engine.mcpView
	}
	return p.platform.mcpView
}

// OperationsFor 实现 handlers.AdminOperationEngines：按会话租户取引擎编排。
func (p *enginePool) OperationsFor(c *gin.Context) handlers.AdminOperations {
	return p.engineForSession(c).orch
}

// EffectsFor 实现 handlers.AdminOperationEngines：按会话租户库建 effect 查询。
func (p *enginePool) EffectsFor(c *gin.Context) handlers.AdminEffectLookup {
	return admin.NewQueryService(p.engineForSession(c).db)
}
