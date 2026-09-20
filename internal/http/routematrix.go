package api

// RouteMatrix 是全量路由权限矩阵的机器可读权威清单。
// 完备性测试用 gin.Engine.Routes() 对照本表：新增 HTTP 路由未登记则测试红。

type RoleAccess string

const (
	AccessAllow RoleAccess = "allow"
	AccessDeny  RoleAccess = "deny"
	AccessNA    RoleAccess = "n/a"
	// AccessSupportOnly：super_admin 仅在支持访问（带审计）中
	// 放行；tenant_admin/member 列不受影响。
	AccessSupportOnly RoleAccess = "support_only"
)

type NonHumanChannel string

const (
	ChannelNone             NonHumanChannel = "none"
	ChannelPublic           NonHumanChannel = "public"
	ChannelWebhookSignature NonHumanChannel = "webhook_signature"
	ChannelPerRunToken      NonHumanChannel = "per_run_token"
	ChannelRuntimeInternal  NonHumanChannel = "runtime_internal"
)

type RouteClass string

const (
	ClassHealth     RouteClass = "health"
	ClassWeb        RouteClass = "web"
	ClassSPA        RouteClass = "spa"
	ClassAdmin      RouteClass = "admin"
	ClassRuntime    RouteClass = "runtime"
	ClassWebhook    RouteClass = "webhook"
	ClassMCP        RouteClass = "mcp"
	ClassPortal     RouteClass = "portal"
	ClassSuperAdmin RouteClass = "super_admin"
)

const (
	mwNone                = "none"
	mwPortalMember        = "RequirePortalAuth + tenant scope + member+"
	mwPortalTenantAdmin   = "RequirePortalAuth + tenant scope + tenant_admin+"
	mwPortalSuperAdmin    = "RequirePortalAuth + RequireSuperAdmin"
	mwRuntimeGate         = "ConfigurationGate"
	mwWebhookVerify       = "ConfigurationGate + per-repo webhook signature"
	mwMCPToken            = "per-run bearer token (handler)"
	mwPortalAuth          = "RequirePortalAuth"
	mwPortalTenant        = "RequirePortalAuth + RequireTenantScope"
	mwPortalTenantAdminMW = "RequirePortalAuth + RequireTenantScope + RequireTenantAdmin"
	mwSuperAdminNS        = "RequirePortalAuth + RequireSuperAdmin"
)

// MatrixRow 登记一条现役路由（或 SPA 逻辑页）的三角色与非人通道。
// HTTP 行的 Method+Path 必须与 gin 注册路径逐字一致。
type MatrixRow struct {
	Method            string
	Path              string
	Class             RouteClass
	SuperAdmin        RoleAccess
	TenantAdmin       RoleAccess
	Member            RoleAccess
	Unauthenticated   RoleAccess
	NonHuman          NonHumanChannel
	Frozen            bool
	CurrentMiddleware string
	TargetMiddleware  string
	Notes             string
}

func RouteMatrix() []MatrixRow {
	return routeMatrixCatalog
}

func HTTPRouteMatrix() []MatrixRow {
	rows := make([]MatrixRow, 0, len(routeMatrixCatalog))
	for _, row := range routeMatrixCatalog {
		if row.Class == ClassSPA {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

func SPARouteMatrix() []MatrixRow {
	rows := make([]MatrixRow, 0)
	for _, row := range routeMatrixCatalog {
		if row.Class == ClassSPA {
			rows = append(rows, row)
		}
	}
	return rows
}

func routeKey(method, path string) string {
	return method + " " + path
}

func mr(method, path string, class RouteClass, sa, ta, mem, unauth RoleAccess, nh NonHumanChannel, frozen bool, current, target, notes string) MatrixRow {
	return MatrixRow{
		Method: method, Path: path, Class: class,
		SuperAdmin: sa, TenantAdmin: ta, Member: mem, Unauthenticated: unauth,
		NonHuman: nh, Frozen: frozen,
		CurrentMiddleware: current, TargetMiddleware: target, Notes: notes,
	}
}

const (
	allow        = AccessAllow
	deny         = AccessDeny
	na           = AccessNA
	support_only = AccessSupportOnly
)

var routeMatrixCatalog = concatMatrix(
	healthRows,
	spaShellRows,
	webhookRows,
	mcpRows,
	adminQueryRows,
	adminAgentRows,
	adminOperationRows,
	adminConfigurationRows,
	adminTenantConfigurationRows,
	adminPeopleRows,
	adminRepositoryRows,
	adminRequirementRows,
	adminConnectorRows,
	portalAuthRows,
	portalTenantRows,
	superAdminRows,
	spaPageRows,
)

func concatMatrix(groups ...[]MatrixRow) []MatrixRow {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	out := make([]MatrixRow, 0, total)
	for _, group := range groups {
		out = append(out, group...)
	}
	return out
}

var healthRows = []MatrixRow{
	mr("GET", "/healthz", ClassHealth, allow, allow, allow, allow, ChannelPublic, false, mwNone, mwNone, "存活探针，无会话"),
	mr("GET", "/readyz", ClassHealth, allow, allow, allow, allow, ChannelPublic, false, mwNone, mwNone, "就绪探针，含运行配置 mode"),
}

var spaShellRows = []MatrixRow{
	mr("GET", "/admin", ClassWeb, allow, allow, allow, allow, ChannelPublic, false, mwNone, mwNone, "SPA 入口；鉴权在前端"),
	mr("GET", "/admin/*path", ClassWeb, allow, allow, allow, allow, ChannelPublic, false, mwNone, mwNone, "SPA catch-all；逻辑页见 spa 表"),
}

var webhookRows = []MatrixRow{
	mr("POST", "/api/events/gitlab", ClassWebhook, na, na, na, na, ChannelWebhookSignature, true, mwWebhookVerify, mwWebhookVerify, "冻结：按仓验签，零改动"),
	mr("POST", "/api/events/gitea", ClassWebhook, na, na, na, na, ChannelWebhookSignature, true, mwWebhookVerify, mwWebhookVerify, "冻结：按仓验签，零改动"),
	mr("POST", "/api/events/github", ClassWebhook, na, na, na, na, ChannelWebhookSignature, true, mwWebhookVerify, mwWebhookVerify, "冻结：按仓验签，零改动"),
}

var mcpRows = []MatrixRow{
	mr("POST", "/mcp", ClassMCP, na, na, na, na, ChannelPerRunToken, true, mwMCPToken, mwMCPToken, "冻结：per-run token 结果回传"),
	mr("GET", "/mcp", ClassMCP, na, na, na, na, ChannelPerRunToken, true, mwMCPToken, mwMCPToken, "冻结：MCP GET"),
	mr("DELETE", "/mcp", ClassMCP, na, na, na, na, ChannelPerRunToken, true, mwMCPToken, mwMCPToken, "冻结：MCP 关会话"),
}

var adminQueryRows = []MatrixRow{
	mr("GET", "/api/admin/overview", ClassAdmin, allow, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "查询类：member 只读"),
	mr("GET", "/api/admin/metrics", ClassAdmin, allow, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "查询类"),
	mr("GET", "/api/admin/tasks", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "查询类"),
	mr("GET", "/api/admin/tasks/:task_id", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "查询类"),
	mr("GET", "/api/admin/tasks/:task_id/timeline", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "查询类"),
	mr("GET", "/api/admin/runs", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "查询类"),
	mr("GET", "/api/admin/runs/:run_id", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "查询类"),
	mr("GET", "/api/admin/artifacts", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "查询类"),
	mr("GET", "/api/admin/artifacts/:artifact_id", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "查询类"),
	mr("GET", "/api/admin/dependencies", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "查询类"),
	mr("GET", "/api/admin/dependencies/:dependency_id", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "查询类"),
	mr("GET", "/api/admin/writebacks", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "查询类"),
	mr("GET", "/api/admin/writebacks/:writeback_id", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "查询类"),
	mr("GET", "/api/admin/effects", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "查询类"),
	mr("GET", "/api/admin/effects/:effect_id", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "查询类"),
	mr("GET", "/api/admin/inbound-events", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "查询类"),
	mr("GET", "/api/admin/inbound-events/:event_id", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "查询类"),
	mr("GET", "/api/admin/result-submissions", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "查询类"),
	mr("GET", "/api/admin/result-submissions/:submission_id", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "查询类"),
}

var adminAgentRows = []MatrixRow{
	mr("GET", "/api/admin/agents/executions/summary", ClassAdmin, allow, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "观测只读"),
	mr("GET", "/api/admin/agents/executions/executions", ClassAdmin, allow, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "观测只读"),
	mr("GET", "/api/admin/agents/executions/executions/:execution_id", ClassAdmin, allow, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "观测只读"),
}

var adminOperationRows = []MatrixRow{
	mr("GET", "/api/admin/scheduler/defaults", ClassAdmin, allow, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "operations 配置类；member 403"),
	mr("POST", "/api/admin/scheduler/tick", ClassAdmin, allow, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin+" + "+mwRuntimeGate, mwPortalTenantAdmin+" + "+mwRuntimeGate, "写操作；另挂 ConfigurationGate"),
	mr("POST", "/api/admin/effects/:effect_id/retry", ClassAdmin, allow, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin+" + "+mwRuntimeGate, mwPortalTenantAdmin+" + "+mwRuntimeGate, "任务写操作"),
	mr("POST", "/api/admin/tasks/:task_id/rebind", ClassAdmin, allow, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin+" + "+mwRuntimeGate, mwPortalTenantAdmin+" + "+mwRuntimeGate, "任务写操作"),
}

var adminConfigurationRows = []MatrixRow{
	mr("GET", "/api/admin/configuration/status", ClassAdmin, allow, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "ConfigurationBoundary 全局探针，member+ 放行；载荷无 config/secrets；配置页本体仍仅超管"),
	mr("GET", "/api/admin/configuration", ClassAdmin, allow, deny, deny, deny, ChannelNone, false, mwPortalSuperAdmin, mwPortalSuperAdmin, "平台级运行配置；分库前仅 super_admin"),
	mr("POST", "/api/admin/configuration/validate", ClassAdmin, allow, deny, deny, deny, ChannelNone, false, mwPortalSuperAdmin, mwPortalSuperAdmin, "仅 super_admin"),
	mr("PUT", "/api/admin/configuration", ClassAdmin, allow, deny, deny, deny, ChannelNone, false, mwPortalSuperAdmin, mwPortalSuperAdmin, "仅 super_admin"),
	mr("POST", "/api/admin/configuration/restart", ClassAdmin, allow, deny, deny, deny, ChannelNone, false, mwPortalSuperAdmin, mwPortalSuperAdmin, "仅 super_admin"),
}

var adminTenantConfigurationRows = []MatrixRow{
	mr("GET", "/api/admin/tenant-configuration", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "组织段运行配置视图；组织管理员"),
	mr("POST", "/api/admin/tenant-configuration/validate", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "组织段配置预校验"),
	mr("PUT", "/api/admin/tenant-configuration", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "组织段配置保存，热生效无重启"),
}

var adminPeopleRows = []MatrixRow{
	mr("POST", "/api/admin/people/codehost-identities/lookup", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "people 写"),
	mr("POST", "/api/admin/people/codehost-identities/lookup-batch", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "people 写"),
	mr("GET", "/api/admin/people", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "人员资产页；member 403"),
	mr("POST", "/api/admin/people", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "people 写"),
	mr("GET", "/api/admin/people/:person_id", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "人员资产页"),
	mr("PATCH", "/api/admin/people/:person_id", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "people 写"),
	mr("DELETE", "/api/admin/people/:person_id", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "people 写"),
}

var adminRepositoryRows = []MatrixRow{
	mr("GET", "/api/admin/repositories", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "member 可读（任务页筛选器）；响应无 token/webhook_secret；管理页 UI 隐藏归后续批次"),
	mr("POST", "/api/admin/repositories", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "repositories 写"),
	mr("GET", "/api/admin/repositories/:repository_id", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "member 可读；响应无 token/webhook_secret"),
	mr("PATCH", "/api/admin/repositories/:repository_id", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "repositories 写"),
	mr("POST", "/api/admin/repositories/:repository_id/disable", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "repositories 写"),
	mr("POST", "/api/admin/repositories/:repository_id/enable", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "repositories 写"),
}

var adminRequirementRows = []MatrixRow{
	mr("GET", "/api/admin/requirements", ClassAdmin, support_only, allow, allow, deny, ChannelNone, false, mwPortalMember, mwPortalMember, "requirements 读"),
	mr("POST", "/api/admin/requirements", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "requirements 写"),
	mr("POST", "/api/admin/requirements/sync", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "requirements 写"),
	mr("PATCH", "/api/admin/requirements/:requirement_id", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "requirements 写"),
	mr("POST", "/api/admin/requirements/:requirement_id/publish", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "requirements 写"),
	mr("POST", "/api/admin/requirements/:requirement_id/cancel", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "requirements 写"),
	mr("POST", "/api/admin/requirements/:requirement_id/complete", ClassAdmin, support_only, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "requirements 写"),
}

var adminConnectorRows = []MatrixRow{
	mr("GET", "/api/admin/connectors", ClassAdmin, allow, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "connectors 配置类"),
	mr("POST", "/api/admin/connectors", ClassAdmin, allow, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "connectors 写"),
	mr("GET", "/api/admin/connectors/:connector_id", ClassAdmin, allow, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "connectors 配置类"),
	mr("PATCH", "/api/admin/connectors/:connector_id", ClassAdmin, allow, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "connectors 写"),
	mr("DELETE", "/api/admin/connectors/:connector_id", ClassAdmin, allow, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "connectors 写"),
	mr("POST", "/api/admin/connectors/:connector_id/probe", ClassAdmin, allow, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "connectors 写"),
	mr("POST", "/api/admin/connectors/:connector_id/runs", ClassAdmin, allow, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "connectors 写"),
	mr("GET", "/api/admin/connectors/:connector_id/runs", ClassAdmin, allow, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "connectors 配置类"),
	mr("GET", "/api/admin/connectors/:connector_id/runs/:run_id", ClassAdmin, allow, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "connectors 配置类"),
	mr("GET", "/api/admin/connectors/:connector_id/records", ClassAdmin, allow, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "connectors 配置类"),
	mr("GET", "/api/admin/connectors/:connector_id/records/:record_id", ClassAdmin, allow, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "connectors 配置类"),
	mr("GET", "/api/admin/connectors/:connector_id/metrics", ClassAdmin, allow, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "connectors 配置类"),
	mr("GET", "/api/admin/connectors/:connector_id/configuration", ClassAdmin, allow, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "connectors 配置类"),
	mr("PUT", "/api/admin/connectors/:connector_id/configuration", ClassAdmin, allow, allow, deny, deny, ChannelNone, false, mwPortalTenantAdmin, mwPortalTenantAdmin, "connectors 写"),
}

var portalAuthRows = []MatrixRow{
	mr("POST", "/api/portal/auth/login", ClassPortal, na, na, na, allow, ChannelNone, false, mwNone, mwNone, "门户登录；现状不变"),
	mr("POST", "/api/portal/auth/register", ClassPortal, na, na, na, allow, ChannelNone, false, mwNone, mwNone, "邀请码注册；现状不变"),
	mr("POST", "/api/portal/auth/logout", ClassPortal, allow, allow, allow, deny, ChannelNone, false, mwPortalAuth, mwPortalAuth, "现状不变"),
	mr("GET", "/api/portal/auth/session", ClassPortal, allow, allow, allow, deny, ChannelNone, false, mwPortalAuth, mwPortalAuth, "现状不变"),
	mr("POST", "/api/portal/auth/select-tenant", ClassPortal, allow, allow, allow, deny, ChannelNone, false, mwPortalAuth, mwPortalAuth, "现状不变"),
	mr("POST", "/api/portal/auth/change-password", ClassPortal, allow, allow, allow, deny, ChannelNone, false, mwPortalAuth, mwPortalAuth, "现状不变"),
}

var portalTenantRows = []MatrixRow{
	mr("GET", "/api/portal/tenant/profile", ClassPortal, deny, allow, allow, deny, ChannelNone, false, mwPortalTenant, mwPortalTenant, "超管默认 403；支持访问时放行（现状）"),
	mr("GET", "/api/portal/tenant/members", ClassPortal, deny, allow, deny, deny, ChannelNone, false, mwPortalTenantAdminMW, mwPortalTenantAdminMW, "组织管理员；支持访问中的超管等同"),
	mr("DELETE", "/api/portal/tenant/members/:user_id", ClassPortal, deny, allow, deny, deny, ChannelNone, false, mwPortalTenantAdminMW, mwPortalTenantAdminMW, "现状不变"),
	mr("POST", "/api/portal/tenant/members/:user_id/reset-password", ClassPortal, deny, allow, deny, deny, ChannelNone, false, mwPortalTenantAdminMW, mwPortalTenantAdminMW, "现状不变"),
	mr("GET", "/api/portal/tenant/invite-codes", ClassPortal, deny, allow, deny, deny, ChannelNone, false, mwPortalTenantAdminMW, mwPortalTenantAdminMW, "现状不变"),
	mr("POST", "/api/portal/tenant/invite-codes", ClassPortal, deny, allow, deny, deny, ChannelNone, false, mwPortalTenantAdminMW, mwPortalTenantAdminMW, "现状不变"),
	mr("DELETE", "/api/portal/tenant/invite-codes/:code_id", ClassPortal, deny, allow, deny, deny, ChannelNone, false, mwPortalTenantAdminMW, mwPortalTenantAdminMW, "现状不变"),
}

var superAdminRows = []MatrixRow{
	mr("GET", "/api/super-admin/tenants", ClassSuperAdmin, allow, deny, deny, deny, ChannelNone, false, mwSuperAdminNS, mwSuperAdminNS, "现状不变"),
	mr("POST", "/api/super-admin/tenants", ClassSuperAdmin, allow, deny, deny, deny, ChannelNone, false, mwSuperAdminNS, mwSuperAdminNS, "现状不变"),
	mr("PATCH", "/api/super-admin/tenants/:id", ClassSuperAdmin, allow, deny, deny, deny, ChannelNone, false, mwSuperAdminNS, mwSuperAdminNS, "现状不变"),
	mr("POST", "/api/super-admin/tenants/:id/disable", ClassSuperAdmin, allow, deny, deny, deny, ChannelNone, false, mwSuperAdminNS, mwSuperAdminNS, "现状不变"),
	mr("POST", "/api/super-admin/tenants/:id/restore", ClassSuperAdmin, allow, deny, deny, deny, ChannelNone, false, mwSuperAdminNS, mwSuperAdminNS, "现状不变"),
	mr("GET", "/api/super-admin/tenants/:id/members", ClassSuperAdmin, allow, deny, deny, deny, ChannelNone, false, mwSuperAdminNS, mwSuperAdminNS, "现状不变"),
	mr("POST", "/api/super-admin/tenants/:id/admins", ClassSuperAdmin, allow, deny, deny, deny, ChannelNone, false, mwSuperAdminNS, mwSuperAdminNS, "现状不变"),
	mr("POST", "/api/super-admin/tenants/:id/admins/:user_id/reset-password", ClassSuperAdmin, allow, deny, deny, deny, ChannelNone, false, mwSuperAdminNS, mwSuperAdminNS, "现状不变"),
	mr("GET", "/api/super-admin/tenants/:id/invite-codes", ClassSuperAdmin, allow, deny, deny, deny, ChannelNone, false, mwSuperAdminNS, mwSuperAdminNS, "现状不变"),
	mr("DELETE", "/api/super-admin/tenants/:id/invite-codes/:code_id", ClassSuperAdmin, allow, deny, deny, deny, ChannelNone, false, mwSuperAdminNS, mwSuperAdminNS, "现状不变"),
	mr("GET", "/api/super-admin/tenants/:id/runner-assignments", ClassSuperAdmin, allow, deny, deny, deny, ChannelNone, false, mwSuperAdminNS, mwSuperAdminNS, "控制面静态指派；runner 拓扑动态化归后续批次"),
	mr("PUT", "/api/super-admin/tenants/:id/runner-assignments", ClassSuperAdmin, allow, deny, deny, deny, ChannelNone, false, mwSuperAdminNS, mwSuperAdminNS, "同上"),
	mr("POST", "/api/super-admin/tenants/:id/support-access", ClassSuperAdmin, allow, deny, deny, deny, ChannelNone, false, mwSuperAdminNS, mwSuperAdminNS, "现状不变"),
	mr("DELETE", "/api/super-admin/support-access", ClassSuperAdmin, allow, deny, deny, deny, ChannelNone, false, mwSuperAdminNS, mwSuperAdminNS, "现状不变"),
	mr("GET", "/api/super-admin/audit-log", ClassSuperAdmin, allow, deny, deny, deny, ChannelNone, false, mwSuperAdminNS, mwSuperAdminNS, "现状不变"),
}

var spaPageRows = []MatrixRow{
	mr("GET", "/admin/login", ClassSPA, na, na, na, allow, ChannelPublic, false, mwNone, mwNone, "书签页；前端重定向门户登录，无旧表单"),
	mr("GET", "/admin/", ClassSPA, allow, allow, allow, deny, ChannelNone, false, "AuthenticationBoundary", "portal session + member+", "TA/SA 运行总览；member 默认成员工作台"),
	mr("GET", "/admin/setup", ClassSPA, allow, deny, deny, deny, ChannelNone, false, "AuthenticationBoundary", mwPortalSuperAdmin, "配置向导；仅超管"),
	mr("GET", "/admin/settings", ClassSPA, allow, deny, deny, deny, ChannelNone, false, "AuthenticationBoundary", mwPortalSuperAdmin, "系统配置页；仅超管"),
	mr("GET", "/admin/people", ClassSPA, allow, allow, deny, deny, ChannelNone, false, "AuthenticationBoundary", mwPortalTenantAdmin, "人员资产；member 隐藏"),
	mr("GET", "/admin/requirements", ClassSPA, allow, allow, allow, deny, ChannelNone, false, "AuthenticationBoundary", mwPortalMember, "member 只读；写入口按角色隐藏"),
	mr("GET", "/admin/repositories", ClassSPA, allow, allow, deny, deny, ChannelNone, false, "AuthenticationBoundary", mwPortalTenantAdmin, "仓库管理页；member 隐藏（API GET 仍放行给筛选器）"),
	mr("GET", "/admin/tasks", ClassSPA, allow, allow, allow, deny, ChannelNone, false, "AuthenticationBoundary", mwPortalMember, "任务查询"),
	mr("GET", "/admin/explorer", ClassSPA, allow, allow, allow, deny, ChannelNone, false, "AuthenticationBoundary", mwPortalMember, "数据查询"),
	mr("GET", "/admin/connectors", ClassSPA, allow, allow, deny, deny, ChannelNone, false, "AuthenticationBoundary", mwPortalTenantAdmin, "连接器；member 隐藏"),
	mr("GET", "/admin/operations", ClassSPA, allow, allow, deny, deny, ChannelNone, false, "AuthenticationBoundary", mwPortalTenantAdmin, "安全操作；member 隐藏"),
	mr("GET", "/admin/agents", ClassSPA, allow, allow, allow, deny, ChannelNone, false, "AuthenticationBoundary", mwPortalMember, "Agent 观测只读"),
	mr("GET", "/admin/portal/login", ClassSPA, na, na, na, allow, ChannelPublic, false, mwNone, mwNone, "门户登录；唯一浏览器入口"),
	mr("GET", "/admin/portal/register", ClassSPA, na, na, na, allow, ChannelPublic, false, mwNone, mwNone, "邀请码注册"),
	mr("GET", "/admin/portal/select-tenant", ClassSPA, allow, allow, allow, deny, ChannelNone, false, "PortalSessionGate", "PortalSessionGate", "现状不变"),
	mr("GET", "/admin/portal/change-password", ClassSPA, allow, allow, allow, deny, ChannelNone, false, "PortalSessionGate", "PortalSessionGate", "现状不变"),
	mr("GET", "/admin/org", ClassSPA, deny, allow, deny, deny, ChannelNone, false, "PortalSessionGate", "PortalSessionGate", "组织管理内嵌工作台；member 不可见；支持访问中的超管放行"),
	mr("GET", "/admin/org/settings", ClassSPA, deny, allow, deny, deny, ChannelNone, false, "PortalSessionGate", "PortalSessionGate", "组织配置向导；member 不可见；支持访问中的超管放行"),
	mr("GET", "/admin/super-admin/tenants", ClassSPA, allow, deny, deny, deny, ChannelNone, false, "SuperAdminSessionGate", "SuperAdminSessionGate", "现状不变"),
	mr("GET", "/admin/super-admin/tenants/:id", ClassSPA, allow, deny, deny, deny, ChannelNone, false, "SuperAdminSessionGate", "SuperAdminSessionGate", "现状不变"),
	mr("GET", "/admin/super-admin/audit-log", ClassSPA, allow, deny, deny, deny, ChannelNone, false, "SuperAdminSessionGate", "SuperAdminSessionGate", "现状不变"),
}
