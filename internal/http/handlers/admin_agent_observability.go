package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	agentobservability "sdlc-coordinator/internal/observability/agent"
	"sdlc-coordinator/internal/store/repository"

	"github.com/gin-gonic/gin"
)

type AdminAgentObservabilityHandler struct{ query *agentobservability.Query }

func NewAdminAgentObservabilityHandler(query *agentobservability.Query) *AdminAgentObservabilityHandler {
	return &AdminAgentObservabilityHandler{query: query}
}

func (handler *AdminAgentObservabilityHandler) RegisterRoutes(group *gin.RouterGroup) {
	executions := group.Group("/agents/executions")
	executions.GET("/summary", handler.AgentExecutionSummary)
	executions.GET("/executions", handler.ListAgentExecutions)
	executions.GET("/executions/:execution_id", handler.GetAgentExecution)
}

func (handler *AdminAgentObservabilityHandler) AgentExecutionSummary(c *gin.Context) {
	filter, ok := parseExecutionFilter(c, false)
	if !ok {
		return
	}
	summary, err := handler.query.ExecutionSummary(c.Request.Context(), filter)
	if err != nil {
		writeAdminError(c, http.StatusInternalServerError, "internal_error", "查询 Agent 执行摘要失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"total_executions": summary.TotalExecutions, "status_counts": summary.StatusCounts,
		"average_duration_seconds": summary.AverageDurationSeconds, "p50_duration_seconds": summary.P50DurationSeconds,
		"p90_duration_seconds": summary.P90DurationSeconds, "input_tokens": summary.InputTokens,
		"cached_input_tokens": summary.CachedInputTokens, "output_tokens": summary.OutputTokens,
		"cache_hit_ratio": summary.CacheHitRatio, "turn_count": summary.TurnCount,
		"error_count": summary.ErrorCount, "timeout_count": summary.TimeoutCount, "generated_at": summary.GeneratedAt,
	})
}

func (handler *AdminAgentObservabilityHandler) ListAgentExecutions(c *gin.Context) {
	filter, ok := parseExecutionFilter(c, true)
	if !ok {
		return
	}
	page, err := handler.query.ListExecutions(c.Request.Context(), filter)
	if err != nil {
		writeAdminError(c, http.StatusInternalServerError, "internal_error", "查询 Agent 执行记录失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": page.Items, "page": page.Page, "page_size": page.PageSize, "total": page.Total})
}

func (handler *AdminAgentObservabilityHandler) GetAgentExecution(c *gin.Context) {
	detail, err := handler.query.GetExecution(c.Request.Context(), c.Param("execution_id"))
	if errors.Is(err, repository.ErrNotFound) {
		writeAdminError(c, http.StatusNotFound, "not_found", "Agent 执行记录不存在")
		return
	}
	if err != nil {
		writeAdminError(c, http.StatusInternalServerError, "internal_error", "查询 Agent 执行详情失败")
		return
	}
	c.JSON(http.StatusOK, detail)
}

func parseExecutionFilter(c *gin.Context, withPage bool) (agentobservability.ExecutionFilter, bool) {
	filter := agentobservability.ExecutionFilter{
		Status: strings.TrimSpace(c.Query("status")), Role: strings.TrimSpace(c.Query("role")),
		TaskID: strings.TrimSpace(c.Query("task_id")), RunID: strings.TrimSpace(c.Query("run_id")),
		Provider: strings.TrimSpace(c.Query("provider")),
	}
	if !withPage {
		return filter, true
	}
	var err error
	filter.Page, err = parsePositiveQueryInt(c.Query("page"), 1)
	if err == nil {
		filter.PageSize, err = parsePositiveQueryInt(c.Query("page_size"), 20)
	}
	if err != nil || filter.PageSize > 100 {
		writeAdminError(c, http.StatusBadRequest, "invalid_query", "page 和 page_size 必须是有效正整数，且 page_size 不超过 100")
		return agentobservability.ExecutionFilter{}, false
	}
	return filter, true
}

func parsePositiveQueryInt(raw string, fallback int) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, errors.New("invalid positive integer")
	}
	return value, nil
}
