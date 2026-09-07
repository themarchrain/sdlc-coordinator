package admin

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"
)

func ParseMetricsWindow(raw string) (MetricsWindow, error) {
	value := MetricsWindow(strings.TrimSpace(raw))
	if value == "" {
		return MetricsWindow30Days, nil
	}
	switch value {
	case MetricsWindow7Days, MetricsWindow30Days, MetricsWindowAll:
		return value, nil
	default:
		return "", fmt.Errorf("unsupported metrics window %q", raw)
	}
}

func (service *QueryService) Metrics(ctx context.Context, filter MetricsFilter) (AutomationMetrics, error) {
	if err := service.ensureRepository(ctx, filter.RepositoryID); err != nil {
		return AutomationMetrics{}, err
	}
	window, err := ParseMetricsWindow(string(filter.Window))
	if err != nil {
		return AutomationMetrics{}, err
	}
	now := filter.Now.UTC()
	if filter.Now.IsZero() {
		now = time.Now().UTC()
	}
	taskType := strings.TrimSpace(filter.Type)
	result := AutomationMetrics{
		Window: window, Type: taskType, GeneratedAt: now,
		SuccessfulRunDurations: make([]RunDurationMetric, 0),
	}

	query := service.db.WithContext(ctx).Model(&model.Task{})
	if start := metricsWindowStart(window, now); start != nil {
		query = query.Where("created_at >= ?", *start)
	}
	if taskType != "" {
		query = query.Where("type = ?", taskType)
	}
	query = applyDirectRepositoryFilter(query, filter.RepositoryID)
	var tasks []model.Task
	if err := query.Order("created_at ASC").Order("id ASC").Find(&tasks).Error; err != nil {
		return AutomationMetrics{}, fmt.Errorf("list metric tasks: %w", err)
	}
	result.TaskSampleCount = int64(len(tasks))
	if len(tasks) == 0 {
		return result, nil
	}

	taskIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		taskIDs = append(taskIDs, task.TaskID)
	}

	var runs []model.Run
	if err := service.db.WithContext(ctx).
		Where("task_id IN ?", taskIDs).
		Order("created_at ASC").
		Order("id ASC").
		Find(&runs).Error; err != nil {
		return AutomationMetrics{}, fmt.Errorf("list metric runs: %w", err)
	}
	runsByTask := make(map[string][]model.Run, len(tasks))
	durationsByRole := make(map[string][]float64)
	for _, run := range runs {
		runsByTask[run.TaskID] = append(runsByTask[run.TaskID], run)
		if run.Status != model.RunStatusSucceeded || run.StartedAt == nil || run.CompletedAt == nil || run.CompletedAt.Before(*run.StartedAt) {
			continue
		}
		durationsByRole[run.WorkerRole] = append(durationsByRole[run.WorkerRole], run.CompletedAt.Sub(*run.StartedAt).Seconds())
	}

	var current, firstRun, intervention, recovered RatioMetric
	computeAutomationRatios(tasks, runsByTask, &current, &firstRun, &intervention, &recovered)
	result.CurrentSuccess = ratioMetric(current.Numerator, int64(len(tasks)))
	result.FirstRunSuccess = ratioMetric(firstRun.Numerator, firstRun.Denominator)
	result.InterventionTriggered = ratioMetric(intervention.Numerator, intervention.Denominator)
	result.RecoveredAfterBlock = ratioMetric(recovered.Numerator, recovered.Denominator)
	result.SuccessfulRunDurations = summarizeDurations(durationsByRole)
	result.DailySeries = dailyRatios(tasks, runsByTask)
	return result, nil
}

// computeAutomationRatios 按任务样本计算 4 个自动化比率（总口径与按日口径共用）。
func computeAutomationRatios(tasks []model.Task, runsByTask map[string][]model.Run, current, firstRun, intervention, recovered *RatioMetric) {
	for _, task := range tasks {
		if task.Status == model.TaskStatusSucceeded {
			current.Numerator++
		}
		taskRuns := runsByTask[task.TaskID]
		if len(taskRuns) == 0 {
			continue
		}
		intervention.Denominator++
		first := taskRuns[0]
		if first.CompletedAt != nil && isCompletedRunStatus(first.Status) {
			firstRun.Denominator++
			if first.Status == model.RunStatusSucceeded {
				firstRun.Numerator++
			}
		}
		hadBlockedRun := false
		for _, run := range taskRuns {
			if run.Status == model.RunStatusBlocked {
				hadBlockedRun = true
				break
			}
		}
		if hadBlockedRun || task.Status == model.TaskStatusHumanRequired {
			intervention.Numerator++
		}
		if hadBlockedRun {
			recovered.Denominator++
			if task.Status == model.TaskStatusSucceeded {
				recovered.Numerator++
			}
		}
	}
}

// dailyRatios 把任务按创建日分组，对每天复用总口径计算 4 个比率。
func dailyRatios(tasks []model.Task, runsByTask map[string][]model.Run) []DailyRatioPoint {
	byDay := make(map[string][]model.Task)
	for _, task := range tasks {
		day := task.CreatedAt.UTC().Format(dayFormat)
		byDay[day] = append(byDay[day], task)
	}
	days := make([]string, 0, len(byDay))
	for day := range byDay {
		days = append(days, day)
	}
	sort.Strings(days)
	result := make([]DailyRatioPoint, 0, len(days))
	for _, day := range days {
		group := byDay[day]
		var current, firstRun, intervention, recovered RatioMetric
		computeAutomationRatios(group, runsByTask, &current, &firstRun, &intervention, &recovered)
		result = append(result, DailyRatioPoint{
			Date:                  day,
			CurrentSuccess:        ratioMetric(current.Numerator, int64(len(group))).Value,
			FirstRunSuccess:       ratioMetric(firstRun.Numerator, firstRun.Denominator).Value,
			InterventionTriggered: ratioMetric(intervention.Numerator, intervention.Denominator).Value,
			RecoveredAfterBlock:   ratioMetric(recovered.Numerator, recovered.Denominator).Value,
		})
	}
	return result
}

func metricsWindowStart(window MetricsWindow, now time.Time) *time.Time {
	var start time.Time
	switch window {
	case MetricsWindow7Days:
		start = now.Add(-7 * 24 * time.Hour)
	case MetricsWindow30Days:
		start = now.Add(-30 * 24 * time.Hour)
	default:
		return nil
	}
	return &start
}

func ratioMetric(numerator, denominator int64) RatioMetric {
	metric := RatioMetric{Numerator: numerator, Denominator: denominator}
	if denominator == 0 {
		return metric
	}
	value := float64(numerator) / float64(denominator)
	metric.Value = &value
	return metric
}

func isCompletedRunStatus(status model.RunStatus) bool {
	switch status {
	case model.RunStatusSucceeded, model.RunStatusFailed, model.RunStatusBlocked:
		return true
	default:
		return false
	}
}

func summarizeDurations(valuesByRole map[string][]float64) []RunDurationMetric {
	roles := make([]string, 0, len(valuesByRole))
	for role := range valuesByRole {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	result := make([]RunDurationMetric, 0, len(roles))
	for _, role := range roles {
		values := valuesByRole[role]
		sort.Float64s(values)
		var total float64
		for _, value := range values {
			total += value
		}
		result = append(result, RunDurationMetric{
			WorkerRole: role, SampleCount: len(values), AverageSeconds: total / float64(len(values)),
			P50Seconds: nearestRank(values, 0.5), P90Seconds: nearestRank(values, 0.9),
		})
	}
	return result
}

func nearestRank(sorted []float64, percentile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Ceil(percentile*float64(len(sorted)))) - 1
	return sorted[index]
}
