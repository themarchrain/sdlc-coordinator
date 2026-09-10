package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sdlc-coordinator/internal/adapters/agentrunner"
	"sdlc-coordinator/internal/admin"
	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/configuration"
	"sdlc-coordinator/internal/connectors"
	"sdlc-coordinator/internal/controlplane"
	api "sdlc-coordinator/internal/http"
	handlers "sdlc-coordinator/internal/http/handlers"
	"sdlc-coordinator/internal/model"
	agentobservability "sdlc-coordinator/internal/observability/agent"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/orgconfig"
	"sdlc-coordinator/internal/people"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/requirements"
	"sdlc-coordinator/internal/resulttoken"
	"sdlc-coordinator/internal/runnerpool"
	"sdlc-coordinator/internal/scheduler"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/tenantdb"
	"sdlc-coordinator/internal/tenantmigration"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// resultSubmissionMaxRejections 矫正次数上限：实测 3 次收敛，留余量取 5
const resultSubmissionMaxRejections = handlers.DefaultMaxCorrectionAttempts

// resultTokenTTLGrace 是 token TTL 在任务超时之外的追加余量，覆盖 runner
// 收尾轮询窗口。
const resultTokenTTLGrace = time.Hour

type App struct {
	Config           config.Config
	Logger           *zap.Logger
	DB               *gorm.DB
	Router           *gin.Engine
	Server           *api.Server
	Orch             *orchestrator.Orchestrator
	pool             *enginePool
	Scheduler        *scheduler.Runner
	RequirementsSync *requirements.Runner
	ControlPlane     *controlplane.Service
	TenantStores     tenantdb.Resolver
	Mode             configuration.Mode
}

func New(cfg config.Config, logger *zap.Logger, db *gorm.DB) (*App, error) {
	return NewReady(cfg, logger, db, nil, nil)
}

func NewReady(cfg config.Config, logger *zap.Logger, db *gorm.DB, configurationService *configuration.Service, shutdown configuration.ShutdownRequester) (*App, error) {
	if err := migrateBusinessStore(db); err != nil {
		return nil, fmt.Errorf("migrate platform database: %w", err)
	}

	// 真分库装配：接口不变，Handle 解析到
	// <TENANT_DATA_DIR>/<tenantID>/db.sqlite；Main() 保持平台库句柄。
	tenantDataDir := cfg.TenantDataDir
	if strings.TrimSpace(tenantDataDir) == "" {
		tenantDataDir = config.DefaultTenantDataDir
	}
	platformStore := &tenantdb.Store{DB: db}
	tenantStores := tenantdb.NewSqliteFileResolver(platformStore, tenantDataDir, 0)
	// 启动中途失败时释放已打开的租户句柄（未关闭的 sqlite 文件句柄会阻塞
	// 外层对数据目录的清理）；成功装配后句柄归 App 所有，不再回收。
	closeTenantStores := true
	defer func() {
		if closeTenantStores {
			tenantStores.Close()
		}
	}()

	// 平台库仓行的作用域唯一索引（引擎装配前置）。
	platformRepoStore := registeredrepo.NewStore(db)
	if err := platformRepoStore.EnsureIdentityIndexes(); err != nil {
		return nil, err
	}

	// MCP 结果回传通道签名密钥仅存内存（重启即失效，在途 agent 落入文本
	// 降级通道）；endpoint 取协调器对外可达地址（scheduler.callback_base_url），
	// 缺省未配置时通道保持关闭（纯文本模式）。
	resultTokenSecret, err := resulttoken.NewSecret()
	if err != nil {
		return nil, fmt.Errorf("generate result channel secret: %w", err)
	}
	// 控制面与 ready/bootstrap 共用装配：DSN 与种子超管缺失时显式失败。
	controlPlaneService, err := newControlPlane(cfg.ControlPlane, logger)
	if err != nil {
		return nil, err
	}
	// 数据面编排：存量单租户部署迁移为「默认组织」（主库复制或新建空租户，
	// 幂等；空库新装跳过）→ 开通钩子挂上（新租户同事务开通）→ 启动遍历
	// 逐库迁移（失败中止并列出失败租户）。
	legacyMigrated, err := tenantmigration.Run(context.Background(), tenantmigration.Deps{
		MainDB: db, ControlPlane: controlPlaneService.Repository(),
		TenantsRoot: tenantDataDir, LegacyRunnerEndpoint: strings.TrimSpace(cfg.AgentRunner.ServiceURL),
	})
	if err != nil {
		return nil, fmt.Errorf("legacy tenant migration: %w", err)
	}
	if legacyMigrated && logger != nil {
		logger.Info("legacy platform database copied into the default tenant (slug=tenant-one)")
	}
	controlPlaneService.SetTenantProvisioner(&tenantStoreProvisioner{resolver: tenantStores, root: tenantDataDir})
	if err := migrateTenantStores(context.Background(), controlPlaneService, tenantStores); err != nil {
		return nil, err
	}
	// 租户 runner 池：先对已存在的指派行做启动校验（fail-fast，空表放行）。
	if err := validateRunnerAssignments(context.Background(), controlPlaneService); err != nil {
		return nil, err
	}

	// 引擎池：平台引擎处理索引未命中的存量仓行与无租户 token 兜底，
	// 各租户引擎承接该组织仓行的全部写路径（事件→任务→派发→回传→写回）。
	defaultRunnerEndpoint := strings.TrimRight(strings.TrimSpace(cfg.AgentRunner.ServiceURL), "/")
	shared := engineShared{
		cfg:           cfg,
		logger:        logger,
		poolSource:    runnerpool.NewControlPlanePoolSource(controlPlaneService, []string{defaultRunnerEndpoint}),
		defaultRunner: agentrunner.NewClient(cfg.AgentRunner.ServiceURL, nil),
		runnerClientFactory: func(endpoint string) orchestrator.ReviewerRunnerClient {
			return agentrunner.NewClient(endpoint, nil)
		},
		resultTokenSecret: resultTokenSecret,
	}
	platformEngine, err := buildTenantEngine(db, 0, shared)
	if err != nil {
		return nil, fmt.Errorf("build platform engine: %w", err)
	}
	pool := newEnginePool(platformEngine, tenantStores, controlPlaneService, shared, logger)

	// 以下为 router 兜底层（会话未绑定租户时的默认服务，全部平台库句柄；
	// 租户作用域请求在 handler 层按 BindTenantStore 重建对应版本）。
	peopleService, peopleLookup, err := newPeopleDirectory(db)
	if err != nil {
		return nil, err
	}
	repositoryService := newRepositoryDirectory(db)
	requirementsService := requirements.NewService(
		requirements.NewStore(db), platformRepoStore,
		requirements.NewProviderClientFactory(nil),
	)
	connectorService := connectors.NewService(connectors.NewConnectorRepository(db), connectors.NewHTTPClient())
	mcpHandler := handlers.NewMCPHandler(
		platformEngine.mcpView.Channel, resultTokenSecret, platformEngine.mcpView.Tasks, platformEngine.mcpView.Runs,
		platformEngine.mcpView.Rejections, resultSubmissionMaxRejections,
		handlers.WithMCPEngineSource(pool),
	)
	routerOptions := []any{
		// webhook 验签租户路由：索引命中 → 租户库仓行 secret；
		// 未命中回退平台库仓行（存量过渡期）；皆未命中由 handler fail-closed。
		api.WithWebhookSecretLookup(&tenantRouteSecretLookup{
			index: controlPlaneService.Repository(), resolver: tenantStores, platform: platformRepoStore,
		}),
		// 事件引擎视图按 (provider, path) 路由到所属租户引擎（写路径租户化，
		// 视图内 normalizer 按组织段配置构建）。
		api.WithEngineViewFactory(pool.engineViewForPath),
		// 管理台操作（手动 tick/重试/重绑）按会话租户选择引擎。
		api.WithAdminOperationEngines(pool),
		// 注册/改仓写穿路由索引（租户作用域请求按会话租户）。
		api.WithRouteIndexSync(&controlPlaneRouteIndex{repo: controlPlaneService.Repository()}),
		api.WithTickDefaults(cfg.Scheduler.TickRequest),
		api.WithAdminQueryService(admin.NewQueryService(db)),
		api.WithConnectorService(connectorService),
		api.WithAgentObservability(agentobservability.NewQuery(db)),
		api.WithRuntimeMode(configuration.ModeReady),
		api.WithPeopleService(peopleService, peopleLookup),
		api.WithRepositoryService(repositoryService),
		api.WithRequirementsService(requirementsService),
		api.WithMCPHandler(mcpHandler),
		api.WithTenantStoreResolver(tenantStores),
		api.WithControlPlaneService(controlPlaneService),
	}
	if cfg.ControlPlane.PortalCookieSecure != nil {
		routerOptions = append(routerOptions, api.WithPortalCookieSecure(*cfg.ControlPlane.PortalCookieSecure))
	}
	if configurationService != nil {
		routerOptions = append(routerOptions, api.WithConfigurationService(configurationService, shutdown))
	}
	routerArgs := []any{platformEngine.processor}
	routerArgs = append(routerArgs, routerOptions...)
	router := api.NewRouter(platformEngine.orch, routerArgs...)
	server := api.NewServer(cfg.HTTPAddr, router)
	var schedulerRunner *scheduler.Runner
	if cfg.Scheduler.Enabled {
		schedulerRunner = scheduler.New(cfg.Scheduler.Interval, cfg.Scheduler.TickRequest, pool.tickAll, logger)
	}
	requirementsSyncRunner := requirements.NewRunner(requirements.SyncInterval, pool.requirementsSyncAll, logger)
	closeTenantStores = false
	return &App{
		Config:           cfg,
		Logger:           logger,
		DB:               db,
		Router:           router,
		Server:           server,
		Orch:             platformEngine.orch,
		pool:             pool,
		Scheduler:        schedulerRunner,
		RequirementsSync: requirementsSyncRunner,
		ControlPlane:     controlPlaneService,
		TenantStores:     tenantStores,
		Mode:             configuration.ModeReady,
	}, nil
}

func NewBootstrap(bootstrap config.BootstrapConfig, logger *zap.Logger, db *gorm.DB, configurationService *configuration.Service, shutdown configuration.ShutdownRequester, mode configuration.Mode) (*App, error) {
	if mode != configuration.ModeSetupPending && mode != configuration.ModeConfigurationError {
		return nil, fmt.Errorf("unsupported bootstrap mode %q", mode)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.AgentExecutionEvent{}, &model.Artifact{}, &model.Dependency{}, &model.Writeback{}, &model.Effect{}, &model.InboundEvent{}, &model.ResultSubmission{}, &model.ConnectorInstance{}); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	if err := registeredrepo.NewStore(db).Migrate(); err != nil {
		return nil, err
	}
	// setup 模式无配置可回填;作用域唯一索引直接建立(空表/空 provider 行不冲突)。
	if err := registeredrepo.NewStore(db).EnsureIdentityIndexes(); err != nil {
		return nil, err
	}
	if err := requirements.NewStore(db).Migrate(); err != nil {
		return nil, err
	}
	peopleService, peopleLookup, err := newPeopleDirectory(db)
	if err != nil {
		return nil, err
	}
	repositoryService := newRepositoryDirectory(db)
	controlPlaneService, err := newControlPlane(bootstrap.ControlPlane, logger)
	if err != nil {
		return nil, err
	}
	tenantStores := tenantdb.NewSharedMainResolver(db, 0)
	bootstrapRouterOptions := []any{
		api.WithConfigurationService(configurationService, shutdown),
		api.WithRuntimeMode(mode),
		api.WithPeopleService(peopleService, peopleLookup),
		api.WithRepositoryService(repositoryService),
		api.WithControlPlaneService(controlPlaneService),
		api.WithTenantStoreResolver(tenantStores),
	}
	if bootstrap.ControlPlane.PortalCookieSecure != nil {
		bootstrapRouterOptions = append(bootstrapRouterOptions, api.WithPortalCookieSecure(*bootstrap.ControlPlane.PortalCookieSecure))
	}
	router := api.NewRouter(nil, bootstrapRouterOptions...)
	return &App{
		Config: config.Config{
			HTTPAddr: bootstrap.HTTPAddr, DatabaseDSN: bootstrap.DatabaseDSN,
			LogLevel: bootstrap.LogLevel, LogDir: bootstrap.LogDir, Environment: bootstrap.Environment,
			ControlPlane: bootstrap.ControlPlane,
		},
		Logger:       logger,
		DB:           db,
		Router:       router,
		Server:       api.NewServer(bootstrap.HTTPAddr, router),
		ControlPlane: controlPlaneService,
		TenantStores: tenantStores,
		Mode:         mode,
	}, nil
}

// migrateBusinessStore 对一个业务库执行全部业务域迁移（平台库与租户库同一
// 清单；AutoMigrate 幂等）。
func migrateBusinessStore(db *gorm.DB) error {
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.AgentExecutionEvent{}, &model.Artifact{}, &model.Dependency{}, &model.Writeback{}, &model.Effect{}, &model.InboundEvent{}, &model.ResultSubmission{}, &model.ResultSubmissionRejection{}, &model.ConnectorInstance{}); err != nil {
		return err
	}
	if err := registeredrepo.NewStore(db).Migrate(); err != nil {
		return err
	}
	if err := requirements.NewStore(db).Migrate(); err != nil {
		return err
	}
	if err := people.NewRepository(db).Migrate(); err != nil {
		return err
	}
	return orgconfig.NewStore(db).Migrate()
}

// migrateTenantStores 启动时遍历控制面租户，逐库打开并迁移。
// 任一租户失败 → 中止启动并列出失败租户。
func migrateTenantStores(ctx context.Context, service *controlplane.Service, resolver tenantdb.Resolver) error {
	tenants, err := service.ListTenants(ctx)
	if err != nil {
		return fmt.Errorf("load tenants for migration: %w", err)
	}
	var failures []string
	for _, tenant := range tenants {
		store, err := resolver.Handle(ctx, tenant.ID)
		if err != nil {
			failures = append(failures, tenant.Slug+": "+err.Error())
			continue
		}
		if err := migrateBusinessStore(store.DB); err != nil {
			resolver.Invalidate(tenant.ID)
			failures = append(failures, tenant.Slug+": "+err.Error())
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("migrate tenant stores failed: %s", strings.Join(failures, "; "))
	}
	return nil
}

// tenantStoreProvisioner 把控制面建组织同事务地开通到租户数据面：建目录建库
// → 迁移 → 回写实际 db_locator；失败清理半成品目录（登记行由事务回滚兜底）。
type tenantStoreProvisioner struct {
	resolver *tenantdb.SqliteFileResolver
	root     string
}

func (p *tenantStoreProvisioner) ProvisionTenant(ctx context.Context, tenant *controlplane.Tenant) error {
	tenantDir := filepath.Join(p.root, strconv.FormatUint(uint64(tenant.ID), 10))
	store, err := p.resolver.Handle(ctx, tenant.ID)
	if err != nil {
		_ = os.RemoveAll(tenantDir)
		return fmt.Errorf("open tenant store: %w", err)
	}
	if err := migrateBusinessStore(store.DB); err != nil {
		p.resolver.Invalidate(tenant.ID)
		_ = os.RemoveAll(tenantDir)
		return fmt.Errorf("migrate tenant store: %w", err)
	}
	tenant.DMLocator = "file:" + filepath.ToSlash(filepath.Join(tenantDir, "db.sqlite"))
	return nil
}

// validateRunnerAssignments 在启动时校验控制面 runner 指派形态：同端点跨
// 租户/孤儿租户/URL 非法任一命中即拒绝启动；空表放行。
func validateRunnerAssignments(ctx context.Context, service *controlplane.Service) error {
	rows, err := service.ListAllRunnerAssignments(ctx)
	if err != nil {
		return fmt.Errorf("load runner assignments: %w", err)
	}
	tenants, err := service.ListTenants(ctx)
	if err != nil {
		return fmt.Errorf("load tenants: %w", err)
	}
	known := make(map[uint]bool, len(tenants))
	for _, tenant := range tenants {
		known[tenant.ID] = true
	}
	shapes := make([]runnerpool.AssignmentShape, 0, len(rows))
	for _, row := range rows {
		shapes = append(shapes, runnerpool.AssignmentShape{TenantID: row.TenantID, EndpointURL: row.EndpointURL})
	}
	if err := runnerpool.ValidateAssignments(shapes, known); err != nil {
		return fmt.Errorf("validate runner assignments: %w", err)
	}
	return nil
}

func newRepositoryDirectory(db *gorm.DB) *registeredrepo.Service {
	return registeredrepo.NewService(
		registeredrepo.NewStore(db),
		nil,
		registeredrepo.NewSQLActivityChecker(db),
	)
}

// runnerRepositoryResolver 按任务绑定解析注册仓整行,是 runner 链路 workspace
// URL(CloneURL 派生)与每仓 Token 凭据的唯一权威来源;任务未绑定注册仓时拒绝。
func runnerRepositoryResolver(store *registeredrepo.Store) func(context.Context, *model.Task) (*registeredrepo.RegisteredRepository, error) {
	return func(ctx context.Context, task *model.Task) (*registeredrepo.RegisteredRepository, error) {
		if task == nil || task.RepositoryID == nil || strings.TrimSpace(*task.RepositoryID) == "" {
			return nil, orchestrator.ErrRepositoryNotBound
		}
		repo, err := store.Get(ctx, *task.RepositoryID)
		if err != nil {
			return nil, fmt.Errorf("lookup repository %s: %w", strings.TrimSpace(*task.RepositoryID), err)
		}
		return repo, nil
	}
}

// newPeopleDirectory 构造人员目录与三平台全量的身份 lookup:查询目标由请求
// 自带的作用域(provider/base_url,按仓行解析)决定,装配层不再单选平台。
func newPeopleDirectory(db *gorm.DB) (*people.Service, people.IdentityLookup, error) {
	repository := people.NewRepository(db)
	if err := repository.Migrate(); err != nil {
		return nil, nil, err
	}
	return people.NewService(repository), people.NewMultiProviderLookupClient(nil), nil
}

// peopleMentionDirectory 把人员目录适配为 orchestrator 的提及解析端口;查询
// 作用域按任务所属仓行的 provider/base_url 解析(三键身份),仓缺失或目录缺席时
// 返回空结果,由通知链的 mentions gate 之后的语义完成降级。
type peopleMentionDirectory struct {
	service *people.Service
	repos   *registeredrepo.Store
}

func newPeopleMentionDirectory(service *people.Service, repos *registeredrepo.Store) peopleMentionDirectory {
	return peopleMentionDirectory{service: service, repos: repos}
}

func (d peopleMentionDirectory) ResolveMentionNames(ctx context.Context, repositoryID string, assigneeIDs []int) map[int]orchestrator.AssigneeMention {
	result := make(map[int]orchestrator.AssigneeMention, len(assigneeIDs))
	if d.service == nil || d.repos == nil || strings.TrimSpace(repositoryID) == "" {
		return result
	}
	repo, err := d.repos.Get(ctx, repositoryID)
	if err != nil {
		return result
	}
	resolved := d.service.ResolveMentionNames(ctx, repo.Provider, repo.BaseURL, assigneeIDs)
	for id, mention := range resolved {
		result[id] = orchestrator.AssigneeMention{DisplayName: mention.DisplayName, Username: mention.Username}
	}
	return result
}

// newControlPlane 打开控制面库、迁移并种子超管。DSN 或种子缺失时返回
// config.ErrControlPlaneNotConfigured，不再退内存库。
func newControlPlane(settings config.ControlPlaneSettings, logger *zap.Logger) (*controlplane.Service, error) {
	if err := settings.Validate(); err != nil {
		return nil, err
	}
	db, err := store.Open(settings.DSN)
	if err != nil {
		return nil, fmt.Errorf("open control plane database: %w", err)
	}
	repository := controlplane.NewRepository(db)
	if err := repository.Migrate(); err != nil {
		return nil, err
	}
	service := controlplane.NewService(repository)
	if err := service.SeedSuperAdmin(context.Background(), controlplane.SeedSuperAdminInput{
		Username: settings.SuperAdminUsername,
		Password: settings.SuperAdminPassword,
		Reset:    settings.SuperAdminReset,
	}); err != nil {
		return nil, fmt.Errorf("seed super admin: %w", err)
	}
	if logger != nil {
		logger.Info("control plane super admin ensured", zap.String("username", settings.SuperAdminUsername), zap.Bool("reset", settings.SuperAdminReset))
	}
	return service, nil
}

func (a *App) Run(ctx context.Context) error {
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	if a.Scheduler != nil {
		go a.Scheduler.Run(runCtx)
	}
	if a.RequirementsSync != nil {
		go a.RequirementsSync.Run(runCtx)
	}
	if a.ControlPlane != nil {
		go a.ControlPlane.RunSessionJanitor(runCtx)
	}
	go a.runRepoIndexReconciler(runCtx)

	errCh := make(chan error, 1)

	go func() {
		errCh <- a.Server.Run()
	}()

	select {
	case err := <-errCh:
		cancelRun()
		return err
	case <-ctx.Done():
		cancelRun()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := a.Server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		a.Logger.Info("coordinator stopped")
		return nil
	}
}
