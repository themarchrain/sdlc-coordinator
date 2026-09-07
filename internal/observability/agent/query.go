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

const codexProvider = "codex-cli"

type Query struct{ db *gorm.DB }

func NewQuery(db *gorm.DB) *Query { return &Query{db: db} }

type CodexFilter struct {
	Page        int
	PageSize    int
	Status      string
	Role        string
	TaskID      string
	RunID       string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
}

type CodexSummary struct {
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

type CodexExecution struct {
	ExecutionID  string     `json:"execution_id"`
	RunID        string     `json:"run_id"`
	TaskID       string     `json:"task_id"`
	TaskType     string     `json:"task_type"`
	Role         string     `json:"role"`
	Repository   string     `json:"repository"`
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

type CodexEvent struct {
	Seq        int64          `json:"seq"`
	Type       string         `json:"type"`
	OccurredAt time.Time      `json:"occurred_at"`
	Payload    map[string]any `json:"payload"`
}

type CodexExecutionDetail struct {
	CodexExecution
	Events []CodexEvent `json:"events"`
}

type CodexPage struct {
	Items    []CodexExecution
	Page     int
	PageSize int
	Total    int64
}

func (q *Query) CodexSummary(ctx context.Context, filter CodexFilter) (CodexSummary, error) {
	base := applyCodexJoinedFilter(codexExecutionJoin(q.db.WithContext(ctx)), filter).Select("agent_executions.*")
	var executions []model.AgentExecution
	if err := base.Find(&executions).Error; err != nil {
		return CodexSummary{}, fmt.Errorf("list Codex executions for summary: %w", err)
	}
	summary := CodexSummary{StatusCounts: map[string]int64{}, GeneratedAt: time.Now().UTC()}
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
		if err := q.db.WithContext(ctx).Where("execution_id IN ? AND type = ?", executionIDs, "codex_usage").Find(&events).Error; err != nil {
			return CodexSummary{}, fmt.Errorf("list Codex usage events: %w", err)
		}
		for _, event := range events {
			var payload map[string]any
			if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
				continue
			}
			summary.InputTokens += number(payload["input_tokens"])
			summary.CachedInputTokens += number(payload["cached_input_tokens"])
			summary.OutputTokens += number(payload["output_tokens"])
			summary.TurnCount++
		}
	}
	if summary.InputTokens > 0 {
		ratio := float64(summary.CachedInputTokens) / float64(summary.InputTokens)
		summary.CacheHitRatio = &ratio
	}
	return summary, nil
}

func (q *Query) ListCodexExecutions(ctx context.Context, filter CodexFilter) (CodexPage, error) {
	page, pageSize := normalizePage(filter.Page, filter.PageSize)
	base := codexExecutionJoin(q.db.WithContext(ctx))
	base = applyCodexJoinedFilter(base, filter)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return CodexPage{}, fmt.Errorf("count Codex executions: %w", err)
	}
	var items []CodexExecution
	if err := base.Select(codexExecutionColumns()).Order("agent_executions.created_at DESC").Order("agent_executions.id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Scan(&items).Error; err != nil {
		return CodexPage{}, fmt.Errorf("list Codex executions: %w", err)
	}
	for index := range items {
		items[index].DurationSecs = duration(items[index].StartedAt, items[index].FinishedAt)
	}
	return CodexPage{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (q *Query) GetCodexExecution(ctx context.Context, executionID string) (*CodexExecutionDetail, error) {
	var item CodexExecution
	err := codexExecutionJoin(q.db.WithContext(ctx)).Select(codexExecutionColumns()).Where("agent_executions.execution_id = ?", strings.TrimSpace(executionID)).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get Codex execution: %w", err)
	}
	item.DurationSecs = duration(item.StartedAt, item.FinishedAt)
	var stored []model.AgentExecutionEvent
	if err := q.db.WithContext(ctx).Where("execution_id = ? AND provider = ?", item.ExecutionID, codexProvider).Order("seq ASC").Find(&stored).Error; err != nil {
		return nil, fmt.Errorf("list Codex execution events: %w", err)
	}
	events := make([]CodexEvent, 0, len(stored))
	for _, event := range stored {
		payload := map[string]any{}
		_ = json.Unmarshal([]byte(event.PayloadJSON), &payload)
		if payload == nil {
			payload = map[string]any{}
		}
		events = append(events, CodexEvent{Seq: event.Seq, Type: event.Type, OccurredAt: event.OccurredAt, Payload: payload})
	}
	return &CodexExecutionDetail{CodexExecution: item, Events: events}, nil
}

func codexExecutionJoin(db *gorm.DB) *gorm.DB {
	return db.Table("agent_executions").
		Joins("JOIN runs ON runs.run_id = agent_executions.run_id").
		Joins("JOIN tasks ON tasks.task_id = runs.task_id").
		Where("agent_executions.provider = ?", codexProvider)
}

func codexExecutionColumns() string {
	return `agent_executions.execution_id, agent_executions.run_id, runs.task_id,
		tasks.type AS task_type, runs.worker_role AS role,
		CASE WHEN tasks.target_repo <> '' THEN tasks.target_repo ELSE tasks.source_repo END AS repository,
		agent_executions.status, agent_executions.provider_model AS model,
		agent_executions.provider_effort AS effort, agent_executions.error_code,
		agent_executions.error_message, agent_executions.request_hash,
		agent_executions.result_hash, agent_executions.started_at,
		agent_executions.finished_at, agent_executions.created_at`
}

func applyCodexFilter(query *gorm.DB, filter CodexFilter) *gorm.DB {
	if value := strings.TrimSpace(filter.Status); value != "" {
		query = query.Where("agent_executions.status = ?", value)
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

func applyCodexJoinedFilter(query *gorm.DB, filter CodexFilter) *gorm.DB {
	query = applyCodexFilter(query, filter)
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
