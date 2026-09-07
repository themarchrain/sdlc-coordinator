package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/store/repository"

	"github.com/gin-gonic/gin"
)

type TaskHandler struct {
	orchestrator *orchestrator.Orchestrator
}

func NewTaskHandler(orch *orchestrator.Orchestrator) *TaskHandler {
	return &TaskHandler{orchestrator: orch}
}

type CreateTaskRequest struct {
	DedupeKey          string         `json:"dedupe_key" binding:"required"`
	Type               string         `json:"type" binding:"required"`
	Role               string         `json:"role" binding:"required"`
	SourceRepo         string         `json:"source_repo,omitempty"`
	TargetRepo         string         `json:"target_repo,omitempty"`
	Objective          string         `json:"objective" binding:"required"`
	AcceptanceCriteria string         `json:"acceptance_criteria,omitempty"`
	Context            map[string]any `json:"context,omitempty"`
	CallbackURL        string         `json:"callback_url,omitempty"`
}

type TaskResponse struct {
	TaskID             string           `json:"task_id"`
	DedupeKey          string           `json:"dedupe_key"`
	Type               string           `json:"type"`
	Role               string           `json:"role"`
	SourceRepo         string           `json:"source_repo,omitempty"`
	TargetRepo         string           `json:"target_repo,omitempty"`
	Objective          string           `json:"objective"`
	AcceptanceCriteria string           `json:"acceptance_criteria,omitempty"`
	Status             model.TaskStatus `json:"status"`
	LeaseUntil         *time.Time       `json:"lease_until,omitempty"`
	AttemptCount       int              `json:"attempt_count"`
	CurrentRunID       *string          `json:"current_run_id,omitempty"`
	ContextJSON        string           `json:"context_json,omitempty"`
	CallbackURL        string           `json:"callback_url,omitempty"`
	LastError          string           `json:"last_error,omitempty"`
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
}

func (h *TaskHandler) Create(c *gin.Context) {
	if h == nil || h.orchestrator == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "orchestrator not configured"})
		return
	}

	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid task payload",
			"details": err.Error(),
		})
		return
	}

	task := &model.Task{
		DedupeKey:          req.DedupeKey,
		Type:               req.Type,
		Role:               req.Role,
		SourceRepo:         req.SourceRepo,
		TargetRepo:         req.TargetRepo,
		Objective:          req.Objective,
		AcceptanceCriteria: req.AcceptanceCriteria,
		CallbackURL:        req.CallbackURL,
	}
	if len(req.Context) > 0 {
		if raw, err := json.Marshal(req.Context); err == nil {
			task.ContextJSON = string(raw)
		}
	}

	created, err := h.orchestrator.CreateTask(c.Request.Context(), task)
	if err != nil {
		c.JSON(statusFromError(err), gin.H{
			"error":   "create task failed",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, taskResponse(created))
}

func (h *TaskHandler) Get(c *gin.Context) {
	if h == nil || h.orchestrator == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "orchestrator not configured"})
		return
	}

	task, err := h.orchestrator.GetTask(c.Request.Context(), c.Param("task_id"))
	if err != nil {
		c.JSON(statusFromError(err), gin.H{
			"error":   "get task failed",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, taskResponse(task))
}

func taskResponse(task *model.Task) TaskResponse {
	return TaskResponse{
		TaskID:             task.TaskID,
		DedupeKey:          task.DedupeKey,
		Type:               task.Type,
		Role:               task.Role,
		SourceRepo:         task.SourceRepo,
		TargetRepo:         task.TargetRepo,
		Objective:          task.Objective,
		AcceptanceCriteria: task.AcceptanceCriteria,
		Status:             task.Status,
		LeaseUntil:         task.LeaseUntil,
		AttemptCount:       task.AttemptCount,
		CurrentRunID:       task.CurrentRunID,
		ContextJSON:        task.ContextJSON,
		CallbackURL:        task.CallbackURL,
		LastError:          task.LastError,
		CreatedAt:          task.CreatedAt,
		UpdatedAt:          task.UpdatedAt,
	}
}

func statusFromError(err error) int {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, repository.ErrTaskNotClaimable),
		errors.Is(err, orchestrator.ErrInvalidTransition),
		errors.Is(err, orchestrator.ErrTickInProgress):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
