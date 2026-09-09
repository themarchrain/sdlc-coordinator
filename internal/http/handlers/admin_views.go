package handlers

import (
	"strings"
	"time"

	"sdlc-coordinator/internal/admin"
	"sdlc-coordinator/internal/model"

	"github.com/gin-gonic/gin"
)

// TaskResponse 是任务在 admin 操作与 webhook 响应里的序列化形态。
type TaskResponse struct {
	TaskID             string           `json:"task_id"`
	DedupeKey          string           `json:"dedupe_key"`
	Type               string           `json:"type"`
	Role               string           `json:"role"`
	SourceRepo         string           `json:"source_repo,omitempty"`
	TargetRepo         string           `json:"target_repo,omitempty"`
	Objective          string           `json:"objective"`
	AcceptanceCriteria string           `json:"acceptance_criteria,omitempty"`
	Status             model.TaskStatus `json:"status"`
	LeaseUntil         *time.Time       `json:"lease_until,omitempty"`
	AttemptCount       int              `json:"attempt_count"`
	CurrentRunID       *string          `json:"current_run_id,omitempty"`
	ContextJSON        string           `json:"context_json,omitempty"`
	CallbackURL        string           `json:"callback_url,omitempty"`
	LastError          string           `json:"last_error,omitempty"`
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
}

func taskResponse(task *model.Task) TaskResponse {
	return TaskResponse{
		TaskID:             task.TaskID,
		DedupeKey:          task.DedupeKey,
		Type:               task.Type,
		Role:               task.Role,
		SourceRepo:         task.SourceRepo,
		TargetRepo:         task.TargetRepo,
		Objective:          task.Objective,
		AcceptanceCriteria: task.AcceptanceCriteria,
		Status:             task.Status,
		LeaseUntil:         task.LeaseUntil,
		AttemptCount:       task.AttemptCount,
		CurrentRunID:       task.CurrentRunID,
		ContextJSON:        task.ContextJSON,
		CallbackURL:        task.CallbackURL,
		LastError:          task.LastError,
		CreatedAt:          task.CreatedAt,
		UpdatedAt:          task.UpdatedAt,
	}
}

func runSummary(run model.Run) gin.H {
	return gin.H{"run_id": run.RunID, "task_id": run.TaskID, "worker_role": run.WorkerRole, "status": run.Status,
		"external_system": run.ExternalSystem, "external_id": run.ExternalID, "external_url": run.ExternalURL,
		"error_message": run.ErrorMessage, "started_at": run.StartedAt, "completed_at": run.CompletedAt,
		"created_at": run.CreatedAt, "updated_at": run.UpdatedAt}
}

func runDetail(run model.Run) gin.H {
	result := runSummary(run)
	result["input_json"] = run.InputJSON
	result["output_json"] = run.OutputJSON
	return result
}

func artifactSummary(artifact model.Artifact) gin.H {
	return gin.H{"artifact_id": artifact.ArtifactID, "task_id": artifact.TaskID, "run_id": artifact.RunID,
		"kind": artifact.Kind, "name": artifact.Name, "url": artifact.URL,
		"created_at": artifact.CreatedAt, "updated_at": artifact.UpdatedAt}
}

func artifactDetail(artifact model.Artifact) gin.H {
	result := artifactSummary(artifact)
	result["metadata_json"] = artifact.MetadataJSON
	return result
}

func dependencySummary(dependency model.Dependency) gin.H {
	return gin.H{"dependency_id": dependency.DependencyID, "parent_task_id": dependency.ParentTaskID,
		"child_task_id": dependency.ChildTaskID, "relation": dependency.Relation, "status": dependency.Status,
		"parent_repository_id": dependency.ParentRepositoryID, "child_repository_id": dependency.ChildRepositoryID,
		"created_at": dependency.CreatedAt, "updated_at": dependency.UpdatedAt}
}

func dependencyDetail(dependency model.Dependency) gin.H {
	result := dependencySummary(dependency)
	result["metadata_json"] = dependency.MetadataJSON
	return result
}

func writebackSummary(writeback model.Writeback) gin.H {
	return gin.H{"writeback_id": writeback.WritebackID, "task_id": writeback.TaskID, "run_id": writeback.RunID,
		"dedupe_key": writeback.DedupeKey, "kind": writeback.Kind, "project_id": writeback.ProjectID,
		"target_type": writeback.TargetType, "target_iid": writeback.TargetIID, "status": writeback.Status,
		"attempt_count": writeback.AttemptCount, "reconcile_attempt_count": writeback.ReconcileAttemptCount,
		"last_reconciled_at": writeback.LastReconciledAt, "external_id": writeback.ExternalID,
		"external_url": writeback.ExternalURL, "error_message": writeback.ErrorMessage,
		"created_at": writeback.CreatedAt, "updated_at": writeback.UpdatedAt}
}

func writebackDetail(writeback model.Writeback) gin.H {
	result := writebackSummary(writeback)
	result["operation_marker"] = writeback.OperationMarker
	result["request_json"] = writeback.RequestJSON
	result["response_json"] = writeback.ResponseJSON
	return result
}

func effectSummary(effect model.Effect) gin.H {
	return gin.H{"effect_id": effect.EffectID, "task_id": effect.TaskID, "run_id": effect.RunID,
		"dedupe_key": effect.DedupeKey, "kind": effect.Kind, "target": effect.Target, "critical": effect.Critical,
		"status": effect.Status, "attempt_count": effect.AttemptCount, "error_message": effect.ErrorMessage,
		"created_at": effect.CreatedAt, "updated_at": effect.UpdatedAt}
}

func effectDetail(effect model.Effect) gin.H {
	result := effectSummary(effect)
	result["request_json"] = effect.RequestJSON
	result["response_json"] = effect.ResponseJSON
	return result
}

func inboundEventSummary(event model.InboundEvent) gin.H {
	return gin.H{"event_id": event.EventID, "dedupe_key": event.DedupeKey, "source": event.Source,
		"event_type": event.EventType, "event_action": event.EventAction, "external_id": event.ExternalID,
		"repository_id": event.RepositoryID,
		"status":        event.Status, "error_message": event.ErrorMessage, "received_at": event.ReceivedAt,
		"created_at": event.CreatedAt, "updated_at": event.UpdatedAt}
}

func inboundEventDetail(event model.InboundEvent) gin.H {
	result := inboundEventSummary(event)
	result["payload_json"] = event.PayloadJSON
	result["normalized_json"] = event.NormalizedJSON
	return result
}

func resultSubmissionSummary(submission model.ResultSubmission) gin.H {
	return gin.H{"submission_id": submission.SubmissionID, "task_id": submission.TaskID, "run_id": submission.RunID,
		"source": submission.Source, "tool_name": submission.ToolName, "role": submission.Role,
		"payload_hash": submission.PayloadHash, "status": submission.Status, "attempt_count": submission.AttemptCount,
		"normalization_status": normalizationStatus(submission), "lease_until": submission.LeaseUntil,
		"error_message": submission.ErrorMessage,
		"completed_at":  submission.CompletedAt, "created_at": submission.CreatedAt, "updated_at": submission.UpdatedAt}
}

// normalizationStatus reports whether the thin normalizer rewrote the
// payload: "clean" (byte identical) or "normalized" (repairs recorded).
func normalizationStatus(submission model.ResultSubmission) string {
	if strings.TrimSpace(submission.NormalizationJSON) == "" {
		return "clean"
	}
	return "normalized"
}

func resultSubmissionDetail(submission model.ResultSubmission) gin.H {
	result := resultSubmissionSummary(submission)
	result["raw_payload_json"] = submission.RawPayloadJSON
	result["canonical_json"] = submission.CanonicalJSON
	result["normalization_json"] = submission.NormalizationJSON
	return result
}

func overviewResponse(overview admin.Overview, business admin.BusinessSummary) gin.H {
	failures := make([]gin.H, 0, len(overview.RecentFailures))
	for _, failure := range overview.RecentFailures {
		failures = append(failures, gin.H{"entity_type": failure.EntityType, "entity_id": failure.EntityID,
			"task_id": failure.TaskID, "run_id": failure.RunID, "status": failure.Status,
			"error": failure.Error, "occurred_at": failure.OccurredAt})
	}
	return gin.H{"tasks_by_status": overview.TasksByStatus, "runs_by_status": overview.RunsByStatus,
		"writebacks_by_status": overview.WritebacksByStatus, "effects_by_status": overview.EffectsByStatus,
		"recent_failures": failures, "business_summary": businessSummaryResponse(business), "generated_at": overview.GeneratedAt}
}

func businessSummaryResponse(summary admin.BusinessSummary) gin.H {
	return gin.H{
		"window":                 summary.Window,
		"merge_requests_created": summary.MergeRequestsCreated, "reviews_completed": summary.ReviewsCompleted,
		"reviewer_takeovers": summary.ReviewerTakeovers, "automation_success": ratioMetricResponse(summary.AutomationSuccess),
		"average_successful_run_seconds": summary.AverageSuccessfulRunSeconds,
		"human_required_tasks":           summary.HumanRequiredTasks,
		"human_required_added_today":     summary.HumanRequiredAddedToday,
		"daily_merge_requests_created":   dailyCountsResponse(summary.DailyMergeRequestsCreated),
		"daily_reviews_completed":        dailyCountsResponse(summary.DailyReviewsCompleted),
		"daily_reviewer_takeovers":       dailyCountsResponse(summary.DailyReviewerTakeovers),
		"daily_average_run_seconds":      dailyAveragesResponse(summary.DailyAverageRunSeconds),
		"generated_at":                   summary.GeneratedAt,
	}
}

func dailyCountsResponse(counts []admin.DailyCount) []gin.H {
	result := make([]gin.H, 0, len(counts))
	for _, count := range counts {
		result = append(result, gin.H{"date": count.Date, "count": count.Count})
	}
	return result
}

func dailyAveragesResponse(averages []admin.DailyAverageSeconds) []gin.H {
	result := make([]gin.H, 0, len(averages))
	for _, average := range averages {
		result = append(result, gin.H{"date": average.Date, "seconds": average.Seconds, "sample_count": average.SampleCount})
	}
	return result
}

func automationMetricsResponse(metrics admin.AutomationMetrics) gin.H {
	durations := make([]gin.H, 0, len(metrics.SuccessfulRunDurations))
	for _, duration := range metrics.SuccessfulRunDurations {
		durations = append(durations, gin.H{
			"worker_role": duration.WorkerRole, "sample_count": duration.SampleCount,
			"average_seconds": duration.AverageSeconds, "p50_seconds": duration.P50Seconds,
			"p90_seconds": duration.P90Seconds,
		})
	}
	daily := make([]gin.H, 0, len(metrics.DailySeries))
	for _, point := range metrics.DailySeries {
		daily = append(daily, gin.H{
			"date": point.Date, "current_success": point.CurrentSuccess, "first_run_success": point.FirstRunSuccess,
			"intervention_triggered": point.InterventionTriggered, "recovered_after_block": point.RecoveredAfterBlock,
		})
	}
	return gin.H{
		"window": metrics.Window, "type": metrics.Type, "task_sample_count": metrics.TaskSampleCount,
		"current_success":          ratioMetricResponse(metrics.CurrentSuccess),
		"first_run_success":        ratioMetricResponse(metrics.FirstRunSuccess),
		"intervention_triggered":   ratioMetricResponse(metrics.InterventionTriggered),
		"recovered_after_block":    ratioMetricResponse(metrics.RecoveredAfterBlock),
		"successful_run_durations": durations, "daily_series": daily, "generated_at": metrics.GeneratedAt,
	}
}

func ratioMetricResponse(metric admin.RatioMetric) gin.H {
	return gin.H{"numerator": metric.Numerator, "denominator": metric.Denominator, "value": metric.Value}
}

func timelineEventResponse(event admin.TimelineEvent) gin.H {
	return gin.H{"event_id": event.EventID, "occurred_at": event.OccurredAt, "category": event.Category,
		"action": event.Action, "status": event.Status, "title": event.Title, "summary": event.Summary,
		"entity_type": event.EntityType, "entity_id": event.EntityID}
}
