package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"
)

func (service *QueryService) TaskTimeline(ctx context.Context, taskID string) ([]TimelineEvent, error) {
	task, err := service.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	events := []TimelineEvent{newTimelineEvent(
		"timeline:task:"+task.TaskID+":created", task.CreatedAt, "task", "created", "",
		"Task 已创建", task.Type, "task", task.TaskID,
	)}
	if task.UpdatedAt.After(task.CreatedAt) {
		events = append(events, newTimelineEvent(
			"timeline:task:"+task.TaskID+":snapshot", task.UpdatedAt, "task", "state_snapshot", string(task.Status),
			"Task 当前状态", task.LastError, "task", task.TaskID,
		))
	}

	var runs []model.Run
	if err := service.db.WithContext(ctx).Where("task_id = ?", task.TaskID).Find(&runs).Error; err != nil {
		return nil, fmt.Errorf("timeline runs: %w", err)
	}
	for _, run := range runs {
		events = append(events, newTimelineEvent(
			"timeline:run:"+run.RunID+":created", run.CreatedAt, "run", "created", string(run.Status),
			"Run 已创建", run.WorkerRole, "run", run.RunID,
		))
		if run.StartedAt != nil {
			events = append(events, newTimelineEvent(
				"timeline:run:"+run.RunID+":started", *run.StartedAt, "run", "started", string(run.Status),
				"Worker 已开始执行", run.ExternalID, "run", run.RunID,
			))
		}
		if run.CompletedAt != nil {
			events = append(events, newTimelineEvent(
				"timeline:run:"+run.RunID+":completed", *run.CompletedAt, "run", "completed", string(run.Status),
				"Run 已结束", run.ErrorMessage, "run", run.RunID,
			))
		}
	}

	if err := service.appendSubmissionEvents(ctx, task.TaskID, &events); err != nil {
		return nil, err
	}
	if err := service.appendArtifactEvents(ctx, task.TaskID, &events); err != nil {
		return nil, err
	}
	if err := service.appendDependencyEvents(ctx, task.TaskID, &events); err != nil {
		return nil, err
	}
	if err := service.appendWritebackEvents(ctx, task.TaskID, &events); err != nil {
		return nil, err
	}
	if err := service.appendEffectEvents(ctx, task.TaskID, &events); err != nil {
		return nil, err
	}
	if err := service.appendInboundEventEvents(ctx, task.DedupeKey, &events); err != nil {
		return nil, err
	}

	sort.Slice(events, func(i, j int) bool { return timelineEventLess(events[i], events[j]) })
	return events, nil
}

func (service *QueryService) appendSubmissionEvents(ctx context.Context, taskID string, events *[]TimelineEvent) error {
	var submissions []model.ResultSubmission
	if err := service.db.WithContext(ctx).Where("task_id = ?", taskID).Find(&submissions).Error; err != nil {
		return fmt.Errorf("timeline result submissions: %w", err)
	}
	for _, submission := range submissions {
		*events = append(*events, newTimelineEvent(
			"timeline:result_submission:"+submission.SubmissionID+":received", submission.CreatedAt,
			"result_submission", "received", string(submission.Status), "结果已回传", submission.ToolName,
			"result_submission", submission.SubmissionID,
		))
		if submission.CompletedAt != nil {
			*events = append(*events, newTimelineEvent(
				"timeline:result_submission:"+submission.SubmissionID+":completed", *submission.CompletedAt,
				"result_submission", "completed", string(submission.Status), "结果处理完成", submission.ErrorMessage,
				"result_submission", submission.SubmissionID,
			))
		}
	}
	return nil
}

func (service *QueryService) appendArtifactEvents(ctx context.Context, taskID string, events *[]TimelineEvent) error {
	var artifacts []model.Artifact
	if err := service.db.WithContext(ctx).Where("task_id = ?", taskID).Find(&artifacts).Error; err != nil {
		return fmt.Errorf("timeline artifacts: %w", err)
	}
	for _, artifact := range artifacts {
		*events = append(*events, newTimelineEvent(
			"timeline:artifact:"+artifact.ArtifactID+":materialized", artifact.CreatedAt,
			"artifact", "materialized", "", "Artifact 已物化", artifact.Kind+": "+artifact.Name,
			"artifact", artifact.ArtifactID,
		))
	}
	return nil
}

func (service *QueryService) appendDependencyEvents(ctx context.Context, taskID string, events *[]TimelineEvent) error {
	var dependencies []model.Dependency
	if err := service.db.WithContext(ctx).
		Where("parent_task_id = ? OR child_task_id = ?", taskID, taskID).
		Find(&dependencies).Error; err != nil {
		return fmt.Errorf("timeline dependencies: %w", err)
	}
	for _, dependency := range dependencies {
		*events = append(*events, newTimelineEvent(
			"timeline:dependency:"+dependency.DependencyID+":created", dependency.CreatedAt,
			"dependency", "created", string(dependency.Status), "依赖已建立", dependency.Relation,
			"dependency", dependency.DependencyID,
		))
	}
	return nil
}

func (service *QueryService) appendWritebackEvents(ctx context.Context, taskID string, events *[]TimelineEvent) error {
	var writebacks []model.Writeback
	if err := service.db.WithContext(ctx).Where("task_id = ?", taskID).Find(&writebacks).Error; err != nil {
		return fmt.Errorf("timeline writebacks: %w", err)
	}
	for _, writeback := range writebacks {
		*events = append(*events, newTimelineEvent(
			"timeline:writeback:"+writeback.WritebackID+":created", writeback.CreatedAt,
			"writeback", "created", "", "Writeback 已创建", writeback.Kind,
			"writeback", writeback.WritebackID,
		))
		*events = append(*events, newTimelineEvent(
			"timeline:writeback:"+writeback.WritebackID+":snapshot", writeback.UpdatedAt,
			"writeback", "state_snapshot", string(writeback.Status), "Writeback 当前状态", writeback.ErrorMessage,
			"writeback", writeback.WritebackID,
		))
	}
	return nil
}

func (service *QueryService) appendEffectEvents(ctx context.Context, taskID string, events *[]TimelineEvent) error {
	var effects []model.Effect
	if err := service.db.WithContext(ctx).Where("task_id = ?", taskID).Find(&effects).Error; err != nil {
		return fmt.Errorf("timeline effects: %w", err)
	}
	for _, effect := range effects {
		*events = append(*events, newTimelineEvent(
			"timeline:effect:"+effect.EffectID+":created", effect.CreatedAt,
			"effect", "created", "", "Effect 已创建", effect.Kind,
			"effect", effect.EffectID,
		))
		*events = append(*events, newTimelineEvent(
			"timeline:effect:"+effect.EffectID+":snapshot", effect.UpdatedAt,
			"effect", "state_snapshot", string(effect.Status), "Effect 当前状态", effect.ErrorMessage,
			"effect", effect.EffectID,
		))
	}
	return nil
}

func (service *QueryService) appendInboundEventEvents(ctx context.Context, taskDedupeKey string, events *[]TimelineEvent) error {
	var inboundEvents []model.InboundEvent
	if err := service.db.WithContext(ctx).
		Where("normalized_json LIKE ?", "%"+taskDedupeKey+"%").
		Find(&inboundEvents).Error; err != nil {
		return fmt.Errorf("timeline inbound events: %w", err)
	}
	for _, inbound := range inboundEvents {
		if !jsonContainsExactString(inbound.NormalizedJSON, taskDedupeKey) {
			continue
		}
		occurredAt := inbound.ReceivedAt
		if occurredAt.IsZero() {
			occurredAt = inbound.CreatedAt
		}
		*events = append(*events, newTimelineEvent(
			"timeline:inbound_event:"+inbound.EventID+":received", occurredAt,
			"inbound_event", "received", string(inbound.Status), "外部事件已接收", inbound.EventType,
			"inbound_event", inbound.EventID,
		))
	}
	return nil
}

func newTimelineEvent(eventID string, occurredAt time.Time, category, action, status, title, summary, entityType, entityID string) TimelineEvent {
	return TimelineEvent{
		EventID: eventID, OccurredAt: occurredAt, Category: category, Action: action, Status: status,
		Title: title, Summary: summary, EntityType: entityType, EntityID: entityID,
	}
}

func timelineEventLess(left, right TimelineEvent) bool {
	if !left.OccurredAt.Equal(right.OccurredAt) {
		return left.OccurredAt.Before(right.OccurredAt)
	}
	if left.Category != right.Category {
		return left.Category < right.Category
	}
	return left.EventID < right.EventID
}

func jsonContainsExactString(rawJSON, expected string) bool {
	if strings.TrimSpace(rawJSON) == "" {
		return false
	}
	var value any
	if err := json.Unmarshal([]byte(rawJSON), &value); err != nil {
		return false
	}
	return containsExactString(value, expected)
}

func containsExactString(value any, expected string) bool {
	switch typed := value.(type) {
	case string:
		return typed == expected
	case []any:
		for _, item := range typed {
			if containsExactString(item, expected) {
				return true
			}
		}
	case map[string]any:
		for _, item := range typed {
			if containsExactString(item, expected) {
				return true
			}
		}
	}
	return false
}
