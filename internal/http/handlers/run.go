package handlers

import (
	"net/http"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"

	"github.com/gin-gonic/gin"
)

type RunHandler struct {
	orchestrator *orchestrator.Orchestrator
}

func NewRunHandler(orch *orchestrator.Orchestrator) *RunHandler {
	return &RunHandler{orchestrator: orch}
}

type RunResponse struct {
	RunID          string          `json:"run_id"`
	TaskID         string          `json:"task_id"`
	WorkerRole     string          `json:"worker_role"`
	Status         model.RunStatus `json:"status"`
	ExternalSystem string          `json:"external_system,omitempty"`
	ExternalID     string          `json:"external_id,omitempty"`
	ExternalURL    string          `json:"external_url,omitempty"`
	InputJSON      string          `json:"input_json,omitempty"`
	OutputJSON     string          `json:"output_json,omitempty"`
	ErrorMessage   string          `json:"error_message,omitempty"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func (h *RunHandler) Get(c *gin.Context) {
	if h == nil || h.orchestrator == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "orchestrator not configured"})
		return
	}

	run, err := h.orchestrator.GetRun(c.Request.Context(), c.Param("run_id"))
	if err != nil {
		c.JSON(statusFromError(err), gin.H{
			"error":   "get run failed",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, runResponse(run))
}

func runResponse(run *model.Run) RunResponse {
	return RunResponse{
		RunID:          run.RunID,
		TaskID:         run.TaskID,
		WorkerRole:     run.WorkerRole,
		Status:         run.Status,
		ExternalSystem: run.ExternalSystem,
		ExternalID:     run.ExternalID,
		ExternalURL:    run.ExternalURL,
		InputJSON:      run.InputJSON,
		OutputJSON:     run.OutputJSON,
		ErrorMessage:   run.ErrorMessage,
		StartedAt:      run.StartedAt,
		CompletedAt:    run.CompletedAt,
		CreatedAt:      run.CreatedAt,
		UpdatedAt:      run.UpdatedAt,
	}
}
