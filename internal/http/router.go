package api

import (
	"net/http"

	"sdlc-coordinator/internal/admin"
	"sdlc-coordinator/internal/auth"
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
	adminweb "sdlc-coordinator/web"

	"github.com/gin-gonic/gin"
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
	AuthService          *auth.Service
	RepositoryService    *registeredrepo.Service
	RequirementsService  *requirements.Service
	ControlPlaneService  *controlplane.Service
}

type RouterOption func(*RouterOptions)

// WithWebhookSecretLookup 注入按仓 webhook secret 解析端口,供 webhook 验签使用。
func WithWebhookSecretLookup(lookup handlers.WebhookSecretLookup) RouterOption {
	return func(options *RouterOptions) {
		options.WebhookSecretLookup = lookup
	}
}

// WithReworkAllowedUserIDs 注入平台级 rework 白名单(平台 user id),
// 三平台 normalizer 共用同一份配置。
func WithReworkAllowedUserIDs(ids []int) RouterOption {
	return func(options *RouterOptions) {
		options.ReworkAllowedUserIDs = ids
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

func WithAuthService(service *auth.Service) RouterOption {
	return func(options *RouterOptions) {
		options.AuthService = service
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

// WithControlPlaneService 注入多租户控制面服务（纯增量）；为 nil 时新认证
// 体系整体不挂载，存量路由行为不变。
func WithControlPlaneService(service *controlplane.Service) RouterOption {
	return func(options *RouterOptions) {
		options.ControlPlaneService = service
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
	router.Use(gin.Recovery())
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

	taskHandler := handlers.NewTaskHandler(orch)
	runHandler := handlers.NewRunHandler(orch)
	schedulerHandler := handlers.NewSchedulerHandler(orch)
	effectHandler := handlers.NewEffectHandler(orch)

	runtimeGate := handlers.ConfigurationGate(options.RuntimeMode)

	api := router.Group("/api")
	{
		var adminMiddleware []gin.HandlerFunc
		if options.AuthService != nil {
			authHandler := handlers.NewAuthHandler(options.AuthService)
			api.POST("/auth/login", authHandler.Login)
			api.GET("/auth/session", authHandler.Session)
			adminMiddleware = append(adminMiddleware, authHandler.RequireSession)
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
		adminOperationHandler := handlers.NewAdminOperationHandler(adminOperations, adminEffects, options.TickDefaults)
		adminOperationHandler.RegisterRoutes(adminAPI, runtimeGate)
		if options.ConfigurationService != nil {
			adminConfigurationHandler := handlers.NewAdminConfigurationHandler(options.ConfigurationService, options.ShutdownRequester)
			adminConfigurationHandler.RegisterRoutes(adminAPI)
		}
		if options.PeopleService != nil {
			adminPeopleHandler := handlers.NewAdminPeopleHandler(options.PeopleService, options.PeopleLookup)
			adminPeopleHandler.RegisterRoutes(adminAPI)
		}
		if options.RepositoryService != nil {
			adminRepositoryHandler := handlers.NewAdminRepositoryHandler(options.RepositoryService)
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
		runtimeAPI := api.Group("")
		runtimeAPI.Use(runtimeGate)
		runtimeAPI.POST("/tasks", taskHandler.Create)
		runtimeAPI.GET("/tasks/:task_id", taskHandler.Get)
		runtimeAPI.POST("/tasks/:task_id/claim", taskHandler.Claim)
		runtimeAPI.GET("/runs/:run_id", runHandler.Get)
		runtimeAPI.POST("/effects/retry", effectHandler.Retry)
		runtimeAPI.POST("/scheduler/tick", schedulerHandler.Tick)
		if eventProcessor != nil {
			eventHandler := handlers.NewEventHandler(eventProcessor,
				handlers.WithWebhookSecretLookup(options.WebhookSecretLookup),
				handlers.WithReworkAllowedUserIDs(options.ReworkAllowedUserIDs),
			)
			runtimeAPI.POST("/events/gitlab", eventHandler.GitLab)
			runtimeAPI.POST("/events/gitea", eventHandler.Gitea)
			runtimeAPI.POST("/events/github", eventHandler.GitHub)
		} else if options.RuntimeMode != configuration.ModeReady {
			runtimeAPI.POST("/events/gitlab", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			runtimeAPI.POST("/events/gitea", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			runtimeAPI.POST("/events/github", func(c *gin.Context) { c.Status(http.StatusNoContent) })
		}
	}

	// 控制面（多租户）：独立命名空间 /api/portal/* 与 /api/super-admin/*，
	// 与存量 /api/auth/*、/api/admin/* 并行（切换归后续批次）。
	if options.ControlPlaneService != nil {
		controlPlaneHandler := handlers.NewControlPlaneHandler(options.ControlPlaneService)
		superAdminHandler := handlers.NewSuperAdminHandler(options.ControlPlaneService)
		portal := api.Group("/portal")
		controlPlaneHandler.RegisterAuthRoutes(portal)
		controlPlaneHandler.RegisterTenantRoutes(portal.Group("/tenant"))
		superAdmin := api.Group("/super-admin",
			controlPlaneHandler.RequirePortalAuth, controlPlaneHandler.RequireSuperAdmin)
		superAdminHandler.RegisterRoutes(superAdmin)
	}

	return router
}
