package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

func TestReviewerVerdictLabelMapsAllVerdictsToChinese(t *testing.T) {
	tests := []struct {
		verdict string
		want    string
	}{
		{verdict: "approved", want: "✅ 可合并"},
		{verdict: "changes_requested", want: "❕ 暂不建议合并"},
		{verdict: "human_required", want: "❔ 需人工确认"},
		{verdict: "no_new_changes", want: "☑️ 本轮无新增可审内容"},
	}
	for _, tt := range tests {
		if got := reviewerVerdictLabel(tt.verdict); got != tt.want {
			t.Fatalf("reviewerVerdictLabel(%q) = %q, want %q", tt.verdict, got, tt.want)
		}
	}
}

func TestReviewerVerdictLabelFallsBackToRawVerdict(t *testing.T) {
	if got := reviewerVerdictLabel("unknown_verdict"); got != "unknown_verdict" {
		t.Fatalf("reviewerVerdictLabel(unknown) = %q, want raw value", got)
	}
}

func TestReviewerResultIMShowsChineseVerdictLabel(t *testing.T) {
	tests := []struct {
		verdict string
		want    string
	}{
		{verdict: "approved", want: "审查结果：✅ 可合并"},
		{verdict: "changes_requested", want: "审查结果：❕ 暂不建议合并"},
		{verdict: "human_required", want: "审查结果：❔ 需人工确认"},
	}
	for _, tt := range tests {
		payload := results.CanonicalResult{
			Summary: "审查完成",
			Artifacts: map[string]any{
				"review_verdict":      tt.verdict,
				"review_test_summary": "相关测试通过",
			},
		}
		body, err := reviewerResultIMBody(reviewerMergeRequest{
			IID:        21,
			URL:        "https://gitlab.example/group/project/-/merge_requests/21",
			Title:      "fix null handling",
			ReviewMode: "incremental",
			HeadSHA:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			BaseSHA:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		}, payload)
		if err != nil {
			t.Fatalf("build reviewer IM body for %s: %v", tt.verdict, err)
		}
		if !strings.Contains(body, tt.want) {
			t.Fatalf("reviewer IM body for %s must contain %q, got %q", tt.verdict, tt.want, body)
		}
		if strings.Contains(body, "Verdict") {
			t.Fatalf("reviewer IM body for %s must not use raw Verdict field, got %q", tt.verdict, body)
		}
	}
}

func TestReviewerResultCreatesIMEffectOnlyAfterMergeRequestCommentSucceeds(t *testing.T) {
	ctx := context.Background()
	orch, effects, task, runID := newReviewerFollowupFixture(t, &fakeGitLabWritebackBackend{})

	if _, _, err := orch.ApplyCanonicalResult(ctx, reviewerResultPayload(task, runID)); err != nil {
		t.Fatalf("apply reviewer result: %v", err)
	}

	effect, err := effects.GetByDedupeKey(ctx, "reviewer-result-im:"+task.TaskID+":"+runID)
	if err != nil {
		t.Fatalf("get reviewer IM effect: %v", err)
	}
	if effect.Kind != "send_im_message" || effect.Target != "default" || effect.Critical {
		t.Fatalf("unexpected reviewer IM effect: %#v", effect)
	}
	if effect.Status != model.EffectStatusPending {
		t.Fatalf("expected pending reviewer IM effect, got %s", effect.Status)
	}

	var request struct {
		Body     string   `json:"body"`
		Mentions []string `json:"mentions"`
		Format   string   `json:"format"`
	}
	if err := json.Unmarshal([]byte(effect.RequestJSON), &request); err != nil {
		t.Fatalf("decode reviewer IM request: %v", err)
	}
	for _, expected := range []string{
		"https://gitlab.example/demo-group/lite-list/-/merge_requests/12",
		"源分支新 Push 后增量审查",
		"25b03600",
		"审查结果：✅ 可合并",
		"相关测试通过",
		"没有发现阻塞问题",
	} {
		if !strings.Contains(request.Body, expected) {
			t.Fatalf("expected reviewer IM body to contain %q, got %q", expected, request.Body)
		}
	}
	if request.Format != "markdown" {
		t.Fatalf("expected markdown format, got %q", request.Format)
	}
	if len(request.Mentions) != 2 || request.Mentions[0] != "reviewer@example.com" || request.Mentions[1] != "Demo User" {
		t.Fatalf("unexpected reviewer IM mentions: %#v", request.Mentions)
	}
}

func TestReviewerNoNewChangesIMPreservesPreviousSubstantiveVerdict(t *testing.T) {
	mr := reviewerMergeRequest{
		IID:        21,
		URL:        "https://gitlab.example/group/project/-/merge_requests/21",
		Title:      "sync target branch",
		ReviewMode: "incremental",
		HeadSHA:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		BaseSHA:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	payload := results.CanonicalResult{
		Summary: "target branch sync introduced no reviewable changes",
		Artifacts: map[string]any{
			"review_verdict":      "no_new_changes",
			"review_test_summary": "未运行测试：本轮没有可审查差异",
		},
	}

	body, err := reviewerResultIMBody(mr, payload)
	if err != nil {
		t.Fatalf("build reviewer IM body: %v", err)
	}
	for _, expected := range []string{"本轮无新增可审内容", "不产生新的合并建议", "以最近一次实质审查结论为准"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected no-new-changes IM to contain %q, got %q", expected, body)
		}
	}
	if strings.Contains(body, "AI Review 结论：建议合并") {
		t.Fatalf("no-new-changes IM must not create a new merge recommendation: %q", body)
	}
}

func TestReviewerResultMentionsOriginDeveloperAssigneeInsteadOfAgentAuthor(t *testing.T) {
	ctx := context.Background()
	orch, effects, task, runID := newReviewerFollowupFixture(t, &fakeGitLabWritebackBackend{})
	task.ContextJSON = strings.Replace(task.ContextJSON, "demo.user@example.com", "agent@example.com", 1)
	if err := orch.tasks.Update(ctx, task); err != nil {
		t.Fatalf("update reviewer author: %v", err)
	}

	originTask := &model.Task{
		TaskID: "task-origin-developer", DedupeKey: "gitlab:issue-develop:15594:13", Type: "develop_from_issue", Role: "DeveloperWorker",
		TargetRepo: "demo-group/lite-list", Status: model.TaskStatusSucceeded, ContextJSON: `{"issue":{"iid":13,"assignee_ids":[7101]}}`,
	}
	if err := orch.tasks.Create(ctx, originTask); err != nil {
		t.Fatalf("create origin developer task: %v", err)
	}
	if err := orch.writebacks.UpsertByDedupeKey(ctx, &model.Writeback{
		TaskID: originTask.TaskID, RunID: "run-origin-developer", DedupeKey: "developer-origin-mr", Kind: CodeHostWritebackCreateMergeRequest,
		ProjectID: "demo-group/lite-list", Status: model.WritebackStatusSucceeded, ExternalID: "12",
	}); err != nil {
		t.Fatalf("create origin MR writeback: %v", err)
	}

	if _, _, err := orch.ApplyCanonicalResult(ctx, reviewerResultPayload(task, runID)); err != nil {
		t.Fatalf("apply reviewer result: %v", err)
	}
	effect, err := effects.GetByDedupeKey(ctx, "reviewer-result-im:"+task.TaskID+":"+runID)
	if err != nil {
		t.Fatalf("get reviewer IM effect: %v", err)
	}
	var request struct {
		Mentions []string `json:"mentions"`
	}
	if err := json.Unmarshal([]byte(effect.RequestJSON), &request); err != nil {
		t.Fatalf("decode reviewer IM request: %v", err)
	}
	if len(request.Mentions) != 2 || request.Mentions[0] != "reviewer@example.com" || request.Mentions[1] != "alice" {
		t.Fatalf("unexpected origin reviewer mentions: %#v", request.Mentions)
	}
	for _, mention := range request.Mentions {
		if mention == "agent@example.com" {
			t.Fatalf("agent commit author must not be mentioned: %#v", request.Mentions)
		}
	}
}

func TestReviewerResultDoesNotCreateIMEffectWhenMergeRequestCommentFails(t *testing.T) {
	ctx := context.Background()
	orch, effects, task, runID := newReviewerFollowupFixture(t, &fakeGitLabWritebackBackend{err: errors.New("gitlab unavailable")})

	if _, _, err := orch.ApplyCanonicalResult(ctx, reviewerResultPayload(task, runID)); err == nil {
		t.Fatal("expected failed merge request comment writeback")
	}
	if _, err := effects.GetByDedupeKey(ctx, "reviewer-result-im:"+task.TaskID+":"+runID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("reviewer IM effect must not exist before MR comment succeeds, got %v", err)
	}
}

func TestReviewerResultDoesNotFailWhenNonCriticalIMContextIsIncomplete(t *testing.T) {
	ctx := context.Background()
	orch, effects, task, runID := newReviewerFollowupFixture(t, &fakeGitLabWritebackBackend{})
	task.ContextJSON = `{"merge_request":{"iid":12}}`
	if err := orch.tasks.Update(ctx, task); err != nil {
		t.Fatalf("update reviewer task context: %v", err)
	}

	updatedTask, updatedRun, err := orch.ApplyCanonicalResult(ctx, reviewerResultPayload(task, runID))
	if err != nil {
		t.Fatalf("noncritical reviewer IM must not fail result finalization: %v", err)
	}
	if updatedTask.Status != model.TaskStatusSucceeded || updatedRun.Status != model.RunStatusSucceeded {
		t.Fatalf("expected reviewer result to succeed, got task=%s run=%s", updatedTask.Status, updatedRun.Status)
	}
	if _, err := effects.GetByDedupeKey(ctx, "reviewer-result-im:"+task.TaskID+":"+runID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("incomplete IM context must be skipped, got %v", err)
	}
}

func newReviewerFollowupFixture(
	t *testing.T,
	backend *fakeGitLabWritebackBackend,
) (*Orchestrator, *repository.EffectRepository, *model.Task, string) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Writeback{}, &model.Effect{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	tasks := repository.NewTaskRepository(db)
	runs := repository.NewRunRepository(db)
	writebacks := repository.NewWritebackRepository(db)
	effects := repository.NewEffectRepository(db)
	runID := "run-reviewer-followup"
	repositoryID := fixtureRepositoryID
	task := &model.Task{
		TaskID:       "task-reviewer-followup",
		DedupeKey:    "gitlab:mr-review:15594:12:25b03600",
		Type:         "review_mr",
		Role:         "ReviewerWorker",
		TargetRepo:   "demo-group/lite-list",
		RepositoryID: &repositoryID,
		Status:       model.TaskStatusRunning,
		CurrentRunID: &runID,
		ContextJSON: `{
			"project": {"id": "15594", "path": "demo-group/lite-list"},
			"merge_request": {
				"iid": 12,
				"url": "https://gitlab.example/demo-group/lite-list/-/merge_requests/12",
				"title": "Fix status normalization",
				"review_mode": "incremental",
				"head_sha": "25b0360000000000000000000000000000000000",
				"base_sha": "de51000000000000000000000000000000000000",
				"author": {"name": "Demo User", "email": "demo.user@example.com"}
			}
		}`,
	}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runs.Create(ctx, &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	orch := New(
		tasks,
		runs,
		WithWritebackRepository(writebacks),
		WithEffectRepository(effects),
		WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, backend)),
		WithDeveloperClarificationIMTarget("team-review-group"),
		WithAssigneeMentionDirectory(newFakeAssigneeDirectory(map[int]string{7101: "alice"})),
		WithReviewerIMMentions([]string{"reviewer@example.com", "reviewer@example.com"}),
	)
	return orch, effects, task, runID
}

func reviewerResultPayload(task *model.Task, runID string) results.CanonicalResult {
	return results.CanonicalResult{
		TaskID:  task.TaskID,
		RunID:   runID,
		Status:  results.StatusSucceeded,
		Summary: "没有发现阻塞问题",
		Artifacts: map[string]any{
			"review_verdict":      "approved",
			"review_findings":     []any{},
			"review_test_summary": "相关测试通过",
		},
		Writebacks: []results.CodeHostWritebackIntent{{
			Kind:            CodeHostWritebackCreateMergeRequestNote,
			ProjectID:       task.TargetRepo,
			MergeRequestIID: 12,
			Body:            "本次增量审查未发现阻塞问题。",
		}},
	}
}
