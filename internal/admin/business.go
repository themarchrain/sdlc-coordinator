package admin

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"
)

type BusinessSummary struct {
	Window                      MetricsWindow
	MergeRequestsCreated        int64
	ReviewsCompleted            int64
	ReviewerTakeovers           int64
	AutomationSuccess           RatioMetric
	AverageSuccessfulRunSeconds *float64
	HumanRequiredTasks          int64
	HumanRequiredAddedToday     int64
	DailyMergeRequestsCreated   []DailyCount
	DailyReviewsCompleted       []DailyCount
	DailyReviewerTakeovers      []DailyCount
	DailyAverageRunSeconds      []DailyAverageSeconds
	GeneratedAt                 time.Time
}

func (service *QueryService) BusinessSummary(ctx context.Context, filter MetricsFilter) (BusinessSummary, error) {
	if err := service.ensureRepository(ctx, filter.RepositoryID); err != nil {
		return BusinessSummary{}, err
	}
	window, err := ParseMetricsWindow(string(filter.Window))
	if err != nil {
		return BusinessSummary{}, err
	}
	now := filter.Now.UTC()
	if filter.Now.IsZero() {
		now = time.Now().UTC()
	}
	result := BusinessSummary{Window: window, GeneratedAt: now}
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dailyReviews := make(map[string]int64)

	taskQuery := service.db.WithContext(ctx).Model(&model.Task{})
	if start := metricsWindowStart(window, now); start != nil {
		taskQuery = taskQuery.Where("created_at >= ?", *start)
	}
	taskQuery = applyDirectRepositoryFilter(taskQuery, filter.RepositoryID)
	var tasks []model.Task
	if err := taskQuery.Order("created_at ASC").Order("id ASC").Find(&tasks).Error; err != nil {
		return BusinessSummary{}, fmt.Errorf("list business tasks: %w", err)
	}
	result.AutomationSuccess.Denominator = int64(len(tasks))
	taskIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		taskIDs = append(taskIDs, task.TaskID)
		day := task.CreatedAt.UTC().Format(dayFormat)
		if task.Status == model.TaskStatusSucceeded {
			result.AutomationSuccess.Numerator++
		}
		if task.Status == model.TaskStatusHumanRequired {
			result.HumanRequiredTasks++
			if !task.UpdatedAt.Before(todayStart) {
				result.HumanRequiredAddedToday++
			}
		}
		if task.Type == "review_mr" && task.Status == model.TaskStatusSucceeded {
			result.ReviewsCompleted++
			dailyReviews[day]++
		}
	}
	result.AutomationSuccess = ratioMetric(result.AutomationSuccess.Numerator, result.AutomationSuccess.Denominator)
	result.DailyReviewsCompleted = dailyCounts(dailyReviews)

	if len(taskIDs) > 0 {
		var runs []model.Run
		if err := service.db.WithContext(ctx).
			Where("task_id IN ? AND status = ? AND started_at IS NOT NULL AND completed_at IS NOT NULL", taskIDs, model.RunStatusSucceeded).
			Find(&runs).Error; err != nil {
			return BusinessSummary{}, fmt.Errorf("list business runs: %w", err)
		}
		dailyDurations := make(map[string]*dailyDurationAgg)
		var totalSeconds float64
		for _, run := range runs {
			if run.CompletedAt.Before(*run.StartedAt) {
				continue
			}
			seconds := run.CompletedAt.Sub(*run.StartedAt).Seconds()
			totalSeconds += seconds
			day := run.StartedAt.UTC().Format(dayFormat)
			agg := dailyDurations[day]
			if agg == nil {
				agg = &dailyDurationAgg{}
				dailyDurations[day] = agg
			}
			agg.totalSeconds += seconds
			agg.count++
		}
		if len(runs) > 0 {
			average := totalSeconds / float64(len(runs))
			result.AverageSuccessfulRunSeconds = &average
		}
		result.DailyAverageRunSeconds = dailyAverages(dailyDurations)
	}

	writebackQuery := service.db.WithContext(ctx).Model(&model.Writeback{}).
		Where("status = ?", model.WritebackStatusSucceeded).
		Where("kind IN ?", []string{"create_merge_request", "add_merge_request_labels"})
	if start := metricsWindowStart(window, now); start != nil {
		writebackQuery = writebackQuery.Where("created_at >= ?", *start)
	}
	writebackQuery = applyJoinedTaskRepositoryFilter(writebackQuery, filter.RepositoryID)
	var writebacks []model.Writeback
	if err := writebackQuery.Find(&writebacks).Error; err != nil {
		return BusinessSummary{}, fmt.Errorf("list business writebacks: %w", err)
	}
	dailyMRs := make(map[string]int64)
	dailyTakeovers := make(map[string]int64)
	for _, writeback := range writebacks {
		day := writeback.CreatedAt.UTC().Format(dayFormat)
		switch writeback.Kind {
		case "create_merge_request":
			result.MergeRequestsCreated++
			dailyMRs[day]++
		case "add_merge_request_labels":
			if strings.HasPrefix(writeback.DedupeKey, "reviewer-takeover-label:") {
				result.ReviewerTakeovers++
				dailyTakeovers[day]++
			}
		}
	}
	result.DailyMergeRequestsCreated = dailyCounts(dailyMRs)
	result.DailyReviewerTakeovers = dailyCounts(dailyTakeovers)
	return result, nil
}

const dayFormat = "2006-01-02"

type dailyDurationAgg struct {
	totalSeconds float64
	count        int64
}

func dailyCounts(counts map[string]int64) []DailyCount {
	days := make([]string, 0, len(counts))
	for day := range counts {
		days = append(days, day)
	}
	sort.Strings(days)
	result := make([]DailyCount, 0, len(days))
	for _, day := range days {
		result = append(result, DailyCount{Date: day, Count: counts[day]})
	}
	return result
}

func dailyAverages(aggregates map[string]*dailyDurationAgg) []DailyAverageSeconds {
	days := make([]string, 0, len(aggregates))
	for day := range aggregates {
		days = append(days, day)
	}
	sort.Strings(days)
	result := make([]DailyAverageSeconds, 0, len(days))
	for _, day := range days {
		agg := aggregates[day]
		result = append(result, DailyAverageSeconds{
			Date: day, Seconds: agg.totalSeconds / float64(agg.count), SampleCount: agg.count,
		})
	}
	return result
}

