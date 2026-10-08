package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sdlc-coordinator/internal/controlplane"

	"github.com/gin-gonic/gin"
)

// PortalSessionCookieName 是门户会话 cookie，这是浏览器登录的唯一通道。
const PortalSessionCookieName = "coordinator_portal_session"

// ControlPlaneHandler 承载新认证体系 HTTP 面：/api/portal/auth/*（认证）
// 与 /api/portal/tenant/*（组织作用域管理）。控制面已装配时，
// /api/admin/* 也走本 handler 的门户会话 + 矩阵 RBAC。
type ControlPlaneHandler struct {
	service *controlplane.Service
	// cookieSecure 控制门户会话 cookie 的 Secure 属性，默认 true；
	// 明文 http 部署可经 PORTAL_COOKIE_SECURE=0 关闭。
	cookieSecure bool
	adminPolicy  AdminRoutePolicy
}

func NewControlPlaneHandler(service *controlplane.Service) *ControlPlaneHandler {
	return &ControlPlaneHandler{service: service, cookieSecure: true}
}

// SetCookieSecure 覆写门户会话 cookie 的 Secure 属性（装配期调用）。
func (handler *ControlPlaneHandler) SetCookieSecure(secure bool) {
	handler.cookieSecure = secure
}

// AnnotateAuditSourceIP 把 Gin ClientIP（TrustedProxies=nil 时为直连对端）写入
// 请求 ctx，控制面审计与业务同事务读取。
func AnnotateAuditSourceIP() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(controlplane.WithAuditSourceIP(c.Request.Context(), c.ClientIP()))
		c.Next()
	}
}

// RegisterAuthRoutes 挂载认证端点：登录/注册公开，其余须有效会话。
func (handler *ControlPlaneHandler) RegisterAuthRoutes(group *gin.RouterGroup) {
	group.Use(AnnotateAuditSourceIP())
	group.POST("/auth/login", handler.Login)
	group.POST("/auth/register", handler.Register)
	authed := group.Group("/auth", handler.RequirePortalAuth)
	authed.POST("/logout", handler.Logout)
	authed.GET("/session", handler.Session)
	authed.POST("/select-tenant", handler.SelectTenant)
	authed.POST("/change-password", handler.ChangePassword)
	authed.POST("/switch-account", handler.SwitchAccount)
	authed.POST("/revoke-saved", handler.RevokeSaved)
}

// RegisterTenantRoutes 挂载组织作用域端点：成员与邀请码管理归组织管理员
// （tenant_admin），profile 面向全体成员。
func (handler *ControlPlaneHandler) RegisterTenantRoutes(group *gin.RouterGroup) {
	group.Use(AnnotateAuditSourceIP(), handler.RequirePortalAuth, handler.RequireTenantScope)
	group.GET("/profile", handler.TenantProfile)
	adminOnly := group.Group("", handler.RequireTenantAdmin)
	adminOnly.GET("/members", handler.ListMembers)
	adminOnly.DELETE("/members/:user_id", handler.RemoveMember)
	adminOnly.POST("/members/:user_id/reset-password", handler.ResetMemberPassword)
	adminOnly.GET("/invite-codes", handler.ListInviteCodes)
	adminOnly.POST("/invite-codes", handler.CreateInviteCode)
	adminOnly.DELETE("/invite-codes/:code_id", handler.RevokeInviteCode)
}

// ---- 会话上下文（gin keys） ----

const (
	cpSessionKey = "cp_session_info"
	cpTenantKey  = "cp_effective_tenant"
	cpRoleKey    = "cp_effective_role"
	cpSupportKey = "cp_support_mode"
	cpUserKey    = "cp_user"
	cpActorKey   = "cp_actor"
)

func (handler *ControlPlaneHandler) sessionInfo(c *gin.Context) *controlplane.SessionInfo {
	return CurrentPortalSession(c)
}

func currentActor(c *gin.Context) *controlplane.Actor {
	value, _ := c.Get(cpActorKey)
	actor, _ := value.(*controlplane.Actor)
	return actor
}

// ---- 中间件 ----

// RequirePortalAuth 校验会话与账号状态；强制改密账号仅放行
// 登出/会话查询/改密三个端点（首登强制改密）。
func (handler *ControlPlaneHandler) RequirePortalAuth(c *gin.Context) {
	info, err := handler.service.SessionInfo(c.Request.Context(), portalCookieToken(c))
	if err != nil {
		writeControlPlaneError(c, err)
		c.Abort()
		return
	}
	if info.User.MustChangePassword {
		switch c.FullPath() {
		case "/api/portal/auth/change-password", "/api/portal/auth/logout", "/api/portal/auth/session":
		default:
			writeAdminError(c, http.StatusForbidden, "password_change_required", "首次登录请先修改密码")
			c.Abort()
			return
		}
	}
	c.Set(cpSessionKey, info)
	c.Set(cpUserKey, info.User)
	c.Set(cpActorKey, &controlplane.Actor{ID: info.User.ID, Username: info.User.Username})
	c.Next()
}

// RequireTenantScope 要求有效组织作用域：
// 超管未走支持访问审计 → 403 super_admin_scope_required（隔离头条①）；
// 无作用域成员 → 403 tenant_scope_required；组织禁用且非支持访问 → 403。
func (handler *ControlPlaneHandler) RequireTenantScope(c *gin.Context) {
	info := handler.sessionInfo(c)
	if info == nil {
		writeAdminError(c, http.StatusUnauthorized, "unauthenticated", "请先登录")
		c.Abort()
		return
	}
	switch {
	case info.SupportMode:
		// 超管支持访问：带审计进入的组织上下文
	case info.SuperAdminMode:
		writeAdminError(c, http.StatusForbidden, "super_admin_scope_required", "超级管理员需通过支持访问进入组织上下文")
		c.Abort()
		return
	case info.EffectiveTenant == nil:
		writeAdminError(c, http.StatusForbidden, "tenant_scope_required", "请先选择组织")
		c.Abort()
		return
	case info.EffectiveTenant.Status != controlplane.TenantStatusActive:
		writeAdminError(c, http.StatusForbidden, "tenant_disabled", "该组织已被停用")
		c.Abort()
		return
	}
	c.Set(cpTenantKey, info.EffectiveTenant)
	c.Set(cpRoleKey, info.EffectiveRole)
	c.Set(cpSupportKey, info.SupportMode)
	c.Next()
}

// RequireTenantAdmin 要求组织管理员角色；支持访问中的超管等同管理员视野。
func (handler *ControlPlaneHandler) RequireTenantAdmin(c *gin.Context) {
	info := handler.sessionInfo(c)
	role, _ := c.Get(cpRoleKey)
	roleName, _ := role.(string)
	if !info.SupportMode && roleName != controlplane.RoleTenantAdmin {
		// 隔离头条②：普通成员对管理 API 一律 403
		writeAdminError(c, http.StatusForbidden, "forbidden", "需要组织管理员权限")
		c.Abort()
		return
	}
	c.Next()
}

// RequireSuperAdmin 是 /api/super-admin/* 命名空间的门禁。
func (handler *ControlPlaneHandler) RequireSuperAdmin(c *gin.Context) {
	info := handler.sessionInfo(c)
	if info == nil || !info.SuperAdminMode {
		writeAdminError(c, http.StatusForbidden, "forbidden", "需要超级管理员权限")
		c.Abort()
		return
	}
	c.Next()
}

// ---- 认证端点 ----

type portalLoginBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (handler *ControlPlaneHandler) Login(c *gin.Context) {
	var body portalLoginBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeAdminError(c, http.StatusBadRequest, "invalid_request", "请输入账号和密码")
		return
	}
	result, err := handler.service.Login(c.Request.Context(), body.Username, body.Password, c.ClientIP())
	if err != nil {
		writeControlPlaneError(c, err)
		return
	}
	handler.setPortalSessionCookie(c, result.Token)
	c.JSON(http.StatusOK, portalAuthPayloadWithToken(result.User, result.Scope, result.Tenants, result.Token))
}

type portalRegisterBody struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	InviteCode string `json:"invite_code"`
}

func (handler *ControlPlaneHandler) Register(c *gin.Context) {
	var body portalRegisterBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeAdminError(c, http.StatusBadRequest, "invalid_request", "请填写用户名、密码和邀请码")
		return
	}
	result, err := handler.service.RegisterWithInviteCode(c.Request.Context(), body.Username, body.Password, body.InviteCode)
	if err != nil {
		writeControlPlaneError(c, err)
		return
	}
	handler.setPortalSessionCookie(c, result.Token)
	c.JSON(http.StatusOK, portalAuthPayloadWithToken(result.User, result.Scope, result.Tenants, result.Token))
}

type portalSelectTenantBody struct {
	TenantID uint `json:"tenant_id"`
}

func (handler *ControlPlaneHandler) SelectTenant(c *gin.Context) {
	var body portalSelectTenantBody
	if err := c.ShouldBindJSON(&body); err != nil || body.TenantID == 0 {
		writeAdminError(c, http.StatusBadRequest, "invalid_request", "请选择组织")
		return
	}
	result, err := handler.service.SelectTenant(c.Request.Context(), portalCookieToken(c), body.TenantID)
	if err != nil {
		writeControlPlaneError(c, err)
		return
	}
	handler.setPortalSessionCookie(c, result.Token)
	c.JSON(http.StatusOK, portalAuthPayloadWithToken(result.User, result.Scope, result.Tenants, result.Token))
}

type portalChangePasswordBody struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (handler *ControlPlaneHandler) ChangePassword(c *gin.Context) {
	var body portalChangePasswordBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeAdminError(c, http.StatusBadRequest, "invalid_request", "请填写当前密码和新密码")
		return
	}
	actor := currentActor(c)
	if actor == nil {
		writeAdminError(c, http.StatusUnauthorized, "unauthenticated", "请先登录")
		return
	}
	if err := handler.service.ChangePassword(c.Request.Context(), actor.ID, body.CurrentPassword, body.NewPassword); err != nil {
		writeControlPlaneError(c, err)
		return
	}
	// 改密已吊销全部会话：清除本端 cookie，引导重新登录
	handler.clearPortalSessionCookie(c)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (handler *ControlPlaneHandler) Logout(c *gin.Context) {
	// 撤销失败不得伪报已登出：保留会话 cookie 供用户重试，错误走统一包络。
	if err := handler.service.Logout(c.Request.Context(), portalCookieToken(c)); err != nil {
		writeControlPlaneError(c, err)
		return
	}
	handler.clearPortalSessionCookie(c)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type portalSwitchAccountBody struct {
	Token string `json:"token"`
}

// SwitchAccount 把会话 cookie 改写为注册表中的另一账号 token：服务端校验
// 该 token 指向有效会话后原样写回，不新建也不吊销任何会话。
func (handler *ControlPlaneHandler) SwitchAccount(c *gin.Context) {
	var body portalSwitchAccountBody
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Token) == "" {
		writeAdminError(c, http.StatusBadRequest, "invalid_request", "请提供要切换的会话")
		return
	}
	if err := handler.service.SwitchAccount(c.Request.Context(), body.Token); err != nil {
		writeControlPlaneError(c, err)
		return
	}
	handler.setPortalSessionCookie(c, body.Token)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type portalRevokeSavedBody struct {
	Tokens []string `json:"tokens"`
}

// RevokeSaved 批量撤销已保存账号的会话并清除本端 cookie：token 不存在或
// 已过期按已撤销处理（幂等）。
func (handler *ControlPlaneHandler) RevokeSaved(c *gin.Context) {
	var body portalRevokeSavedBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeAdminError(c, http.StatusBadRequest, "invalid_request", "请提供要退出的账号列表")
		return
	}
	if err := handler.service.RevokeSavedSessions(c.Request.Context(), body.Tokens); err != nil {
		writeControlPlaneError(c, err)
		return
	}
	handler.clearPortalSessionCookie(c)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (handler *ControlPlaneHandler) Session(c *gin.Context) {
	info := handler.sessionInfo(c)
	if info == nil {
		writeAdminError(c, http.StatusUnauthorized, "unauthenticated", "请先登录")
		return
	}
	payload := portalAuthPayload(info.User, info.Scope, info.Tenants)
	if info.SupportMode {
		payload["support"] = gin.H{
			"active":     true,
			"reason":     info.SupportReason,
			"entered_at": formatTimePtr(info.SupportEnteredAt),
		}
	}
	c.JSON(http.StatusOK, payload)
}

// ---- 组织作用域端点（最小集与成员/邀请码全量管理） ----

func (handler *ControlPlaneHandler) TenantProfile(c *gin.Context) {
	info := handler.sessionInfo(c)
	if info == nil || info.EffectiveTenant == nil {
		writeAdminError(c, http.StatusUnauthorized, "unauthenticated", "请先登录")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"tenant":  tenantView(info.EffectiveTenant),
		"role":    info.EffectiveRole,
		"support": info.SupportMode,
	})
}

func (handler *ControlPlaneHandler) ListMembers(c *gin.Context) {
	tenant := effectiveTenant(c)
	members, err := handler.service.ListMembers(c.Request.Context(), tenant.ID)
	if err != nil {
		writeControlPlaneError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"members": members})
}

func (handler *ControlPlaneHandler) RemoveMember(c *gin.Context) {
	userID, ok := parseUintParam(c, "user_id")
	if !ok {
		return
	}
	tenant := effectiveTenant(c)
	if _, err := handler.service.RemoveMember(c.Request.Context(), currentActor(c), tenant.ID, userID); err != nil {
		writeControlPlaneError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (handler *ControlPlaneHandler) ResetMemberPassword(c *gin.Context) {
	userID, ok := parseUintParam(c, "user_id")
	if !ok {
		return
	}
	tenant := effectiveTenant(c)
	initial, err := handler.service.ResetMemberPassword(c.Request.Context(), currentActor(c), tenant.ID, userID)
	if err != nil {
		writeControlPlaneError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"initial_password": initial})
}

func (handler *ControlPlaneHandler) ListInviteCodes(c *gin.Context) {
	tenant := effectiveTenant(c)
	codes, err := handler.service.ListInviteCodes(c.Request.Context(), tenant.ID)
	if err != nil {
		writeControlPlaneError(c, err)
		return
	}
	views := make([]gin.H, 0, len(codes))
	for i := range codes {
		views = append(views, inviteCodeView(&codes[i]))
	}
	c.JSON(http.StatusOK, gin.H{"invite_codes": views})
}

func (handler *ControlPlaneHandler) RevokeInviteCode(c *gin.Context) {
	codeID, ok := parseUintParam(c, "code_id")
	if !ok {
		return
	}
	tenant := effectiveTenant(c)
	if err := handler.service.RevokeInviteCode(c.Request.Context(), currentActor(c), tenant.ID, codeID); err != nil {
		writeControlPlaneError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type portalInviteCodeBody struct {
	ExpiresInHours int `json:"expires_in_hours"`
	MaxUses        int `json:"max_uses"`
}

func (handler *ControlPlaneHandler) CreateInviteCode(c *gin.Context) {
	var body portalInviteCodeBody
	if err := c.ShouldBindJSON(&body); err != nil {
		body = portalInviteCodeBody{}
	}
	// 参数夹紧：0 取默认，越界直接 400，避免签出「已过期/零次」废码。
	if body.ExpiresInHours < 0 || body.ExpiresInHours > 720 {
		writeAdminError(c, http.StatusBadRequest, "invalid_request", "有效期须在 1-720 小时之间（留空默认 72）")
		return
	}
	if body.MaxUses < 0 || body.MaxUses > 1000 {
		writeAdminError(c, http.StatusBadRequest, "invalid_request", "次数上限须在 1-1000 之间（留空默认 10）")
		return
	}
	tenant := effectiveTenant(c)
	issued, err := handler.service.IssueInviteCode(c.Request.Context(), currentActor(c), tenant.ID, controlplane.InviteCodeInput{
		ExpiresInHours: body.ExpiresInHours, MaxUses: body.MaxUses,
	})
	if err != nil {
		writeControlPlaneError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":   issued.Plaintext,
		"record": inviteCodeView(issued.Record),
	})
}

// ---- 视图与工具 ----

func portalAuthPayload(user *controlplane.User, scope controlplane.SessionScope, tenants []controlplane.TenantOption) gin.H {
	payload := gin.H{
		"user": gin.H{
			"id":                   user.ID,
			"username":             user.Username,
			"role":                 user.Role,
			"must_change_password": user.MustChangePassword,
		},
		"scope": scopeView(scope),
	}
	if scope.Kind == controlplane.ScopeUnscoped {
		payload["tenants"] = tenants
	}
	return payload
}

// portalAuthPayloadWithToken 在登录/注册/选择组织响应中附带会话 token：
// 前端多账号注册表凭此登记当前账号。session 查询端点不得使用本变体
// （会话令牌只在颁发时刻下发，不随查询回显）。
func portalAuthPayloadWithToken(user *controlplane.User, scope controlplane.SessionScope, tenants []controlplane.TenantOption, token string) gin.H {
	payload := portalAuthPayload(user, scope, tenants)
	payload["session_token"] = token
	return payload
}

func scopeView(scope controlplane.SessionScope) gin.H {
	view := gin.H{"kind": scope.Kind, "role": scope.Role}
	if scope.Tenant != nil {
		view["tenant"] = tenantView(scope.Tenant)
	}
	return view
}

func tenantView(tenant *controlplane.Tenant) gin.H {
	if tenant == nil {
		return nil
	}
	return gin.H{
		"id":           tenant.ID,
		"slug":         tenant.Slug,
		"display_name": tenant.DisplayName,
		"status":       tenant.Status,
		"created_at":   tenant.CreatedAt.UTC().Format(timeRFC3339),
	}
}

func inviteCodeView(code *controlplane.InviteCode) gin.H {
	if code == nil {
		return nil
	}
	return gin.H{
		"id":         code.ID,
		"role":       code.Role,
		"expires_at": code.ExpiresAt.UTC().Format(timeRFC3339),
		"max_uses":   code.MaxUses,
		"used_count": code.UsedCount,
		"revoked_at": formatTimePtr(code.RevokedAt),
		"created_by": code.CreatedBy,
		"created_at": code.CreatedAt.UTC().Format(timeRFC3339),
	}
}

func effectiveTenant(c *gin.Context) *controlplane.Tenant {
	value, _ := c.Get(cpTenantKey)
	tenant, _ := value.(*controlplane.Tenant)
	return tenant
}

func portalCookieToken(c *gin.Context) string {
	cookie, err := c.Request.Cookie(PortalSessionCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func (handler *ControlPlaneHandler) setPortalSessionCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(PortalSessionCookieName, token, int(controlplane.DefaultSessionTTL.Seconds()), "/", "", handler.cookieSecure, true)
}

func (handler *ControlPlaneHandler) clearPortalSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(PortalSessionCookieName, "", -1, "/", "", handler.cookieSecure, true)
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

func formatTimePtr(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(timeRFC3339)
}

// writeControlPlaneError 把控制面服务错误统一映射为 HTTP 错误包络（中文文案，
// 「租户」一律写「组织」）。
func writeControlPlaneError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, controlplane.ErrUnauthenticated):
		writeAdminError(c, http.StatusUnauthorized, "unauthenticated", "请先登录")
	case errors.Is(err, controlplane.ErrSessionRevoked):
		writeAdminError(c, http.StatusUnauthorized, "session_revoked", "会话已失效，请重新登录")
	case errors.Is(err, controlplane.ErrInvalidCredentials):
		writeAdminError(c, http.StatusUnauthorized, "invalid_credentials", "账号或密码错误")
	case errors.Is(err, controlplane.ErrRateLimited):
		c.Header("Retry-After", "60")
		writeAdminError(c, http.StatusTooManyRequests, "rate_limited", "尝试过于频繁，请稍后再试")
	case errors.Is(err, controlplane.ErrPasswordChangeRequired):
		writeAdminError(c, http.StatusForbidden, "password_change_required", "请先修改密码")
	case errors.Is(err, controlplane.ErrInvalidInput):
		writeAdminError(c, http.StatusBadRequest, "invalid_request", "输入不合法")
	case errors.Is(err, controlplane.ErrReasonRequired):
		writeAdminError(c, http.StatusBadRequest, "reason_required", "请填写支持访问理由")
	case errors.Is(err, controlplane.ErrCannotRemoveSelf):
		writeAdminError(c, http.StatusBadRequest, "cannot_remove_self", "不能移除自己的成员关系")
	case errors.Is(err, controlplane.ErrUsernameTaken):
		writeAdminError(c, http.StatusConflict, "username_taken", "用户名已被占用")
	case errors.Is(err, controlplane.ErrMembershipExists):
		writeAdminError(c, http.StatusConflict, "membership_exists", "该成员关系已存在")
	case errors.Is(err, controlplane.ErrSlugTaken):
		writeAdminError(c, http.StatusConflict, "slug_taken", "组织标识已被占用")
	case errors.Is(err, controlplane.ErrTenantAlreadyDisabled):
		writeAdminError(c, http.StatusConflict, "state_conflict", "组织状态已处于目标状态")
	case errors.Is(err, controlplane.ErrTenantNotDisabled):
		writeAdminError(c, http.StatusConflict, "state_conflict", "组织状态已处于目标状态")
	case errors.Is(err, controlplane.ErrInviteInvalid):
		writeAdminError(c, http.StatusBadRequest, "invalid_invite_code", "邀请码无效或已过期")
	case errors.Is(err, controlplane.ErrTenantDisabled):
		writeAdminError(c, http.StatusForbidden, "tenant_disabled", "该组织已被停用")
	case errors.Is(err, controlplane.ErrNoMemberships):
		writeAdminError(c, http.StatusForbidden, "no_memberships", "账号未加入任何可用组织")
	case errors.Is(err, controlplane.ErrForbidden):
		writeAdminError(c, http.StatusForbidden, "forbidden", "没有执行该操作的权限")
	case errors.Is(err, controlplane.ErrTenantNotFound):
		writeAdminError(c, http.StatusNotFound, "not_found", "资源不存在")
	case errors.Is(err, controlplane.ErrUserNotFound):
		writeAdminError(c, http.StatusNotFound, "not_found", "资源不存在")
	case errors.Is(err, controlplane.ErrMembershipNotFound):
		writeAdminError(c, http.StatusNotFound, "not_found", "资源不存在")
	default:
		writeAdminError(c, http.StatusInternalServerError, "internal_error", "服务暂不可用")
	}
}

// parseUintParam 解析路径参数为 uint；非法时写 400 并返回 false。
func parseUintParam(c *gin.Context, name string) (uint, bool) {
	value, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || value == 0 {
		writeAdminError(c, http.StatusBadRequest, "invalid_request", "路径参数不合法")
		return 0, false
	}
	return uint(value), true
}
