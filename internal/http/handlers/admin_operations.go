package handlers

import (
	"context"
	"errors"
	"net/http"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/store/repository"

	"github.com/gin-gonic/gin"
)

type AdminOperations interface {
	Tick(context.Context, orchestrator.TickRequest) (*orchestrator.TickResult, error)
	RetryEffect(context.Context, string) (*model.Effect, error)
	RebindTask(context.Context, string, string) (*model.Task, error)
}

type AdminEffectLookup interface {
	GetEffect(context.Context, string) (*model.Effect, error)
}

type AdminOperationHandler struct {
	operations AdminOperations
	effects    AdminEffectLookup
	defaults   orchestrator.TickRequest
}

func NewAdminOperationHandler(operations AdminOperations, effects AdminEffectLookup, defaults orchestrator.TickRequest) *AdminOperationHandler {
	return &AdminOperationHandler{operations: operations, effects: effects, defaults: defaults}
}

func (handler *AdminOperationHandler) RegisterRoutes(group *gin.RouterGroup, middleware ...gin.HandlerFunc) {
	group.GET("/scheduler/defaults", handler.TickDefaults)
	group.POST("/scheduler/tick", append(middleware, handler.Tick)...)
	group.POST("/effects/:effect_id/retry", append(middleware, handler.RetryEffect)...)
	group.POST("/tasks/:task_id/rebind", append(middleware, handler.RebindTask)...)
}

func (handler *AdminOperationHandler) TickDefaults(c *gin.Context) {
	c.JSON(http.StatusOK, handler.defaults)
}

func (handler *AdminOperationHandler) Tick(c *gin.Context) {
	if handler.operations == nil {
		writeAdminError(c, http.StatusServiceUnavailable, "operations_unavailable", "Scheduler 操作不可用")
		return
	}
	var request orchestrator.TickRequest
	if !decodeAdminConnectorBody(c, &request) {
		return
	}
	result, err := handler.operations.Tick(c.Request.Context(), request)
	if err != nil {
		writeAdminError(c, statusFromError(err), "scheduler_tick_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (handler *AdminOperationHandler) RetryEffect(c *gin.Context) {
	if handler.operations == nil || handler.effects == nil {
		writeAdminError(c, http.StatusServiceUnavailable, "operations_unavailable", "Effect 重试操作不可用")
		return
	}
	effect, err := handler.effects.GetEffect(c.Request.Context(), c.Param("effect_id"))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeAdminError(c, http.StatusNotFound, "not_found", "Effect 不存在")
			return
		}
		writeAdminError(c, http.StatusInternalServerError, "internal_error", "查询 Effect 失败")
		return
	}
	retried, err := handler.operations.RetryEffect(c.Request.Context(), effect.DedupeKey)
	if err != nil {
		if errors.Is(err, orchestrator.ErrEffectNotRetryable) {
			writeAdminError(c, http.StatusConflict, "effect_not_retryable", "Effect 当前状态不可重试")
			return
		}
		writeAdminError(c, statusFromError(err), "effect_retry_failed", err.Error())
		return
	}
	c.JSON(http.StatusAccepted, effectResponse(retried))
}

type RebindTaskRequest struct {
	RepositoryID string `json:"repository_id" binding:"required"`
}

func (handler *AdminOperationHandler) RebindTask(c *gin.Context) {
	if handler.operations == nil {
		writeAdminError(c, http.StatusServiceUnavailable, "operations_unavailable", "Scheduler 操作不可用")
		return
	}
	var request RebindTaskRequest
	if !decodeAdminConnectorBody(c, &request) {
		return
	}
	task, err := handler.operations.RebindTask(c.Request.Context(), c.Param("task_id"), request.RepositoryID)
	if err != nil {
		writeAdminError(c, statusFromError(err), "task_rebind_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, taskResponse(task))
}
