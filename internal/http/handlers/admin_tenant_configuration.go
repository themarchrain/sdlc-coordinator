package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"sdlc-coordinator/internal/orgconfig"

	"github.com/gin-gonic/gin"
)

// AdminTenantConfigurationHandler 服务组织段运行配置：按会话组织作用域取
// 租户库读写，保存即热生效（无 restart 端点）。
type AdminTenantConfigurationHandler struct{}

func NewAdminTenantConfigurationHandler() *AdminTenantConfigurationHandler {
	return &AdminTenantConfigurationHandler{}
}

func (handler *AdminTenantConfigurationHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/tenant-configuration", handler.Get)
	group.POST("/tenant-configuration/validate", handler.Validate)
	group.PUT("/tenant-configuration", handler.Update)
}

func (handler *AdminTenantConfigurationHandler) storeFrom(c *gin.Context) (*orgconfig.Store, bool) {
	store, ok := TenantStoreFrom(c)
	if !ok {
		writeAdminError(c, http.StatusForbidden, "tenant_scope_required", "请先选择组织")
		return nil, false
	}
	return orgconfig.NewStore(store.DB), true
}

func (handler *AdminTenantConfigurationHandler) Get(c *gin.Context) {
	store, ok := handler.storeFrom(c)
	if !ok {
		return
	}
	config, revision, configured, err := store.Load(c.Request.Context())
	if err != nil {
		writeAdminError(c, http.StatusInternalServerError, "internal_error", "读取组织配置失败")
		return
	}
	view := orgconfig.Mask(config)
	view.Revision, view.Configured = revision, configured
	c.JSON(http.StatusOK, view)
}

func (handler *AdminTenantConfigurationHandler) Validate(c *gin.Context) {
	request, ok := decodeTenantConfigurationUpdate(c)
	if !ok {
		return
	}
	store, ok := handler.storeFrom(c)
	if !ok {
		return
	}
	current, _, _, err := store.Load(c.Request.Context())
	if err != nil {
		writeAdminError(c, http.StatusInternalServerError, "internal_error", "读取组织配置失败")
		return
	}
	errs := orgconfig.Validate(orgconfig.Merge(current, request))
	c.JSON(http.StatusOK, gin.H{
		"valid":        len(errs) == 0,
		"field_errors": errs,
	})
}

func (handler *AdminTenantConfigurationHandler) Update(c *gin.Context) {
	request, ok := decodeTenantConfigurationUpdate(c)
	if !ok {
		return
	}
	store, ok := handler.storeFrom(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	current, _, _, err := store.Load(ctx)
	if err != nil {
		writeAdminError(c, http.StatusInternalServerError, "internal_error", "读取组织配置失败")
		return
	}
	candidate := orgconfig.Merge(current, request)
	if errs := orgconfig.Validate(candidate); len(errs) > 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": gin.H{
			"code": "validation_failed", "message": "组织配置校验失败", "request_id": "", "details": errs,
		}})
		return
	}
	if _, err := store.Save(ctx, candidate, request.ExpectedRevision); err != nil {
		if errors.Is(err, orgconfig.ErrConfigConflict) {
			writeAdminError(c, http.StatusConflict, "config_conflict", "配置已被他人更新，请刷新后重试")
			return
		}
		writeAdminError(c, http.StatusInternalServerError, "internal_error", "保存组织配置失败")
		return
	}
	handler.Get(c)
}

// tenantConfigMaxRequestBytes 限制组织配置保存/校验请求体积：合法提交
// 不超过几 KB；上限防的是超大集合载荷(mention map/白名单)自伤式占用
// 解码内存与 text 列存储。
const tenantConfigMaxRequestBytes = 1 << 20 // 1 MiB

func decodeTenantConfigurationUpdate(c *gin.Context) (orgconfig.UpdateRequest, bool) {
	var request orgconfig.UpdateRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, tenantConfigMaxRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeAdminError(c, http.StatusBadRequest, "invalid_request", "请求 JSON 不符合组织配置契约")
		return orgconfig.UpdateRequest{}, false
	}
	if err := ensureConfigurationJSONEOF(decoder); err != nil {
		writeAdminError(c, http.StatusBadRequest, "invalid_request", "请求只能包含一个 JSON 对象")
		return orgconfig.UpdateRequest{}, false
	}
	return request, true
}
