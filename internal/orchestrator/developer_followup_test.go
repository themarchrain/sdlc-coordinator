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

func TestDeveloperClarificationDoesNotMaterializeIMEffectWhenIssueNoteFails(t *testing.T) {
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
	currentRunID := "run-clarification-writeback-failure"
	repositoryID := fixtureRepositoryID
	task := &model.Task{
		TaskID:       "task-clarification-writeback-failure",
		DedupeKey:    "gitlab:issue-develop:15594:2",
		Type:         "develop_from_issue",
		Role:         "DeveloperWorker",
		TargetRepo:   "demo-group/lite-list",
		RepositoryID: &repositoryID,
		Status:       model.TaskStatusRunning,
		CurrentRunID: &currentRunID,
		ContextJSON:  `{"issue":{"iid":2,"assignee_ids":[7101]}}`,
	}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runs.Create(ctx, &model.Run{RunID: currentRunID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	orch := New(
		tasks,
		runs,
		WithWritebackRepository(writebacks),
		WithEffectRepository(effects),
		WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, &fakeGitLabWritebackBackend{err: errors.New("gitlab unavailable")})),
		WithAssigneeMentionDirectory(newFakeAssigneeDirectory(map[int]string{7101: "负责人"})),
		WithDeveloperClarificationIMTarget("team-review-group"),
	)
	reason := "缺少接口字段定义"
	_, _, err = orch.ApplyCanonicalResult(ctx, results.CanonicalResult{
		TaskID: task.TaskID, RunID: currentRunID, Status: results.StatusHumanRequired, BlockerReason: &reason,
	})
	if err == nil {
		t.Fatal("expected failed GitLab clarification writeback")
	}

	if _, err := effects.GetByDedupeKey(ctx, "developer-clarification-im:"+task.TaskID+":"+currentRunID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("IM effect must not exist before the clarification comment succeeds, got %v", err)
	}
}

func TestDeveloperClarificationBodyRequestsDirectIssueComment(t *testing.T) {
	reason := "请补充 status 字段的允许值。"
	body, err := developerClarificationBody(results.CanonicalResult{BlockerReason: &reason})
	if err != nil {
		t.Fatalf("build clarification body: %v", err)
	}
	for _, expected := range []string{"Developer Agent", reason, "本 Issue", "直接补充必要信息"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("clarification body missing %q: %s", expected, body)
		}
	}
	if strings.Contains(body, ": resume") {
		t.Fatalf("clarification body must not require a resume keyword: %s", body)
	}
}

func TestDeveloperDeliveryCreatesIMEffectOnlyAfterMergeRequestWritebackSucceeds(t *testing.T) {
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
	currentRunID := "run-developer-delivery-im"
	repositoryID := fixtureRepositoryID
	task := &model.Task{
		TaskID:       "task-developer-delivery-im",
		DedupeKey:    "gitlab:issue-develop:15594:11",
		Type:         "develop_from_issue",
		Role:         "DeveloperWorker",
		TargetRepo:   "demo-group/lite-list",
		RepositoryID: &repositoryID,
		Status:       model.TaskStatusRunning,
		CurrentRunID: &currentRunID,
		ContextJSON:  `{"issue":{"iid":11,"url":"https://gitlab.example/demo-group/lite-list/-/issues/11","assignee_ids":[7101]}}`,
	}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runs.Create(ctx, &model.Run{RunID: currentRunID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	orch := New(
		tasks,
		runs,
		WithWritebackRepository(writebacks),
		WithEffectRepository(effects),
		WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, &fakeGitLabWritebackBackend{})),
		WithAssigneeMentionDirectory(newFakeAssigneeDirectory(map[int]string{7101: "负责人"})),
		WithDeveloperClarificationIMTarget("team-review-group"),
	)
	payload := developerDeliveryPayload(task, currentRunID)
	if _, _, err := orch.ApplyCanonicalResult(ctx, payload); err != nil {
		t.Fatalf("apply developer delivery result: %v", err)
	}

	effect, err := effects.GetByDedupeKey(ctx, "developer-delivery-im:"+task.TaskID+":"+currentRunID)
	if err != nil {
		t.Fatalf("get developer delivery IM effect: %v", err)
	}
	if effect.Kind != "send_im_message" || effect.Target != "default" || effect.Critical {
		t.Fatalf("unexpected delivery IM effect: %#v", effect)
	}
	var request struct {
		Body     string   `json:"body"`
		Mentions []string `json:"mentions"`
	}
	if err := json.Unmarshal([]byte(effect.RequestJSON), &request); err != nil {
		t.Fatalf("decode delivery IM request: %v", err)
	}
	if !strings.Contains(request.Body, "https://gitlab.example/group/framework/-/merge_requests/7") || !strings.Contains(request.Body, "Issue: [#11](https://gitlab.example/demo-group/lite-list/-/issues/11)") {
		t.Fatalf("delivery IM body must contain confirmed MR URL and linked Issue, got %q", request.Body)
	}
	if len(request.Mentions) != 1 || request.Mentions[0] != "负责人" {
		t.Fatalf("unexpected delivery IM mentions: %#v", request.Mentions)
	}
}

func TestDeveloperDeliveryStillNotifiesWithoutResolvedMentions(t *testing.T) {
	// mentions gate 翻转:人员解析缺席时交付通知照发,body 无 @ 纯文本。
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
	currentRunID := "run-developer-delivery-unmapped"
	repositoryID := fixtureRepositoryID
	task := &model.Task{
		TaskID:       "task-developer-delivery-unmapped",
		DedupeKey:    "gitlab:issue-develop:15594:unmapped",
		Type:         "develop_from_issue",
		Role:         "DeveloperWorker",
		TargetRepo:   "demo-group/lite-list",
		RepositoryID: &repositoryID,
		Status:       model.TaskStatusRunning,
		CurrentRunID: &currentRunID,
		ContextJSON:  `{"issue":{"iid":11,"assignee_ids":[7101]}}`,
	}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runs.Create(ctx, &model.Run{RunID: currentRunID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	orch := New(tasks, runs,
		WithWritebackRepository(writebacks),
		WithEffectRepository(effects),
		WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, &fakeGitLabWritebackBackend{})),
		WithAssigneeMentionDirectory(fakeAssigneeDirectory{}),
		WithDeveloperClarificationIMTarget("team-review-group"),
	)
	if _, _, err := orch.ApplyCanonicalResult(ctx, developerDeliveryPayload(task, currentRunID)); err != nil {
		t.Fatalf("apply developer delivery result: %v", err)
	}

	effect, err := effects.GetByDedupeKey(ctx, "developer-delivery-im:"+task.TaskID+":"+currentRunID)
	if err != nil {
		t.Fatalf("delivery IM effect must exist without resolved mentions: %v", err)
	}
	var request struct {
		Mentions []string `json:"mentions"`
	}
	if err := json.Unmarshal([]byte(effect.RequestJSON), &request); err != nil {
		t.Fatalf("decode delivery IM request: %v", err)
	}
	if len(request.Mentions) != 0 {
		t.Fatalf("unresolved mentions must degrade to plain text, got %#v", request.Mentions)
	}
}

func TestDeveloperDeliveryDoesNotCreateIMEffectWhenWritebackFailed(t *testing.T) {
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
	currentRunID := "run-developer-delivery-failed-writeback"
	repositoryID := fixtureRepositoryID
	task := &model.Task{
		TaskID:       "task-developer-delivery-failed-writeback",
		DedupeKey:    "gitlab:issue-develop:15594:failed",
		Type:         "develop_from_issue",
		Role:         "DeveloperWorker",
		TargetRepo:   "demo-group/lite-list",
		RepositoryID: &repositoryID,
		Status:       model.TaskStatusRunning,
		CurrentRunID: &currentRunID,
		ContextJSON:  `{"issue":{"iid":11,"assignee_ids":[7101]}}`,
	}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runs.Create(ctx, &model.Run{RunID: currentRunID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	orch := New(tasks, runs,
		WithWritebackRepository(writebacks),
		WithEffectRepository(effects),
		WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, &fakeGitLabWritebackBackend{err: errors.New("gitlab unavailable")})),
		WithAssigneeMentionDirectory(newFakeAssigneeDirectory(map[int]string{7101: "负责人"})),
		WithDeveloperClarificationIMTarget("team-review-group"),
	)
	if _, _, err := orch.ApplyCanonicalResult(ctx, developerDeliveryPayload(task, currentRunID)); err == nil {
		t.Fatal("expected failed merge request writeback")
	}
	if _, err := effects.GetByDedupeKey(ctx, "developer-delivery-im:"+task.TaskID+":"+currentRunID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("delivery IM effect must not exist on failed writeback, got %v", err)
	}
}

func TestDeveloperClarificationIMBody(t *testing.T) {
	reason := "请补充 status 字段的允许值。"
	body, err := developerClarificationBody(results.CanonicalResult{BlockerReason: &reason})
	if err != nil {
		t.Fatalf("build clarification body: %v", err)
	}

	t.Run("linked issue", func(t *testing.T) {
		imBody, err := developerClarificationIMBody(developerIssue{IID: 30, URL: "https://gitlab.example/demo-group/lite-list/-/issues/30"}, body)
		if err != nil {
			t.Fatalf("build clarification IM body: %v", err)
		}
		if !strings.Contains(imBody, "> - Issue: [#30](https://gitlab.example/demo-group/lite-list/-/issues/30)") {
			t.Fatalf("clarification IM body must contain linked issue reference, got %q", imBody)
		}
		for _, expected := range []string{reason, "请在本 Issue 下新建评论，直接补充必要信息即可。"} {
			if !strings.Contains(imBody, expected) {
				t.Fatalf("clarification IM body missing %q: %s", expected, imBody)
			}
		}
		segments := strings.Split(imBody, "\n\n")
		if len(segments) < 2 || segments[1] != "> - Issue: [#30](https://gitlab.example/demo-group/lite-list/-/issues/30)" {
			t.Fatalf("link line must sit in the second paragraph, got %q", imBody)
		}
	})

	t.Run("multi-paragraph reason preserved verbatim", func(t *testing.T) {
		multiReason := "第一段说明。\n\n第二段补充说明。"
		multiBody, err := developerClarificationBody(results.CanonicalResult{BlockerReason: &multiReason})
		if err != nil {
			t.Fatalf("build clarification body: %v", err)
		}
		imBody, err := developerClarificationIMBody(developerIssue{IID: 30, URL: "https://gitlab.example/demo-group/lite-list/-/issues/30"}, multiBody)
		if err != nil {
			t.Fatalf("build clarification IM body: %v", err)
		}
		if !strings.Contains(imBody, "第一段说明。\n\n第二段补充说明。") {
			t.Fatalf("clarification IM body must embed multi-paragraph reason verbatim, got %q", imBody)
		}
	})

	t.Run("unlinked issue falls back to bare iid", func(t *testing.T) {
		imBody, err := developerClarificationIMBody(developerIssue{IID: 30}, body)
		if err != nil {
			t.Fatalf("build clarification IM body: %v", err)
		}
		if !strings.Contains(imBody, "> - Issue: #30") {
			t.Fatalf("clarification IM body must fall back to bare issue iid, got %q", imBody)
		}
	})

	t.Run("empty base body", func(t *testing.T) {
		if _, err := developerClarificationIMBody(developerIssue{IID: 30}, "  "); err == nil {
			t.Fatal("expected error for empty base body")
		}
	})
}

func TestDeveloperClarificationDerivesIssueCommentAndIM(t *testing.T) {
	orch := newDeveloperClarificationFollowupOrchestrator(t)
	task := &model.Task{
		TaskID: "task-clarification", Type: "develop_from_issue", Role: "DeveloperWorker", TargetRepo: "demo-group/lite-list",
		ContextJSON: `{"issue":{"iid":30,"url":"https://gitlab.example/demo-group/lite-list/-/issues/30","assignee_ids":[7101]}}`,
	}
	reason := "请补充接口字段定义。"
	payload := results.CanonicalResult{
		TaskID: task.TaskID, RunID: "run-clarification", Status: results.StatusHumanRequired, BlockerReason: &reason,
	}
	updated, err := orch.withDeveloperClarificationFollowups(task, payload)
	if err != nil {
		t.Fatalf("derive clarification: %v", err)
	}
	if len(updated.Writebacks) != 1 || len(updated.Effects) != 1 {
		t.Fatalf("expected one issue comment and one IM, got %#v / %#v", updated.Writebacks, updated.Effects)
	}
	if strings.Contains(updated.Writebacks[0].Body, "> - Issue:") {
		t.Fatalf("gitlab comment must not carry a link line: %s", updated.Writebacks[0].Body)
	}
	imBody, ok := updated.Effects[0].Request["body"].(string)
	if !ok {
		t.Fatalf("clarification IM request must carry a body: %#v", updated.Effects[0].Request)
	}
	if !strings.Contains(imBody, "> - Issue: [#30](https://gitlab.example/demo-group/lite-list/-/issues/30)") {
		t.Fatalf("clarification IM body must contain linked issue reference, got %q", imBody)
	}
}

func newDeveloperClarificationFollowupOrchestrator(t *testing.T) *Orchestrator {
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
		WithAssigneeMentionDirectory(newFakeAssigneeDirectory(map[int]string{7101: "负责人"})),
		WithDeveloperClarificationIMTarget("default"),
	)
}

func developerDeliveryPayload(task *model.Task, runID string) results.CanonicalResult {
	return results.CanonicalResult{
		TaskID:  task.TaskID,
		RunID:   runID,
		Status:  results.StatusSucceeded,
		Summary: "开发完成，测试通过",
		Writebacks: []results.CodeHostWritebackIntent{{
			Kind:         CodeHostWritebackCreateMergeRequest,
			ProjectID:    task.TargetRepo,
			Title:        "fix: requested behavior",
			Description:  "完成修复",
			SourceBranch: model.DeliveryBranchForTask(task.TaskID),
			TargetBranch: "main",
		}},
	}
}
