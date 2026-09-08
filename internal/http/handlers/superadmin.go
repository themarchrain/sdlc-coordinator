package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"sdlc-coordinator/internal/controlplane"

	"github.com/gin-gonic/gin"
)

// SuperAdminHandler 承载超管独立路由命名空间 /api/super-admin/*：
// 组织 CRUD、管理员开通、跨组织邀请码管理、审计查看、带审计支持访问与
// runner 静态指派。超管默认看不到任何组织业务数据。
type SuperAdminHandler struct{ service *controlplane.Service }

func NewSuperAdminHandler(service *controlplane.Service) *SuperAdminHandler {
	return &SuperAdminHandler{service: service}
}

// RegisterRoutes 挂载超管端点；中间件（RequirePortalAuth + RequireSuperAdmin）
// 由路由装配层挂在外层 group 上。
func (handler *SuperAdminHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/tenants", handler.ListTenants)
	group.POST("/tenants", handler.CreateTenant)
	group.PATCH("/tenants/:id", handler.UpdateTenant)
	group.POST("/tenants/:id/disable", handler.DisableTenant)
	group.POST("/tenants/:id/restore", handler.RestoreTenant)
	group.GET("/tenants/:id/members", handler.ListTenantMembers)
	group.POST("/tenants/:id/admins", handler.ProvisionTenantAdmin)
	group.POST("/tenants/:id/admins/:user_id/reset-password", handler.ResetTenantAdminPassword)
	group.GET("/tenants/:id/invite-codes", handler.ListTenantInviteCodes)
	group.DELETE("/tenants/:id/invite-codes/:code_id", handler.RevokeTenantInviteCode)
	group.GET("/tenants/:id/runner-assignments", handler.ListRunnerAssignments)
	group.PUT("/tenants/:id/runner-assignments", handler.ReplaceRunnerAssignments)
	group.POST("/tenants/:id/support-access", handler.EnterSupportAccess)
	group.DELETE("/support-access", handler.ExitSupportAccess)
	group.GET("/audit-log", handler.ListAuditLog)
}

type superTenantBody struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
}

func (handler *SuperAdminHandler) ListTenants(c *gin.Context) {
	tenants, err := handler.service.ListTenants(c.Request.Context())
	if err != nil {
		writeControlPlaneError(c, err)
		return
	}
	views := make([]gin.H, 0, len(tenants))
	for i := range tenants {
		views = append(views, tenantSummaryView(&tenants[i]))
	}
	c.JSON(http.StatusOK, gin.H{"tenants": views})
}

func (handler *SuperAdminHandler) CreateTenant(c *gin.Context) {
	var body superTenantBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeAdminError(c, http.StatusBadRequest, "invalid_request", "请填写组织标识和名称")
		return
	}
	tenant, err := handler.service.CreateTenant(c.Request.Context(), currentActor(c), controlplane.TenantInput{
		Slug: body.Slug, DisplayName: body.DisplayName,
	})
	if err != nil {
		writeControlPlaneError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tenant": tenantView(tenant)})
}

func (handler *SuperAdminHandler) UpdateTenant(c *gin.Context) {
	tenantID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var body superTenantBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeAdminError(c, http.StatusBadRequest, "invalid_request", "请填写组织名称")
		return
	}
	tenant, err := handler.service.UpdateTenant(c.Request.Context(), currentActor(c), tenantID, body.DisplayName)
	if err != nil {
		writeControlPlaneError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tenant": tenantView(tenant)})
}

func (handler *SuperAdminHandler) DisableTenant(c *gin.Context) {
	tenantID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	tenant, err := handler.service.DisableTenant(c.Request.Context(), currentActor(c), tenantID)
	if err != nil {
		writeControlPlaneError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tenant": tenantView(tenant)})
}

func (handler *SuperAdminHandler) RestoreTenant(c *gin.Context) {
	tenantID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	tenant, err := handler.service.RestoreTenant(c.Request.Context(), currentActor(c), tenantID)
	if err != nil {
		writeControlPlaneError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tenant": tenantView(tenant)})
}

func (handler *SuperAdminHandler) ListTenantMembers(c *gin.Context) {
	tenantID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	members, err := handler.service.ListMembers(c.Request.Context(), tenantID)
	if err != nil {
		writeControlPlaneError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"members": members})
}

type superProvisionAdminBody struct {
	Username string `json:"username"`
}

func (handler *SuperAdminHandler) ProvisionTenantAdmin(c *gin.Context) {
	tenantID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var body superProvisionAdminBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeAdminError(c, http.StatusBadRequest, "invalid_request", "请填写用户名")
		return
	}
	provisioned, err := handler.service.ProvisionTenantAdmin(c.Request.Context(), currentActor(c), tenantID, body.Username)
	if err != nil {
		writeControlPlaneError(c, err)
		return
	}
	payload := gin.H{
		"user_id":  provisioned.User.ID,
		"username": provisioned.User.Username,
	}
	if provisioned.InitialPassword != "" {
		payload["initial_password"] = provisioned.InitialPassword
	}
	c.JSON(http.StatusOK, payload)
}

func (handler *SuperAdminHandler) ResetTenantAdminPassword(c *gin.Context) {
	tenantID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	userID, ok := parseUintParam(c, "user_id")
	if !ok {
		return
	}
	initial, err := handler.service.ResetTenantAdminPassword(c.Request.Context(), currentActor(c), tenantID, userID)
	if err != nil {
		writeControlPlaneError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"initial_password": initial})
}

func (handler *SuperAdminHandler) ListTenantInviteCodes(c *gin.Context) {
	tenantID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	codes, err := handler.service.ListTenantInviteCodes(c.Request.Context(), tenantID)
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

func (handler *SuperAdminHandler) RevokeTenantInviteCode(c *gin.Context) {
	tenantID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	codeID, ok := parseUintParam(c, "code_id")
	if !ok {
		return
	}
	if err := handler.service.RevokeInviteCode(c.Request.Context(), currentActor(c), tenantID, codeID); err != nil {
		writeControlPlaneError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (handler *SuperAdminHandler) ListRunnerAssignments(c *gin.Context) {
	tenantID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	assignments, err := handler.service.ListRunnerAssignments(c.Request.Context(), tenantID)
	if err != nil {
		writeControlPlaneError(c, err)
		return
	}
	views := make([]gin.H, 0, len(assignments))
	for i := range assignments {
		views = append(views, gin.H{
			"id":           assignments[i].ID,
			"endpoint_url": assignments[i].EndpointURL,
			"created_at":   assignments[i].CreatedAt.UTC().Format(timeRFC3339),
		})
	}
	c.JSON(http.StatusOK, gin.H{"runner_assignments": views})
}

type superRunnerAssignmentsBody struct {
	Endpoints []string `json:"endpoints"`
}

func (handler *SuperAdminHandler) ReplaceRunnerAssignments(c *gin.Context) {
	tenantID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var body superRunnerAssignmentsBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeAdminError(c, http.StatusBadRequest, "invalid_request", "请提供 runner 端点列表")
		return
	}
	if err := handler.service.ReplaceRunnerAssignments(c.Request.Context(), currentActor(c), tenantID, body.Endpoints); err != nil {
		writeControlPlaneError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type superSupportAccessBody struct {
	Reason string `json:"reason"`
}

func (handler *SuperAdminHandler) EnterSupportAccess(c *gin.Context) {
	tenantID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var body superSupportAccessBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeAdminError(c, http.StatusBadRequest, "reason_required", "请填写支持访问理由")
		return
	}
	if err := handler.service.EnterSupportAccess(c.Request.Context(), portalCookieToken(c), tenantID, body.Reason); err != nil {
		writeControlPlaneError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (handler *SuperAdminHandler) ExitSupportAccess(c *gin.Context) {
	if err := handler.service.ExitSupportAccess(c.Request.Context(), portalCookieToken(c)); err != nil {
		writeControlPlaneError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (handler *SuperAdminHandler) ListAuditLog(c *gin.Context) {
	filter := controlplane.AuditFilter{Action: strings.TrimSpace(c.Query("action"))}
	if raw := strings.TrimSpace(c.Query("tenant_id")); raw != "" {
		tenantID, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || tenantID == 0 {
			writeAdminError(c, http.StatusBadRequest, "invalid_request", "tenant_id 不合法")
			return
		}
		id := uint(tenantID)
		filter.TenantID = &id
	}
	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		if page, err := strconv.Atoi(raw); err == nil {
			filter.Page = page
		}
	}
	if raw := strings.TrimSpace(c.Query("page_size")); raw != "" {
		if pageSize, err := strconv.Atoi(raw); err == nil {
			filter.PageSize = pageSize
		}
	}
	logs, total, err := handler.service.ListAuditLogs(c.Request.Context(), filter)
	if err != nil {
		writeControlPlaneError(c, err)
		return
	}
	items := make([]gin.H, 0, len(logs))
	for i := range logs {
		items = append(items, gin.H{
			"id":             logs[i].ID,
			"actor_user_id":  logs[i].ActorUserID,
			"actor_username": logs[i].ActorUsername,
			"tenant_id":      logs[i].TenantID,
			"action":         logs[i].Action,
			"detail":         logs[i].Detail,
			"created_at":     logs[i].CreatedAt.UTC().Format(timeRFC3339),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      filter.Page,
		"page_size": filter.PageSize,
	})
}

func tenantSummaryView(summary *controlplane.TenantSummary) gin.H {
	return gin.H{
		"id":           summary.ID,
		"slug":         summary.Slug,
		"display_name": summary.DisplayName,
		"status":       summary.Status,
		"member_count": summary.MemberCount,
		"created_at":   summary.CreatedAt.UTC().Format(timeRFC3339),
	}
}
