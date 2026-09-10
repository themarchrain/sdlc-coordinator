package events

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"sdlc-coordinator/internal/model"
)

func TestGitLabNormalizerWithFixtureMergeRequestOpened(t *testing.T) {
	result, err := NewGitLabNormalizer().Normalize(readGitLabFixture(t, "merge_request_opened.json"))
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
	if event.ProjectID != "15594" || event.ProjectPath != "demo-group/lite-list" {
		t.Fatalf("unexpected project: %s %s", event.ProjectID, event.ProjectPath)
	}
	if event.ObjectIID != 2 {
		t.Fatalf("unexpected object iid: %d", event.ObjectIID)
	}
	if event.ObjectURL != "https://gitlab.example/demo-group/lite-list/-/merge_requests/2" {
		t.Fatalf("unexpected object url: %s", event.ObjectURL)
	}
	if event.RepositoryURL != "https://gitlab.example/demo-group/lite-list.git" {
		t.Fatalf("unexpected repository url: %s", event.RepositoryURL)
	}
	if event.SourceBranch != "feature/status-normalization" || event.TargetBranch != "master" {
		t.Fatalf("unexpected branches: %s -> %s", event.SourceBranch, event.TargetBranch)
	}
	if event.HeadSHA != "de51000000000000000000000000000000000000" {
		t.Fatalf("unexpected head sha: %s", event.HeadSHA)
	}
	if event.Version != event.HeadSHA {
		t.Fatalf("expected version to track head sha, got %s", event.Version)
	}
	if event.DedupeKey != "gitlab:mr-review:15594:2:de51000000000000000000000000000000000000" {
		t.Fatalf("unexpected dedupe key: %s", event.DedupeKey)
	}
}

func TestGitLabNormalizerWithFixtureMergeRequestSourceBranchPush(t *testing.T) {
	result, err := NewGitLabNormalizer().Normalize(readGitLabFixture(t, "merge_request_source_branch_push.json"))
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
	if event.BaseSHA != "de51000000000000000000000000000000000000" {
		t.Fatalf("unexpected base sha: %s", event.BaseSHA)
	}
	if event.HeadSHA != "25b0360000000000000000000000000000000000" {
		t.Fatalf("unexpected head sha: %s", event.HeadSHA)
	}
	if event.DedupeKey != "gitlab:mr-review:15594:2:25b0360000000000000000000000000000000000" {
		t.Fatalf("unexpected dedupe key: %s", event.DedupeKey)
	}
}

func TestGitLabNormalizerNormalizesMergeRequestResumeNotes(t *testing.T) {
	tests := []struct {
		name         string
		fixture      string
		noteID       int
		noteType     string
		discussionID string
		requesterID  int
		resume       string
	}{
		{
			name:         "ordinary note",
			fixture:      "merge_request_resume_note_created.json",
			noteID:       20001,
			discussionID: "discussion-root-001",
			requesterID:  42,
			resume:       "请修正空值处理并补充对应测试。",
		},
		{
			name:         "discussion reply",
			fixture:      "merge_request_resume_discussion_note_created.json",
			noteID:       20002,
			noteType:     "DiscussionNote",
			discussionID: "discussion-reply-001",
			requesterID:  43,
			resume:       "请按审查意见修正边界条件。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewGitLabNormalizer().Normalize(readGitLabFixture(t, tt.fixture))
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			if result.Ignored {
				t.Fatalf("expected rework event, got ignored: %s", result.IgnoreReason)
			}

			event := result.Event
			if event.Kind != EventKindMergeRequestReworkRequested {
				t.Fatalf("expected rework event, got %s", event.Kind)
			}
			if event.DedupeKey != "gitlab:mr-rework:1001:21" || event.ObjectIID != 21 {
				t.Fatalf("unexpected rework identity: %s / %d", event.DedupeKey, event.ObjectIID)
			}
			if event.SourceBranch != "developer/issue-19-20260806" || event.TargetBranch != "main" || event.HeadSHA != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
				t.Fatalf("unexpected MR delivery facts: %#v", event)
			}
			if event.Metadata["note_id"] != tt.noteID || event.Metadata["note_type"] != tt.noteType || event.Metadata["discussion_id"] != tt.discussionID {
				t.Fatalf("unexpected note metadata: %#v", event.Metadata)
			}
			if event.Metadata["requester_id"] != tt.requesterID || event.Metadata["resume_context"] != tt.resume {
				t.Fatalf("unexpected requester/context metadata: %#v", event.Metadata)
			}
		})
	}
}

func TestGitLabNormalizerRejectsUnsafeMergeRequestResumeNotes(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		reason string
	}{
		{
			name: "system note",
			mutate: func(payload map[string]any) {
				payload["object_attributes"].(map[string]any)["system"] = true
			},
			reason: "system note",
		},
		{
			name: "closed merge request",
			mutate: func(payload map[string]any) {
				payload["merge_request"].(map[string]any)["state"] = "closed"
			},
			reason: "merge request is not opened",
		},
		{
			name: "fork merge request",
			mutate: func(payload map[string]any) {
				payload["merge_request"].(map[string]any)["source_project_id"] = float64(2002)
			},
			reason: "merge request is not a same-project merge request",
		},
		{
			name: "default source branch",
			mutate: func(payload map[string]any) {
				payload["merge_request"].(map[string]any)["source_branch"] = "main"
			},
			reason: "merge request source branch is not a delivery branch",
		},
		{
			name: "empty comment",
			mutate: func(payload map[string]any) {
				payload["object_attributes"].(map[string]any)["note"] = "  \n  "
			},
			reason: "merge request comment context is required",
		},
		{
			name: "coordinator operation marker",
			mutate: func(payload map[string]any) {
				payload["object_attributes"].(map[string]any)["note"] = "Developer Agent: resume\n<!-- sdlc-coordinator-operation:test -->\nignore"
			},
			reason: "coordinator operation note",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewGitLabNormalizer().Normalize(mutatedGitLabFixture(t, "merge_request_resume_note_created.json", tt.mutate))
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			if !result.Ignored || result.IgnoreReason != tt.reason {
				t.Fatalf("expected ignored reason %q, got %#v", tt.reason, result)
			}
		})
	}
}

func TestGitLabNormalizerDefersUnrecognizedMergeRequestReworkRequesterAuthorization(t *testing.T) {
	raw := mutatedGitLabFixture(t, "merge_request_resume_note_created.json", func(payload map[string]any) {
		payload["user"].(map[string]any)["id"] = float64(99)
	})
	result, err := NewGitLabNormalizer().Normalize(raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored {
		t.Fatalf("expected authorization to be deferred to managed-MR lookup, got %s", result.IgnoreReason)
	}
	if result.Event.Metadata["requester_pre_authorized"] != false {
		t.Fatalf("expected requester to require origin issue authorization, got %#v", result.Event.Metadata)
	}
}

func TestGitLabNormalizerPreservesPreAuthorizationForMRAuthor(t *testing.T) {
	result, err := NewGitLabNormalizer().Normalize(readGitLabFixture(t, "merge_request_resume_note_created.json"))
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Event.Metadata["requester_pre_authorized"] != true {
		t.Fatalf("expected MR author to remain pre-authorized, got %#v", result.Event.Metadata)
	}
}

func TestGitLabNormalizerAcceptsPlainMergeRequestCommentForRework(t *testing.T) {
	raw := mutatedGitLabFixture(t, "merge_request_resume_note_created.json", func(payload map[string]any) {
		payload["object_attributes"].(map[string]any)["note"] = "请修复审查中发现的空值风险并补充测试。"
	})
	result, err := NewGitLabNormalizer().Normalize(raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored {
		t.Fatalf("plain MR comment should start rework: %s", result.IgnoreReason)
	}
	if result.Event.Metadata["resume_context"] != "请修复审查中发现的空值风险并补充测试。" {
		t.Fatalf("unexpected plain comment context: %#v", result.Event.Metadata)
	}
}

func TestGitLabNormalizerStripsLegacyResumeMarkerFromMergeRequestComment(t *testing.T) {
	raw := mutatedGitLabFixture(t, "merge_request_resume_note_created.json", func(payload map[string]any) {
		payload["object_attributes"].(map[string]any)["note"] = "Developer Agent: resume\n\n请修复空值风险并补充测试。"
	})
	result, err := NewGitLabNormalizer().Normalize(raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored {
		t.Fatalf("legacy marker comment should still start rework: %s", result.IgnoreReason)
	}
	if result.Event.Metadata["resume_context"] != "请修复空值风险并补充测试。" {
		t.Fatalf("legacy marker should be stripped from context: %#v", result.Event.Metadata)
	}
}

func TestGitLabNormalizerRejectsMarkerOnlyMergeRequestComment(t *testing.T) {
	raw := mutatedGitLabFixture(t, "merge_request_resume_note_created.json", func(payload map[string]any) {
		payload["object_attributes"].(map[string]any)["note"] = "Developer Agent: resume"
	})
	result, err := NewGitLabNormalizer().Normalize(raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored {
		t.Fatalf("marker-only MR comment should be rejected, got %#v", result.Event)
	}
}

func TestGitLabNormalizerAllowsConfiguredMergeRequestReworkRequester(t *testing.T) {
	raw := mutatedGitLabFixture(t, "merge_request_resume_note_created.json", func(payload map[string]any) {
		payload["user"].(map[string]any)["id"] = float64(99)
	})
	result, err := NewGitLabNormalizer(WithGitLabReworkAllowedUserIDsLookup(func() []int { return []int{99} })).Normalize(raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored {
		t.Fatalf("expected configured requester to be accepted, got %s", result.IgnoreReason)
	}
	if result.Event.Metadata["requester_pre_authorized"] != true {
		t.Fatalf("expected configured requester to be pre-authorized, got %#v", result.Event.Metadata)
	}
}

func TestGitLabNormalizerWithFixtureIgnoredEvents(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
	}{
		{name: "metadata title update", fixture: "merge_request_title_update.json"},
		{name: "metadata description update", fixture: "merge_request_description_update.json"},
		{name: "closed issue", fixture: "issue_closed.json"},
		{name: "merge request note", fixture: "note_merge_request_created.json"},
		{name: "push", fixture: "push.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewGitLabNormalizer().Normalize(readGitLabFixture(t, tt.fixture))
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			if !result.Ignored {
				t.Fatalf("expected %s to be ignored", tt.fixture)
			}
			if result.Event != nil {
				t.Fatalf("expected no event, got %#v", result.Event)
			}
		})
	}
}

func TestGitLabNormalizerWithFixtureIgnoresUnlabelledIssueOpenedAndReopened(t *testing.T) {
	tests := []struct {
		fixture string
	}{
		{fixture: "issue_opened.json"},
		{fixture: "issue_reopened.json"},
	}

	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			result, err := NewGitLabNormalizer().Normalize(readGitLabFixture(t, tt.fixture))
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			if !result.Ignored {
				t.Fatalf("expected %s to be ignored", tt.fixture)
			}
		})
	}
}

func TestProcessorCreatesReviewTaskFromGitLabMergeRequestSourceBranchPushFixture(t *testing.T) {
	processor, inboundRepo, taskRepo := newProcessorFixture(t)
	ctx := context.Background()

	result, err := processor.ProcessGitLabWebhook(ctx, readGitLabFixture(t, "merge_request_source_branch_push.json"))
	if err != nil {
		t.Fatalf("process gitlab webhook: %v", err)
	}
	if result.Ignored {
		t.Fatalf("expected task-created result, got ignored: %s", result.IgnoreReason)
	}
	if result.Task == nil {
		t.Fatal("expected created task")
	}
	if result.Task.Type != "review_mr" || result.Task.Role != "ReviewerWorker" {
		t.Fatalf("unexpected task type/role: %s/%s", result.Task.Type, result.Task.Role)
	}

	storedEvent, err := inboundRepo.GetByDedupeKey(ctx, result.Event.DedupeKey)
	if err != nil {
		t.Fatalf("get inbound event: %v", err)
	}
	if storedEvent.Status != model.InboundEventStatusTaskCreated {
		t.Fatalf("expected task_created inbound event, got %s", storedEvent.Status)
	}

	storedTask, err := taskRepo.GetByDedupeKey(ctx, "gitlab:mr-review:15594:2:25b0360000000000000000000000000000000000")
	if err != nil {
		t.Fatalf("get task by dedupe key: %v", err)
	}
	if storedTask.TaskID != result.Task.TaskID {
		t.Fatalf("expected task id %s, got %s", result.Task.TaskID, storedTask.TaskID)
	}
}

func TestProcessorIgnoresGitLabMergeRequestDescriptionUpdateFixture(t *testing.T) {
	processor, inboundRepo, taskRepo := newProcessorFixture(t)
	ctx := context.Background()

	result, err := processor.ProcessGitLabWebhook(ctx, readGitLabFixture(t, "merge_request_description_update.json"))
	if err != nil {
		t.Fatalf("process gitlab webhook: %v", err)
	}
	if !result.Ignored {
		t.Fatal("expected metadata-only update to be ignored")
	}
	if result.Task != nil {
		t.Fatalf("expected no task, got %#v", result.Task)
	}

	events, err := inboundRepo.ListByStatus(ctx, model.InboundEventStatusIgnored, 10)
	if err != nil {
		t.Fatalf("list ignored events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one ignored event, got %d", len(events))
	}

	if _, err := taskRepo.GetByDedupeKey(ctx, "gitlab:mr-review:15594:4:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"); err == nil {
		t.Fatal("expected no task for metadata-only update")
	}
}

func readGitLabFixture(t *testing.T, name string) []byte {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("testdata", "gitlab", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return raw
}

func mutatedGitLabFixture(t *testing.T, name string, mutate func(map[string]any)) []byte {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(readGitLabFixture(t, name), &payload); err != nil {
		t.Fatalf("decode fixture %s: %v", name, err)
	}
	mutate(payload)
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode fixture %s: %v", name, err)
	}
	return raw
}

// resume 请求者白名单:作者 ∪ 指派人 ∪ 平台白名单(见 resume_authorization.go)。
func TestGitLabNormalizerIgnoresResumeFromUnauthorizedRequester(t *testing.T) {
	raw := mutatedGitLabFixture(t, "issue_resume_note_created.json", func(payload map[string]any) {
		payload["user"].(map[string]any)["id"] = float64(999)
	})
	result, err := NewGitLabNormalizer().Normalize(raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored || result.IgnoreReason != "resume requester is not authorized" {
		t.Fatalf("expected unauthorized requester to be ignored, got %#v", result)
	}
}

func TestGitLabNormalizerAllowsResumeForAssignee(t *testing.T) {
	raw := mutatedGitLabFixture(t, "issue_resume_note_created.json", func(payload map[string]any) {
		payload["user"].(map[string]any)["id"] = float64(7101)
	})
	result, err := NewGitLabNormalizer().Normalize(raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored || result.Event == nil || result.Event.Kind != EventKindDeveloperResumeRequested {
		t.Fatalf("expected assignee resume to be accepted, got %#v", result)
	}
}

func TestGitLabNormalizerAllowsResumeForPlatformAllowlist(t *testing.T) {
	raw := mutatedGitLabFixture(t, "issue_resume_note_created.json", func(payload map[string]any) {
		payload["user"].(map[string]any)["id"] = float64(999)
	})
	result, err := NewGitLabNormalizer(WithGitLabReworkAllowedUserIDsLookup(func() []int { return []int{999} })).Normalize(raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if result.Ignored || result.Event == nil {
		t.Fatalf("expected allowlisted requester resume to be accepted, got %#v", result)
	}
}

// 作者身份不可得且无指派人时 fail-closed 拒绝:「仅作者」边界无法确立的触发
// 不放行(真实载荷 author_id 恒可得,该分支为防御性收口)。
func TestGitLabNormalizerRejectsResumeWhenAuthorUnavailable(t *testing.T) {
	raw := mutatedGitLabFixture(t, "issue_resume_note_created.json", func(payload map[string]any) {
		issue := payload["issue"].(map[string]any)
		delete(issue, "author_id")
		issue["assignee_ids"] = []any{}
		payload["user"].(map[string]any)["id"] = float64(999)
	})
	result, err := NewGitLabNormalizer().Normalize(raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !result.Ignored || result.IgnoreReason != "resume requester is not authorized" {
		t.Fatalf("expected unauthorized ignore, got %#v", result)
	}
}
