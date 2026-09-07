package handlers

import (
	"net/http"

	"sdlc-coordinator/internal/configuration"

	"github.com/gin-gonic/gin"
)

type ConfigurationStatusProvider interface {
	Status() configuration.StatusView
}

type HealthHandler struct {
	configuration ConfigurationStatusProvider
}

func NewHealthHandler(provider ConfigurationStatusProvider) *HealthHandler {
	return &HealthHandler{configuration: provider}
}

func Health(c *gin.Context) {
	NewHealthHandler(nil).Health(c)
}

func (handler *HealthHandler) Health(c *gin.Context) {
	if handler.configuration == nil {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
		return
	}
	status := handler.status()
	c.JSON(http.StatusOK, gin.H{
		"status":              "ok",
		"mode":                status.Mode,
		"revision":            status.Revision,
		"source":              status.Source,
		"applied_at":          status.AppliedAt,
		"configuration_error": status.ConfigurationError,
	})
}

func (handler *HealthHandler) Ready(c *gin.Context) {
	status := handler.status()
	if status.Mode != configuration.ModeReady {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
			"code": "configuration_required", "message": "Coordinator 运行配置尚未生效", "request_id": "", "details": gin.H{"mode": status.Mode},
		}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready", "mode": status.Mode, "revision": status.Revision})
}

func (handler *HealthHandler) status() configuration.StatusView {
	if handler.configuration == nil {
		return configuration.StatusView{Mode: configuration.ModeReady}
	}
	return handler.configuration.Status()
}
