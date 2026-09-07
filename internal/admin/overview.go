package admin

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"
)

const recentFailureLimit = 20

type statusCount struct {
	Status string
	Count  int64
}

func (service *QueryService) Overview(ctx context.Context, filter ...OverviewFilter) (Overview, error) {
	repositoryID := ""
	if len(filter) > 0 {
		repositoryID = strings.TrimSpace(filter[0].RepositoryID)
	}
	if err := service.ensureRepository(ctx, repositoryID); err != nil {
		return Overview{}, err
	}
	tasksByStatus, err := service.countStatuses(ctx, &model.Task{}, repositoryID)
	if err != nil {
		return Overview{}, fmt.Errorf("count task statuses: %w", err)
	}
	runsByStatus, err := service.countStatuses(ctx, &model.Run{}, repositoryID)
	if err != nil {
		return Overview{}, fmt.Errorf("count run statuses: %w", err)
	}
	writebacksByStatus, err := service.countStatuses(ctx, &model.Writeback{}, repositoryID)
	if err != nil {
		return Overview{}, fmt.Errorf("count writeback statuses: %w", err)
	}
	effectsByStatus, err := service.countStatuses(ctx, &model.Effect{}, repositoryID)
	if err != nil {
		return Overview{}, fmt.Errorf("count effect statuses: %w", err)
	}

	recentFailures, err := service.recentFailures(ctx, repositoryID)
	if err != nil {
		return Overview{}, err
	}

	submissionsByNormalization, err := service.countSubmissionNormalizations(ctx, repositoryID)
	if err != nil {
		return Overview{}, fmt.Errorf("count submission normalizations: %w", err)
	}

	return Overview{
		TasksByStatus:              tasksByStatus,
		RunsByStatus:               runsByStatus,
		WritebacksByStatus:         writebacksByStatus,
		EffectsByStatus:            effectsByStatus,
		SubmissionsByNormalization: submissionsByNormalization,
		RecentFailures:             recentFailures,
		GeneratedAt:                time.Now().UTC(),
	}, nil
}

// countSubmissionNormalizations buckets accepted submissions by whether the
// thin normalizer touched them, plus rejected validation attempts (which
// never created a submission row).
func (service *QueryService) countSubmissionNormalizations(ctx context.Context, repositoryID string) (map[string]int64, error) {
	submissionQuery := applyJoinedTaskRepositoryFilter(service.db.WithContext(ctx).Model(&model.ResultSubmission{}), repositoryID)
	var rows []struct {
		Bucket string
		Count  int64
	}
	if err := submissionQuery.
		Select("CASE WHEN normalization_json IS NULL OR normalization_json = '' THEN 'clean' ELSE 'normalized' END AS bucket, COUNT(*) AS count").
		Group("bucket").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := map[string]int64{"clean": 0, "normalized": 0, "rejected": 0}
	for _, row := range rows {
		result[row.Bucket] = row.Count
	}
	rejectionQuery := service.db.WithContext(ctx).Model(&model.ResultSubmissionRejection{})
	if value := strings.TrimSpace(repositoryID); value != "" {
		rejectionQuery = rejectionQuery.Where("task_id IN (SELECT task_id FROM tasks WHERE repository_id = ?)", value)
	}
	var rejected int64
	if err := rejectionQuery.Count(&rejected).Error; err != nil {
		return nil, err
	}
	result["rejected"] = rejected
	return result, nil
}

func (service *QueryService) countStatuses(ctx context.Context, entity any, repositoryID string) (map[string]int64, error) {
	query := service.db.WithContext(ctx).Model(entity)
	switch entity.(type) {
	case *model.Task:
		query = applyDirectRepositoryFilter(query, repositoryID)
	default:
		query = applyJoinedTaskRepositoryFilter(query, repositoryID)
	}
	var rows []statusCount
	if err := query.
		Select("status, COUNT(*) AS count").
		Group("status").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[string]int64, len(rows))
	for _, row := range rows {
		result[row.Status] = row.Count
	}
	return result, nil
}

func (service *QueryService) recentFailures(ctx context.Context, repositoryID string) ([]FailureSummary, error) {
	failures := make([]FailureSummary, 0)

	var tasks []model.Task
	taskQuery := applyDirectRepositoryFilter(service.db.WithContext(ctx).Model(&model.Task{}), repositoryID)
	if err := taskQuery.
		Where("status IN ?", []model.TaskStatus{model.TaskStatusBlocked, model.TaskStatusHumanRequired}).
		Order("updated_at DESC").
		Limit(recentFailureLimit).
		Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("list recent failed tasks: %w", err)
	}
	for _, task := range tasks {
		failures = append(failures, FailureSummary{
			EntityType: "task", EntityID: task.TaskID, TaskID: task.TaskID,
			Status: string(task.Status), Error: task.LastError, OccurredAt: task.UpdatedAt,
		})
	}

	var runs []model.Run
	runQuery := applyJoinedTaskRepositoryFilter(service.db.WithContext(ctx).Model(&model.Run{}), repositoryID)
	if err := runQuery.
		Where("status IN ?", []model.RunStatus{model.RunStatusFailed, model.RunStatusBlocked}).
		Order("updated_at DESC").
		Limit(recentFailureLimit).
		Find(&runs).Error; err != nil {
		return nil, fmt.Errorf("list recent failed runs: %w", err)
	}
	for _, run := range runs {
		failures = append(failures, FailureSummary{
			EntityType: "run", EntityID: run.RunID, TaskID: run.TaskID, RunID: run.RunID,
			Status: string(run.Status), Error: run.ErrorMessage, OccurredAt: run.UpdatedAt,
		})
	}

	var writebacks []model.Writeback
	writebackQuery := applyJoinedTaskRepositoryFilter(service.db.WithContext(ctx).Model(&model.Writeback{}), repositoryID)
	if err := writebackQuery.
		Where("status IN ?", []model.WritebackStatus{model.WritebackStatusFailed, model.WritebackStatusUnknown}).
		Order("updated_at DESC").
		Limit(recentFailureLimit).
		Find(&writebacks).Error; err != nil {
		return nil, fmt.Errorf("list recent failed writebacks: %w", err)
	}
	for _, writeback := range writebacks {
		failures = append(failures, FailureSummary{
			EntityType: "writeback", EntityID: writeback.WritebackID, TaskID: writeback.TaskID, RunID: writeback.RunID,
			Status: string(writeback.Status), Error: writeback.ErrorMessage, OccurredAt: writeback.UpdatedAt,
		})
	}

	var effects []model.Effect
	effectQuery := applyJoinedTaskRepositoryFilter(service.db.WithContext(ctx).Model(&model.Effect{}), repositoryID)
	if err := effectQuery.
		Where("status = ?", model.EffectStatusFailed).
		Order("updated_at DESC").
		Limit(recentFailureLimit).
		Find(&effects).Error; err != nil {
		return nil, fmt.Errorf("list recent failed effects: %w", err)
	}
	for _, effect := range effects {
		failures = append(failures, FailureSummary{
			EntityType: "effect", EntityID: effect.EffectID, TaskID: effect.TaskID, RunID: effect.RunID,
			Status: string(effect.Status), Error: effect.ErrorMessage, OccurredAt: effect.UpdatedAt,
		})
	}

	var submissions []model.ResultSubmission
	submissionQuery := applyJoinedTaskRepositoryFilter(service.db.WithContext(ctx).Model(&model.ResultSubmission{}), repositoryID)
	if err := submissionQuery.
		Where("status IN ?", []model.ResultSubmissionStatus{model.ResultSubmissionStatusFailed, model.ResultSubmissionStatusUnknown}).
		Order("updated_at DESC").
		Limit(recentFailureLimit).
		Find(&submissions).Error; err != nil {
		return nil, fmt.Errorf("list recent failed result submissions: %w", err)
	}
	for _, submission := range submissions {
		failures = append(failures, FailureSummary{
			EntityType: "result_submission", EntityID: submission.SubmissionID, TaskID: submission.TaskID, RunID: submission.RunID,
			Status: string(submission.Status), Error: submission.ErrorMessage, OccurredAt: submission.UpdatedAt,
		})
	}

	sort.Slice(failures, func(i, j int) bool {
		if !failures[i].OccurredAt.Equal(failures[j].OccurredAt) {
			return failures[i].OccurredAt.After(failures[j].OccurredAt)
		}
		if failures[i].EntityType != failures[j].EntityType {
			return failures[i].EntityType < failures[j].EntityType
		}
		return failures[i].EntityID < failures[j].EntityID
	})
	if len(failures) > recentFailureLimit {
		failures = failures[:recentFailureLimit]
	}
	return failures, nil
}
