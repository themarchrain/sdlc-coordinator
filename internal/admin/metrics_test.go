package admin

import (
	"context"
	"math"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
)

func TestInterruptedRunCountsAsCompleted(t *testing.T) {
	if !isCompletedRunStatus(model.RunStatusInterrupted) {
		t.Fatal("interrupted run excluded from completed sample")
	}
}

func TestParseMetricsWindow(t *testing.T) {
	for _, test := range []struct {
		raw  string
		want MetricsWindow
	}{
		{raw: "", want: MetricsWindow30Days},
		{raw: "7d", want: MetricsWindow7Days},
		{raw: "30d", want: MetricsWindow30Days},
		{raw: "all", want: MetricsWindowAll},
	} {
		got, err := ParseMetricsWindow(test.raw)
		if err != nil || got != test.want {
			t.Fatalf("parse %q: got=%q err=%v", test.raw, got, err)
		}
	}
	if _, err := ParseMetricsWindow("90d"); err == nil {
		t.Fatal("expected unsupported metrics window to fail")
	}
}

func TestAutomationMetricsUsesExistingTaskAndRunFacts(t *testing.T) {
	service, db := newQueryServiceTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)

	seedTasks(t, db,
		metricTask("task-auto", "develop_from_issue", model.TaskStatusSucceeded, now.Add(-5*24*time.Hour)),
		metricTask("task-recovered", "develop_from_issue", model.TaskStatusSucceeded, now.Add(-4*24*time.Hour)),
		metricTask("task-human", "develop_from_issue", model.TaskStatusHumanRequired, now.Add(-3*24*time.Hour)),
		metricTask("task-running", "develop_from_issue", model.TaskStatusRunning, now.Add(-2*24*time.Hour)),
		metricTask("task-review", "review_mr", model.TaskStatusSucceeded, now.Add(-24*time.Hour)),
		metricTask("task-old", "develop_from_issue", model.TaskStatusSucceeded, now.Add(-40*24*time.Hour)),
	)
	seedRuns(t, db,
		metricRun("run-auto", "task-auto", "DeveloperWorker", model.RunStatusSucceeded, now.Add(-5*24*time.Hour), 60),
		metricRun("run-recovered-1", "task-recovered", "DeveloperWorker", model.RunStatusBlocked, now.Add(-4*24*time.Hour), 300),
		metricRun("run-recovered-2", "task-recovered", "DeveloperWorker", model.RunStatusSucceeded, now.Add(-4*24*time.Hour+10*time.Minute), 120),
		metricRun("run-human", "task-human", "DeveloperWorker", model.RunStatusBlocked, now.Add(-3*24*time.Hour), 180),
		metricRunningRun("run-running", "task-running", "DeveloperWorker", now.Add(-2*24*time.Hour)),
		metricRun("run-review", "task-review", "ReviewerWorker", model.RunStatusSucceeded, now.Add(-24*time.Hour), 30),
		metricRun("run-old", "task-old", "DeveloperWorker", model.RunStatusSucceeded, now.Add(-40*24*time.Hour), 10),
	)

	got, err := service.Metrics(ctx, MetricsFilter{Window: MetricsWindow30Days, Now: now})
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	if got.TaskSampleCount != 5 || got.Window != MetricsWindow30Days || !got.GeneratedAt.Equal(now) {
		t.Fatalf("unexpected metrics header: %#v", got)
	}
	assertRatio(t, got.CurrentSuccess, 3, 5)
	assertRatio(t, got.FirstRunSuccess, 2, 4)
	assertRatio(t, got.InterventionTriggered, 2, 5)
	assertRatio(t, got.RecoveredAfterBlock, 1, 2)
	if len(got.SuccessfulRunDurations) != 2 {
		t.Fatalf("unexpected duration groups: %#v", got.SuccessfulRunDurations)
	}
	developer := got.SuccessfulRunDurations[0]
	if developer.WorkerRole != "DeveloperWorker" || developer.SampleCount != 2 || developer.AverageSeconds != 90 || developer.P50Seconds != 60 || developer.P90Seconds != 120 {
		t.Fatalf("unexpected developer duration: %#v", developer)
	}
	reviewer := got.SuccessfulRunDurations[1]
	if reviewer.WorkerRole != "ReviewerWorker" || reviewer.SampleCount != 1 || reviewer.AverageSeconds != 30 || reviewer.P50Seconds != 30 || reviewer.P90Seconds != 30 {
		t.Fatalf("unexpected reviewer duration: %#v", reviewer)
	}

	developOnly, err := service.Metrics(ctx, MetricsFilter{Window: MetricsWindow30Days, Type: " develop_from_issue ", Now: now})
	if err != nil {
		t.Fatalf("filtered metrics: %v", err)
	}
	if developOnly.TaskSampleCount != 4 || developOnly.Type != "develop_from_issue" || len(developOnly.SuccessfulRunDurations) != 1 {
		t.Fatalf("unexpected filtered metrics: %#v", developOnly)
	}
	assertRatio(t, developOnly.CurrentSuccess, 2, 4)
}

func TestAutomationMetricsReturnsNullRatiosForEmptySample(t *testing.T) {
	service, _ := newQueryServiceTestDB(t)
	now := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	got, err := service.Metrics(context.Background(), MetricsFilter{Window: MetricsWindow7Days, Now: now})
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	for name, metric := range map[string]RatioMetric{
		"current": got.CurrentSuccess, "first": got.FirstRunSuccess,
		"intervention": got.InterventionTriggered, "recovery": got.RecoveredAfterBlock,
	} {
		if metric.Numerator != 0 || metric.Denominator != 0 || metric.Value != nil {
			t.Fatalf("%s ratio: %#v", name, metric)
		}
	}
	if got.SuccessfulRunDurations == nil || len(got.SuccessfulRunDurations) != 0 {
		t.Fatalf("expected an empty duration list, got %#v", got.SuccessfulRunDurations)
	}
}

func TestAutomationMetricsDailySeriesGroupsByTaskCreationDay(t *testing.T) {
	service, db := newQueryServiceTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)

	seedTasks(t, db,
		metricTask("task-day1", "develop_from_issue", model.TaskStatusSucceeded, now.Add(-5*24*time.Hour)),
		metricTask("task-day2", "develop_from_issue", model.TaskStatusSucceeded, now.Add(-4*24*time.Hour)),
		metricTask("task-day3", "develop_from_issue", model.TaskStatusHumanRequired, now.Add(-3*24*time.Hour)),
		metricTask("task-day4", "develop_from_issue", model.TaskStatusRunning, now.Add(-2*24*time.Hour)),
		metricTask("task-day5", "review_mr", model.TaskStatusSucceeded, now.Add(-24*time.Hour)),
	)
	seedRuns(t, db,
		metricRun("run-day1", "task-day1", "DeveloperWorker", model.RunStatusSucceeded, now.Add(-5*24*time.Hour), 60),
		metricRun("run-day2-1", "task-day2", "DeveloperWorker", model.RunStatusBlocked, now.Add(-4*24*time.Hour), 300),
		metricRun("run-day2-2", "task-day2", "DeveloperWorker", model.RunStatusSucceeded, now.Add(-4*24*time.Hour+10*time.Minute), 120),
		metricRun("run-day3", "task-day3", "DeveloperWorker", model.RunStatusBlocked, now.Add(-3*24*time.Hour), 180),
		metricRunningRun("run-day4", "task-day4", "DeveloperWorker", now.Add(-2*24*time.Hour)),
		metricRun("run-day5", "task-day5", "ReviewerWorker", model.RunStatusSucceeded, now.Add(-24*time.Hour), 30),
	)

	got, err := service.Metrics(ctx, MetricsFilter{Window: MetricsWindow30Days, Now: now})
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	if len(got.DailySeries) != 5 {
		t.Fatalf("expected five daily points, got %#v", got.DailySeries)
	}
	if got.DailySeries[0].Date != "2026-08-04" || got.DailySeries[4].Date != "2026-08-08" {
		t.Fatalf("unexpected daily date order: %#v", got.DailySeries)
	}
	day1 := got.DailySeries[0]
	assertFloatPtr(t, day1.CurrentSuccess, 1)
	assertFloatPtr(t, day1.FirstRunSuccess, 1)
	assertFloatPtr(t, day1.InterventionTriggered, 0)
	if day1.RecoveredAfterBlock != nil {
		t.Fatalf("day1 recovered must be nil (no blocked run): %#v", day1.RecoveredAfterBlock)
	}
	day2 := got.DailySeries[1]
	assertFloatPtr(t, day2.CurrentSuccess, 1)
	assertFloatPtr(t, day2.FirstRunSuccess, 0) // 首次 run 是 blocked
	assertFloatPtr(t, day2.InterventionTriggered, 1)
	assertFloatPtr(t, day2.RecoveredAfterBlock, 1)
	day4 := got.DailySeries[3]
	assertFloatPtr(t, day4.CurrentSuccess, 0)
	if day4.FirstRunSuccess != nil {
		t.Fatalf("day4 first-run must be nil (no completed run): %#v", day4.FirstRunSuccess)
	}
	if day4.RecoveredAfterBlock != nil {
		t.Fatalf("day4 recovered must be nil (no blocked run): %#v", day4.RecoveredAfterBlock)
	}
	day5 := got.DailySeries[4]
	assertFloatPtr(t, day5.CurrentSuccess, 1)
	assertFloatPtr(t, day5.FirstRunSuccess, 1)
}

func assertFloatPtr(t *testing.T, value *float64, want float64) {
	t.Helper()
	if value == nil || math.Abs(*value-want) > 1e-9 {
		t.Fatalf("float pointer: got=%v want=%f", value, want)
	}
}

func metricTask(taskID, taskType string, status model.TaskStatus, createdAt time.Time) model.Task {
	return model.Task{TaskID: taskID, DedupeKey: taskID, Type: taskType, Role: "DeveloperWorker", Status: status, CreatedAt: createdAt, UpdatedAt: createdAt}
}

func metricRun(runID, taskID, role string, status model.RunStatus, startedAt time.Time, durationSeconds int) model.Run {
	completedAt := startedAt.Add(time.Duration(durationSeconds) * time.Second)
	return model.Run{RunID: runID, TaskID: taskID, WorkerRole: role, Status: status, StartedAt: &startedAt, CompletedAt: &completedAt, CreatedAt: startedAt, UpdatedAt: completedAt}
}

func metricRunningRun(runID, taskID, role string, startedAt time.Time) model.Run {
	return model.Run{RunID: runID, TaskID: taskID, WorkerRole: role, Status: model.RunStatusRunning, StartedAt: &startedAt, CreatedAt: startedAt, UpdatedAt: startedAt}
}

func assertRatio(t *testing.T, got RatioMetric, numerator, denominator int64) {
	t.Helper()
	if got.Numerator != numerator || got.Denominator != denominator || got.Value == nil {
		t.Fatalf("ratio: got=%#v want=%d/%d", got, numerator, denominator)
	}
	want := float64(numerator) / float64(denominator)
	if math.Abs(*got.Value-want) > 1e-9 {
		t.Fatalf("ratio value: got=%f want=%f", *got.Value, want)
	}
}
