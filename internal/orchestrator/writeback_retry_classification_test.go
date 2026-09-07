package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/adapters/gitlab"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

// scriptedWritebackBackend 在 fakeGitLabWritebackBackend 之上按调用次序回放
// CreateMergeRequest 的错误,用于证明 submission 重放是否真正重执行远端调用;
// 错误表末位重复( [429] = 恒 429;[429, nil] = 首次 429 后成功)。
type scriptedWritebackBackend struct {
	*fakeGitLabWritebackBackend
	mutationErrs []error
}

func (s *scriptedWritebackBackend) mutationErr() error {
	if len(s.mutationErrs) == 0 {
		return nil
	}
	idx := s.createMergeRequestCalls - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(s.mutationErrs) {
		idx = len(s.mutationErrs) - 1
	}
	return s.mutationErrs[idx]
}

func (s *scriptedWritebackBackend) CreateMergeRequest(ctx context.Context, projectID string, req codehost.CreateMergeRequestRequest) (*codehost.MergeRequest, error) {
	s.createMergeRequestCalls++
	if err := s.mutationErr(); err != nil {
		return nil, err
	}
	return &codehost.MergeRequest{IID: 7, WebURL: "https://gitlab.example/" + projectID + "/-/merge_requests/7"}, nil
}

func newWritebackRetryClassificationFixture(t *testing.T) (context.Context, *repository.TaskRepository, *repository.RunRepository, *repository.ResultSubmissionRepository, *repository.WritebackRepository, *scriptedWritebackBackend, *Orchestrator) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.Artifact{}, &model.Writeback{}, &model.ResultSubmission{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	tasks := repository.NewTaskRepository(db)
	runs := repository.NewRunRepository(db)
	submissions := repository.NewResultSubmissionRepository(db)
	writebacks := repository.NewWritebackRepository(db)
	backend := &scriptedWritebackBackend{fakeGitLabWritebackBackend: &fakeGitLabWritebackBackend{}}
	orch := New(
		tasks,
		runs,
		WithArtifactRepository(repository.NewArtifactRepository(db)),
		WithWritebackRepository(writebacks),
		WithResultSubmissionRepository(submissions),
		WithCodeHostBackendResolver(backendResolverFor(fixtureRepositoryID, backend)),
	)
	createGitLabWritebackFixture(t, ctx, tasks, runs, "task-wb-classify", "run-wb-classify")
	return ctx, tasks, runs, submissions, writebacks, backend, orch
}

func writebackRetryClassificationRequest() SubmitResultRequest {
	return SubmitResultRequest{
		Source: "mcp_tester",
		Canonical: results.CanonicalResult{
			TaskID: "task-wb-classify", RunID: "run-wb-classify", Status: results.StatusSucceeded, Summary: "delivered",
			Writebacks: []results.CodeHostWritebackIntent{
				{
					DedupeKey:    "codehost:mr:framework:classify",
					Kind:         CodeHostWritebackCreateMergeRequest,
					ProjectID:    "group/framework",
					Title:        "fix framework validator",
					Description:  "created by coordinator",
					SourceBranch: model.DeliveryBranchForTask("task-wb-classify"),
					TargetBranch: "main",
					Labels:       []string{"agent:generated"},
				},
			},
		},
	}
}

func gitlabStatusErr(status int) error {
	return &gitlab.StatusError{
		Method:     http.MethodPost,
		Path:       "/api/v4/projects/group/framework/merge_requests",
		Status:     fmt.Sprintf("%d status", status),
		StatusCode: status,
		Body:       "rejected",
	}
}

// 场景 1(缺口 A 主场景):429 后 submission 重放必须真正重执行远端调用,
// 第二次调用成功则任务正常完成,不整任务重跑。
func TestTransient429WritebackReexecutesOnSubmissionReplay(t *testing.T) {
	ctx, tasks, _, submissions, writebacks, backend, orch := newWritebackRetryClassificationFixture(t)
	backend.mutationErrs = []error{
		codehost.NewTransientRejection(gitlabStatusErr(http.StatusTooManyRequests)),
		nil,
	}

	if _, err := orch.SubmitResult(ctx, writebackRetryClassificationRequest()); err == nil {
		t.Fatal("expected first delivery to fail with 429")
	}
	stored, err := writebacks.GetByDedupeKey(ctx, "codehost:mr:framework:classify")
	if err != nil {
		t.Fatalf("get writeback after 429: %v", err)
	}
	if stored.Status != model.WritebackStatusPending {
		t.Fatalf("429 rejection must keep writeback re-executable (pending), got %s", stored.Status)
	}

	finalized, err := orch.FinalizeRecoverableSubmissions(ctx, 10)
	if err != nil {
		t.Fatalf("finalize recoverable submissions: %v", err)
	}
	if len(finalized) != 1 || !finalized[0].Finalized || finalized[0].Status != model.ResultSubmissionStatusSucceeded {
		t.Fatalf("expected replayed finalization to succeed, got %#v", finalized)
	}
	if backend.createMergeRequestCalls != 2 {
		t.Fatalf("submission replay must re-execute remote call (want 2 calls), got %d", backend.createMergeRequestCalls)
	}
	submission, err := submissions.GetByRunID(ctx, "run-wb-classify")
	if err != nil {
		t.Fatalf("get submission: %v", err)
	}
	if submission.Status != model.ResultSubmissionStatusSucceeded {
		t.Fatalf("expected succeeded submission, got %s", submission.Status)
	}
	task, err := tasks.GetByTaskID(ctx, "task-wb-classify")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.Status != model.TaskStatusSucceeded {
		t.Fatalf("expected succeeded task without full re-run, got %s", task.Status)
	}
}

// 场景 2(有界性):429 持续失败 ×3,submission 终态失败,不无限重试。
func TestTransient429WritebackStaysBounded(t *testing.T) {
	ctx, tasks, runs, submissions, _, backend, orch := newWritebackRetryClassificationFixture(t)
	backend.mutationErrs = []error{
		codehost.NewTransientRejection(gitlabStatusErr(http.StatusTooManyRequests)),
	}

	if _, err := orch.SubmitResult(ctx, writebackRetryClassificationRequest()); err == nil {
		t.Fatal("expected first delivery to fail with 429")
	}
	for attempt := 1; attempt <= 3; attempt++ {
		if _, err := orch.FinalizeRecoverableSubmissions(ctx, 10); err != nil {
			t.Fatalf("finalization attempt %d: %v", attempt, err)
		}
	}
	submission, err := submissions.GetByRunID(ctx, "run-wb-classify")
	if err != nil {
		t.Fatalf("get submission: %v", err)
	}
	if submission.Status != model.ResultSubmissionStatusFailed || submission.AttemptCount != 3 {
		t.Fatalf("expected exhausted failed submission, got status=%s attempts=%d", submission.Status, submission.AttemptCount)
	}
	task, err := tasks.GetByTaskID(ctx, "task-wb-classify")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.Status != model.TaskStatusHumanRequired {
		t.Fatalf("expected human_required task after exhaustion, got %s", task.Status)
	}
	run, err := runs.GetByRunID(ctx, "run-wb-classify")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != model.RunStatusBlocked {
		t.Fatalf("expected blocked run after exhaustion, got %s", run.Status)
	}
	again, err := orch.FinalizeRecoverableSubmissions(ctx, 10)
	if err != nil {
		t.Fatalf("list after exhaustion: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("exhausted submission must leave automatic queue, got %#v", again)
	}
}

// 场景 3(持久错误短路不变):400 后 submission 重放不重执行远端调用。
func TestPersistent400WritebackShortCircuitUnchanged(t *testing.T) {
	ctx, _, _, _, _, backend, orch := newWritebackRetryClassificationFixture(t)
	backend.mutationErrs = []error{gitlabStatusErr(http.StatusBadRequest)}

	if _, err := orch.SubmitResult(ctx, writebackRetryClassificationRequest()); err == nil {
		t.Fatal("expected first delivery to fail with 400")
	}
	if backend.createMergeRequestCalls != 1 {
		t.Fatalf("expected single remote call, got %d", backend.createMergeRequestCalls)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if _, err := orch.FinalizeRecoverableSubmissions(ctx, 10); err != nil {
			t.Fatalf("finalization attempt %d: %v", attempt, err)
		}
	}
	if backend.createMergeRequestCalls != 1 {
		t.Fatalf("persistent failure must short-circuit replays (want 1 call), got %d", backend.createMergeRequestCalls)
	}
}

// 场景 5(歧义错误不盲重执行):Unknown 期间 submission 重放不得触发远端调用,
// 只能等对账裁决。
func TestAmbiguousWritebackReplayDoesNotReexecuteWithoutReconciliation(t *testing.T) {
	ctx, _, _, _, _, backend, orch := newWritebackRetryClassificationFixture(t)
	backend.mutationErrs = []error{codehost.NewAmbiguousError(errors.New("connection reset during request"))}

	if _, err := orch.SubmitResult(ctx, writebackRetryClassificationRequest()); err == nil {
		t.Fatal("expected first delivery to fail with ambiguous error")
	}
	if backend.createMergeRequestCalls != 1 {
		t.Fatalf("expected single remote call, got %d", backend.createMergeRequestCalls)
	}
	finalized, err := orch.FinalizeRecoverableSubmissions(ctx, 10)
	if err != nil {
		t.Fatalf("finalize recoverable submissions: %v", err)
	}
	if len(finalized) != 1 || finalized[0].Error == "" {
		t.Fatalf("expected failed finalization awaiting reconciliation, got %#v", finalized)
	}
	if backend.createMergeRequestCalls != 1 {
		t.Fatalf("unknown writeback must not re-execute remote call (want 1), got %d", backend.createMergeRequestCalls)
	}
}

// 场景 9(缺口 B 实测):写回 Unknown 期间,submission 重放拿到的是
// "requires reconciliation" 错误——这是纯等待,不是失败尝试,不得消耗
// 重试预算。实测(修复前):3 次重放烧穿 attempt=3;修复后:等待不计次,
// 对账裁决(Found)后的下一次 Finalize 自动收敛,任务不进人工。
// 注:对账探活确认未应用(Absent)后的重执行失败是真实尝试,照常计次,
// 由既有有界性测试锁定。
func TestReconciliationWaitDoesNotConsumeSubmissionAttempts(t *testing.T) {
	ctx, tasks, _, submissions, _, backend, orch := newWritebackRetryClassificationFixture(t)
	backend.mutationErrs = []error{codehost.NewAmbiguousError(errors.New("connection reset during request"))}

	if _, err := orch.SubmitResult(ctx, writebackRetryClassificationRequest()); err == nil {
		t.Fatal("expected first delivery to fail with ambiguous error")
	}
	for pass := 1; pass <= 3; pass++ {
		finalized, err := orch.FinalizeRecoverableSubmissions(ctx, 10)
		if err != nil {
			t.Fatalf("finalization pass %d: %v", pass, err)
		}
		if len(finalized) != 1 || finalized[0].Error == "" {
			t.Fatalf("expected pass %d to await reconciliation, got %#v", pass, finalized)
		}
	}
	submission, err := submissions.GetByRunID(ctx, "run-wb-classify")
	if err != nil {
		t.Fatalf("get submission after outage: %v", err)
	}
	if submission.AttemptCount >= 3 {
		t.Fatalf("reconciliation wait must not consume submission attempts, got attempts=%d status=%s error=%s",
			submission.AttemptCount, submission.Status, submission.ErrorMessage)
	}
	if backend.createMergeRequestCalls != 1 {
		t.Fatalf("waiting must not re-execute remote call (want 1), got %d", backend.createMergeRequestCalls)
	}

	// 对账裁决 Found:写回物化成功,submission 自动 Finalize。
	backend.fakeGitLabWritebackBackend.reconcileResult = codehost.ReconcileResult{
		State:       codehost.ReconcileStateFound,
		ExternalID:  "7",
		ExternalURL: "https://gitlab.example/group/framework/-/merge_requests/7",
	}
	if _, err := orch.Tick(ctx, TickRequest{PollLimit: 10, ReconciliationLimit: 10, FinalizationLimit: 10, DispatchLimit: 10, RecoveryLimit: 10}); err != nil {
		t.Fatalf("recovery tick: %v", err)
	}
	submission, err = submissions.GetByRunID(ctx, "run-wb-classify")
	if err != nil {
		t.Fatalf("get submission after recovery: %v", err)
	}
	if submission.Status != model.ResultSubmissionStatusSucceeded {
		t.Fatalf("expected automatic finalization after reconciliation, got %s (error=%s)", submission.Status, submission.ErrorMessage)
	}
	task, err := tasks.GetByTaskID(ctx, "task-wb-classify")
	if err != nil {
		t.Fatalf("get task after recovery: %v", err)
	}
	if task.Status != model.TaskStatusSucceeded {
		t.Fatalf("expected succeeded task after reconciliation, got %s", task.Status)
	}
}
