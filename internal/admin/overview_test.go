package admin

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

func TestOverviewCountsCurrentStatesAndRecentFailures(t *testing.T) {
	service, db := newQueryServiceTestDB(t)
	now := time.Now().UTC()
	seedOverviewEntities(t, db, now)

	got, err := service.Overview(context.Background())
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if got.TasksByStatus[string(model.TaskStatusRunning)] != 1 {
		t.Fatalf("unexpected task counts: %#v", got.TasksByStatus)
	}
	if got.TasksByStatus[string(model.TaskStatusHumanRequired)] != 1 {
		t.Fatalf("unexpected task counts: %#v", got.TasksByStatus)
	}
	if got.RunsByStatus[string(model.RunStatusFailed)] != 1 {
		t.Fatalf("unexpected run counts: %#v", got.RunsByStatus)
	}
	if got.WritebacksByStatus[string(model.WritebackStatusFailed)] != 1 {
		t.Fatalf("unexpected writeback counts: %#v", got.WritebacksByStatus)
	}
	if got.EffectsByStatus[string(model.EffectStatusFailed)] != 1 {
		t.Fatalf("unexpected effect counts: %#v", got.EffectsByStatus)
	}
	if len(got.RecentFailures) != 4 {
		t.Fatalf("expected four recent failures, got %#v", got.RecentFailures)
	}
	if got.RecentFailures[0].EntityType != "effect" || got.RecentFailures[0].EntityID != "effect-overview" {
		t.Fatalf("expected newest failure first, got %#v", got.RecentFailures)
	}
}

func TestBusinessSummaryUsesOutcomeFactsInOneWindow(t *testing.T) {
	service, db := newQueryServiceTestDB(t)
	now := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)

	devTask := &model.Task{
		TaskID: "task-dev", DedupeKey: "task-dev", Type: "develop_from_issue", Role: "DeveloperWorker",
		Status: model.TaskStatusSucceeded, ContextJSON: `{}`,
		CreatedAt: now.Add(-2 * 24 * time.Hour), UpdatedAt: now.Add(-2 * 24 * time.Hour),
	}
	reviewTask := &model.Task{
		TaskID: "task-review", DedupeKey: "task-review", Type: "review_mr", Role: "ReviewerWorker",
		Status: model.TaskStatusSucceeded, ContextJSON: `{}`,
		CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-24 * time.Hour),
	}
	humanTask := &model.Task{
		TaskID: "task-human", DedupeKey: "task-human", Type: "develop_from_issue", Role: "DeveloperWorker",
		Status: model.TaskStatusHumanRequired, ContextJSON: `{}`,
		CreatedAt: now.Add(-12 * time.Hour), UpdatedAt: now.Add(-12 * time.Hour),
	}
	oldTask := &model.Task{
		TaskID: "task-old", DedupeKey: "task-old", Type: "review_mr", Role: "ReviewerWorker",
		Status: model.TaskStatusSucceeded, ContextJSON: `{}`,
		CreatedAt: now.Add(-40 * 24 * time.Hour), UpdatedAt: now.Add(-40 * 24 * time.Hour),
	}
	for _, task := range []*model.Task{devTask, reviewTask, humanTask, oldTask} {
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("seed task: %v", err)
		}
	}

	seedBusinessRun(t, db, "run-dev", devTask.TaskID, now.Add(-2*24*time.Hour), 60)
	seedBusinessRun(t, db, "run-review", reviewTask.TaskID, now.Add(-24*time.Hour), 120)
	seedBusinessRun(t, db, "run-old", oldTask.TaskID, now.Add(-40*24*time.Hour), 30)
	for _, writeback := range []*model.Writeback{
		{WritebackID: "wb-mr", TaskID: devTask.TaskID, RunID: "run-dev", DedupeKey: "mr:dev", Kind: "create_merge_request", ProjectID: "group/repo", Status: model.WritebackStatusSucceeded, CreatedAt: now.Add(-2 * 24 * time.Hour), UpdatedAt: now.Add(-2 * 24 * time.Hour)},
		{WritebackID: "wb-takeover", TaskID: reviewTask.TaskID, RunID: "run-review", DedupeKey: "reviewer-takeover-label:task-review", Kind: "add_merge_request_labels", ProjectID: "group/repo", Status: model.WritebackStatusSucceeded, CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-24 * time.Hour)},
		{WritebackID: "wb-old", TaskID: oldTask.TaskID, RunID: "run-old", DedupeKey: "mr:old", Kind: "create_merge_request", ProjectID: "group/repo", Status: model.WritebackStatusSucceeded, CreatedAt: now.Add(-40 * 24 * time.Hour), UpdatedAt: now.Add(-40 * 24 * time.Hour)},
	} {
		if err := db.Create(writeback).Error; err != nil {
			t.Fatalf("seed writeback: %v", err)
		}
	}

	got, err := service.BusinessSummary(context.Background(), MetricsFilter{Window: MetricsWindow30Days, Now: now})
	if err != nil {
		t.Fatalf("business summary: %v", err)
	}
	if got.Window != MetricsWindow30Days || got.MergeRequestsCreated != 1 || got.ReviewsCompleted != 1 || got.ReviewerTakeovers != 1 || got.HumanRequiredTasks != 1 {
		t.Fatalf("unexpected business counts: %#v", got)
	}
	assertRatio(t, got.AutomationSuccess, 2, 3)
	if got.AverageSuccessfulRunSeconds == nil || *got.AverageSuccessfulRunSeconds != 90 {
		t.Fatalf("unexpected average run duration: %#v", got.AverageSuccessfulRunSeconds)
	}
}

func TestBusinessSummaryEmptyWindow(t *testing.T) {
	service, _ := newQueryServiceTestDB(t)
	now := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)

	got, err := service.BusinessSummary(context.Background(), MetricsFilter{Window: MetricsWindow30Days, Now: now})
	if err != nil {
		t.Fatalf("business summary: %v", err)
	}
	if got.MergeRequestsCreated != 0 || got.ReviewsCompleted != 0 || got.ReviewerTakeovers != 0 || got.HumanRequiredTasks != 0 {
		t.Fatalf("expected zero business counts on empty window, got %#v", got)
	}
	if got.AutomationSuccess.Numerator != 0 || got.AutomationSuccess.Denominator != 0 || got.AutomationSuccess.Value != nil {
		t.Fatalf("expected empty ratio on empty window, got %#v", got.AutomationSuccess)
	}
	if got.AverageSuccessfulRunSeconds != nil {
		t.Fatalf("expected nil average on empty window, got %v", *got.AverageSuccessfulRunSeconds)
	}
}

func TestBusinessSummaryExcludesNonOutcomeFacts(t *testing.T) {
	service, db := newQueryServiceTestDB(t)
	now := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)

	tasks := []*model.Task{
		{TaskID: "task-valid", DedupeKey: "task-valid", Type: "review_mr", Role: "ReviewerWorker", Status: model.TaskStatusSucceeded, ContextJSON: `{}`, CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-24 * time.Hour)},
		{TaskID: "task-failed-review", DedupeKey: "task-failed-review", Type: "review_mr", Role: "ReviewerWorker", Status: model.TaskStatusBlocked, ContextJSON: `{}`, CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-24 * time.Hour)},
		{TaskID: "task-bad-json", DedupeKey: "task-bad-json", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusSucceeded, ContextJSON: `not-json`, CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-24 * time.Hour)},
	}
	for _, task := range tasks {
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("seed task: %v", err)
		}
	}
	seedBusinessRun(t, db, "run-valid", "task-valid", now.Add(-24*time.Hour), 60)
	if err := db.Create(&model.Run{RunID: "run-no-timestamps", TaskID: "task-valid", WorkerRole: "DeveloperWorker", Status: model.RunStatusSucceeded, CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-24 * time.Hour)}).Error; err != nil {
		t.Fatalf("seed run without timestamps: %v", err)
	}
	writebacks := []*model.Writeback{
		{WritebackID: "wb-mr-ok", TaskID: "task-valid", RunID: "run-valid", DedupeKey: "mr:valid", Kind: "create_merge_request", ProjectID: "group/repo", Status: model.WritebackStatusSucceeded, CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-24 * time.Hour)},
		{WritebackID: "wb-mr-failed", TaskID: "task-valid", RunID: "run-valid", DedupeKey: "mr:failed", Kind: "create_merge_request", ProjectID: "group/repo", Status: model.WritebackStatusFailed, CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-24 * time.Hour)},
		{WritebackID: "wb-takeover-ok", TaskID: "task-valid", RunID: "run-valid", DedupeKey: "reviewer-takeover-label:task-valid", Kind: "add_merge_request_labels", ProjectID: "group/repo", Status: model.WritebackStatusSucceeded, CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-24 * time.Hour)},
		{WritebackID: "wb-takeover-failed", TaskID: "task-failed-review", RunID: "run-failed-review", DedupeKey: "reviewer-takeover-label:task-failed-review", Kind: "add_merge_request_labels", ProjectID: "group/repo", Status: model.WritebackStatusFailed, CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-24 * time.Hour)},
		{WritebackID: "wb-label-other", TaskID: "task-valid", RunID: "run-valid", DedupeKey: "normal-label:whatever", Kind: "add_merge_request_labels", ProjectID: "group/repo", Status: model.WritebackStatusSucceeded, CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-24 * time.Hour)},
	}
	for _, writeback := range writebacks {
		if err := db.Create(writeback).Error; err != nil {
			t.Fatalf("seed writeback: %v", err)
		}
	}

	got, err := service.BusinessSummary(context.Background(), MetricsFilter{Window: MetricsWindow30Days, Now: now})
	if err != nil {
		t.Fatalf("business summary: %v", err)
	}
	if got.MergeRequestsCreated != 1 || got.ReviewsCompleted != 1 || got.ReviewerTakeovers != 1 {
		t.Fatalf("unexpected business counts: %#v", got)
	}
	assertRatio(t, got.AutomationSuccess, 2, 3) // succeeded 任务 2 个（含畸形 JSON），failed review 不计
	if got.AverageSuccessfulRunSeconds == nil || *got.AverageSuccessfulRunSeconds != 60 {
		t.Fatalf("nil-timestamp run must be excluded from average: %#v", got.AverageSuccessfulRunSeconds)
	}
}

func TestBusinessSummaryWindowSevenDaysAndAll(t *testing.T) {
	service, db := newQueryServiceTestDB(t)
	now := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)
	tenDaysAgo := now.Add(-10 * 24 * time.Hour)

	task := &model.Task{TaskID: "task-10d", DedupeKey: "task-10d", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusSucceeded, ContextJSON: `{}`, CreatedAt: tenDaysAgo, UpdatedAt: tenDaysAgo}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("seed task: %v", err)
	}
	seedBusinessRun(t, db, "run-10d", task.TaskID, tenDaysAgo, 60)
	if err := db.Create(&model.Writeback{WritebackID: "wb-10d", TaskID: task.TaskID, RunID: "run-10d", DedupeKey: "mr:10d", Kind: "create_merge_request", ProjectID: "group/repo", Status: model.WritebackStatusSucceeded, CreatedAt: tenDaysAgo, UpdatedAt: tenDaysAgo}).Error; err != nil {
		t.Fatalf("seed writeback: %v", err)
	}

	got7d, err := service.BusinessSummary(context.Background(), MetricsFilter{Window: MetricsWindow7Days, Now: now})
	if err != nil {
		t.Fatalf("business summary 7d: %v", err)
	}
	if got7d.MergeRequestsCreated != 0 || got7d.AutomationSuccess.Denominator != 0 || got7d.AverageSuccessfulRunSeconds != nil {
		t.Fatalf("7d window must exclude 10-day-old facts: %#v", got7d)
	}

	for _, window := range []MetricsWindow{MetricsWindow30Days, MetricsWindowAll} {
		got, err := service.BusinessSummary(context.Background(), MetricsFilter{Window: window, Now: now})
		if err != nil {
			t.Fatalf("business summary %s: %v", window, err)
		}
		if got.MergeRequestsCreated != 1 {
			t.Fatalf("%s window must include 10-day-old facts: %#v", window, got)
		}
		assertRatio(t, got.AutomationSuccess, 1, 1)
		if got.AverageSuccessfulRunSeconds == nil || *got.AverageSuccessfulRunSeconds != 60 {
			t.Fatalf("%s window average: %#v", window, got.AverageSuccessfulRunSeconds)
		}
	}
}

func TestBusinessSummaryDailySeriesAndTodayAdded(t *testing.T) {
	service, db := newQueryServiceTestDB(t)
	now := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)
	today := "2026-08-14"
	yesterday := "2026-08-13"

	tasks := []*model.Task{
		{TaskID: "task-dev-yesterday", DedupeKey: "task-dev-yesterday", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusSucceeded, ContextJSON: `{}`, CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-24 * time.Hour)},
		{TaskID: "task-dev-today", DedupeKey: "task-dev-today", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusSucceeded, ContextJSON: `{}`, CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour)},
		{TaskID: "task-review-today", DedupeKey: "task-review-today", Type: "review_mr", Role: "ReviewerWorker", Status: model.TaskStatusSucceeded, ContextJSON: `{}`, CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour)},
		{TaskID: "task-human-today", DedupeKey: "task-human-today", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusHumanRequired, ContextJSON: `{}`, CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-time.Hour)},
		{TaskID: "task-human-yesterday", DedupeKey: "task-human-yesterday", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusHumanRequired, ContextJSON: `{}`, CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-20 * time.Hour)},
	}
	for _, task := range tasks {
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("seed task: %v", err)
		}
	}
	seedBusinessRun(t, db, "run-yesterday", "task-dev-yesterday", now.Add(-24*time.Hour), 60)
	seedBusinessRun(t, db, "run-today", "task-dev-today", now.Add(-2*time.Hour), 120)
	writebacks := []*model.Writeback{
		{WritebackID: "wb-mr-yesterday", TaskID: "task-dev-yesterday", RunID: "run-yesterday", DedupeKey: "mr:yesterday", Kind: "create_merge_request", ProjectID: "group/repo", Status: model.WritebackStatusSucceeded, CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-24 * time.Hour)},
		{WritebackID: "wb-mr-today", TaskID: "task-dev-today", RunID: "run-today", DedupeKey: "mr:today", Kind: "create_merge_request", ProjectID: "group/repo", Status: model.WritebackStatusSucceeded, CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour)},
		{WritebackID: "wb-takeover-today", TaskID: "task-review-today", RunID: "run-today", DedupeKey: "reviewer-takeover-label:task-review-today", Kind: "add_merge_request_labels", ProjectID: "group/repo", Status: model.WritebackStatusSucceeded, CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour)},
	}
	for _, writeback := range writebacks {
		if err := db.Create(writeback).Error; err != nil {
			t.Fatalf("seed writeback: %v", err)
		}
	}

	got, err := service.BusinessSummary(context.Background(), MetricsFilter{Window: MetricsWindow30Days, Now: now})
	if err != nil {
		t.Fatalf("business summary: %v", err)
	}
	if got.HumanRequiredAddedToday != 1 { // task-human-yesterday 的 updated_at 在昨天，不计今日
		t.Fatalf("unexpected today-added count: %#v", got)
	}
	if len(got.DailyMergeRequestsCreated) != 2 || got.DailyMergeRequestsCreated[0].Date != yesterday || got.DailyMergeRequestsCreated[1].Date != today || got.DailyMergeRequestsCreated[1].Count != 1 {
		t.Fatalf("unexpected daily mrs: %#v", got.DailyMergeRequestsCreated)
	}
	if len(got.DailyReviewsCompleted) != 1 || got.DailyReviewsCompleted[0].Date != today || got.DailyReviewsCompleted[0].Count != 1 {
		t.Fatalf("unexpected daily reviews: %#v", got.DailyReviewsCompleted)
	}
	if len(got.DailyReviewerTakeovers) != 1 || got.DailyReviewerTakeovers[0].Date != today || got.DailyReviewerTakeovers[0].Count != 1 {
		t.Fatalf("unexpected daily takeovers: %#v", got.DailyReviewerTakeovers)
	}
	if len(got.DailyAverageRunSeconds) != 2 || got.DailyAverageRunSeconds[0].Date != yesterday || got.DailyAverageRunSeconds[0].Seconds != 60 || got.DailyAverageRunSeconds[0].SampleCount != 1 || got.DailyAverageRunSeconds[1].Date != today || got.DailyAverageRunSeconds[1].Seconds != 120 {
		t.Fatalf("unexpected daily averages: %#v", got.DailyAverageRunSeconds)
	}
}

func TestBusinessSummaryGroupsByUTCDayAcrossLocalMidnight(t *testing.T) {
	service, db := newQueryServiceTestDB(t)
	now := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)

	tasks := []*model.Task{
		// 本地 8/14 01:00（+08:00）= UTC 8/13 17:00，必须归到 8/13 桶
		{TaskID: "task-local-midnight", DedupeKey: "task-local-midnight", Type: "review_mr", Role: "ReviewerWorker", Status: model.TaskStatusSucceeded, ContextJSON: `{}`, CreatedAt: time.Date(2026, 8, 13, 17, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 8, 13, 17, 0, 0, 0, time.UTC)},
		// 昨日 23:59:59Z 变为 human_required：不计今日新增
		{TaskID: "task-human-yesterday", DedupeKey: "task-human-yesterday", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusHumanRequired, ContextJSON: `{}`, CreatedAt: time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 8, 13, 23, 59, 59, 0, time.UTC)},
		// 今日 00:00:01Z 变为 human_required：计今日新增
		{TaskID: "task-human-today", DedupeKey: "task-human-today", Type: "develop_from_issue", Role: "DeveloperWorker", Status: model.TaskStatusHumanRequired, ContextJSON: `{}`, CreatedAt: time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 8, 14, 0, 0, 1, 0, time.UTC)},
	}
	for _, task := range tasks {
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("seed task: %v", err)
		}
	}

	got, err := service.BusinessSummary(context.Background(), MetricsFilter{Window: MetricsWindow30Days, Now: now})
	if err != nil {
		t.Fatalf("business summary: %v", err)
	}
	dailyReviewsFound := false
	for _, item := range got.DailyReviewsCompleted {
		if item.Date == "2026-08-13" && item.Count == 1 {
			dailyReviewsFound = true
		}
	}
	if !dailyReviewsFound {
		t.Fatalf("local-midnight task must fall into the 2026-08-13 utc bucket: %#v", got.DailyReviewsCompleted)
	}
	if got.HumanRequiredAddedToday != 1 { // 23:59:59Z 不计、00:00:01Z 计
		t.Fatalf("unexpected today-added count across utc midnight: %d", got.HumanRequiredAddedToday)
	}
}

func seedBusinessRun(t *testing.T, db *gorm.DB, runID, taskID string, startedAt time.Time, durationSeconds int) {
	t.Helper()
	completedAt := startedAt.Add(time.Duration(durationSeconds) * time.Second)
	if err := db.Create(&model.Run{RunID: runID, TaskID: taskID, WorkerRole: "DeveloperWorker", Status: model.RunStatusSucceeded, StartedAt: &startedAt, CompletedAt: &completedAt, CreatedAt: startedAt, UpdatedAt: completedAt}).Error; err != nil {
		t.Fatalf("seed run: %v", err)
	}
}

func seedOverviewEntities(t *testing.T, db *gorm.DB, now time.Time) {
	t.Helper()

	entities := []any{
		&model.Task{
			TaskID:    "task-running",
			DedupeKey: "task-running",
			Type:      "develop_from_issue",
			Role:      "DeveloperWorker",
			Status:    model.TaskStatusRunning,
			CreatedAt: now.Add(-8 * time.Minute),
			UpdatedAt: now.Add(-8 * time.Minute),
		},
		&model.Task{
			TaskID:    "task-human",
			DedupeKey: "task-human",
			Type:      "develop_from_issue",
			Role:      "DeveloperWorker",
			Status:    model.TaskStatusHumanRequired,
			LastError: "missing context",
			CreatedAt: now.Add(-7 * time.Minute),
			UpdatedAt: now.Add(-4 * time.Minute),
		},
		&model.Run{
			RunID:        "run-failed",
			TaskID:       "task-running",
			WorkerRole:   "DeveloperWorker",
			Status:       model.RunStatusFailed,
			ErrorMessage: "conversation failed",
			CreatedAt:    now.Add(-6 * time.Minute),
			UpdatedAt:    now.Add(-3 * time.Minute),
		},
		&model.Run{
			RunID:      "run-running",
			TaskID:     "task-running",
			WorkerRole: "DeveloperWorker",
			Status:     model.RunStatusRunning,
			CreatedAt:  now.Add(-5 * time.Minute),
			UpdatedAt:  now.Add(-5 * time.Minute),
		},
		&model.Writeback{
			WritebackID:  "writeback-overview",
			TaskID:       "task-running",
			RunID:        "run-failed",
			DedupeKey:    "writeback-overview",
			Kind:         "create_merge_request",
			ProjectID:    "group/repo",
			Status:       model.WritebackStatusFailed,
			ErrorMessage: "gitlab unavailable",
			CreatedAt:    now.Add(-2 * time.Minute),
			UpdatedAt:    now.Add(-2 * time.Minute),
		},
		&model.Effect{
			EffectID:     "effect-overview",
			TaskID:       "task-running",
			RunID:        "run-failed",
			DedupeKey:    "effect-overview",
			Kind:         "send_im_message",
			Target:       "default",
			Status:       model.EffectStatusFailed,
			ErrorMessage: "im unavailable",
			CreatedAt:    now.Add(-time.Minute),
			UpdatedAt:    now.Add(-time.Minute),
		},
	}

	for _, entity := range entities {
		if err := db.Create(entity).Error; err != nil {
			t.Fatalf("seed overview entity %T: %v", entity, err)
		}
	}
}
