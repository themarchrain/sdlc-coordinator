package admin

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

func TestTaskTimelineCombinesRelatedEntitiesInStableOrder(t *testing.T) {
	service, db := newQueryServiceTestDB(t)
	seedTimelineEntities(t, db, "task-timeline")

	events, err := service.TaskTimeline(context.Background(), "task-timeline")
	if err != nil {
		t.Fatalf("task timeline: %v", err)
	}
	categories := make(map[string]bool)
	for index, event := range events {
		categories[event.Category] = true
		if index > 0 && timelineEventLess(event, events[index-1]) {
			t.Fatalf("timeline is not stably ordered: %#v", events)
		}
	}
	for _, category := range []string{"task", "inbound_event", "run", "result_submission", "artifact", "dependency", "writeback", "effect"} {
		if !categories[category] {
			t.Fatalf("missing category %q in %#v", category, events)
		}
	}
	for _, event := range events {
		if event.EntityID == "event-substring" {
			t.Fatalf("substring-only inbound event must not be included: %#v", events)
		}
	}
}

func TestTaskTimelineSeparatesCreationFromCurrentTerminalState(t *testing.T) {
	service, db := newQueryServiceTestDB(t)
	seedTimelineEntities(t, db, "task-timeline")

	events, err := service.TaskTimeline(context.Background(), "task-timeline")
	if err != nil {
		t.Fatalf("task timeline: %v", err)
	}

	assertTimelineStateTransition(t, events, "writeback-timeline", "succeeded")
	assertTimelineStateTransition(t, events, "effect-timeline", "succeeded")
}

func assertTimelineStateTransition(t *testing.T, events []TimelineEvent, entityID, terminalStatus string) {
	t.Helper()
	createdWithoutTerminalState := false
	terminalSnapshot := false
	for _, event := range events {
		if event.EntityID != entityID {
			continue
		}
		if event.Action == "created" && event.Status == "" {
			createdWithoutTerminalState = true
		}
		if event.Action == "state_snapshot" && event.Status == terminalStatus {
			terminalSnapshot = true
		}
	}
	if !createdWithoutTerminalState || !terminalSnapshot {
		t.Fatalf("expected creation and terminal snapshot for %s, got %#v", entityID, events)
	}
}

func seedTimelineEntities(t *testing.T, db *gorm.DB, taskID string) {
	t.Helper()

	base := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	startedAt := base.Add(2 * time.Minute)
	completedAt := base.Add(7 * time.Minute)
	submissionCompletedAt := base.Add(6 * time.Minute)
	entities := []any{
		&model.Task{
			TaskID:    taskID,
			DedupeKey: "timeline-task",
			Type:      "develop_from_issue",
			Role:      "DeveloperWorker",
			Status:    model.TaskStatusSucceeded,
			CreatedAt: base,
			UpdatedAt: base.Add(9 * time.Minute),
		},
		&model.Run{
			RunID:       "run-timeline",
			TaskID:      taskID,
			WorkerRole:  "DeveloperWorker",
			Status:      model.RunStatusSucceeded,
			StartedAt:   &startedAt,
			CompletedAt: &completedAt,
			CreatedAt:   base.Add(time.Minute),
			UpdatedAt:   completedAt,
		},
		&model.ResultSubmission{
			SubmissionID:   "submission-timeline",
			TaskID:         taskID,
			RunID:          "run-timeline",
			Source:         "mcp",
			ToolName:       "submit_developer_result",
			Role:           "DeveloperWorker",
			PayloadHash:    "timeline-hash",
			RawPayloadJSON: `{}`,
			CanonicalJSON:  `{}`,
			Status:         model.ResultSubmissionStatusSucceeded,
			CompletedAt:    &submissionCompletedAt,
			CreatedAt:      base.Add(5 * time.Minute),
			UpdatedAt:      submissionCompletedAt,
		},
		&model.Artifact{
			ArtifactID: "artifact-timeline",
			TaskID:     taskID,
			RunID:      "run-timeline",
			Kind:       "diff",
			Name:       "change.patch",
			CreatedAt:  base.Add(6 * time.Minute),
			UpdatedAt:  base.Add(6 * time.Minute),
		},
		&model.Dependency{
			DependencyID: "dependency-timeline",
			ParentTaskID: taskID,
			ChildTaskID:  "task-child",
			Relation:     "upstream_fix",
			Status:       model.DependencyStatusResolved,
			CreatedAt:    base.Add(3 * time.Minute),
			UpdatedAt:    base.Add(4 * time.Minute),
		},
		&model.Writeback{
			WritebackID: "writeback-timeline",
			TaskID:      taskID,
			RunID:       "run-timeline",
			DedupeKey:   "writeback-timeline",
			Kind:        "create_merge_request",
			ProjectID:   "group/repo",
			Status:      model.WritebackStatusSucceeded,
			CreatedAt:   base.Add(7 * time.Minute),
			UpdatedAt:   base.Add(8 * time.Minute),
		},
		&model.Effect{
			EffectID:  "effect-timeline",
			TaskID:    taskID,
			RunID:     "run-timeline",
			DedupeKey: "effect-timeline",
			Kind:      "send_im_message",
			Target:    "default",
			Status:    model.EffectStatusSucceeded,
			CreatedAt: base.Add(8 * time.Minute),
			UpdatedAt: base.Add(8 * time.Minute),
		},
		&model.InboundEvent{
			EventID:        "event-timeline",
			DedupeKey:      "event-timeline",
			Source:         "gitlab",
			EventType:      "issue",
			Status:         model.InboundEventStatusTaskCreated,
			NormalizedJSON: `{"dedupe_key":"timeline-task"}`,
			ReceivedAt:     base.Add(30 * time.Second),
			CreatedAt:      base.Add(30 * time.Second),
			UpdatedAt:      base.Add(30 * time.Second),
		},
		&model.InboundEvent{
			EventID:        "event-substring",
			DedupeKey:      "event-substring",
			Source:         "gitlab",
			EventType:      "issue",
			Status:         model.InboundEventStatusTaskCreated,
			NormalizedJSON: `{"dedupe_key":"timeline-task-extra"}`,
			ReceivedAt:     base.Add(45 * time.Second),
			CreatedAt:      base.Add(45 * time.Second),
			UpdatedAt:      base.Add(45 * time.Second),
		},
	}

	for _, entity := range entities {
		if err := db.Create(entity).Error; err != nil {
			t.Fatalf("seed timeline entity %T: %v", entity, err)
		}
	}
}
