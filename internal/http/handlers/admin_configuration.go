package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"sdlc-coordinator/internal/configuration"

	"github.com/gin-gonic/gin"
)

const defaultConfigurationRestartDelay = 150 * time.Millisecond

type AdminConfigurationHandler struct {
	service      *configuration.Service
	shutdown     configuration.ShutdownRequester
	restartDelay time.Duration
}

func NewAdminConfigurationHandler(service *configuration.Service, shutdown configuration.ShutdownRequester) *AdminConfigurationHandler {
	return &AdminConfigurationHandler{service: service, shutdown: shutdown, restartDelay: defaultConfigurationRestartDelay}
}

func (handler *AdminConfigurationHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/configuration/status", handler.Status)
	group.GET("/configuration", handler.Get)
	group.POST("/configuration/validate", handler.Validate)
	group.PUT("/configuration", handler.Update)
	group.POST("/configuration/restart", handler.Restart)
}

func (handler *AdminConfigurationHandler) Status(c *gin.Context) {
	c.JSON(http.StatusOK, handler.service.Status())
}

func (handler *AdminConfigurationHandler) Get(c *gin.Context) {
	c.JSON(http.StatusOK, handler.service.Get())
}

func (handler *AdminConfigurationHandler) Validate(c *gin.Context) {
	request, ok := decodeConfigurationUpdate(c)
	if !ok {
		return
	}
	validationErrors := handler.service.Validate(request)
	c.JSON(http.StatusOK, gin.H{
		"valid":        len(validationErrors) == 0,
		"field_errors": validationErrors,
	})
}

func (handler *AdminConfigurationHandler) Update(c *gin.Context) {
	request, ok := decodeConfigurationUpdate(c)
	if !ok {
		return
	}
	result, validationErrors, err := handler.service.Update(c.Request.Context(), request)
	if err != nil {
		if errors.Is(err, configuration.ErrRevisionConflict) {
			writeAdminError(c, http.StatusConflict, "revision_conflict", "运行配置已被其他管理员更新，请刷新页面后重新提交")
			return
		}
		if errors.Is(err, configuration.ErrExpectedRevisionRequired) {
			writeAdminError(c, http.StatusBadRequest, "invalid_request", "请求缺少 expected_revision")
			return
		}
		writeAdminError(c, http.StatusInternalServerError, "internal_error", "保存运行配置失败")
		return
	}
	if len(validationErrors) > 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": gin.H{
			"code": "validation_failed", "message": "运行配置校验失败", "request_id": "", "details": validationErrors,
		}})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (handler *AdminConfigurationHandler) Restart(c *gin.Context) {
	if handler.shutdown == nil {
		writeAdminError(c, http.StatusServiceUnavailable, "restart_unavailable", "当前运行方式不支持应用内重启")
		return
	}
	if handler.service.Status().Mode != configuration.ModeRestartRequired {
		newer, err := handler.service.HasNewerRevision(c.Request.Context())
		if err != nil {
			writeAdminError(c, http.StatusInternalServerError, "internal_error", "读取运行配置版本失败")
			return
		}
		if !newer {
			writeAdminError(c, http.StatusConflict, "restart_not_required", "当前没有等待生效的配置")
			return
		}
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "shutdown_scheduled"})
	delay := handler.restartDelay
	go func() {
		if delay > 0 {
			time.Sleep(delay)
		}
		handler.shutdown.RequestShutdown()
	}()
}

func decodeConfigurationUpdate(c *gin.Context) (configuration.UpdateRequest, bool) {
	var request configuration.UpdateRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeAdminError(c, http.StatusBadRequest, "invalid_request", "请求 JSON 不符合配置契约")
		return configuration.UpdateRequest{}, false
	}
	if err := ensureConfigurationJSONEOF(decoder); err != nil {
		writeAdminError(c, http.StatusBadRequest, "invalid_request", "请求只能包含一个 JSON 对象")
		return configuration.UpdateRequest{}, false
	}
	return request, true
}

func ensureConfigurationJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values")
	}
	return err
}
