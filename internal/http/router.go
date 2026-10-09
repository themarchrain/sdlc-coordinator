package api

import (
	"net/http"

	"sdlc-coordinator/internal/admin"
	"sdlc-coordinator/internal/configuration"
	"sdlc-coordinator/internal/connectors"
	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/events"
	"sdlc-coordinator/internal/http/handlers"
	agentobservability "sdlc-coordinator/internal/observability/agent"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/people"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/requirements"
	"sdlc-coordinator/internal/tenantdb"
	adminweb "sdlc-coordinator/web"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type RouterOptions struct {
	WebhookSecretLookup  handlers.WebhookSecretLookup
	ReworkAllowedUserIDs []int
	TickDefaults         orchestrator.TickRequest
	AdminQueryService    *admin.QueryService
	AgentObservability   *agentobservability.Query
	ConnectorService     *connectors.Service
	AdminWebHandler      http.Handler
	ConfigurationService *configuration.Service
	ShutdownRequester    configuration.ShutdownRequester
	RuntimeMode          configuration.Mode
	PeopleService        *people.Service
	PeopleLookup         people.IdentityLookup
	MCPHandler           *handlers.MCPHandler
	RepositoryService    *registeredrepo.Service
	RequirementsService  *requirements.Service
	ControlPlaneService  *controlplane.Service
	PortalCookieSecure   *bool
	TenantStores         tenantdb.Resolver
	RouteIndexSync       registeredrepo.RouteIndexSync
	Logger               *zap.Logger
	// 写路径租户化端口（均可空：空时行为与单库形态一致）。
	EngineViewFactory     handlers.EngineViewFactory
	AdminOperationEngines handlers.AdminOperationEngines
}

// WithRouteIndexSync 注入控制面路由索引写穿端口。
func WithRouteIndexSync(sync registeredrepo.RouteIndexSync) RouterOption {
	return func(options *RouterOptions) {
		options.RouteIndexSync = sync
	}
}

type RouterOption func(*RouterOptions)

// WithWebhookSecretLookup 注入按仓 webhook secret 解析端口,供 webhook 验签使用。
func WithWebhookSecretLookup(lookup handlers.WebhookSecretLookup) RouterOption {
	return func(options *RouterOptions) {
		options.WebhookSecretLookup = lookup
	}
}

func WithTickDefaults(defaults orchestrator.TickRequest) RouterOption {
	return func(options *RouterOptions) {
		options.TickDefaults = defaults
	}
}

func WithAdminQueryService(service *admin.QueryService) RouterOption {
	return func(options *RouterOptions) {
		options.AdminQueryService = service
	}
}

func WithAgentObservability(query *agentobservability.Query) RouterOption {
	return func(options *RouterOptions) { options.AgentObservability = query }
}

func WithConnectorService(service *connectors.Service) RouterOption {
	return func(options *RouterOptions) {
		options.ConnectorService = service
	}
}

func WithAdminWebHandler(handler http.Handler) RouterOption {
	return func(options *RouterOptions) {
		options.AdminWebHandler = handler
	}
}

// WithMCPHandler 注册 /mcp 结果回传端点；nil（未配置 issuer）时不注册——
// 纯文本模式。鉴权在 handler 内做 per-run bearer token 校验，不走 session 门禁。
func WithMCPHandler(handler *handlers.MCPHandler) RouterOption {
	return func(options *RouterOptions) {
		options.MCPHandler = handler
	}
}

func WithConfigurationService(service *configuration.Service, shutdown configuration.ShutdownRequester) RouterOption {
	return func(options *RouterOptions) {
		options.ConfigurationService = service
		options.ShutdownRequester = shutdown
	}
}

func WithRuntimeMode(mode configuration.Mode) RouterOption {
	return func(options *RouterOptions) {
		options.RuntimeMode = mode
	}
}

func WithPeopleService(service *people.Service, lookup people.IdentityLookup) RouterOption {
	return func(options *RouterOptions) {
		options.PeopleService = service
		options.PeopleLookup = lookup
	}
}

func WithRepositoryService(service *registeredrepo.Service) RouterOption {
	return func(options *RouterOptions) {
		options.RepositoryService = service
	}
}

func WithRequirementsService(service *requirements.Service) RouterOption {
	return func(options *RouterOptions) {
		options.RequirementsService = service
	}
}

// WithControlPlaneService 注入多租户控制面；为 nil 时不挂门户/超管路由，
// /api/admin 也无会话门禁（仅供 handler 隔离测试）。生产装配始终注入。
func WithControlPlaneService(service *controlplane.Service) RouterOption {
	return func(options *RouterOptions) {
		options.ControlPlaneService = service
	}
}

func WithTenantStoreResolver(resolver tenantdb.Resolver) RouterOption {
	return func(options *RouterOptions) {
		options.TenantStores = resolver
	}
}

// WithEngineViewFactory 注入按仓路由的引擎视图工厂（webhook 写路径租户化，
// 视图内 normalizer 按组织段配置构建）；nil 视图回落构造期平台视图。
func WithEngineViewFactory(factory handlers.EngineViewFactory) RouterOption {
	return func(options *RouterOptions) {
		options.EngineViewFactory = factory
	}
}

// WithPortalCookieSecure 覆写门户会话 cookie 的 Secure 属性（默认 true；
// 供明文 http 部署显式关闭）。
func WithPortalCookieSecure(secure bool) RouterOption {
	return func(options *RouterOptions) {
		options.PortalCookieSecure = &secure
	}
}

// WithAdminOperationEngines 注入按会话租户选择操作引擎与 effect 查询的端口。
func WithAdminOperationEngines(engines handlers.AdminOperationEngines) RouterOption {
	return func(options *RouterOptions) {
		options.AdminOperationEngines = engines
	}
}

// WithRuntimeLogger 注入协调器日志，用于记录 HTTP 框架的运行诊断。
func WithRuntimeLogger(logger *zap.Logger) RouterOption {
	return func(options *RouterOptions) {
		options.Logger = logger
	}
}

func NewRouter(orch *orchestrator.Orchestrator, args ...any) *gin.Engine {
	var eventProcessor *events.Processor
	options := RouterOptions{RuntimeMode: configuration.ModeReady}
	for _, arg := range args {
		switch value := arg.(type) {
		case *events.Processor:
			if eventProcessor == nil {
				eventProcessor = value
			}
		case RouterOption:
			value(&options)
		}
	}

	router := gin.New()
	router.Use(gin.RecoveryWithWriter(newZapLogWriter(options.Logger, "gin_recovery")))
	// 不信任任何代理：gin 默认信任一切代理，直连形态下
	// ClientIP() 会取攻击者自设的 X-Forwarded-For，登录限速键可被逐请求
	// 轮换 XFF 整体绕过。收身后 ClientIP=直连对端地址；反代部署的授信清单
	// 属部署侧配置，随部署文档一并给出。全仓 ClientIP 调用点已核对
	// （仅 portal 登录限速一处），不影响 webhook 与业务路由。
	_ = router.SetTrustedProxies(nil)
	var healthProvider handlers.ConfigurationStatusProvider
	if options.ConfigurationService != nil {
		healthProvider = options.ConfigurationService
	}
	healthHandler := handlers.NewHealthHandler(healthProvider)
	router.GET("/healthz", healthHandler.Health)
	router.GET("/readyz", healthHandler.Ready)
	if options.MCPHandler != nil {
		router.POST("/mcp", options.MCPHandler.Handle)
		router.DELETE("/mcp", options.MCPHandler.CloseSession)
		router.GET("/mcp", options.MCPHandler.Handle)
	}
	adminWebHandler := options.AdminWebHandler
	if adminWebHandler == nil {
		adminWebHandler = adminweb.Handler()
	}
	adminHandler := gin.WrapH(adminWebHandler)
	router.GET("/admin", adminHandler)
	router.GET("/admin/*path", adminHandler)

	runtimeGate := handlers.ConfigurationGate(options.RuntimeMode)

	api := router.Group("/api")
	{
		var controlPlaneHandler *handlers.ControlPlaneHandler
		if options.ControlPlaneService != nil {
			controlPlaneHandler = handlers.NewControlPlaneHandler(options.ControlPlaneService)
			controlPlaneHandler.SetAdminRoutePolicy(adminMatrixPolicy{})
			if options.PortalCookieSecure != nil {
				controlPlaneHandler.SetCookieSecure(*options.PortalCookieSecure)
			}
		}
		var adminMiddleware []gin.HandlerFunc
		if controlPlaneHandler != nil {
			adminMiddleware = append(adminMiddleware, controlPlaneHandler.RequirePortalAuth, controlPlaneHandler.RequireAdminMatrix)
			if options.TenantStores != nil {
				adminMiddleware = append(adminMiddleware, handlers.BindTenantStore(options.TenantStores))
			}
		}
		adminAPI := api.Group("/admin", adminMiddleware...)
		var adminOperations handlers.AdminOperations
		if orch != nil {
			adminOperations = orch
		}
		var adminEffects handlers.AdminEffectLookup
		if options.AdminQueryService != nil {
			adminEffects = options.AdminQueryService
		}
		if options.AgentObservability != nil {
			handlers.NewAdminAgentObservabilityHandler(options.AgentObservability).RegisterRoutes(adminAPI)
		}
		adminOperationHandler := handlers.NewAdminOperationHandler(adminOperations, adminEffects, options.TickDefaults, options.AdminOperationEngines)
		adminOperationHandler.RegisterRoutes(adminAPI, runtimeGate)
		if options.ConfigurationService != nil {
			adminConfigurationHandler := handlers.NewAdminConfigurationHandler(options.ConfigurationService, options.ShutdownRequester)
			adminConfigurationHandler.RegisterRoutes(adminAPI)
		}
		// 组织段运行配置:按会话组织作用域读写租户库,保存即热生效。
		handlers.NewAdminTenantConfigurationHandler().RegisterRoutes(adminAPI)
		if options.PeopleService != nil {
			adminPeopleHandler := handlers.NewAdminPeopleHandler(options.PeopleService, options.PeopleLookup)
			adminPeopleHandler.RegisterRoutes(adminAPI)
		}
		if options.RepositoryService != nil {
			adminRepositoryHandler := handlers.NewAdminRepositoryHandler(options.RepositoryService)
			if options.RouteIndexSync != nil {
				adminRepositoryHandler.SetRouteIndexSync(options.RouteIndexSync)
			}
			adminRepositoryHandler.RegisterRoutes(adminAPI)
		}
		if options.RequirementsService != nil {
			adminRequirementHandler := handlers.NewAdminRequirementHandler(options.RequirementsService)
			adminRequirementHandler.RegisterRoutes(adminAPI)
		}
		if options.AdminQueryService != nil {
			adminQueryHandler := handlers.NewAdminQueryHandler(options.AdminQueryService)
			adminQueryHandler.RegisterRoutes(adminAPI)
		}
		if options.ConnectorService != nil {
			adminConnectorHandler := handlers.NewAdminConnectorHandler(options.ConnectorService)
			adminConnectorHandler.RegisterRoutes(adminAPI)
		}
		// 未认证的 runtime 内部 API（POST /api/tasks、GET /api/tasks/:id、
		// claim、GET /api/runs/:id、effects/retry、scheduler/tick）已整体
		// 移除：它们此前只挂 ConfigurationGate，公网部署下等于无认证读写
		// 业务数据。人工触发调度/重试走 /api/admin 等价路由（会话+矩阵）；
		// webhook 验签通道不受影响。
		runtimeAPI := api.Group("")
		runtimeAPI.Use(runtimeGate)
		if eventProcessor != nil {
			eventHandler := handlers.NewEventHandler(eventProcessor,
				handlers.WithWebhookSecretLookup(options.WebhookSecretLookup),
				handlers.WithEngineViewFactory(options.EngineViewFactory),
			)
			runtimeAPI.POST("/events/gitlab", eventHandler.GitLab)
			runtimeAPI.POST("/events/gitea", eventHandler.Gitea)
			runtimeAPI.POST("/events/github", eventHandler.GitHub)
		} else if options.RuntimeMode != configuration.ModeReady {
			runtimeAPI.POST("/events/gitlab", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			runtimeAPI.POST("/events/gitea", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			runtimeAPI.POST("/events/github", func(c *gin.Context) { c.Status(http.StatusNoContent) })
		}

		// 控制面：/api/portal/* 与 /api/super-admin/*。同一 handler 也门禁 /api/admin/*。
		if controlPlaneHandler != nil {
			superAdminHandler := handlers.NewSuperAdminHandler(options.ControlPlaneService)
			portal := api.Group("/portal")
			controlPlaneHandler.RegisterAuthRoutes(portal)
			controlPlaneHandler.RegisterTenantRoutes(portal.Group("/tenant"))
			superAdmin := api.Group("/super-admin",
				controlPlaneHandler.RequirePortalAuth, controlPlaneHandler.RequireSuperAdmin)
			superAdminHandler.RegisterRoutes(superAdmin)
		}
	}

	return router
}
