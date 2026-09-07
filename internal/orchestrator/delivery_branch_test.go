package orchestrator

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

func TestPrepareDeliveryBranchPrefersFirstGitLabUsername(t *testing.T) {
	task := issueDeveloperTask(`{"issue":{"iid":13,"assignees":[{"id":7101,"username":"Alice"},{"id":7102,"username":"second.user"}],"assignee_ids":[7101,7102]}}`)
	task.CreatedAt = time.Date(2026, 7, 27, 16, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	orch := New(nil, nil, WithAssigneeMentionDirectory(fakeAssigneeDirectory{}))

	if err := orch.prepareDeliveryBranch(task); err != nil {
		t.Fatalf("prepare branch: %v", err)
	}
	if task.DeliveryBranch != "alice/issue-13-20260727" {
		t.Fatalf("unexpected delivery branch %q", task.DeliveryBranch)
	}
}

func TestPrepareDeliveryBranchFallsBackToDirectoryUsernameThenDeveloperAgent(t *testing.T) {
	createdAt := time.Date(2026, 7, 27, 1, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		context   string
		directory fakeAssigneeDirectory
		expected  string
	}{
		{
			name:      "first directory username",
			context:   `{"issue":{"iid":18,"assignees":[{"id":7101,"username":""},{"id":7102,"username":""}],"assignee_ids":[7101,7102]}}`,
			directory: fakeAssigneeDirectory{mentions: map[int]AssigneeMention{7102: {Username: "Second.Owner"}}},
			expected:  "second.owner/issue-18-20260727",
		},
		{
			name:      "no usable identity",
			context:   `{"issue":{"iid":19,"assignee_ids":[7101]}}`,
			directory: fakeAssigneeDirectory{},
			expected:  "developer-agent/issue-19-20260727",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := issueDeveloperTask(tt.context)
			task.CreatedAt = createdAt
			orch := New(nil, nil, WithAssigneeMentionDirectory(tt.directory))
			if err := orch.prepareDeliveryBranch(task); err != nil {
				t.Fatalf("prepare branch: %v", err)
			}
			if task.DeliveryBranch != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, task.DeliveryBranch)
			}
		})
	}
}

func TestPrepareDeliveryBranchKeepsLegacyAndExistingValues(t *testing.T) {
	legacy := &model.Task{TaskID: "task-legacy", Type: "upstream_fix", Role: "DeveloperWorker", CreatedAt: time.Now()}
	orch := New(nil, nil)
	if err := orch.prepareDeliveryBranch(legacy); err != nil {
		t.Fatalf("prepare legacy branch: %v", err)
	}
	if legacy.DeliveryBranch != "agent/task-legacy" {
		t.Fatalf("unexpected legacy branch %q", legacy.DeliveryBranch)
	}

	existing := issueDeveloperTask(`{"issue":{"iid":13}}`)
	existing.DeliveryBranch = "fixed/issue-13-20260726"
	if err := orch.prepareDeliveryBranch(existing); err != nil {
		t.Fatalf("preserve existing branch: %v", err)
	}
	if existing.DeliveryBranch != "fixed/issue-13-20260726" {
		t.Fatalf("existing branch changed to %q", existing.DeliveryBranch)
	}
}

func TestCreateTaskPersistsDeliveryBranchAcrossClockChanges(t *testing.T) {
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := repository.NewTaskRepository(db)
	now := time.Date(2026, 7, 27, 23, 59, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	orch := New(repo, nil,
		WithClock(func() time.Time { return now }),
		WithAssigneeMentionDirectory(fakeAssigneeDirectory{}),
	)
	task := issueDeveloperTask(`{"issue":{"iid":21,"assignees":[{"id":7101,"username":"owner"}]}}`)
	task.DedupeKey = "branch-persistence"

	created, err := orch.CreateTask(context.Background(), task)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if created.DeliveryBranch != "owner/issue-21-20260727" {
		t.Fatalf("unexpected created branch %q", created.DeliveryBranch)
	}
	now = now.Add(48 * time.Hour)
	loaded, err := repo.GetByTaskID(context.Background(), created.TaskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if loaded.DeliveryBranch != created.DeliveryBranch {
		t.Fatalf("persisted branch changed from %q to %q", created.DeliveryBranch, loaded.DeliveryBranch)
	}
}

func issueDeveloperTask(contextJSON string) *model.Task {
	return &model.Task{
		TaskID:      "task-issue-delivery",
		DedupeKey:   "issue-delivery",
		Type:        "develop_from_issue",
		Role:        "DeveloperWorker",
		ContextJSON: contextJSON,
	}
}
