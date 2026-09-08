package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"sdlc-coordinator/internal/admin"
	"sdlc-coordinator/internal/connectors"
	"sdlc-coordinator/internal/model"
	storerepository "sdlc-coordinator/internal/store/repository"

	"github.com/gin-gonic/gin"
)

type AdminConnectorHandler struct {
	service *connectors.Service
}

type createConnectorBody struct {
	Kind              string `json:"kind"`
	DisplayName       string `json:"display_name"`
	BaseURL           string `json:"base_url"`
	Enabled           *bool  `json:"enabled"`
	TargetProjectPath string `json:"target_project_path"`
}

type updateConnectorBody struct {
	Kind              *string `json:"kind"`
	DisplayName       *string `json:"display_name"`
	BaseURL           *string `json:"base_url"`
	Enabled           *bool   `json:"enabled"`
	TargetProjectPath *string `json:"target_project_path"`
}

func NewAdminConnectorHandler(service *connectors.Service) *AdminConnectorHandler {
	return &AdminConnectorHandler{service: service}
}

func (handler *AdminConnectorHandler) connectorsFor(c *gin.Context) *connectors.Service {
	if store, ok := TenantStoreFrom(c); ok && handler.service != nil {
		return handler.service.WithDB(store.DB)
	}
	return handler.service
}

func (handler *AdminConnectorHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/connectors", handler.List)
	group.POST("/connectors", handler.Create)
	group.GET("/connectors/:connector_id", handler.Get)
	group.PATCH("/connectors/:connector_id", handler.Update)
	group.DELETE("/connectors/:connector_id", handler.Delete)
	group.POST("/connectors/:connector_id/probe", handler.Probe)
	group.POST("/connectors/:connector_id/runs", handler.TriggerRun)
	group.GET("/connectors/:connector_id/runs", handler.ListRuns)
	group.GET("/connectors/:connector_id/runs/:run_id", handler.GetRun)
	group.GET("/connectors/:connector_id/records", handler.ListRecords)
	group.GET("/connectors/:connector_id/records/:record_id", handler.GetRecord)
	group.GET("/connectors/:connector_id/metrics", handler.Metrics)
	group.GET("/connectors/:connector_id/configuration", handler.Configuration)
	group.PUT("/connectors/:connector_id/configuration", handler.SaveConfiguration)
}

func (handler *AdminConnectorHandler) Create(c *gin.Context) {
	var body createConnectorBody
	if !decodeAdminConnectorBody(c, &body) {
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	connector, err := handler.connectorsFor(c).Create(c.Request.Context(), connectors.CreateConnectorRequest{
		Kind: body.Kind, DisplayName: body.DisplayName, BaseURL: body.BaseURL,
		Enabled: enabled, TargetProjectPath: body.TargetProjectPath,
	})
	if err != nil {
		writeAdminConnectorError(c, err)
		return
	}
	c.JSON(http.StatusCreated, connectorResponse(*connector))
}

func (handler *AdminConnectorHandler) List(c *gin.Context) {
	pageRequest, ok := parseAdminPageRequest(c)
	if !ok {
		return
	}
	enabled, ok := parseOptionalAdminBool(c, "enabled")
	if !ok {
		return
	}
	page, err := handler.connectorsFor(c).List(c.Request.Context(), connectors.ConnectorFilter{
		PageRequest: pageRequestToConnector(pageRequest), Kind: c.Query("kind"), Enabled: enabled,
		TargetProjectPath: c.Query("target_project_path"),
	})
	if err != nil {
		writeAdminConnectorError(c, err)
		return
	}
	items := make([]gin.H, 0, len(page.Items))
	for _, connector := range page.Items {
		items = append(items, connectorResponse(connector))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "page": page.Page, "page_size": page.PageSize, "total": page.Total})
}

func (handler *AdminConnectorHandler) Get(c *gin.Context) {
	connector, err := handler.connectorsFor(c).Get(c.Request.Context(), c.Param("connector_id"))
	if err != nil {
		writeAdminConnectorError(c, err)
		return
	}
	c.JSON(http.StatusOK, connectorResponse(*connector))
}

func (handler *AdminConnectorHandler) Update(c *gin.Context) {
	var body updateConnectorBody
	if !decodeAdminConnectorBody(c, &body) {
		return
	}
	connector, err := handler.connectorsFor(c).Update(c.Request.Context(), c.Param("connector_id"), connectors.UpdateConnectorRequest{
		Kind: body.Kind, DisplayName: body.DisplayName, BaseURL: body.BaseURL,
		Enabled: body.Enabled, TargetProjectPath: body.TargetProjectPath,
	})
	if err != nil {
		writeAdminConnectorError(c, err)
		return
	}
	c.JSON(http.StatusOK, connectorResponse(*connector))
}

func (handler *AdminConnectorHandler) Delete(c *gin.Context) {
	if err := handler.connectorsFor(c).Delete(c.Request.Context(), c.Param("connector_id")); err != nil {
		writeAdminConnectorError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (handler *AdminConnectorHandler) Probe(c *gin.Context) {
	result, err := handler.connectorsFor(c).Probe(c.Request.Context(), c.Param("connector_id"))
	if err != nil {
		writeAdminConnectorError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"connector": connectorResponse(result.Connector), "manifest": result.Manifest, "health": result.Health,
	})
}

func (handler *AdminConnectorHandler) TriggerRun(c *gin.Context) {
	result, err := handler.connectorsFor(c).TriggerRun(c.Request.Context(), c.Param("connector_id"), "")
	if err != nil {
		writeAdminConnectorError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"request_id": result.RequestID, "run": result.Run})
}

func (handler *AdminConnectorHandler) ListRuns(c *gin.Context) {
	page, ok := parseConnectorRemotePage(c)
	if !ok {
		return
	}
	result, err := handler.connectorsFor(c).ListRuns(c.Request.Context(), c.Param("connector_id"), page)
	if err != nil {
		writeAdminConnectorError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (handler *AdminConnectorHandler) GetRun(c *gin.Context) {
	result, err := handler.connectorsFor(c).GetRun(c.Request.Context(), c.Param("connector_id"), c.Param("run_id"))
	if err != nil {
		writeAdminConnectorError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (handler *AdminConnectorHandler) ListRecords(c *gin.Context) {
	page, ok := parseConnectorRemotePage(c)
	if !ok {
		return
	}
	result, err := handler.connectorsFor(c).ListRecords(c.Request.Context(), c.Param("connector_id"), page)
	if err != nil {
		writeAdminConnectorError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (handler *AdminConnectorHandler) GetRecord(c *gin.Context) {
	result, err := handler.connectorsFor(c).GetRecord(c.Request.Context(), c.Param("connector_id"), c.Param("record_id"))
	if err != nil {
		writeAdminConnectorError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (handler *AdminConnectorHandler) Metrics(c *gin.Context) {
	result, err := handler.connectorsFor(c).Metrics(c.Request.Context(), c.Param("connector_id"), "")
	if err != nil {
		writeAdminConnectorError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (handler *AdminConnectorHandler) Configuration(c *gin.Context) {
	result, err := handler.connectorsFor(c).Configuration(c.Request.Context(), c.Param("connector_id"))
	if err != nil {
		writeAdminConnectorError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (handler *AdminConnectorHandler) SaveConfiguration(c *gin.Context) {
	var body connectors.SaveConfigurationRequest
	if !decodeAdminConnectorBody(c, &body) {
		return
	}
	result, err := handler.connectorsFor(c).SaveConfiguration(c.Request.Context(), c.Param("connector_id"), body)
	if err != nil {
		writeAdminConnectorError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func decodeAdminConnectorBody(c *gin.Context, target any) bool {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeAdminError(c, http.StatusBadRequest, "invalid_body", "请求 JSON 非法")
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeAdminError(c, http.StatusBadRequest, "invalid_body", "请求只能包含一个 JSON 对象")
		return false
	}
	return true
}

func parseOptionalAdminBool(c *gin.Context, name string) (*bool, bool) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, true
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		writeAdminError(c, http.StatusBadRequest, "invalid_query", name+" 必须是布尔值")
		return nil, false
	}
	return &value, true
}

func parseConnectorRemotePage(c *gin.Context) (connectors.PageRequest, bool) {
	page, ok := parseAdminPageRequest(c)
	if !ok {
		return connectors.PageRequest{}, false
	}
	return pageRequestToConnector(page), true
}

func pageRequestToConnector(page admin.PageRequest) connectors.PageRequest {
	return connectors.PageRequest{Page: page.Page, PageSize: page.PageSize}
}

func connectorResponse(connector model.ConnectorInstance) gin.H {
	capabilities := make([]string, 0)
	_ = json.Unmarshal([]byte(connector.CapabilitiesJSON), &capabilities)
	return gin.H{
		"connector_id": connector.ConnectorID, "kind": connector.Kind, "display_name": connector.DisplayName,
		"base_url": connector.BaseURL, "enabled": connector.Enabled, "target_project_path": connector.TargetProjectPath,
		"capabilities": capabilities, "last_health_status": connector.LastHealthStatus,
		"last_health_message": connector.LastHealthMessage, "last_health_at": connector.LastHealthAt,
		"created_at": connector.CreatedAt, "updated_at": connector.UpdatedAt,
	}
}

func writeAdminConnectorError(c *gin.Context, err error) {
	var remoteError *connectors.RemoteError
	switch {
	case errors.Is(err, storerepository.ErrNotFound):
		writeAdminError(c, http.StatusNotFound, "not_found", "Connector 不存在")
	case errors.Is(err, connectors.ErrInvalidConnector):
		writeAdminError(c, http.StatusBadRequest, "invalid_connector", err.Error())
	case errors.Is(err, connectors.ErrConnectorDisabled):
		writeAdminError(c, http.StatusConflict, "connector_disabled", "Connector 已禁用")
	case errors.Is(err, connectors.ErrConnectorChanged):
		writeAdminError(c, http.StatusConflict, "connector_changed", "Connector 配置已变化，请重新执行")
	case errors.Is(err, connectors.ErrConnectorContract):
		writeAdminError(c, http.StatusBadGateway, "connector_contract_error", err.Error())
	case errors.As(err, &remoteError) && remoteError.Kind == connectors.RemoteErrorTimeout:
		writeAdminError(c, http.StatusGatewayTimeout, "connector_timeout", "Connector 请求超时")
	case errors.As(err, &remoteError) && remoteError.Kind == connectors.RemoteErrorRemote && remoteError.StatusCode == http.StatusBadRequest:
		writeAdminError(c, http.StatusBadRequest, "invalid_connector_configuration", remoteError.Error())
	case errors.As(err, &remoteError):
		writeAdminError(c, http.StatusBadGateway, "connector_"+string(remoteError.Kind), "无法连接同步器 "+remoteError.Error())
	default:
		writeAdminError(c, http.StatusInternalServerError, "internal_error", "Connector 操作失败")
	}
}
