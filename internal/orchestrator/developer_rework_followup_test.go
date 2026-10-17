package orchestrator

import (
	"context"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

func TestDeveloperReworkSuccessDerivesIMWithoutWriteback(t *testing.T) {
	orch := newDeveloperReworkFollowupOrchestrator(t)
	task := developerReworkFollowupTask()
	payload := results.CanonicalResult{
		TaskID:  task.TaskID,
		RunID:   "run-rework-followup",
		Status:  results.StatusSucceeded,
		Summary: "已修正空值处理并更新原 MR 分支。",
		Artifacts: map[string]any{
			"commit_sha":   "def4567890",
			"test_results": []results.TestEvidence{{Command: "go test ./...", Passed: true, Summary: "all passed"}},
		},
	}

	updated, err := orch.withDeveloperReworkFollowups(context.Background(), task, payload)
	if err != nil {
		t.Fatalf("derive rework followups: %v", err)
	}
	if len(updated.Writebacks) != 0 {
		t.Fatalf("rework success must not create a writeback: %#v", updated.Writebacks)
	}
	if len(updated.Effects) != 1 {
		t.Fatalf("expected one rework IM effect, got %#v", updated.Effects)
	}
	effect := updated.Effects[0]
	if effect.DedupeKey != "developer-rework-im:task-rework-followup:run-rework-followup" || effect.Kind != "send_im_message" || effect.Critical {
		t.Fatalf("unexpected rework effect identity: %#v", effect)
	}
	body, _ := effect.Request["body"].(string)
	for _, expected := range []string{
		"[!21](https://gitlab.example/demo-group/demo-project/-/merge_requests/21)",
		"developer/issue-19-20260806",
		"def45678",
		"已修正空值处理",
		"go test ./...",
		"将进入增量代码审查",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("rework IM body missing %q: %s", expected, body)
		}
	}
	mentions, _ := effect.Request["mentions"].([]string)
	if strings.Join(mentions, ",") != "负责人,审查人,requester@example.com" {
		t.Fatalf("unexpected rework mentions: %#v", mentions)
	}
}

func TestDeveloperReworkInsufficientContextDerivesMRCommentAndIM(t *testing.T) {
	orch := newDeveloperReworkFollowupOrchestrator(t)
	task := developerReworkFollowupTask()
	reason := "请明确空值时应返回默认值还是错误。"
	payload := results.CanonicalResult{
		TaskID: task.TaskID, RunID: "run-rework-clarification", Status: results.StatusHumanRequired,
		Summary: "需要业务确认", BlockerReason: &reason,
	}

	updated, err := orch.withDeveloperReworkFollowups(context.Background(), task, payload)
	if err != nil {
		t.Fatalf("derive rework clarification: %v", err)
	}
	if len(updated.Writebacks) != 1 || len(updated.Effects) != 1 {
		t.Fatalf("expected one MR comment and one IM, got %#v / %#v", updated.Writebacks, updated.Effects)
	}
	writeback := updated.Writebacks[0]
	if writeback.DedupeKey != "developer-rework-clarification:task-rework-followup:run-rework-clarification" || writeback.Kind != CodeHostWritebackCreateMergeRequestNote || writeback.MergeRequestIID != 21 {
		t.Fatalf("unexpected clarification writeback: %#v", writeback)
	}
	for _, expected := range []string{"Developer Agent", reason, "直接补充必要信息", "MR"} {
		if !strings.Contains(writeback.Body, expected) {
			t.Fatalf("clarification body missing %q: %s", expected, writeback.Body)
		}
	}
	if strings.Contains(writeback.Body, ": resume") {
		t.Fatalf("clarification body must not require a resume keyword: %s", writeback.Body)
	}
	if strings.Contains(writeback.Body, "> - MR:") || strings.Contains(writeback.Body, "> - Issue:") {
		t.Fatalf("gitlab comment must not carry link lines: %s", writeback.Body)
	}
	imBody, ok := updated.Effects[0].Request["body"].(string)
	if !ok {
		t.Fatalf("clarification IM request must carry a body: %#v", updated.Effects[0].Request)
	}
	if !strings.Contains(imBody, "> - MR: [!21](https://gitlab.example/demo-group/demo-project/-/merge_requests/21)") {
		t.Fatalf("clarification IM body must contain linked MR reference, got %q", imBody)
	}
	if !strings.Contains(imBody, "> - Issue: #19") {
		t.Fatalf("clarification IM body must fall back to bare origin issue iid, got %q", imBody)
	}
	if updated.Effects[0].DedupeKey != "developer-rework-clarification-im:task-rework-followup:run-rework-clarification" {
		t.Fatalf("unexpected clarification effect: %#v", updated.Effects[0])
	}
}

func TestDeveloperReworkClarificationIMBody(t *testing.T) {
	reason := "请明确空值时应返回默认值还是错误。"
	body, err := developerReworkClarificationBody(results.CanonicalResult{BlockerReason: &reason})
	if err != nil {
		t.Fatalf("build rework clarification body: %v", err)
	}

	t.Run("linked mr and origin issue", func(t *testing.T) {
		contextValue, err := decodeDeveloperReworkContext(`{
			"origin_issue":{"iid":19,"url":"https://gitlab.example/demo-group/demo-project/-/issues/19","assignee_ids":[42]},
			"merge_request":{"iid":21,"url":"https://gitlab.example/demo-group/demo-project/-/merge_requests/21","source_branch":"developer/issue-19-20260806","assignee_ids":[42],"reviewer_ids":[43]},
			"rework_request":{"requester":{"id":44,"email":"requester@example.com"}}
		}`)
		if err != nil {
			t.Fatalf("decode rework context: %v", err)
		}
		imBody, err := developerReworkClarificationIMBody(contextValue, body)
		if err != nil {
			t.Fatalf("build rework clarification IM body: %v", err)
		}
		for _, expected := range []string{
			"> - MR: [!21](https://gitlab.example/demo-group/demo-project/-/merge_requests/21)",
			"> - Issue: [#19](https://gitlab.example/demo-group/demo-project/-/issues/19)",
			reason,
			"请在本 MR 下新建评论，直接补充必要信息即可。",
		} {
			if !strings.Contains(imBody, expected) {
				t.Fatalf("rework clarification IM body missing %q: %s", expected, imBody)
			}
		}
		segments := strings.Split(imBody, "\n\n")
		if len(segments) < 2 || segments[1] != "> - MR: [!21](https://gitlab.example/demo-group/demo-project/-/merge_requests/21)\n> - Issue: [#19](https://gitlab.example/demo-group/demo-project/-/issues/19)" {
			t.Fatalf("link lines must sit in the second paragraph, got %q", imBody)
		}
	})

	t.Run("unlinked origin issue falls back to bare iid", func(t *testing.T) {
		contextValue, err := decodeDeveloperReworkContext(`{
			"origin_issue":{"iid":19,"assignee_ids":[42]},
			"merge_request":{"iid":21,"url":"https://gitlab.example/demo-group/demo-project/-/merge_requests/21","source_branch":"developer/issue-19-20260806","assignee_ids":[42],"reviewer_ids":[43]},
			"rework_request":{"requester":{"id":44,"email":"requester@example.com"}}
		}`)
		if err != nil {
			t.Fatalf("decode rework context: %v", err)
		}
		imBody, err := developerReworkClarificationIMBody(contextValue, body)
		if err != nil {
			t.Fatalf("build rework clarification IM body: %v", err)
		}
		if !strings.Contains(imBody, "> - MR: [!21](https://gitlab.example/demo-group/demo-project/-/merge_requests/21)") {
			t.Fatalf("rework clarification IM body must keep linked MR reference, got %q", imBody)
		}
		if !strings.Contains(imBody, "> - Issue: #19") {
			t.Fatalf("rework clarification IM body must fall back to bare origin issue iid, got %q", imBody)
		}
	})

	t.Run("unmanaged merge request omits the issue link line", func(t *testing.T) {
		contextValue, err := decodeDeveloperReworkContext(`{
			"merge_request":{"iid":21,"url":"https://github.com/themarchrain/sdlc-coordinator/pull/21","source_branch":"feat/account-center"}
		}`)
		if err != nil {
			t.Fatalf("decode rework context: %v", err)
		}
		imBody, err := developerReworkClarificationIMBody(contextValue, body)
		if err != nil {
			t.Fatalf("build rework clarification IM body: %v", err)
		}
		segments := strings.Split(imBody, "\n\n")
		if len(segments) < 2 || segments[1] != "> - MR: [!21](https://github.com/themarchrain/sdlc-coordinator/pull/21)" {
			t.Fatalf("link block must carry only the MR line when there is no origin issue, got %q", imBody)
		}
	})

	t.Run("empty base body", func(t *testing.T) {
		if _, err := developerReworkClarificationIMBody(developerReworkContext{}, "  "); err == nil {
			t.Fatal("expected error for empty base body")
		}
	})
}

func newDeveloperReworkFollowupOrchestrator(t *testing.T) *Orchestrator {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(
		repository.NewTaskRepository(db),
		repository.NewRunRepository(db),
		WithAssigneeMentionDirectory(newFakeAssigneeDirectory(map[int]string{42: "负责人", 43: "审查人"})),
		WithDeveloperClarificationIMTarget("default"),
	)
}

func developerReworkFollowupTask() *model.Task {
	return &model.Task{
		TaskID: "task-rework-followup", Type: model.TaskTypeReworkMergeRequest, Role: "DeveloperWorker", TargetRepo: "demo-group/demo-project",
		ContextJSON: `{
			"origin_issue":{"iid":19,"assignee_ids":[42]},
			"merge_request":{"iid":21,"url":"https://gitlab.example/demo-group/demo-project/-/merge_requests/21","source_branch":"developer/issue-19-20260806","assignee_ids":[42],"reviewer_ids":[43]},
			"rework_request":{"requester":{"id":44,"email":"requester@example.com"}}
		}`,
	}
}

func TestDeveloperReworkUnmanagedMergeRequestWithoutOriginIssue(t *testing.T) {
	unmanaged := developerReworkFollowupTask()
	unmanaged.ContextJSON = `{
		"merge_request":{"iid":21,"url":"https://github.com/themarchrain/sdlc-coordinator/pull/21","source_branch":"feat/account-center","assignee_ids":[42],"reviewer_ids":[43]},
		"rework_request":{"requester":{"id":44,"email":"requester@example.com"}}
	}`

	t.Run("success derives the IM without an origin issue", func(t *testing.T) {
		orch := newDeveloperReworkFollowupOrchestrator(t)
		payload := results.CanonicalResult{
			TaskID: unmanaged.TaskID, RunID: "run-unmanaged-success", Status: results.StatusSucceeded,
			Summary: "已按最新评论清理退出全部按钮的状态逻辑。",
			Artifacts: map[string]any{
				"commit_sha":   "a78d201361f203f19a94d0c9f1f7588ace8782c7",
				"test_results": []results.TestEvidence{{Command: "git diff --check", Passed: true, Summary: "clean"}},
			},
		}
		updated, err := orch.withDeveloperReworkFollowups(context.Background(), unmanaged, payload)
		if err != nil {
			t.Fatalf("derive unmanaged rework followups: %v", err)
		}
		if len(updated.Writebacks) != 0 || len(updated.Effects) != 1 {
			t.Fatalf("expected one IM effect and no writeback, got %#v / %#v", updated.Writebacks, updated.Effects)
		}
		effect := updated.Effects[0]
		if effect.DedupeKey != "developer-rework-im:task-rework-followup:run-unmanaged-success" || effect.Critical {
			t.Fatalf("unexpected rework effect identity: %#v", effect)
		}
		body, _ := effect.Request["body"].(string)
		for _, expected := range []string{
			"[!21](https://github.com/themarchrain/sdlc-coordinator/pull/21)",
			"feat/account-center",
			"a78d2013",
			"git diff --check",
			"将进入增量代码审查",
		} {
			if !strings.Contains(body, expected) {
				t.Fatalf("unmanaged rework IM body missing %q: %s", expected, body)
			}
		}
		if strings.Contains(body, "Issue") {
			t.Fatalf("rework success body must not reference an origin issue: %s", body)
		}
	})

	t.Run("clarification derives the MR note and IM without an origin issue", func(t *testing.T) {
		orch := newDeveloperReworkFollowupOrchestrator(t)
		reason := "请确认空值时应返回默认值还是错误。"
		payload := results.CanonicalResult{
			TaskID: unmanaged.TaskID, RunID: "run-unmanaged-clarification", Status: results.StatusHumanRequired,
			Summary: "需要业务确认", BlockerReason: &reason,
		}
		updated, err := orch.withDeveloperReworkFollowups(context.Background(), unmanaged, payload)
		if err != nil {
			t.Fatalf("derive unmanaged rework clarification: %v", err)
		}
		if len(updated.Writebacks) != 1 || len(updated.Effects) != 1 {
			t.Fatalf("expected one MR comment and one IM, got %#v / %#v", updated.Writebacks, updated.Effects)
		}
		if updated.Writebacks[0].Kind != CodeHostWritebackCreateMergeRequestNote || updated.Writebacks[0].MergeRequestIID != 21 {
			t.Fatalf("unexpected clarification writeback: %#v", updated.Writebacks[0])
		}
		imBody, _ := updated.Effects[0].Request["body"].(string)
		if !strings.Contains(imBody, "> - MR: [!21](https://github.com/themarchrain/sdlc-coordinator/pull/21)") {
			t.Fatalf("clarification IM must keep the linked MR reference, got %q", imBody)
		}
		if strings.Contains(imBody, "Issue") {
			t.Fatalf("clarification IM must omit the issue line without an origin issue, got %q", imBody)
		}
	})
}

func TestDecodeDeveloperReworkContextRequiresOnlyMergeRequest(t *testing.T) {
	contextValue, err := decodeDeveloperReworkContext(`{
		"merge_request":{"iid":21,"url":"https://github.com/themarchrain/sdlc-coordinator/pull/21","source_branch":"feat/account-center"},
		"rework_request":{"requester":{"email":"requester@example.com"}}
	}`)
	if err != nil {
		t.Fatalf("unmanaged rework context must decode without an origin issue: %v", err)
	}
	if contextValue.OriginIssue.IID != 0 || contextValue.MergeRequest.IID != 21 {
		t.Fatalf("unexpected decoded rework context: %#v", contextValue)
	}

	rejected := map[string]string{
		"missing merge request iid":    `{"merge_request":{"url":"https://example.test/pull/21","source_branch":"feat/x"}}`,
		"missing merge request url":    `{"merge_request":{"iid":21,"source_branch":"feat/x"}}`,
		"missing merge request branch": `{"merge_request":{"iid":21,"url":"https://example.test/pull/21"}}`,
		"invalid json":                 `{`,
	}
	for name, raw := range rejected {
		if _, err := decodeDeveloperReworkContext(raw); err == nil {
			t.Fatalf("%s must still be rejected", name)
		}
	}
}
