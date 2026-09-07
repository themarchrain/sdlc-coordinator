package handlers

import (
	"net/http"

	"sdlc-coordinator/internal/model"

	"github.com/gin-gonic/gin"
)

type ClaimTaskRequest struct {
	RunID string `json:"run_id,omitempty"`
}

func (h *TaskHandler) Claim(c *gin.Context) {
	if h == nil || h.orchestrator == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "orchestrator not configured"})
		return
	}

	var req ClaimTaskRequest
	if c.Request.Body != nil && c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid claim payload",
				"details": err.Error(),
			})
			return
		}
	}

	runID := req.RunID
	if runID == "" {
		runID = model.NewID("run")
	}

	task, err := h.orchestrator.ClaimTask(c.Request.Context(), c.Param("task_id"), runID)
	if err != nil {
		c.JSON(statusFromError(err), gin.H{
			"error":   "claim task failed",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, taskResponse(task))
}
