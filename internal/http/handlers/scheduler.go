package handlers

import (
	"net/http"

	"sdlc-coordinator/internal/orchestrator"

	"github.com/gin-gonic/gin"
)

type SchedulerHandler struct {
	orchestrator *orchestrator.Orchestrator
}

func NewSchedulerHandler(orch *orchestrator.Orchestrator) *SchedulerHandler {
	return &SchedulerHandler{orchestrator: orch}
}

func (h *SchedulerHandler) Tick(c *gin.Context) {
	if h == nil || h.orchestrator == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "orchestrator not configured"})
		return
	}

	var req orchestrator.TickRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid scheduler tick payload",
			"details": err.Error(),
		})
		return
	}

	result, err := h.orchestrator.Tick(c.Request.Context(), req)
	if err != nil {
		c.JSON(statusFromError(err), gin.H{
			"error":   "scheduler tick failed",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}
