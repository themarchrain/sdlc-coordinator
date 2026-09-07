package events

import (
	"strings"
	"testing"
)

func TestGitLabNormalizerNormalizesMergeRequestOpen(t *testing.T) {
	result, err := NewGitLabNormalizer().Normalize([]byte(gitLabMROpenPayload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored {
		t.Fatalf("expected event, got ignored: %s", result.IgnoreReason)
	}

	event := result.Event
	if event.Kind != EventKindChangeRequestUpdated {
		t.Fatalf("expected change request event, got %s", event.Kind)
	}
	if event.Source != "gitlab" {
		t.Fatalf("unexpected source: %s", event.Source)
	}
	if event.EventType != "merge_request" || event.EventAction != "open" {
		t.Fatalf("unexpected event type/action: %s/%s", event.EventType, event.EventAction)
	}
	if event.ProjectID != "24207" || event.ProjectPath != "group/project" {
		t.Fatalf("unexpected project: %s %s", event.ProjectID, event.ProjectPath)
	}
	if event.ObjectIID != 12 {
		t.Fatalf("unexpected object iid: %d", event.ObjectIID)
	}
	if event.ObjectURL != "https://gitlab.example/group/project/-/merge_requests/12" {
		t.Fatalf("unexpected object url: %s", event.ObjectURL)
	}
	if event.RepositoryURL != "https://gitlab.example/group/project.git" {
		t.Fatalf("unexpected repository url: %s", event.RepositoryURL)
	}
	if event.SourceBranch != "feature/demo" || event.TargetBranch != "master" {
		t.Fatalf("unexpected branches: %s -> %s", event.SourceBranch, event.TargetBranch)
	}
	if event.HeadSHA != "abc123" || event.Version != "abc123" {
		t.Fatalf("unexpected sha/version: %s %s", event.HeadSHA, event.Version)
	}
	if event.DedupeKey != "gitlab:mr-review:24207:12:abc123" {
		t.Fatalf("unexpected dedupe key: %s", event.DedupeKey)
	}
}

func TestGitLabNormalizerCapturesMergeRequestNotificationActor(t *testing.T) {
	result, err := NewGitLabNormalizer().Normalize(readGitLabFixture(t, "merge_request_opened.json"))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored {
		t.Fatalf("expected event, got ignored: %s", result.IgnoreReason)
	}

	metadata := result.Event.Metadata
	if metadata["author_name"] != "Demo User" || metadata["author_email"] != "demo.user@example.com" {
		t.Fatalf("unexpected merge request author metadata: %#v", metadata)
	}
}

func TestGitLabNormalizerNormalizesMergeRequestUpdateWithOldRev(t *testing.T) {
	result, err := NewGitLabNormalizer().Normalize([]byte(gitLabMRUpdateWithOldRevPayload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored {
		t.Fatalf("expected event, got ignored: %s", result.IgnoreReason)
	}

	event := result.Event
	if event.Kind != EventKindChangeRequestUpdated {
		t.Fatalf("expected change request event, got %s", event.Kind)
	}
	if event.EventAction != "update" {
		t.Fatalf("unexpected action: %s", event.EventAction)
	}
	if event.HeadSHA != "def456" || event.BaseSHA != "abc123" {
		t.Fatalf("unexpected old/new sha: %s -> %s", event.BaseSHA, event.HeadSHA)
	}
	if event.DedupeKey != "gitlab:mr-review:24207:12:def456" {
		t.Fatalf("unexpected dedupe key: %s", event.DedupeKey)
	}
}

func TestGitLabNormalizerIgnoresMergeRequestUpdateWithoutOldRev(t *testing.T) {
	result, err := NewGitLabNormalizer().Normalize([]byte(gitLabMRUpdateWithoutOldRevPayload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored {
		t.Fatal("expected metadata-only MR update to be ignored")
	}
	if result.Event != nil {
		t.Fatalf("expected no event, got %#v", result.Event)
	}
}

func TestGitLabNormalizerIgnoresMergeRequestReopen(t *testing.T) {
	payload := strings.Replace(gitLabMROpenPayload, `"action": "open"`, `"action": "reopen"`, 1)
	result, err := NewGitLabNormalizer().Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored {
		t.Fatal("expected reopened merge request to be ignored")
	}
}

func TestGitLabNormalizerNormalizesClosedMergeRequestLifecycle(t *testing.T) {
	result, err := NewGitLabNormalizer().Normalize([]byte(gitLabMRClosedPayload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored || result.Event.Kind != EventKindMergeRequestLifecycleChanged {
		t.Fatalf("expected closed lifecycle event, got %#v", result)
	}
	if result.Event.Metadata["lifecycle_state"] != "closed" {
		t.Fatalf("unexpected closed lifecycle metadata: %#v", result.Event.Metadata)
	}
}

func TestGitLabNormalizerNormalizesMergedMergeRequestLifecycle(t *testing.T) {
	payload := strings.ReplaceAll(gitLabMRClosedPayload, `"action": "close"`, `"action": "merge"`)
	payload = strings.ReplaceAll(payload, `"state": "closed"`, `"state": "merged"`)
	result, err := NewGitLabNormalizer().Normalize([]byte(payload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored || result.Event.Kind != EventKindMergeRequestLifecycleChanged {
		t.Fatalf("expected merged lifecycle event, got %#v", result)
	}
	if result.Event.DedupeKey != "gitlab:mr-rework:24207:12" || result.Event.Metadata["lifecycle_state"] != "merged" {
		t.Fatalf("unexpected merged lifecycle event: %#v", result.Event)
	}
}

func TestGitLabNormalizerRejectsMergeRequestWithoutProjectID(t *testing.T) {
	_, err := NewGitLabNormalizer().Normalize([]byte(gitLabMRMissingProjectIDPayload))
	if err == nil {
		t.Fatal("expected missing project id to fail")
	}
}

func TestGitLabNormalizerNormalizesIssueOpen(t *testing.T) {
	result, err := NewGitLabNormalizer().Normalize(readGitLabFixture(t, "issue_opened_developer_agent.json"))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored {
		t.Fatalf("expected event, got ignored: %s", result.IgnoreReason)
	}

	event := result.Event
	if event.Kind != EventKindWorkItemUpdated {
		t.Fatalf("expected work item event, got %s", event.Kind)
	}
	if event.EventType != "issue" || event.EventAction != "open" {
		t.Fatalf("unexpected issue type/action: %s/%s", event.EventType, event.EventAction)
	}
	if event.ObjectIID != 2 {
		t.Fatalf("unexpected issue iid: %d", event.ObjectIID)
	}
	if event.ObjectURL != "https://gitlab.example/demo-group/lite-list/-/issues/2" {
		t.Fatalf("unexpected issue url: %s", event.ObjectURL)
	}
	if event.Version != "2026-07-26T12:00:00Z" {
		t.Fatalf("unexpected issue version: %s", event.Version)
	}
	if event.DedupeKey != "gitlab:issue-develop:15594:2" {
		t.Fatalf("unexpected issue dedupe key: %s", event.DedupeKey)
	}
}

func TestGitLabNormalizerPreservesOrderedIssueAssignees(t *testing.T) {
	result, err := NewGitLabNormalizer().Normalize([]byte(gitLabIssueOpenWithAssigneesPayload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored {
		t.Fatalf("expected event, got ignored: %s", result.IgnoreReason)
	}

	assignees, ok := result.Event.Metadata["assignees"].([]map[string]any)
	if !ok || len(assignees) != 2 {
		t.Fatalf("unexpected assignees metadata: %#v", result.Event.Metadata["assignees"])
	}
	if assignees[0]["id"] != 7101 || assignees[0]["username"] != "alice" {
		t.Fatalf("unexpected first assignee: %#v", assignees[0])
	}
	if assignees[1]["id"] != 7102 || assignees[1]["username"] != "second.user" {
		t.Fatalf("unexpected second assignee: %#v", assignees[1])
	}
}

func TestGitLabNormalizerIgnoresClosedIssue(t *testing.T) {
	result, err := NewGitLabNormalizer().Normalize([]byte(gitLabIssueClosedPayload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored {
		t.Fatal("expected closed issue to be ignored")
	}
}

func TestGitLabNormalizerAcceptsOnlyOpenedDeveloperIssue(t *testing.T) {
	result, err := NewGitLabNormalizer().Normalize(readGitLabFixture(t, "issue_opened_developer_agent.json"))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored {
		t.Fatalf("expected developer issue event, got ignored: %s", result.IgnoreReason)
	}

	event := result.Event
	if event.Kind != EventKindWorkItemUpdated {
		t.Fatalf("unexpected event kind: %s", event.Kind)
	}
	if event.DedupeKey != "gitlab:issue-develop:15594:2" {
		t.Fatalf("unexpected dedupe key: %s", event.DedupeKey)
	}
	assigneeIDs, ok := event.Metadata["assignee_ids"].([]int)
	if !ok || len(assigneeIDs) != 1 || assigneeIDs[0] != 7101 {
		t.Fatalf("unexpected assignee ids: %#v", event.Metadata["assignee_ids"])
	}
}

func TestGitLabNormalizerIgnoresIssueWithoutDeveloperAgentLabel(t *testing.T) {
	result, err := NewGitLabNormalizer().Normalize([]byte(gitLabIssueOpenPayload))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored {
		t.Fatal("expected unlabelled issue to be ignored")
	}
}

func TestGitLabNormalizerIgnoresIssueUpdateAndReopen(t *testing.T) {
	for _, payload := range []string{gitLabIssueUpdatePayload, gitLabIssueReopenPayload} {
		result, err := NewGitLabNormalizer().Normalize([]byte(payload))
		if err != nil {
			t.Fatalf("normalize: %v", err)
		}
		if !result.Ignored {
			t.Fatal("expected non-open issue event to be ignored")
		}
	}
}

func TestGitLabNormalizerNormalizesExactIssueResumeNote(t *testing.T) {
	result, err := NewGitLabNormalizer().Normalize(readGitLabFixture(t, "issue_resume_note_created.json"))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored {
		t.Fatalf("expected resume event, got ignored: %s", result.IgnoreReason)
	}

	event := result.Event
	if string(event.Kind) != "developer_resume_requested" {
		t.Fatalf("unexpected event kind: %s", event.Kind)
	}
	commentID, ok := event.Metadata["comment_id"].(int)
	if event.DedupeKey != "gitlab:issue-develop:15594:2" || !ok || commentID != 169424 {
		t.Fatalf("unexpected resume key/comment id: %s/%#v", event.DedupeKey, event.Metadata["comment_id"])
	}
	if event.Metadata["resume_context"] != "接口字段为 status，允许的值是 ready 和 blocked。" {
		t.Fatalf("unexpected resume body: %q", event.Metadata["resume_context"])
	}
}

func TestGitLabNormalizerAcceptsPlainIssueCommentForDeveloperResume(t *testing.T) {
	raw := mutatedGitLabFixture(t, "issue_resume_note_created.json", func(payload map[string]any) {
		payload["object_attributes"].(map[string]any)["note"] = "接口字段为 status，允许的值是 ready 和 blocked。"
	})
	result, err := NewGitLabNormalizer().Normalize(raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored {
		t.Fatalf("plain Issue comment should resume developer task: %s", result.IgnoreReason)
	}
	if result.Event.Metadata["resume_context"] != "接口字段为 status，允许的值是 ready 和 blocked。" {
		t.Fatalf("unexpected plain comment context: %#v", result.Event.Metadata)
	}
}

func TestGitLabNormalizerStripsDeveloperAgentResumeMarkerFromIssueComment(t *testing.T) {
	raw := mutatedGitLabFixture(t, "issue_resume_note_created.json", func(payload map[string]any) {
		payload["object_attributes"].(map[string]any)["note"] = "Developer Agent: resume\n接口字段为 status，允许的值是 ready 和 blocked。"
	})
	result, err := NewGitLabNormalizer().Normalize(raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored {
		t.Fatalf("Developer Agent marker Issue comment should resume: %s", result.IgnoreReason)
	}
	if result.Event.Metadata["resume_context"] != "接口字段为 status，允许的值是 ready 和 blocked。" {
		t.Fatalf("Developer Agent marker should be stripped from issue context: %#v", result.Event.Metadata)
	}
}

func TestGitLabNormalizerRejectsMarkerOnlyIssueComment(t *testing.T) {
	raw := mutatedGitLabFixture(t, "issue_resume_note_created.json", func(payload map[string]any) {
		payload["object_attributes"].(map[string]any)["note"] = "Developer Agent: resume"
	})
	result, err := NewGitLabNormalizer().Normalize(raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored {
		t.Fatalf("marker-only Issue comment should be rejected, got %#v", result.Event)
	}
}

func TestGitLabNormalizerIgnoresCoordinatorIssueCommentWithoutKeywordGate(t *testing.T) {
	raw := mutatedGitLabFixture(t, "issue_resume_note_created.json", func(payload map[string]any) {
		payload["object_attributes"].(map[string]any)["note"] = "请补充接口定义。\n\n<!-- agent-coordinator-operation:codehost:test -->"
	})
	result, err := NewGitLabNormalizer().Normalize(raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored || result.IgnoreReason != "coordinator operation note" {
		t.Fatalf("expected coordinator comment to stay ignored: %#v", result)
	}
}

const gitLabMROpenPayload = `{
  "object_kind": "merge_request",
  "event_type": "merge_request",
  "project": {
    "id": 24207,
    "path_with_namespace": "group/project",
    "git_ssh_url": "git@gitlab.example:group/project.git",
    "git_http_url": "https://gitlab.example/group/project.git"
  },
  "object_attributes": {
    "iid": 12,
    "action": "open",
    "state": "opened",
    "title": "Improve service",
    "description": "Review the service change",
    "source_branch": "feature/demo",
    "target_branch": "master",
    "url": "https://gitlab.example/group/project/-/merge_requests/12",
    "last_commit": {
      "id": "abc123"
    }
  }
}`

const gitLabMRUpdateWithOldRevPayload = `{
  "object_kind": "merge_request",
  "event_type": "merge_request",
  "project": {
    "id": 24207,
    "path_with_namespace": "group/project",
    "git_ssh_url": "git@gitlab.example:group/project.git"
  },
  "object_attributes": {
    "iid": 12,
    "action": "update",
    "state": "opened",
    "title": "Improve service",
    "description": "Review the service change",
    "source_branch": "feature/demo",
    "target_branch": "master",
    "url": "https://gitlab.example/group/project/-/merge_requests/12",
    "oldrev": "abc123",
    "last_commit": {
      "id": "def456"
    }
  }
}`

const gitLabMRUpdateWithoutOldRevPayload = `{
  "object_kind": "merge_request",
  "project": {
    "id": 24207,
    "path_with_namespace": "group/project",
    "git_ssh_url": "git@gitlab.example:group/project.git"
  },
  "object_attributes": {
    "iid": 12,
    "action": "update",
    "state": "opened",
    "title": "Improve service",
    "source_branch": "feature/demo",
    "target_branch": "master",
    "url": "https://gitlab.example/group/project/-/merge_requests/12",
    "last_commit": {
      "id": "def456"
    }
  }
}`

const gitLabMRClosedPayload = `{
  "object_kind": "merge_request",
  "project": {
    "id": 24207,
    "path_with_namespace": "group/project",
    "git_ssh_url": "git@gitlab.example:group/project.git"
  },
  "object_attributes": {
    "iid": 12,
    "action": "close",
    "state": "closed",
    "title": "Improve service",
    "source_branch": "feature/demo",
    "target_branch": "master",
    "url": "https://gitlab.example/group/project/-/merge_requests/12",
    "last_commit": {
      "id": "abc123"
    }
  }
}`

const gitLabMRMissingProjectIDPayload = `{
  "object_kind": "merge_request",
  "project": {
    "path_with_namespace": "group/project",
    "git_ssh_url": "git@gitlab.example:group/project.git"
  },
  "object_attributes": {
    "iid": 12,
    "action": "open",
    "state": "opened",
    "title": "Improve service",
    "source_branch": "feature/demo",
    "target_branch": "master",
    "url": "https://gitlab.example/group/project/-/merge_requests/12",
    "last_commit": {
      "id": "abc123"
    }
  }
}`

const gitLabIssueOpenPayload = `{
  "object_kind": "issue",
  "event_type": "issue",
  "project": {
    "id": 24207,
    "path_with_namespace": "group/project",
    "git_ssh_url": "git@gitlab.example:group/project.git",
    "default_branch": "master"
  },
  "object_attributes": {
    "iid": 34,
    "action": "open",
    "state": "opened",
    "title": "Fix empty status bug",
    "description": "A small and clear task",
    "url": "https://gitlab.example/group/project/-/issues/34",
    "updated_at": "2026-07-19T08:30:00Z"
  }
}`

const gitLabIssueOpenWithAssigneesPayload = `{
  "object_kind": "issue",
  "event_type": "issue",
  "project": {
    "id": 24207,
    "path_with_namespace": "group/project",
    "git_ssh_url": "git@gitlab.example:group/project.git",
    "default_branch": "master"
  },
  "object_attributes": {
    "iid": 34,
    "action": "open",
    "state": "opened",
    "title": "Fix empty status bug",
    "description": "A small and clear task",
    "url": "https://gitlab.example/group/project/-/issues/34",
    "updated_at": "2026-07-19T08:30:00Z",
    "assignee_ids": [7101, 7102]
  },
  "labels": [{"title": "developer-agent"}],
  "assignees": [
    {"id": 7101, "username": "alice"},
    {"id": 7102, "username": "second.user"}
  ]
}`

const gitLabIssueUpdatePayload = `{
  "object_kind": "issue",
  "project": {"id": 24207, "path_with_namespace": "group/project", "git_ssh_url": "git@gitlab.example:group/project.git", "default_branch": "master"},
  "object_attributes": {"iid": 34, "action": "update", "state": "opened", "title": "Fix empty status bug", "url": "https://gitlab.example/group/project/-/issues/34", "updated_at": "2026-07-26T12:00:00Z"},
  "labels": [{"title": "developer-agent"}]
}`

const gitLabIssueReopenPayload = `{
  "object_kind": "issue",
  "project": {"id": 24207, "path_with_namespace": "group/project", "git_ssh_url": "git@gitlab.example:group/project.git", "default_branch": "master"},
  "object_attributes": {"iid": 34, "action": "reopen", "state": "opened", "title": "Fix empty status bug", "url": "https://gitlab.example/group/project/-/issues/34", "updated_at": "2026-07-26T12:00:00Z"},
  "labels": [{"title": "developer-agent"}]
}`

const gitLabIssueClosedPayload = `{
  "object_kind": "issue",
  "project": {
    "id": 24207,
    "path_with_namespace": "group/project",
    "git_ssh_url": "git@gitlab.example:group/project.git",
    "default_branch": "master"
  },
  "object_attributes": {
    "iid": 34,
    "action": "close",
    "state": "closed",
    "title": "Fix empty status bug",
    "url": "https://gitlab.example/group/project/-/issues/34",
    "updated_at": "2026-07-19T08:30:00Z"
  }
}`
