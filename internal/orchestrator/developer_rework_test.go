package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

func TestStartMergeRequestReworkCreatesTaskFromManagedMergeRequest(t *testing.T) {
	orch, tasks, writebacks := newDeveloperReworkFixture(t)
	ctx := context.Background()
	origin := seedManagedMergeRequest(t, ctx, tasks, writebacks)

	created, err := orch.StartOrResumeMergeRequestRework(ctx, sampleMergeRequestReworkRequest())
	if err != nil {
		t.Fatalf("start rework: %v", err)
	}
	if created.Type != model.TaskTypeReworkMergeRequest || created.Role != "DeveloperWorker" || created.Status != model.TaskStatusPending {
		t.Fatalf("unexpected rework task identity: %#v", created)
	}
	if created.DedupeKey != "gitlab:mr-rework:1001:21" || created.DeliveryBranch != "developer/issue-19-20260806" {
		t.Fatalf("unexpected rework task delivery: %#v", created)
	}

	var taskContext map[string]any
	if err := json.Unmarshal([]byte(created.ContextJSON), &taskContext); err != nil {
		t.Fatalf("decode rework context: %v", err)
	}
	for _, key := range []string{"origin_issue", "merge_request", "rework_request", "repos"} {
		if taskContext[key] == nil {
			t.Fatalf("expected %s in rework context: %#v", key, taskContext)
		}
	}
	mergeRequest := taskContext["merge_request"].(map[string]any)
	if mergeRequest["expected_head_sha"] != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || mergeRequest["source_branch"] != created.DeliveryBranch {
		t.Fatalf("unexpected merge request context: %#v", mergeRequest)
	}
	originIssue := taskContext["origin_issue"].(map[string]any)
	if originIssue["iid"] != float64(19) || originIssue["objective"] != origin.Objective {
		t.Fatalf("unexpected origin issue context: %#v", originIssue)
	}
}

func TestStartMergeRequestReworkAllowsOriginIssueAssignee(t *testing.T) {
	orch, tasks, writebacks := newDeveloperReworkFixture(t)
	ctx := context.Background()
	seedManagedMergeRequest(t, ctx, tasks, writebacks)

	request := sampleMergeRequestReworkRequest()
	request.RequesterPreAuthorized = false
	request.AssigneeIDs = []int{777}
	request.ReviewerIDs = []int{778}
	created, err := orch.StartOrResumeMergeRequestRework(ctx, request)
	if err != nil {
		t.Fatalf("expected origin issue assignee to be authorized: %v", err)
	}
	if created == nil || created.Status != model.TaskStatusPending {
		t.Fatalf("unexpected rework task: %#v", created)
	}
}

func TestStartMergeRequestReworkAllowsOriginIssueAssigneeFromTypedContext(t *testing.T) {
	orch, tasks, writebacks := newDeveloperReworkFixture(t)
	ctx := context.Background()
	seedManagedMergeRequest(t, ctx, tasks, writebacks)
	origin, err := tasks.GetByTaskID(ctx, "task-origin-issue-19")
	if err != nil {
		t.Fatalf("get origin task: %v", err)
	}
	origin.ContextJSON = `{"issue":{"iid":19,"assignee_ids":[42]}}`
	if err := tasks.Update(ctx, origin); err != nil {
		t.Fatalf("update typed origin context: %v", err)
	}

	request := sampleMergeRequestReworkRequest()
	request.RequesterPreAuthorized = false
	created, err := orch.StartOrResumeMergeRequestRework(ctx, request)
	if err != nil {
		t.Fatalf("expected typed origin issue assignee to be authorized: %v", err)
	}
	if created == nil {
		t.Fatal("expected rework task")
	}
}

func TestIssueAssigneeIDsAcceptsPersistedAndTypedValues(t *testing.T) {
	for name, value := range map[string]any{
		"persisted json": []any{float64(42), float64(43)},
		"typed ints":     []int{42, 43},
		"mixed numbers":  []any{int64(42), 43, 44.0, 44.5},
	} {
		t.Run(name, func(t *testing.T) {
			ids := issueAssigneeIDs(value)
			if len(ids) < 2 || ids[0] != 42 || ids[1] != 43 {
				t.Fatalf("unexpected assignee ids: %#v", ids)
			}
		})
	}
}

func TestStartMergeRequestReworkRejectsRequesterOutsideManagedIssueAndMR(t *testing.T) {
	orch, tasks, writebacks := newDeveloperReworkFixture(t)
	ctx := context.Background()
	seedManagedMergeRequest(t, ctx, tasks, writebacks)

	request := sampleMergeRequestReworkRequest()
	request.RequesterID = 99
	request.RequesterPreAuthorized = false
	request.AssigneeIDs = []int{777}
	request.ReviewerIDs = []int{778}
	_, err := orch.StartOrResumeMergeRequestRework(ctx, request)
	if !errors.Is(err, model.ErrMergeRequestReworkRequesterUnauthorized) {
		t.Fatalf("expected unauthorized requester error, got %v", err)
	}
}

func TestStartMergeRequestReworkIncludesLatestReviewerComment(t *testing.T) {
	orch, tasks, writebacks := newDeveloperReworkFixture(t)
	ctx := context.Background()
	seedManagedMergeRequest(t, ctx, tasks, writebacks)
	seedReviewerComment(t, ctx, tasks, writebacks, "review-old", "run-review-old", "旧审查结论")
	seedReviewerComment(t, ctx, tasks, writebacks, "review-new", "run-review-new", "最新审查结论：修复空值风险")

	created, err := orch.StartOrResumeMergeRequestRework(ctx, sampleMergeRequestReworkRequest())
	if err != nil {
		t.Fatalf("start rework: %v", err)
	}
	var taskContext map[string]any
	if err := json.Unmarshal([]byte(created.ContextJSON), &taskContext); err != nil {
		t.Fatalf("decode rework context: %v", err)
	}
	latestReview, ok := taskContext["latest_review"].(map[string]any)
	if !ok {
		t.Fatalf("expected latest_review context: %#v", taskContext)
	}
	if latestReview["task_id"] != "review-new" || latestReview["run_id"] != "run-review-new" {
		t.Fatalf("unexpected latest review identity: %#v", latestReview)
	}
	if latestReview["comment_body"] != "最新审查结论：修复空值风险" {
		t.Fatalf("unexpected latest review body: %#v", latestReview)
	}
	if strings.Contains(latestReview["comment_body"].(string), "sdlc-coordinator-operation") {
		t.Fatalf("latest review body must not expose operation marker: %#v", latestReview)
	}
}

func TestStartMergeRequestReworkOmitsLatestReviewWhenNoReviewerCommentExists(t *testing.T) {
	orch, tasks, writebacks := newDeveloperReworkFixture(t)
	ctx := context.Background()
	seedManagedMergeRequest(t, ctx, tasks, writebacks)

	created, err := orch.StartOrResumeMergeRequestRework(ctx, sampleMergeRequestReworkRequest())
	if err != nil {
		t.Fatalf("start rework: %v", err)
	}
	var taskContext map[string]any
	if err := json.Unmarshal([]byte(created.ContextJSON), &taskContext); err != nil {
		t.Fatalf("decode rework context: %v", err)
	}
	if _, exists := taskContext["latest_review"]; exists {
		t.Fatalf("expected latest_review to be omitted: %#v", taskContext)
	}
}

func TestStartMergeRequestReworkSkipsMalformedLatestReviewerComment(t *testing.T) {
	orch, tasks, writebacks := newDeveloperReworkFixture(t)
	ctx := context.Background()
	seedManagedMergeRequest(t, ctx, tasks, writebacks)
	reviewTask := &model.Task{TaskID: "review-malformed", DedupeKey: "review-malformed", Type: "review_mr", Role: "ReviewerWorker", Status: model.TaskStatusSucceeded}
	if err := tasks.Create(ctx, reviewTask); err != nil {
		t.Fatalf("create malformed review task: %v", err)
	}
	marker := "<!-- sdlc-coordinator-operation:codehost:review-malformed:run-malformed:0:create_merge_request_note -->"
	requestJSON, err := json.Marshal(map[string]string{"body": "review without the persisted marker"})
	if err != nil {
		t.Fatalf("encode malformed review: %v", err)
	}
	if err := writebacks.Create(ctx, &model.Writeback{TaskID: reviewTask.TaskID, RunID: "run-malformed", DedupeKey: "review-malformed-note", Kind: "create_merge_request_note", ProjectID: "demo-group/demo-project", TargetType: "merge_request", TargetIID: 21, Status: model.WritebackStatusSucceeded, OperationMarker: marker, RequestJSON: string(requestJSON)}); err != nil {
		t.Fatalf("create malformed review writeback: %v", err)
	}

	created, err := orch.StartOrResumeMergeRequestRework(ctx, sampleMergeRequestReworkRequest())
	if err != nil {
		t.Fatalf("malformed review must not block rework: %v", err)
	}
	var taskContext map[string]any
	if err := json.Unmarshal([]byte(created.ContextJSON), &taskContext); err != nil {
		t.Fatalf("decode rework context: %v", err)
	}
	if _, exists := taskContext["latest_review"]; exists {
		t.Fatalf("malformed review must be omitted: %#v", taskContext)
	}
}

func TestStartMergeRequestReworkReusesWaitingTaskWithNewRunContext(t *testing.T) {
	orch, tasks, writebacks := newDeveloperReworkFixture(t)
	ctx := context.Background()
	seedManagedMergeRequest(t, ctx, tasks, writebacks)
	first, err := orch.StartOrResumeMergeRequestRework(ctx, sampleMergeRequestReworkRequest())
	if err != nil {
		t.Fatalf("create first rework task: %v", err)
	}
	previousRunID := "run-first-rework"
	first.Status = model.TaskStatus("waiting_merge")
	first.CurrentRunID = &previousRunID
	if err := tasks.Update(ctx, first); err != nil {
		t.Fatalf("mark task waiting_merge: %v", err)
	}

	request := sampleMergeRequestReworkRequest()
	request.NoteID = 20002
	request.ResumeContext = "请继续修正并补充空白输入测试。"
	resumed, err := orch.StartOrResumeMergeRequestRework(ctx, request)
	if err != nil {
		t.Fatalf("resume rework task: %v", err)
	}
	if resumed.TaskID != first.TaskID || resumed.Status != model.TaskStatusPending || resumed.CurrentRunID != nil {
		t.Fatalf("expected stable pending rework task, got %#v", resumed)
	}
	if !strings.Contains(resumed.ContextJSON, `"note_id":20002`) || !strings.Contains(resumed.ContextJSON, request.ResumeContext) {
		t.Fatalf("expected latest rework request context, got %s", resumed.ContextJSON)
	}
}

func TestStartMergeRequestReworkRejectsConcurrentActiveTask(t *testing.T) {
	orch, tasks, writebacks := newDeveloperReworkFixture(t)
	ctx := context.Background()
	seedManagedMergeRequest(t, ctx, tasks, writebacks)
	created, err := orch.StartOrResumeMergeRequestRework(ctx, sampleMergeRequestReworkRequest())
	if err != nil {
		t.Fatalf("create rework task: %v", err)
	}
	created.Status = model.TaskStatusRunning
	if err := tasks.Update(ctx, created); err != nil {
		t.Fatalf("mark rework task running: %v", err)
	}

	request := sampleMergeRequestReworkRequest()
	request.NoteID = 20002
	_, err = orch.StartOrResumeMergeRequestRework(ctx, request)
	if !errors.Is(err, repository.ErrTaskNotClaimable) {
		t.Fatalf("expected active rework task to reject concurrent resume, got %v", err)
	}
}

func TestApplyMergeRequestLifecycleFinalizesActiveReworkOnMerge(t *testing.T) {
	orch, tasks, writebacks := newDeveloperReworkFixture(t)
	ctx := context.Background()
	seedManagedMergeRequest(t, ctx, tasks, writebacks)
	task, err := orch.StartOrResumeMergeRequestRework(ctx, sampleMergeRequestReworkRequest())
	if err != nil {
		t.Fatalf("create rework task: %v", err)
	}
	runID := "run-active-rework"
	task.Status = model.TaskStatusRunning
	task.CurrentRunID = &runID
	if err := tasks.Update(ctx, task); err != nil {
		t.Fatalf("mark task running: %v", err)
	}
	if err := orch.runs.Create(ctx, &model.Run{RunID: runID, TaskID: task.TaskID, WorkerRole: task.Role, Status: model.RunStatusRunning}); err != nil {
		t.Fatalf("create active run: %v", err)
	}

	finalized, err := orch.ApplyMergeRequestLifecycle(ctx, model.MergeRequestLifecycleRequest{
		DedupeKey: "gitlab:mr-rework:1001:21", ProjectID: "1001", ProjectPath: "demo-group/demo-project",
		MergeRequestIID: 21, Action: "merge", State: "merged", HeadSHA: "def456",
	})
	if err != nil {
		t.Fatalf("apply merged lifecycle: %v", err)
	}
	if finalized.Status != model.TaskStatusSucceeded || finalized.LeaseUntil != nil {
		t.Fatalf("expected succeeded rework task, got %#v", finalized)
	}
	storedRun, err := orch.runs.GetByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("get finalized run: %v", err)
	}
	if storedRun.Status != model.RunStatusBlocked || storedRun.CompletedAt == nil {
		t.Fatalf("expected active run blocked by merge, got %#v", storedRun)
	}
}

func TestApplyMergeRequestLifecycleBlocksWaitingReworkOnClose(t *testing.T) {
	orch, tasks, writebacks := newDeveloperReworkFixture(t)
	ctx := context.Background()
	seedManagedMergeRequest(t, ctx, tasks, writebacks)
	task, err := orch.StartOrResumeMergeRequestRework(ctx, sampleMergeRequestReworkRequest())
	if err != nil {
		t.Fatalf("create rework task: %v", err)
	}
	task.Status = model.TaskStatusWaitingMerge
	if err := tasks.Update(ctx, task); err != nil {
		t.Fatalf("mark task waiting_merge: %v", err)
	}

	finalized, err := orch.ApplyMergeRequestLifecycle(ctx, model.MergeRequestLifecycleRequest{
		DedupeKey: "gitlab:mr-rework:1001:21", ProjectID: "1001", ProjectPath: "demo-group/demo-project",
		MergeRequestIID: 21, Action: "close", State: "closed", HeadSHA: "def456",
	})
	if err != nil {
		t.Fatalf("apply closed lifecycle: %v", err)
	}
	if finalized.Status != model.TaskStatusBlocked {
		t.Fatalf("expected closed MR to block rework task, got %#v", finalized)
	}
}

func TestStartMergeRequestReworkCreatesTaskFromUnmanagedMergeRequest(t *testing.T) {
	orch, _, _ := newDeveloperReworkFixture(t)
	ctx := context.Background()

	request := sampleMergeRequestReworkRequest()
	request.RequesterPreAuthorized = true

	created, err := orch.StartOrResumeMergeRequestRework(ctx, request)
	if err != nil {
		t.Fatalf("start rework on unmanaged merge request: %v", err)
	}
	if created.Type != model.TaskTypeReworkMergeRequest || created.Role != "DeveloperWorker" || created.Status != model.TaskStatusPending {
		t.Fatalf("unexpected rework task identity: %#v", created)
	}
	if created.DedupeKey != "gitlab:mr-rework:1001:21" || created.DeliveryBranch != "developer/issue-19-20260806" {
		t.Fatalf("unexpected rework task delivery: %#v", created)
	}

	var taskContext map[string]any
	if err := json.Unmarshal([]byte(created.ContextJSON), &taskContext); err != nil {
		t.Fatalf("decode rework context: %v", err)
	}
	if taskContext["origin_issue"] != nil {
		t.Fatalf("unmanaged rework must not carry origin_issue: %#v", taskContext)
	}
	for _, key := range []string{"merge_request", "rework_request", "repos"} {
		if taskContext[key] == nil {
			t.Fatalf("expected %s in rework context: %#v", key, taskContext)
		}
	}
}

func TestStartMergeRequestReworkRejectsUnauthorizedRequesterOnUnmanagedMergeRequest(t *testing.T) {
	orch, _, _ := newDeveloperReworkFixture(t)
	ctx := context.Background()

	request := sampleMergeRequestReworkRequest()
	request.RequesterID = 99
	request.RequesterPreAuthorized = false
	request.AssigneeIDs = []int{777}
	request.ReviewerIDs = []int{778}

	_, err := orch.StartOrResumeMergeRequestRework(ctx, request)
	if !errors.Is(err, model.ErrMergeRequestReworkRequesterUnauthorized) {
		t.Fatalf("expected unauthorized requester error on unmanaged MR, got %v", err)
	}
}

// objective 按事件来源平台命名业务对象:GitLab 叫 Merge Request(序号 !),
// Gitea/GitHub 叫 Pull Request(序号 #),避免 Gitea 任务出现 "GitLab MR" 误导文案。
func TestStartMergeRequestReworkNamesObjectiveAfterSourcePlatform(t *testing.T) {
	orch, tasks, writebacks := newDeveloperReworkFixture(t)
	ctx := context.Background()
	seedManagedMergeRequest(t, ctx, tasks, writebacks)

	request := sampleMergeRequestReworkRequest()
	created, err := orch.StartOrResumeMergeRequestRework(ctx, request)
	if err != nil {
		t.Fatalf("start gitlab rework: %v", err)
	}
	if !strings.Contains(created.Objective, "GitLab MR !21") {
		t.Fatalf("gitlab objective must keep MR naming: %q", created.Objective)
	}

	request.Source = "gitea"
	request.DedupeKey = "gitea:mr-rework:1001:21"
	if _, err := orch.StartOrResumeMergeRequestRework(ctx, request); err != nil {
		t.Fatalf("start gitea rework: %v", err)
	}
	giteaTask, err := tasks.GetByDedupeKey(ctx, "gitea:mr-rework:1001:21")
	if err != nil {
		t.Fatalf("get gitea rework task: %v", err)
	}
	if !strings.Contains(giteaTask.Objective, "Pull Request #21") {
		t.Fatalf("gitea objective must use pull request naming: %q", giteaTask.Objective)
	}
}

func TestStartMergeRequestReworkRejectsUnmanagedMergeRequestWithoutWritebackRepository(t *testing.T) {
	orch, _, _ := newDeveloperReworkFixture(t)
	orch.writebacks = nil
	_, err := orch.StartOrResumeMergeRequestRework(context.Background(), sampleMergeRequestReworkRequest())
	if !errors.Is(err, ErrMergeRequestNotManaged) {
		t.Fatalf("expected unmanaged MR error without writeback repository, got %v", err)
	}
}

func sampleMergeRequestReworkRequest() model.MergeRequestReworkRequest {
	return model.MergeRequestReworkRequest{
		DedupeKey:               "gitlab:mr-rework:1001:21",
		Source:                  "gitlab",
		ProjectID:               "1001",
		ProjectPath:             "demo-group/demo-project",
		RepositoryURL:           "https://gitlab.example/demo-group/demo-project.git",
		MergeRequestIID:         21,
		MergeRequestURL:         "https://gitlab.example/demo-group/demo-project/-/merge_requests/21",
		MergeRequestTitle:       "Fix null handling",
		MergeRequestDescription: "Implement null-safe normalization.",
		SourceBranch:            "developer/issue-19-20260806",
		TargetBranch:            "main",
		ExpectedHeadSHA:         "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		NoteID:                  20001,
		NoteURL:                 "https://gitlab.example/demo-group/demo-project/-/merge_requests/21#note_20001",
		DiscussionID:            "discussion-root-001",
		ResumeContext:           "请修正空值处理并补充对应测试。",
		RequesterID:             42,
		RequesterName:           "Demo Developer",
		RequesterUsername:       "developer",
		RequesterEmail:          "developer@example.com",
		AssigneeIDs:             []int{42},
		ReviewerIDs:             []int{43},
	}
}

func newDeveloperReworkFixture(t *testing.T) (*Orchestrator, *repository.TaskRepository, *repository.WritebackRepository) {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Writeback{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	tasks := repository.NewTaskRepository(db)
	writebacks := repository.NewWritebackRepository(db)
	orch := New(tasks, repository.NewRunRepository(db), WithWritebackRepository(writebacks))
	return orch, tasks, writebacks
}

func seedManagedMergeRequest(t *testing.T, ctx context.Context, tasks *repository.TaskRepository, writebacks *repository.WritebackRepository) *model.Task {
	t.Helper()
	origin := &model.Task{
		TaskID:             "task-origin-issue-19",
		DedupeKey:          "gitlab:issue-develop:1001:19",
		Type:               "develop_from_issue",
		Role:               "DeveloperWorker",
		SourceRepo:         "demo-group/demo-project",
		TargetRepo:         "demo-group/demo-project",
		Objective:          "Implement GitLab Issue #19 in demo-group/demo-project: Handle null status safely",
		AcceptanceCriteria: "Complete code changes and relevant verification.",
		Status:             model.TaskStatusSucceeded,
		ContextJSON: `{
			"event":{"source":"gitlab"},
			"project":{"id":"1001","path":"demo-group/demo-project"},
			"issue":{"iid":19,"url":"https://gitlab.example/demo-group/demo-project/-/issues/19","title":"Handle null status safely","description":"Normalize null and blank status values.","assignee_ids":[42]},
			"repos":[{"name":"demo-project","url":"https://gitlab.example/demo-group/demo-project.git","branch":"main","access":"writable"}]
		}`,
	}
	if err := tasks.Create(ctx, origin); err != nil {
		t.Fatalf("create origin task: %v", err)
	}
	if err := writebacks.Create(ctx, &model.Writeback{
		TaskID: origin.TaskID, RunID: "run-origin", DedupeKey: "origin-mr", Kind: "create_merge_request",
		ProjectID: "demo-group/demo-project", ExternalID: "21", ExternalURL: "https://gitlab.example/demo-group/demo-project/-/merge_requests/21", Status: model.WritebackStatusSucceeded,
	}); err != nil {
		t.Fatalf("create managed MR writeback: %v", err)
	}
	return origin
}

func seedReviewerComment(t *testing.T, ctx context.Context, tasks *repository.TaskRepository, writebacks *repository.WritebackRepository, taskID, runID, body string) {
	t.Helper()
	reviewTask := &model.Task{
		TaskID: taskID, DedupeKey: taskID, Type: "review_mr", Role: "ReviewerWorker",
		SourceRepo: "demo-group/demo-project", TargetRepo: "demo-group/demo-project", Status: model.TaskStatusSucceeded,
	}
	if err := tasks.Create(ctx, reviewTask); err != nil {
		t.Fatalf("create review task: %v", err)
	}
	marker := "<!-- sdlc-coordinator-operation:codehost:" + taskID + ":" + runID + ":0:create_merge_request_note -->"
	requestJSON, err := json.Marshal(map[string]string{"body": body + "\n\n" + marker})
	if err != nil {
		t.Fatalf("encode review comment: %v", err)
	}
	if err := writebacks.Create(ctx, &model.Writeback{
		TaskID: taskID, RunID: runID, DedupeKey: "review-note:" + taskID,
		Kind: "create_merge_request_note", ProjectID: "demo-group/demo-project", TargetType: "merge_request", TargetIID: 21,
		Status: model.WritebackStatusSucceeded, OperationMarker: marker, RequestJSON: string(requestJSON), ExternalID: "20000",
	}); err != nil {
		t.Fatalf("create review comment writeback: %v", err)
	}
}

func TestStartMergeRequestReworkInheritsRepositoryID(t *testing.T) {
	orch, tasks, writebacks := newDeveloperReworkFixture(t)
	ctx := context.Background()
	seedManagedMergeRequest(t, ctx, tasks, writebacks)

	request := sampleMergeRequestReworkRequest()
	repositoryID := "repo-demo-project"
	request.RepositoryID = &repositoryID
	created, err := orch.StartOrResumeMergeRequestRework(ctx, request)
	if err != nil {
		t.Fatalf("start rework: %v", err)
	}
	if created.RepositoryID == nil || *created.RepositoryID != repositoryID {
		t.Fatalf("expected rework task to inherit repository_id %q, got %v", repositoryID, created.RepositoryID)
	}
}

func TestStartMergeRequestReworkInheritsRepositoryIDFromOrigin(t *testing.T) {
	orch, tasks, writebacks := newDeveloperReworkFixture(t)
	ctx := context.Background()
	origin := seedManagedMergeRequest(t, ctx, tasks, writebacks)
	repositoryID := "repo-demo-project"
	origin.RepositoryID = &repositoryID
	if err := tasks.Update(ctx, origin); err != nil {
		t.Fatalf("update origin repository id: %v", err)
	}

	created, err := orch.StartOrResumeMergeRequestRework(ctx, sampleMergeRequestReworkRequest())
	if err != nil {
		t.Fatalf("start rework: %v", err)
	}
	if created.RepositoryID == nil || *created.RepositoryID != repositoryID {
		t.Fatalf("expected rework task to inherit repository_id %q from origin, got %v", repositoryID, created.RepositoryID)
	}
}

// Gitea 评论载荷不携带 MR 分支;coordinator 自建 MR 的写回请求是分支信息
// 的权威来源,orchestrator 需先补齐再校验。评论时刻的 head 无法回溯,
// expected_head_sha 是提示性字段,允许为空。
func TestStartMergeRequestReworkEnrichesBranchesFromManagedWriteback(t *testing.T) {
	orch, tasks, writebacks := newDeveloperReworkFixture(t)
	ctx := context.Background()
	seedManagedMergeRequest(t, ctx, tasks, writebacks)

	writeback, err := writebacks.GetSucceededMergeRequest(ctx, "demo-group/demo-project", 21)
	if err != nil {
		t.Fatalf("get managed merge request writeback: %v", err)
	}
	writeback.RequestJSON = `{"kind":"create_merge_request","project_id":"demo-group/demo-project","title":"Fix null handling","source_branch":"developer/issue-19-20260806","target_branch":"main"}`
	if err := writebacks.Update(ctx, writeback); err != nil {
		t.Fatalf("update writeback request: %v", err)
	}

	request := sampleMergeRequestReworkRequest()
	request.SourceBranch = ""
	request.TargetBranch = ""
	request.ExpectedHeadSHA = ""
	created, err := orch.StartOrResumeMergeRequestRework(ctx, request)
	if err != nil {
		t.Fatalf("start rework with gitea-style request: %v", err)
	}
	if created.DeliveryBranch != "developer/issue-19-20260806" {
		t.Fatalf("expected delivery branch from writeback, got %q", created.DeliveryBranch)
	}
	var taskContext map[string]any
	if err := json.Unmarshal([]byte(created.ContextJSON), &taskContext); err != nil {
		t.Fatalf("decode rework context: %v", err)
	}
	mergeRequest := taskContext["merge_request"].(map[string]any)
	if mergeRequest["source_branch"] != "developer/issue-19-20260806" || mergeRequest["target_branch"] != "main" {
		t.Fatalf("expected enriched branches in context, got %#v", mergeRequest)
	}
}

func TestStartMergeRequestReworkStillRequiresBranchesWhenWritebackLacksRequest(t *testing.T) {
	orch, tasks, writebacks := newDeveloperReworkFixture(t)
	ctx := context.Background()
	seedManagedMergeRequest(t, ctx, tasks, writebacks)

	request := sampleMergeRequestReworkRequest()
	request.SourceBranch = ""
	request.TargetBranch = ""
	request.ExpectedHeadSHA = ""
	if _, err := orch.StartOrResumeMergeRequestRework(ctx, request); err == nil {
		t.Fatal("expected missing branches to be rejected when writeback cannot enrich")
	}
}

type stubMergeRequestReader struct {
	mergeRequest *codehost.MergeRequest
	err          error
	calls        int
	seenRepo     string
	seenIID      int
	blockCtx     bool
}

func (stub *stubMergeRequestReader) GetMergeRequest(ctx context.Context, repo string, mrIID int) (*codehost.MergeRequest, error) {
	stub.calls++
	stub.seenRepo, stub.seenIID = repo, mrIID
	if stub.err != nil {
		return nil, stub.err
	}
	if stub.blockCtx {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return stub.mergeRequest, nil
}

type stubMergeRequestReaderResolver struct {
	reader codehost.MergeRequestReader
}

func (resolver *stubMergeRequestReaderResolver) ResolveMergeRequestReader(ctx context.Context, repositoryID string) (codehost.MergeRequestReader, error) {
	return resolver.reader, nil
}

// externalGitHubReworkRequest 复刻 GitHub 外部 PR 评论事件经 processor 装配后的
// 请求形态:载荷不携带 head/base(平台设计差异),且无 coordinator 写回记录。
func externalGitHubReworkRequest() model.MergeRequestReworkRequest {
	repositoryID := "repo-external-github"
	return model.MergeRequestReworkRequest{
		DedupeKey:               "github:mr-rework:72573381:17",
		Source:                  "github",
		ProjectID:               "72573381",
		ProjectPath:             "octo-org/external",
		RepositoryID:            &repositoryID,
		RepositoryURL:           "https://github.com/octo-org/external.git",
		MergeRequestIID:         17,
		MergeRequestURL:         "https://github.com/octo-org/external/pull/17",
		MergeRequestTitle:       "External contribution",
		MergeRequestDescription: "External PR without coordinator writeback.",
		NoteID:                  50001,
		NoteURL:                 "https://github.com/octo-org/external/pull/17#issuecomment-50001",
		ResumeContext:           "请按评审意见调整实现。",
		RequesterID:             71,
		RequesterName:           "External Author",
		RequesterUsername:       "author1",
		RequesterPreAuthorized:  true,
		AssigneeIDs:             []int{71},
	}
}

// TestStartMergeRequestReworkEnrichesExternalGitHubPullRequest 外部 PR(GitHub
// 评论载荷不携带 head/base、无写回记录)经平台 API 补齐分支后创建任务。
func TestStartMergeRequestReworkEnrichesExternalGitHubPullRequest(t *testing.T) {
	orch, _, _ := newDeveloperReworkFixture(t)
	stub := &stubMergeRequestReader{mergeRequest: &codehost.MergeRequest{
		IID:          17,
		WebURL:       "https://github.com/octo-org/external/pull/17",
		SourceBranch: "feature/external-pr",
		TargetBranch: "main",
		HeadSHA:      "cccccccccccccccccccccccccccccccccccccccc",
	}}
	orch.mergeRequestReaderResolver = &stubMergeRequestReaderResolver{reader: stub}

	created, err := orch.StartOrResumeMergeRequestRework(context.Background(), externalGitHubReworkRequest())
	if err != nil {
		t.Fatalf("start external github rework: %v", err)
	}
	if created.Type != model.TaskTypeReworkMergeRequest || created.Role != "DeveloperWorker" || created.Status != model.TaskStatusPending {
		t.Fatalf("unexpected rework task identity: %#v", created)
	}
	if created.DeliveryBranch != "feature/external-pr" {
		t.Fatalf("unexpected delivery branch %q", created.DeliveryBranch)
	}
	if stub.calls != 1 || stub.seenRepo != "octo-org/external" || stub.seenIID != 17 {
		t.Fatalf("unexpected enrichment call: calls=%d repo=%q iid=%d", stub.calls, stub.seenRepo, stub.seenIID)
	}

	var taskContext map[string]any
	if err := json.Unmarshal([]byte(created.ContextJSON), &taskContext); err != nil {
		t.Fatalf("decode rework context: %v", err)
	}
	mergeRequest := taskContext["merge_request"].(map[string]any)
	if mergeRequest["source_branch"] != "feature/external-pr" || mergeRequest["target_branch"] != "main" ||
		mergeRequest["expected_head_sha"] != "cccccccccccccccccccccccccccccccccccccccc" {
		t.Fatalf("unexpected enriched context: %#v", mergeRequest)
	}
}

// TestStartMergeRequestReworkFailsClosedWhenGitHubEnrichmentFails 外部 PR 的
// 分支补链 API 失败时请求照旧被拒(fail-closed),不创建任务、不静默放行。
func TestStartMergeRequestReworkFailsClosedWhenGitHubEnrichmentFails(t *testing.T) {
	orch, tasks, _ := newDeveloperReworkFixture(t)
	orch.mergeRequestReaderResolver = &stubMergeRequestReaderResolver{reader: &stubMergeRequestReader{
		err: errors.New("github 404 Not Found"),
	}}
	ctx := context.Background()

	_, err := orch.StartOrResumeMergeRequestRework(ctx, externalGitHubReworkRequest())
	if err == nil || !strings.Contains(err.Error(), "fetch github pull request octo-org/external#17") {
		t.Fatalf("expected fail-closed enrichment error, got %v", err)
	}
	if _, err := tasks.GetByDedupeKey(ctx, "github:mr-rework:72573381:17"); err == nil {
		t.Fatal("expected no rework task to be created")
	}
}

// TestStartMergeRequestReworkTimesOutGitHubEnrichment 出网超时同样 fail-closed,
// 错误链带 context deadline exceeded。
func TestStartMergeRequestReworkTimesOutGitHubEnrichment(t *testing.T) {
	orch, _, _ := newDeveloperReworkFixture(t)
	orch.mergeRequestReaderResolver = &stubMergeRequestReaderResolver{reader: &stubMergeRequestReader{blockCtx: true}}
	orch.reworkMergeRequestFetchTimeout = 50 * time.Millisecond

	_, err := orch.StartOrResumeMergeRequestRework(context.Background(), externalGitHubReworkRequest())
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func seedManagedGitHubMergeRequest(t *testing.T, ctx context.Context, tasks *repository.TaskRepository, writebacks *repository.WritebackRepository) {
	t.Helper()
	origin := &model.Task{
		TaskID:             "task-origin-issue-gh-9",
		DedupeKey:          "github:issue-develop:72573381:9",
		Type:               "develop_from_issue",
		Role:               "DeveloperWorker",
		SourceRepo:         "octo-org/external",
		TargetRepo:         "octo-org/external",
		Objective:          "Implement GitHub Issue #9 in octo-org/external: external smoke",
		AcceptanceCriteria: "Complete code changes and relevant verification.",
		Status:             model.TaskStatusSucceeded,
		ContextJSON: `{
			"event":{"source":"github"},
			"project":{"id":"72573381","path":"octo-org/external"},
			"issue":{"iid":9,"url":"https://github.com/octo-org/external/issues/9","title":"external smoke","description":"smoke","assignee_ids":[71]},
			"repos":[{"name":"external","url":"https://github.com/octo-org/external.git","branch":"main","access":"writable"}]
		}`,
	}
	if err := tasks.Create(ctx, origin); err != nil {
		t.Fatalf("create origin task: %v", err)
	}
	if err := writebacks.Create(ctx, &model.Writeback{
		TaskID: origin.TaskID, RunID: "run-origin-gh", DedupeKey: "origin-mr-gh", Kind: "create_merge_request",
		ProjectID: "octo-org/external", ExternalID: "17", ExternalURL: "https://github.com/octo-org/external/pull/17",
		Status: model.WritebackStatusSucceeded, RequestJSON: `{"source_branch":"feature/managed-pr","target_branch":"main"}`,
	}); err != nil {
		t.Fatalf("create managed github MR writeback: %v", err)
	}
}

// TestStartMergeRequestReworkSkipsPlatformEnrichmentWhenWritebackProvidesBranches
// coordinator 自建 GitHub PR:分支来自写回记录,补链 API 零调用。
func TestStartMergeRequestReworkSkipsPlatformEnrichmentWhenWritebackProvidesBranches(t *testing.T) {
	orch, tasks, writebacks := newDeveloperReworkFixture(t)
	ctx := context.Background()
	seedManagedGitHubMergeRequest(t, ctx, tasks, writebacks)
	stub := &stubMergeRequestReader{err: errors.New("platform enrichment must not be called")}
	orch.mergeRequestReaderResolver = &stubMergeRequestReaderResolver{reader: stub}

	created, err := orch.StartOrResumeMergeRequestRework(ctx, externalGitHubReworkRequest())
	if err != nil {
		t.Fatalf("start managed github rework: %v", err)
	}
	if created == nil || created.Status != model.TaskStatusPending || created.DeliveryBranch != "feature/managed-pr" {
		t.Fatalf("unexpected rework task: %#v", created)
	}
	if stub.calls != 0 {
		t.Fatalf("expected zero platform API calls for writeback-enriched rework, got %d", stub.calls)
	}
}

// TestStartMergeRequestReworkSkipsPlatformEnrichmentForNonGitHubSources
// 非 GitHub 平台零出网:分支已在载荷(GitLab/Gitea 直读);缺失时保持既有拒绝
// 语义不变(Gitea 直读路径零改动)。
func TestStartMergeRequestReworkSkipsPlatformEnrichmentForNonGitHubSources(t *testing.T) {
	orch, tasks, writebacks := newDeveloperReworkFixture(t)
	seedManagedMergeRequest(t, context.Background(), tasks, writebacks)
	stub := &stubMergeRequestReader{err: errors.New("platform enrichment must not be called")}
	orch.mergeRequestReaderResolver = &stubMergeRequestReaderResolver{reader: stub}
	ctx := context.Background()

	created, err := orch.StartOrResumeMergeRequestRework(ctx, sampleMergeRequestReworkRequest())
	if err != nil {
		t.Fatalf("start gitlab rework: %v", err)
	}
	if created == nil || created.Status != model.TaskStatusPending {
		t.Fatalf("unexpected gitlab rework task: %#v", created)
	}

	giteaRequest := sampleMergeRequestReworkRequest()
	giteaRequest.Source = "gitea"
	giteaRequest.DedupeKey = "gitea:mr-rework:1001:21"
	giteaRequest.SourceBranch = ""
	giteaRequest.TargetBranch = ""
	// 校验按 map 迭代报缺失字段,source/target 报谁先取决于迭代顺序,
	// 断言收敛到「因分支缺失被拒」这一不变语义。
	if _, err := orch.StartOrResumeMergeRequestRework(ctx, giteaRequest); err == nil || !strings.Contains(err.Error(), "branch is required") {
		t.Fatalf("expected unchanged gitea rejection, got %v", err)
	}
	if stub.calls != 0 {
		t.Fatalf("expected zero platform API calls for non-github sources, got %d", stub.calls)
	}
}
