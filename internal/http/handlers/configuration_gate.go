package handlers

import (
	"net/http"

	"sdlc-coordinator/internal/configuration"

	"github.com/gin-gonic/gin"
)

func ConfigurationGate(startupMode configuration.Mode) gin.HandlerFunc {
	return func(c *gin.Context) {
		if startupMode == configuration.ModeReady {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
			"code":       "configuration_required",
			"message":    "Coordinator 尚未完成运行配置，当前入口不会推进状态机",
			"request_id": "",
			"details":    gin.H{"mode": startupMode},
		}})
	}
}
