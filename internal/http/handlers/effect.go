package handlers

import (
	"errors"
	"net/http"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"

	"github.com/gin-gonic/gin"
)

type EffectHandler struct {
	orchestrator *orchestrator.Orchestrator
}

type RetryEffectRequest struct {
	DedupeKey string `json:"dedupe_key" binding:"required"`
}

type EffectResponse struct {
	EffectID     string             `json:"effect_id"`
	TaskID       string             `json:"task_id"`
	RunID        string             `json:"run_id"`
	DedupeKey    string             `json:"dedupe_key"`
	Kind         string             `json:"kind"`
	Target       string             `json:"target"`
	Critical     bool               `json:"critical"`
	Status       model.EffectStatus `json:"status"`
	AttemptCount int                `json:"attempt_count"`
	RequestJSON  string             `json:"request_json,omitempty"`
	ResponseJSON string             `json:"response_json,omitempty"`
	ErrorMessage string             `json:"error_message,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

func NewEffectHandler(orch *orchestrator.Orchestrator) *EffectHandler {
	return &EffectHandler{orchestrator: orch}
}

func (h *EffectHandler) Retry(c *gin.Context) {
	if h == nil || h.orchestrator == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "orchestrator not configured"})
		return
	}

	var req RetryEffectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid effect retry payload",
			"details": err.Error(),
		})
		return
	}

	effect, err := h.orchestrator.RetryEffect(c.Request.Context(), req.DedupeKey)
	if err != nil {
		status := statusFromError(err)
		if errors.Is(err, orchestrator.ErrEffectNotRetryable) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{
			"error":   "effect retry failed",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusAccepted, effectResponse(effect))
}

func effectResponse(effect *model.Effect) EffectResponse {
	return EffectResponse{
		EffectID:     effect.EffectID,
		TaskID:       effect.TaskID,
		RunID:        effect.RunID,
		DedupeKey:    effect.DedupeKey,
		Kind:         effect.Kind,
		Target:       effect.Target,
		Critical:     effect.Critical,
		Status:       effect.Status,
		AttemptCount: effect.AttemptCount,
		RequestJSON:  effect.RequestJSON,
		ResponseJSON: effect.ResponseJSON,
		ErrorMessage: effect.ErrorMessage,
		CreatedAt:    effect.CreatedAt,
		UpdatedAt:    effect.UpdatedAt,
	}
}
