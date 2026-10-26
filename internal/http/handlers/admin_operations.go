package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

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

// AdminOperationEngines 按请求会话的租户作用域选择操作引擎与 effect 查询；
// 注入后返回 nil 即「租户引擎不可用」（503），不得回落构造期默认——那会
// 把租户作用域操作写进平台库。未注入端口才用构造期默认（单库形态/测试）。
type AdminOperationEngines interface {
	OperationsFor(c *gin.Context) AdminOperations
	EffectsFor(c *gin.Context) AdminEffectLookup
}

type AdminOperationHandler struct {
	operations AdminOperations
	effects    AdminEffectLookup
	engines    AdminOperationEngines
	defaults   orchestrator.TickRequest
}

func NewAdminOperationHandler(operations AdminOperations, effects AdminEffectLookup, defaults orchestrator.TickRequest, engines ...AdminOperationEngines) *AdminOperationHandler {
	handler := &AdminOperationHandler{operations: operations, effects: effects, defaults: defaults}
	for _, engine := range engines {
		if engine != nil {
			handler.engines = engine
		}
	}
	return handler
}

// operationsFor 取本请求的操作引擎：注入引擎端口时 nil 即不可用（503）。
func (handler *AdminOperationHandler) operationsFor(c *gin.Context) AdminOperations {
	if handler.engines != nil {
		return handler.engines.OperationsFor(c)
	}
	return handler.operations
}

// effectsFor 取本请求的 effect 查询：注入引擎端口时 nil 即不可用（503）。
func (handler *AdminOperationHandler) effectsFor(c *gin.Context) AdminEffectLookup {
	if handler.engines != nil {
		return handler.engines.EffectsFor(c)
	}
	return handler.effects
}

func (handler *AdminOperationHandler) RegisterRoutes(group *gin.RouterGroup, middleware ...gin.HandlerFunc) {
	group.GET("/scheduler/defaults", handler.TickDefaults)
	group.POST("/scheduler/tick", append(middleware, handler.Tick)...)
	group.POST("/effects/:effect_id/retry", append(middleware, handler.RetryEffect)...)
	group.POST("/tasks/:task_id/rebind", append(middleware, handler.RebindTask)...)
	group.POST("/tasks/:task_id/stop", handler.stopTarget("task", "task_id"))
	group.POST("/runs/:run_id/stop", handler.stopTarget("run", "run_id"))
	group.POST("/agents/executions/executions/:execution_id/stop", handler.stopTarget("execution", "execution_id"))
}

func (handler *AdminOperationHandler) TickDefaults(c *gin.Context) {
	c.JSON(http.StatusOK, handler.defaults)
}

func (handler *AdminOperationHandler) Tick(c *gin.Context) {
	operations := handler.operationsFor(c)
	if operations == nil {
		writeAdminError(c, http.StatusServiceUnavailable, "operations_unavailable", "Scheduler 操作不可用")
		return
	}
	var request orchestrator.TickRequest
	if !decodeAdminConnectorBody(c, &request) {
		return
	}
	result, err := operations.Tick(c.Request.Context(), request)
	if err != nil {
		writeAdminError(c, statusFromError(err), "scheduler_tick_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func (handler *AdminOperationHandler) RetryEffect(c *gin.Context) {
	operations := handler.operationsFor(c)
	effects := handler.effectsFor(c)
	if operations == nil || effects == nil {
		writeAdminError(c, http.StatusServiceUnavailable, "operations_unavailable", "Effect 重试操作不可用")
		return
	}
	effect, err := effects.GetEffect(c.Request.Context(), c.Param("effect_id"))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeAdminError(c, http.StatusNotFound, "not_found", "Effect 不存在")
			return
		}
		writeAdminError(c, http.StatusInternalServerError, "internal_error", "查询 Effect 失败")
		return
	}
	retried, err := operations.RetryEffect(c.Request.Context(), effect.DedupeKey)
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
	operations := handler.operationsFor(c)
	if operations == nil {
		writeAdminError(c, http.StatusServiceUnavailable, "operations_unavailable", "Scheduler 操作不可用")
		return
	}
	var request RebindTaskRequest
	if !decodeAdminConnectorBody(c, &request) {
		return
	}
	task, err := operations.RebindTask(c.Request.Context(), c.Param("task_id"), request.RepositoryID)
	if err != nil {
		writeAdminError(c, statusFromError(err), "task_rebind_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, taskResponse(task))
}

// statusFromError 把仓库/编排错误映射到 HTTP 状态码。
func statusFromError(err error) int {
	switch {
	case errors.Is(err, orchestrator.ErrStopUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, repository.ErrInvalidStopRequest):
		return http.StatusBadRequest
	case errors.Is(err, repository.ErrStopNotCurrentRun), errors.Is(err, repository.ErrStopAlreadyTerminal), errors.Is(err, repository.ErrTaskInterrupted):
		return http.StatusConflict
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

// EffectResponse 是 effect 在 admin 操作响应里的序列化形态。
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
