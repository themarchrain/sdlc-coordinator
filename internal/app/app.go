package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"sdlc-coordinator/internal/adapters/agentrunner"
	"sdlc-coordinator/internal/adapters/im"
	"sdlc-coordinator/internal/admin"
	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/configuration"
	"sdlc-coordinator/internal/connectors"
	"sdlc-coordinator/internal/controlplane"
	effectexec "sdlc-coordinator/internal/effects"
	"sdlc-coordinator/internal/events"
	api "sdlc-coordinator/internal/http"
	handlers "sdlc-coordinator/internal/http/handlers"
	"sdlc-coordinator/internal/model"
	agentobservability "sdlc-coordinator/internal/observability/agent"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/people"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/requirements"
	"sdlc-coordinator/internal/resulttoken"
	"sdlc-coordinator/internal/runner"
	"sdlc-coordinator/internal/scheduler"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/tenantdb"
	"sdlc-coordinator/internal/workspace"

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
	reworkAllowedUserIDs, err := config.ParseCodeHostReworkAllowedUserIDs(cfg.CodeHostReworkAllowedUserIDs)
	if err != nil {
		return nil, fmt.Errorf("parse codehost rework allowed user ids: %w", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.AgentExecutionEvent{}, &model.Artifact{}, &model.Dependency{}, &model.Writeback{}, &model.Effect{}, &model.InboundEvent{}, &model.ResultSubmission{}, &model.ResultSubmissionRejection{}, &model.ConnectorInstance{}); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	if err := registeredrepo.NewStore(db).Migrate(); err != nil {
		return nil, err
	}
	if err := requirements.NewStore(db).Migrate(); err != nil {
		return nil, err
	}

	tenantStores := tenantdb.NewSharedMainResolver(db, 0)
	db = tenantStores.Main().DB

	taskRepo := repository.NewTaskRepository(db)
	runRepo := repository.NewRunRepository(db)
	agentExecutionRepo := repository.NewAgentExecutionRepository(db)
	artifactRepo := repository.NewArtifactRepository(db)
	dependencyRepo := repository.NewDependencyRepository(db)
	writebackRepo := repository.NewWritebackRepository(db)
	effectRepo := repository.NewEffectRepository(db)
	inboundEventRepo := repository.NewInboundEventRepository(db)
	resultSubmissionRepo := repository.NewResultSubmissionRepository(db)
	connectorRemote := connectors.NewHTTPClient()
	connectorService := connectors.NewService(connectors.NewConnectorRepository(db), connectorRemote)
	opts := []orchestrator.Option{
		orchestrator.WithArtifactRepository(artifactRepo),
		orchestrator.WithDependencyRepository(dependencyRepo),
		orchestrator.WithWritebackRepository(writebackRepo),
		orchestrator.WithEffectRepository(effectRepo),
		orchestrator.WithResultSubmissionRepository(resultSubmissionRepo),
		orchestrator.WithDeveloperClarificationIMTarget(""),
		orchestrator.WithReviewerIMMentions(cfg.ReviewerIMMentions),
	}
	imBackend, err := newIMBackend(cfg)
	if err != nil {
		return nil, err
	}
	if imBackend != nil {
		opts = append(opts, orchestrator.WithEffectExecutor(effectexec.NewExecutor(imBackend)))
	}
	repoStore := registeredrepo.NewStore(db)
	// 平台身份回填与作用域唯一索引。无 provider/base_url 的仓行用全局配置补齐。
	// 顺序固定 Migrate → BackfillIdentity → Ensure。
	if backfilled, err := repoStore.BackfillIdentity(context.Background(), cfg.CodeHostProvider, cfg.CodeHostBaseURL); err != nil {
		return nil, fmt.Errorf("backfill repository platform identity: %w", err)
	} else if backfilled > 0 {
		// 计数语义为「填充的身份列数」(provider 与 base_url 各计一次),不是仓数。
		logger.Info("backfilled repository platform identity from global codehost config", zap.Int64("identity_columns_backfilled", backfilled))
	}
	if cfg.CodeHostBaseURL == "" {
		logger.Warn("global codehost base url is empty; repository rows without platform identity were not backfilled")
	}
	if err := repoStore.EnsureIdentityIndexes(); err != nil {
		return nil, err
	}
	// 平台身份按仓行解析:resolver 与写回探针按任务所属仓的 provider/base_url
	// 分派。全局 codehost 配置只作为注册表单默认值与空列回填来源,运行时不读取。
	codeHostResolver := registeredrepo.NewCodeHostResolver(repoStore, nil)
	opts = append(opts,
		orchestrator.WithCodeHostBackendResolver(codeHostResolver),
		orchestrator.WithMergeRequestReaderResolver(codeHostResolver),
		orchestrator.WithWritebackAuthHandler(registeredrepo.NewWritebackAuthHandler(repoStore, nil)),
	)
	peopleService, peopleLookup, err := newPeopleDirectory(db)
	if err != nil {
		return nil, err
	}
	opts = append(opts, orchestrator.WithAssigneeMentionDirectory(newPeopleMentionDirectory(peopleService, repoStore)))
	// 事件必须命中注册仓,任务必须绑定注册仓。写回凭据取该仓 Token。
	opts = append(opts, orchestrator.WithRepositoryLookup(repoStore))
	// Agent Runner 执行层：reviewer/developer 都经 agent-runner 派发 CLI 子进程。
	runnerClient := agentrunner.NewClient(cfg.AgentRunner.ServiceURL, nil)
	providerHeaders := make(map[string]string, len(cfg.AgentRunner.CustomHeaders))
	for name, value := range cfg.AgentRunner.CustomHeaders {
		providerHeaders[name] = value
	}
	runnerSettings := orchestrator.ReviewerRunnerSettings{
		WorkspaceRoot: cfg.AgentRunner.WorkspaceRoot,
		TaskTimeout:   cfg.AgentRunner.TaskTimeout,
		Provider: runner.ProviderConfig{
			Name: cfg.AgentRunner.Provider, Revision: int64(cfg.AgentRunner.ProviderConfigRevision),
			BaseURL: cfg.AgentRunner.ProviderBaseURL, APIKey: cfg.AgentRunner.APIKey,
			Headers: providerHeaders, Model: cfg.AgentRunner.Model, Effort: cfg.AgentRunner.Effort,
		},
		ResolveRepository: runnerRepositoryResolver(repoStore),
	}
	var orch *orchestrator.Orchestrator
	orchSubmitter := orchestrator.ReviewerResultSubmitterFunc(func(ctx context.Context, request orchestrator.SubmitResultRequest) (*orchestrator.SubmitResultResponse, error) {
		return orch.SubmitResult(ctx, request)
	})
	reviewerRunnerCoordinator := orchestrator.NewReviewerRunnerCoordinator(
		taskRepo, runRepo, agentExecutionRepo, workspace.NewReviewerPreparer(), runnerClient,
		orchSubmitter,
	)
	developerRunnerCoordinator := orchestrator.NewDeveloperRunnerCoordinator(
		taskRepo, runRepo, agentExecutionRepo, workspace.NewDeveloperPreparer(), workspace.NewDeveloperFinalizer(), runnerClient,
		orchSubmitter,
		runnerSettings,
	)
	opts = append(opts,
		orchestrator.WithReviewerRunner(reviewerRunnerCoordinator, runnerSettings),
		orchestrator.WithDeveloperRunner(developerRunnerCoordinator, runnerSettings),
		orchestrator.WithAgentExecutionLookup(agentExecutionRepo),
		orchestrator.WithRunnerArtifactStore(runnerClient),
	)
	// MCP 结果回传通道：签名密钥仅存内存（重启即失效，在途 agent 落入文本
	// 降级通道）；endpoint 取协调器对外可达地址（scheduler.callback_base_url），
	// 缺省未配置时通道保持关闭（纯文本模式）。
	resultTokenSecret, err := resulttoken.NewSecret()
	if err != nil {
		return nil, fmt.Errorf("generate result channel secret: %w", err)
	}
	ingressIssuer := orchestrator.RunTokenIssuerFunc(func(taskID, runID, role string) (string, error) {
		return resulttoken.Issue(resultTokenSecret, resulttoken.Claims{
			TaskID: taskID, RunID: runID, Role: role,
			TTL: cfg.AgentRunner.TaskTimeout + resultTokenTTLGrace,
		}, time.Now())
	})
	mcpChannel := orchestrator.NewResultChannelCoordinator(
		workspace.NewDeveloperFinalizer(),
		orchSubmitter,
		runnerSettings, "mcp",
	)
	mcpHandler := handlers.NewMCPHandler(
		mcpChannel, resultTokenSecret, taskRepo, runRepo,
		repository.NewResultRejectionRepository(db), resultSubmissionMaxRejections,
	)
	opts = append(opts, orchestrator.WithResultIngress(ingressIssuer, cfg.Scheduler.TickRequest.CallbackBaseURL))
	orch = orchestrator.New(taskRepo, runRepo, opts...)
	eventProcessor := events.NewProcessor(inboundEventRepo, orch, events.WithCodeHostReworkAllowedUserIDs(reworkAllowedUserIDs))
	eventProcessor.SetRepositoryLookup(repoStore)
	repositoryService := newRepositoryDirectory(db)
	requirementsService := requirements.NewService(
		requirements.NewStore(db), repoStore,
		requirements.NewProviderClientFactory(nil),
	)
	requirementsSyncRunner := requirements.NewRunner(requirements.SyncInterval, requirementsService.SyncOnce, logger)
	// 控制面与 ready/bootstrap 共用装配：DSN 与种子超管缺失时显式失败。
	controlPlaneService, err := newControlPlane(cfg.ControlPlane, logger)
	if err != nil {
		return nil, err
	}

	routerOptions := []any{
		// webhook secret 按仓持有:验签凭据来自注册仓行,无全局回退。
		api.WithWebhookSecretLookup(repoStore),
		api.WithReworkAllowedUserIDs(reworkAllowedUserIDs),
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
	if configurationService != nil {
		routerOptions = append(routerOptions, api.WithConfigurationService(configurationService, shutdown))
	}
	routerArgs := []any{eventProcessor}
	routerArgs = append(routerArgs, routerOptions...)
	router := api.NewRouter(orch, routerArgs...)
	server := api.NewServer(cfg.HTTPAddr, router)
	var schedulerRunner *scheduler.Runner
	if cfg.Scheduler.Enabled {
		schedulerRunner = scheduler.New(cfg.Scheduler.Interval, cfg.Scheduler.TickRequest, orch.Tick, logger)
	}
	return &App{
		Config:           cfg,
		Logger:           logger,
		DB:               db,
		Router:           router,
		Server:           server,
		Orch:             orch,
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
	router := api.NewRouter(
		nil,
		api.WithConfigurationService(configurationService, shutdown),
		api.WithRuntimeMode(mode),
		api.WithPeopleService(peopleService, peopleLookup),
		api.WithRepositoryService(repositoryService),
		api.WithControlPlaneService(controlPlaneService),
		api.WithTenantStoreResolver(tenantStores),
	)
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

func newRepositoryDirectory(db *gorm.DB) *registeredrepo.Service {
	return registeredrepo.NewService(
		registeredrepo.NewStore(db),
		nil,
		registeredrepo.NewSQLActivityChecker(db),
	)
}

func newIMBackend(cfg config.Config) (im.Backend, error) {
	telegramMentions, err := parseMentionMap(cfg.IM.Telegram.MentionMap)
	if err != nil {
		return nil, fmt.Errorf("parse im_telegram_mention_map: %w", err)
	}
	feishuMentions, err := parseMentionMap(cfg.IM.Feishu.MentionMap)
	if err != nil {
		return nil, fmt.Errorf("parse im_feishu_mention_map: %w", err)
	}
	return im.NewBackend(im.Settings{
		Provider: strings.ToLower(strings.TrimSpace(cfg.IM.Provider)),
		Telegram: im.TelegramSettings{
			BotToken:      cfg.IM.Telegram.BotToken,
			APIBase:       cfg.IM.Telegram.APIBase,
			DefaultChatID: cfg.IM.Telegram.DefaultChatID,
			Mentions:      telegramMentions,
		},
		Feishu: im.FeishuSettings{
			WebhookURL: cfg.IM.Feishu.WebhookURL,
			Secret:     cfg.IM.Feishu.Secret,
			Mentions:   feishuMentions,
		},
	}), nil
}

func parseMentionMap(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parsed := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
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
