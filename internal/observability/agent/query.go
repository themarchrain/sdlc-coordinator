package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"

	"gorm.io/gorm"
)

type Query struct{ db *gorm.DB }

func NewQuery(db *gorm.DB) *Query { return &Query{db: db} }

type ExecutionFilter struct {
	Page        int
	PageSize    int
	Status      string
	Role        string
	TaskID      string
	RunID       string
	Provider    string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
}

type ExecutionSummary struct {
	TotalExecutions        int64
	StatusCounts           map[string]int64
	AverageDurationSeconds float64
	P50DurationSeconds     float64
	P90DurationSeconds     float64
	InputTokens            int64
	CachedInputTokens      int64
	OutputTokens           int64
	CacheHitRatio          *float64
	TurnCount              int64
	ErrorCount             int64
	TimeoutCount           int64
	GeneratedAt            time.Time
}

type ExecutionRecord struct {
	ExecutionID  string     `json:"execution_id"`
	RunID        string     `json:"run_id"`
	TaskID       string     `json:"task_id"`
	TaskType     string     `json:"task_type"`
	Role         string     `json:"role"`
	Repository   string     `json:"repository"`
	Provider     string     `json:"provider"`
	Status       string     `json:"status"`
	Model        string     `json:"model"`
	Effort       string     `json:"effort"`
	ErrorCode    string     `json:"error_code,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	RequestHash  string     `json:"request_hash"`
	ResultHash   string     `json:"result_hash,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	DurationSecs *float64   `json:"duration_seconds,omitempty"`
}

type ExecutionEvent struct {
	Seq        int64          `json:"seq"`
	Type       string         `json:"type"`
	OccurredAt time.Time      `json:"occurred_at"`
	Payload    map[string]any `json:"payload"`
}

type ExecutionDetail struct {
	ExecutionRecord
	Events []ExecutionEvent `json:"events"`
}

type ExecutionPage struct {
	Items    []ExecutionRecord
	Page     int
	PageSize int
	Total    int64
}

func (q *Query) ExecutionSummary(ctx context.Context, filter ExecutionFilter) (ExecutionSummary, error) {
	base := applyExecutionJoinedFilter(executionJoin(q.db.WithContext(ctx)), filter).Select("agent_executions.*")
	var executions []model.AgentExecution
	if err := base.Find(&executions).Error; err != nil {
		return ExecutionSummary{}, fmt.Errorf("list agent executions for summary: %w", err)
	}
	summary := ExecutionSummary{StatusCounts: map[string]int64{}, GeneratedAt: time.Now().UTC()}
	executionIDs := make([]string, 0, len(executions))
	durations := make([]float64, 0, len(executions))
	for _, execution := range executions {
		summary.TotalExecutions++
		summary.StatusCounts[string(execution.Status)]++
		executionIDs = append(executionIDs, execution.ExecutionID)
		if execution.Status == model.AgentExecutionStatusFailed || execution.Status == model.AgentExecutionStatusCancelled || execution.Status == model.AgentExecutionStatusInterrupted {
			summary.ErrorCount++
		}
		if execution.ErrorCode == "execution_timeout" {
			summary.TimeoutCount++
		}
		if execution.StartedAt != nil && execution.FinishedAt != nil && !execution.FinishedAt.Before(*execution.StartedAt) {
			durations = append(durations, execution.FinishedAt.Sub(*execution.StartedAt).Seconds())
		}
	}
	if len(durations) > 0 {
		sort.Float64s(durations)
		for _, duration := range durations {
			summary.AverageDurationSeconds += duration
		}
		summary.AverageDurationSeconds /= float64(len(durations))
		summary.P50DurationSeconds = percentile(durations, 0.50)
		summary.P90DurationSeconds = percentile(durations, 0.90)
	}
	if len(executionIDs) > 0 {
		var events []model.AgentExecutionEvent
		if err := q.db.WithContext(ctx).Where("execution_id IN ? AND type IN ?", executionIDs, usageEventTypes).Find(&events).Error; err != nil {
			return ExecutionSummary{}, fmt.Errorf("list agent usage events: %w", err)
		}
		for _, event := range events {
			var payload map[string]any
			if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
				continue
			}
			input, cached, output := usageTotals(payload)
			summary.InputTokens += input
			summary.CachedInputTokens += cached
			summary.OutputTokens += output
			summary.TurnCount++
		}
	}
	if summary.InputTokens > 0 {
		ratio := float64(summary.CachedInputTokens) / float64(summary.InputTokens)
		summary.CacheHitRatio = &ratio
	}
	return summary, nil
}

func (q *Query) ListExecutions(ctx context.Context, filter ExecutionFilter) (ExecutionPage, error) {
	page, pageSize := normalizePage(filter.Page, filter.PageSize)
	base := executionJoin(q.db.WithContext(ctx))
	base = applyExecutionJoinedFilter(base, filter)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return ExecutionPage{}, fmt.Errorf("count agent executions: %w", err)
	}
	var items []ExecutionRecord
	if err := base.Select(executionColumns()).Order("agent_executions.created_at DESC").Order("agent_executions.id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Scan(&items).Error; err != nil {
		return ExecutionPage{}, fmt.Errorf("list agent executions: %w", err)
	}
	for index := range items {
		items[index].DurationSecs = duration(items[index].StartedAt, items[index].FinishedAt)
	}
	return ExecutionPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (q *Query) GetExecution(ctx context.Context, executionID string) (*ExecutionDetail, error) {
	var item ExecutionRecord
	err := executionJoin(q.db.WithContext(ctx)).Select(executionColumns()).Where("agent_executions.execution_id = ?", strings.TrimSpace(executionID)).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get agent execution: %w", err)
	}
	item.DurationSecs = duration(item.StartedAt, item.FinishedAt)
	var stored []model.AgentExecutionEvent
	if err := q.db.WithContext(ctx).Where("execution_id = ? AND provider = ?", item.ExecutionID, item.Provider).Order("seq ASC").Find(&stored).Error; err != nil {
		return nil, fmt.Errorf("list agent execution events: %w", err)
	}
	events := make([]ExecutionEvent, 0, len(stored))
	for _, event := range stored {
		payload := map[string]any{}
		_ = json.Unmarshal([]byte(event.PayloadJSON), &payload)
		if payload == nil {
			payload = map[string]any{}
		}
		events = append(events, ExecutionEvent{Seq: event.Seq, Type: event.Type, OccurredAt: event.OccurredAt, Payload: payload})
	}
	return &ExecutionDetail{ExecutionRecord: item, Events: events}, nil
}

// usageEventTypes covers both providers: codex_usage is flat, claude_usage nests
// the usage object one level down (see usageTotals).
var usageEventTypes = []string{"codex_usage", "claude_usage"}

func usageTotals(payload map[string]any) (input, cached, output int64) {
	source := payload
	if usage, ok := payload["usage"].(map[string]any); ok {
		source = usage
	}
	return number(source["input_tokens"]),
		number(source["cached_input_tokens"]) + number(source["cache_read_input_tokens"]),
		number(source["output_tokens"])
}

func executionJoin(db *gorm.DB) *gorm.DB {
	return db.Table("agent_executions").
		Joins("JOIN runs ON runs.run_id = agent_executions.run_id").
		Joins("JOIN tasks ON tasks.task_id = runs.task_id")
}

func executionColumns() string {
	return `agent_executions.execution_id, agent_executions.run_id, runs.task_id,
		tasks.type AS task_type, runs.worker_role AS role,
		CASE WHEN tasks.target_repo <> '' THEN tasks.target_repo ELSE tasks.source_repo END AS repository,
		agent_executions.provider, agent_executions.status, agent_executions.provider_model AS model,
		agent_executions.provider_effort AS effort, agent_executions.error_code,
		agent_executions.error_message, agent_executions.request_hash,
		agent_executions.result_hash, agent_executions.started_at,
		agent_executions.finished_at, agent_executions.created_at`
}

func applyExecutionFilter(query *gorm.DB, filter ExecutionFilter) *gorm.DB {
	if value := strings.TrimSpace(filter.Status); value != "" {
		query = query.Where("agent_executions.status = ?", value)
	}
	if value := strings.TrimSpace(filter.Provider); value != "" {
		query = query.Where("agent_executions.provider = ?", value)
	}
	if value := strings.TrimSpace(filter.RunID); value != "" {
		query = query.Where("agent_executions.run_id = ?", value)
	}
	if filter.CreatedFrom != nil {
		query = query.Where("agent_executions.created_at >= ?", *filter.CreatedFrom)
	}
	if filter.CreatedTo != nil {
		query = query.Where("agent_executions.created_at <= ?", *filter.CreatedTo)
	}
	return query
}

func applyExecutionJoinedFilter(query *gorm.DB, filter ExecutionFilter) *gorm.DB {
	query = applyExecutionFilter(query, filter)
	if value := strings.TrimSpace(filter.Role); value != "" {
		query = query.Where("runs.worker_role = ?", value)
	}
	if value := strings.TrimSpace(filter.TaskID); value != "" {
		query = query.Where("runs.task_id = ?", value)
	}
	return query
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func number(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	default:
		return 0
	}
}

func percentile(sorted []float64, ratio float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Ceil(ratio*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	return sorted[index]
}

func duration(started, finished *time.Time) *float64 {
	if started == nil || finished == nil || finished.Before(*started) {
		return nil
	}
	value := finished.Sub(*started).Seconds()
	return &value
}
