package events

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTaskFactoryBuildsReviewerTaskFromMergeRequestEvent(t *testing.T) {
	task, err := NewTaskFactory().BuildTask(NormalizedEvent{
		Source:        "gitlab",
		Kind:          EventKindChangeRequestUpdated,
		EventType:     "merge_request",
		EventAction:   "update",
		ProjectID:     "24207",
		ProjectPath:   "group/project",
		RepositoryURL: "git@gitlab.example:group/project.git",
		ObjectIID:     12,
		ObjectURL:     "https://gitlab.example/group/project/-/merge_requests/12",
		Title:         "Improve service",
		Description:   "Review this service change carefully.",
		SourceBranch:  "feature/demo",
		TargetBranch:  "master",
		HeadSHA:       "abc123",
		BaseSHA:       "def456",
		Version:       "abc123",
		DedupeKey:     "gitlab:mr-review:24207:12:abc123",
		Metadata: map[string]any{
			"author_name":  "Demo User",
			"author_email": "demo.user@example.com",
		},
	})
	if err != nil {
		t.Fatalf("build task: %v", err)
	}

	if task.DedupeKey != "gitlab:mr-review:24207:12:abc123" {
		t.Fatalf("unexpected dedupe key: %s", task.DedupeKey)
	}
	if task.Type != "review_mr" || task.Role != "ReviewerWorker" {
		t.Fatalf("unexpected task type/role: %s/%s", task.Type, task.Role)
	}
	if task.SourceRepo != "group/project" || task.TargetRepo != "group/project" {
		t.Fatalf("unexpected repos: %s/%s", task.SourceRepo, task.TargetRepo)
	}

	context := decodeTaskContext(t, task.ContextJSON)
	if _, exists := context["workspace_strategy"]; exists {
		t.Fatalf("legacy workspace strategy must not be persisted on new tasks: %#v", context["workspace_strategy"])
	}
	mr := context["merge_request"].(map[string]any)
	if mr["iid"].(float64) != 12 || mr["head_sha"] != "abc123" {
		t.Fatalf("unexpected merge request context: %#v", mr)
	}
	if mr["event_action"] != "update" || mr["review_mode"] != "incremental" || mr["description"] != "Review this service change carefully." {
		t.Fatalf("unexpected merge request review context: %#v", mr)
	}
	author := mr["author"].(map[string]any)
	if author["name"] != "Demo User" || author["email"] != "demo.user@example.com" {
		t.Fatalf("unexpected merge request author context: %#v", author)
	}
	repo := context["repos"].([]any)[0].(map[string]any)
	if repo["access"] != "readable" || repo["branch"] != "feature/demo" {
		t.Fatalf("unexpected repo context: %#v", repo)
	}
}

func TestTaskFactoryBuildsDeveloperTaskFromIssueEvent(t *testing.T) {
	task, err := NewTaskFactory().BuildTask(NormalizedEvent{
		Source:        "gitlab",
		Kind:          EventKindWorkItemUpdated,
		EventType:     "issue",
		EventAction:   "open",
		ProjectID:     "24207",
		ProjectPath:   "group/project",
		RepositoryURL: "git@gitlab.example:group/project.git",
		ObjectIID:     34,
		ObjectURL:     "https://gitlab.example/group/project/-/issues/34",
		Title:         "Fix empty status bug",
		Description:   "A small and clear task",
		TargetBranch:  "master",
		Version:       "2026-07-19T08:30:00Z",
		DedupeKey:     "gitlab:issue-develop:24207:34",
		Metadata: map[string]any{
			"assignee_ids": []int{7101},
		},
	})
	if err != nil {
		t.Fatalf("build task: %v", err)
	}

	if task.Type != "develop_from_issue" || task.Role != "DeveloperWorker" {
		t.Fatalf("unexpected task type/role: %s/%s", task.Type, task.Role)
	}
	if task.DedupeKey != "gitlab:issue-develop:24207:34" {
		t.Fatalf("unexpected dedupe key: %s", task.DedupeKey)
	}

	context := decodeTaskContext(t, task.ContextJSON)
	issue := context["issue"].(map[string]any)
	if issue["iid"].(float64) != 34 || issue["url"] != "https://gitlab.example/group/project/-/issues/34" {
		t.Fatalf("unexpected issue context: %#v", issue)
	}
	assigneeIDs, ok := issue["assignee_ids"].([]any)
	if !ok || len(assigneeIDs) != 1 || assigneeIDs[0].(float64) != 7101 {
		t.Fatalf("unexpected issue assignee ids: %#v", issue["assignee_ids"])
	}
	repo := context["repos"].([]any)[0].(map[string]any)
	if repo["access"] != "writable" || repo["branch"] != "master" {
		t.Fatalf("unexpected repo context: %#v", repo)
	}
}

func TestTaskFactoryDoesNotEmitSourceRefAndKeepsOrderedAssignees(t *testing.T) {
	description := "实现需求。\n\n详见：[#1234567 Implement login retry](https://tracker.example.test/project/#/team/example-team/task/example-task)\n\n<!-- source-key: tracker:example-project:example-task -->\n"
	task, err := NewTaskFactory().BuildTask(NormalizedEvent{
		Source:        "gitlab",
		Kind:          EventKindWorkItemUpdated,
		EventType:     "issue",
		EventAction:   "open",
		ProjectID:     "24207",
		ProjectPath:   "group/project",
		RepositoryURL: "git@gitlab.example:group/project.git",
		ObjectIID:     34,
		ObjectURL:     "https://gitlab.example/group/project/-/issues/34",
		Title:         "Fix empty status bug",
		Description:   description,
		TargetBranch:  "master",
		Version:       "2026-07-19T08:30:00Z",
		DedupeKey:     "gitlab:issue-develop:24207:34",
		Metadata: map[string]any{
			"assignee_ids": []int{7101, 7102},
			"assignees": []map[string]any{
				{"id": 7101, "username": "alice"},
				{"id": 7102, "username": "second.user"},
			},
		},
	})
	if err != nil {
		t.Fatalf("build task: %v", err)
	}

	context := decodeTaskContext(t, task.ContextJSON)
	issue := context["issue"].(map[string]any)
	if _, exists := issue["source_ref"]; exists {
		t.Fatalf("source_ref must not be produced: %#v", issue["source_ref"])
	}
	assignees := issue["assignees"].([]any)
	if len(assignees) != 2 || assignees[0].(map[string]any)["username"] != "alice" {
		t.Fatalf("unexpected issue assignees: %#v", assignees)
	}
}

func TestTaskFactoryNeverEmitsSourceRefForAnyDescription(t *testing.T) {
	for _, description := range []string{
		"详见：[#1234567](http://tracker.example.test/project/#/team/team/task/task)",
		"详见：[#1234567](https://tracker.example.test/other/#/team/team/task/task)",
		"详见：[#1](https://tracker.example.test/project/#/team/t/task/x)",
	} {
		task, err := NewTaskFactory().BuildTask(NormalizedEvent{
			Source: "gitlab", Kind: EventKindWorkItemUpdated, ProjectID: "24207", ProjectPath: "group/project",
			RepositoryURL: "git@gitlab.example:group/project.git", ObjectIID: 34,
			ObjectURL: "https://gitlab.example/group/project/-/issues/34", Description: description,
			DedupeKey: "gitlab:issue-develop:24207:34",
		})
		if err != nil {
			t.Fatalf("build task for %q: %v", description, err)
		}
		issue := decodeTaskContext(t, task.ContextJSON)["issue"].(map[string]any)
		if _, exists := issue["source_ref"]; exists {
			t.Fatalf("source_ref must never be produced: %#v", issue["source_ref"])
		}
	}
}

func TestTaskFactoryObjectivesFollowEventSource(t *testing.T) {
	base := NormalizedEvent{
		Kind: EventKindWorkItemUpdated, ProjectID: "42", ProjectPath: "group/project",
		RepositoryURL: "https://gitea.example/group/project.git", ObjectIID: 7,
		Title: "Fix widget", DedupeKey: "gitea:issue-develop:42:7", Metadata: map[string]any{},
	}
	gitea := base
	gitea.Source = "gitea"
	task, err := NewTaskFactory().BuildTask(gitea)
	if err != nil {
		t.Fatalf("build gitea task: %v", err)
	}
	if !strings.Contains(task.Objective, "Implement Gitea Issue #7") {
		t.Fatalf("unexpected gitea objective: %s", task.Objective)
	}

	gitlab := base
	gitlab.Source = "gitlab"
	gitlab.DedupeKey = "gitlab:issue-develop:42:7"
	task, err = NewTaskFactory().BuildTask(gitlab)
	if err != nil {
		t.Fatalf("build gitlab task: %v", err)
	}
	if !strings.Contains(task.Objective, "Implement GitLab Issue #7") {
		t.Fatalf("unexpected gitlab objective: %s", task.Objective)
	}
}

func decodeTaskContext(t *testing.T, raw string) map[string]any {
	t.Helper()

	var context map[string]any
	if err := json.Unmarshal([]byte(raw), &context); err != nil {
		t.Fatalf("unmarshal context: %v", err)
	}
	return context
}

func TestTaskFactoryObjectivesFollowGitHubSource(t *testing.T) {
	event := NormalizedEvent{
		Kind: EventKindWorkItemUpdated, Source: "github", ProjectID: "42", ProjectPath: "org/project",
		RepositoryURL: "https://github.com/org/project.git", ObjectIID: 7,
		Title: "Fix widget", DedupeKey: "github:issue-develop:42:7", Metadata: map[string]any{},
	}
	task, err := NewTaskFactory().BuildTask(event)
	if err != nil {
		t.Fatalf("build github task: %v", err)
	}
	if !strings.Contains(task.Objective, "Implement GitHub Issue #7") {
		t.Fatalf("unexpected github objective: %s", task.Objective)
	}
}
