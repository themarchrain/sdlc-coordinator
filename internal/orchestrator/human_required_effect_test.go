package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
)

// 回归背景:任务因结果校验拒绝进 human_required,用户只能靠翻
// 日志发现。这些测试锁定「任务进入人工介入位必须物化 IM 通知」。

func TestRecordInvalidRunnerResultMaterializesHumanRequiredIMEffect(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, effectRepo := newRecoveryRepositoriesWithEffects(t)
	orch := New(taskRepo, runRepo, WithEffectRepository(effectRepo))
	runID := "run-invalid-result"
	if err := taskRepo.Create(ctx, &model.Task{
		TaskID:       "task-invalid-result",
		DedupeKey:    "dedupe-invalid-result",
		Type:         "bugfix",
		Role:         "DeveloperWorker",
		Status:       model.TaskStatusRunning,
		CurrentRunID: &runID,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runRepo.Create(ctx, &model.Run{
		RunID: runID, TaskID: "task-invalid-result", WorkerRole: "DeveloperWorker",
		Status: model.RunStatusRunning,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	cause := errors.New("result task_id does not match task")
	if err := orch.recordInvalidRunnerResult(ctx, runID, cause); err != nil {
		t.Fatalf("record invalid runner result: %v", err)
	}

	task, err := taskRepo.GetByTaskID(ctx, "task-invalid-result")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.Status != model.TaskStatusHumanRequired {
		t.Fatalf("expected human_required, got %s", task.Status)
	}

	effect, err := effectRepo.GetByDedupeKey(ctx, "human-required-im:task-invalid-result:"+runID)
	if err != nil {
		t.Fatalf("get human-required notification effect: %v", err)
	}
	if effect.TaskID != "task-invalid-result" || effect.RunID != runID {
		t.Fatalf("unexpected effect identity: task=%s run=%s", effect.TaskID, effect.RunID)
	}
	if effect.Kind != "send_im_message" || effect.Target != "default" || effect.Critical {
		t.Fatalf("unexpected effect delivery fields: kind=%s target=%s critical=%v", effect.Kind, effect.Target, effect.Critical)
	}
	if effect.Status != model.EffectStatusPending {
		t.Fatalf("expected pending effect, got %s", effect.Status)
	}
	for _, expected := range []string{"task-invalid-result", runID, "DeveloperWorker", "human_required", "result task_id does not match task"} {
		if !strings.Contains(effect.RequestJSON, expected) {
			t.Fatalf("expected effect request to contain %q, got %s", expected, effect.RequestJSON)
		}
	}
}

// UpsertByDedupeKey 会整行覆盖已存在 effect(含把已执行 Status 重置回 pending),
// 重复 reconcile 不得借它把通知重置重发。
func TestRecordInvalidRunnerResultIMEffectIsIdempotent(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, effectRepo := newRecoveryRepositoriesWithEffects(t)
	orch := New(taskRepo, runRepo, WithEffectRepository(effectRepo))
	runID := "run-invalid-result-idempotent"
	if err := taskRepo.Create(ctx, &model.Task{
		TaskID:       "task-invalid-result-idempotent",
		DedupeKey:    "dedupe-invalid-result-idempotent",
		Type:         "bugfix",
		Role:         "DeveloperWorker",
		Status:       model.TaskStatusRunning,
		CurrentRunID: &runID,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runRepo.Create(ctx, &model.Run{
		RunID: runID, TaskID: "task-invalid-result-idempotent", WorkerRole: "DeveloperWorker",
		Status: model.RunStatusRunning,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	cause := errors.New("result task_id does not match task")
	for i := 0; i < 2; i++ {
		if err := orch.recordInvalidRunnerResult(ctx, runID, cause); err != nil {
			t.Fatalf("record invalid runner result (call %d): %v", i+1, err)
		}
	}

	effects, err := effectRepo.ListPending(ctx, 10)
	if err != nil {
		t.Fatalf("list pending effects: %v", err)
	}
	if len(effects) != 1 {
		t.Fatalf("expected one materialized effect, got %d", len(effects))
	}
}

func TestParkTaskIfWritebackRequiresHumanMaterializesIMEffect(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, effectRepo := newRecoveryRepositoriesWithEffects(t)
	orch := New(taskRepo, runRepo, WithEffectRepository(effectRepo))
	if err := taskRepo.Create(ctx, &model.Task{
		TaskID:    "task-writeback-park",
		DedupeKey: "dedupe-writeback-park",
		Type:      "bugfix",
		Role:      "DeveloperWorker",
		Status:    model.TaskStatusClaimed,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := orch.parkTaskIfWritebackRequiresHuman(ctx, "task-writeback-park", ErrRepositoryNotEnabled); err != nil {
		t.Fatalf("park writeback task: %v", err)
	}

	task, err := taskRepo.GetByTaskID(ctx, "task-writeback-park")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.Status != model.TaskStatusHumanRequired {
		t.Fatalf("expected human_required, got %s", task.Status)
	}
	effect, err := effectRepo.GetByDedupeKey(ctx, "human-required-im:task-writeback-park:")
	if err != nil {
		t.Fatalf("get writeback-park notification effect: %v", err)
	}
	for _, expected := range []string{"task-writeback-park", "human_required", ErrRepositoryNotEnabled.Error()} {
		if !strings.Contains(effect.RequestJSON, expected) {
			t.Fatalf("expected effect request to contain %q, got %s", expected, effect.RequestJSON)
		}
	}
}

func TestTickParksUnsupportedWorkerWithIMNotification(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, effectRepo := newTickEffectRepositories(t)
	orch := New(taskRepo, runRepo, WithEffectRepository(effectRepo))

	repositoryID := fixtureRepositoryID
	if err := taskRepo.Create(ctx, &model.Task{
		TaskID: "task-unsupported-worker-notify", DedupeKey: "dedupe-unsupported-worker-notify",
		Type: "bugfix", RepositoryID: &repositoryID,
		Role: "TesterWorker", Status: model.TaskStatusPending,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	result, err := orch.Tick(ctx, TickRequest{DispatchLimit: 10})
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(result.DispatchFailures) != 1 {
		t.Fatalf("expected one unsupported dispatch failure: %#v", result.DispatchFailures)
	}

	effect, err := effectRepo.GetByDedupeKey(ctx, "human-required-im:task-unsupported-worker-notify:")
	if err != nil {
		t.Fatalf("get unsupported-worker notification effect: %v", err)
	}
	if effect.Status != model.EffectStatusPending {
		t.Fatalf("expected pending effect, got %s", effect.Status)
	}
	for _, expected := range []string{"task-unsupported-worker-notify", "TesterWorker", "human_required"} {
		if !strings.Contains(effect.RequestJSON, expected) {
			t.Fatalf("expected effect request to contain %q, got %s", expected, effect.RequestJSON)
		}
	}
}

func TestEscalateUnknownWritebackMaterializesIMEffect(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, effectRepo := newRecoveryRepositoriesWithEffects(t)
	orch := New(taskRepo, runRepo, WithEffectRepository(effectRepo))
	runID := "run-escalate-notify"
	if err := taskRepo.Create(ctx, &model.Task{
		TaskID:       "task-escalate-notify",
		DedupeKey:    "dedupe-escalate-notify",
		Type:         "code_review",
		Role:         "ReviewerWorker",
		Status:       model.TaskStatusRunning,
		CurrentRunID: &runID,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runRepo.Create(ctx, &model.Run{
		RunID: runID, TaskID: "task-escalate-notify", WorkerRole: "ReviewerWorker",
		Status: model.RunStatusRunning,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	writeback := &model.Writeback{
		WritebackID: "wb-escalate-notify", TaskID: "task-escalate-notify", RunID: runID,
		DedupeKey: "wb-dedupe-escalate-notify", Kind: "create_merge_request_note",
		ProjectID: "group/project", Status: model.WritebackStatusUnknown,
	}
	if err := orch.escalateUnknownWriteback(ctx, writeback, "outcome remains unknown"); err != nil {
		t.Fatalf("escalate unknown writeback: %v", err)
	}

	task, err := taskRepo.GetByTaskID(ctx, "task-escalate-notify")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.Status != model.TaskStatusHumanRequired {
		t.Fatalf("expected human_required, got %s", task.Status)
	}
	if _, err := effectRepo.GetByDedupeKey(ctx, "human-required-im:task-escalate-notify:"+runID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			t.Fatal("expected escalated writeback to materialize human-required IM effect")
		}
		t.Fatalf("get escalated notification effect: %v", err)
	}
}

// 查重守卫保护的核心保证是「已执行 effect 不被重置回 pending 重发」,
// 仅数 pending 数量的用例删掉守卫照样通过,本用例直接锁定该保证。
func TestRecordInvalidRunnerResultDoesNotResetExecutedEffect(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, effectRepo := newRecoveryRepositoriesWithEffects(t)
	orch := New(taskRepo, runRepo, WithEffectRepository(effectRepo))
	runID := "run-invalid-result-executed"
	if err := taskRepo.Create(ctx, &model.Task{
		TaskID:       "task-invalid-result-executed",
		DedupeKey:    "dedupe-invalid-result-executed",
		Type:         "bugfix",
		Role:         "DeveloperWorker",
		Status:       model.TaskStatusRunning,
		CurrentRunID: &runID,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runRepo.Create(ctx, &model.Run{
		RunID: runID, TaskID: "task-invalid-result-executed", WorkerRole: "DeveloperWorker",
		Status: model.RunStatusRunning,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	cause := errors.New("result task_id does not match task")
	if err := orch.recordInvalidRunnerResult(ctx, runID, cause); err != nil {
		t.Fatalf("record invalid runner result: %v", err)
	}
	effect, err := effectRepo.GetByDedupeKey(ctx, "human-required-im:task-invalid-result-executed:"+runID)
	if err != nil {
		t.Fatalf("get effect: %v", err)
	}
	effect.Status = model.EffectStatusSucceeded
	if err := effectRepo.Update(ctx, effect); err != nil {
		t.Fatalf("mark effect executed: %v", err)
	}

	if err := orch.recordInvalidRunnerResult(ctx, runID, cause); err != nil {
		t.Fatalf("repeated record invalid runner result: %v", err)
	}
	replayed, err := effectRepo.GetByDedupeKey(ctx, effect.DedupeKey)
	if err != nil {
		t.Fatalf("get replayed effect: %v", err)
	}
	if replayed.Status != model.EffectStatusSucceeded {
		t.Fatalf("executed effect must not be reset to pending, got %s", replayed.Status)
	}
}

// 存在性读失败(非 NotFound)时宁可漏发一条通知,也不能落库重置已执行 effect
// 造成重复发送。
func TestRecordInvalidRunnerResultSkipsNotificationWhenExistenceCheckFails(t *testing.T) {
	ctx := context.Background()
	taskRepo, runRepo, effectRepo := newRecoveryRepositoriesWithEffects(t)
	orch := New(taskRepo, runRepo, WithEffectRepository(effectRepo))
	runID := "run-invalid-result-flaky-get"
	if err := taskRepo.Create(ctx, &model.Task{
		TaskID:       "task-invalid-result-flaky-get",
		DedupeKey:    "dedupe-invalid-result-flaky-get",
		Type:         "bugfix",
		Role:         "DeveloperWorker",
		Status:       model.TaskStatusRunning,
		CurrentRunID: &runID,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := runRepo.Create(ctx, &model.Run{
		RunID: runID, TaskID: "task-invalid-result-flaky-get", WorkerRole: "DeveloperWorker",
		Status: model.RunStatusRunning,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	cause := errors.New("result task_id does not match task")
	if err := orch.recordInvalidRunnerResult(ctx, runID, cause); err != nil {
		t.Fatalf("record invalid runner result: %v", err)
	}
	effect, err := effectRepo.GetByDedupeKey(ctx, "human-required-im:task-invalid-result-flaky-get:"+runID)
	if err != nil {
		t.Fatalf("get effect: %v", err)
	}
	effect.Status = model.EffectStatusSucceeded
	if err := effectRepo.Update(ctx, effect); err != nil {
		t.Fatalf("mark effect executed: %v", err)
	}

	flaky := New(taskRepo, runRepo, WithEffectRepository(failingGetEffectRepository{
		EffectRepository: effectRepo,
		err:              errors.New("effect storage read failed"),
	}))
	if err := flaky.recordInvalidRunnerResult(ctx, runID, cause); err != nil {
		t.Fatalf("record invalid runner result with flaky read: %v", err)
	}
	replayed, err := effectRepo.GetByDedupeKey(ctx, effect.DedupeKey)
	if err != nil {
		t.Fatalf("get replayed effect: %v", err)
	}
	if replayed.Status != model.EffectStatusSucceeded {
		t.Fatalf("flaky existence read must skip materialization instead of resetting, got %s", replayed.Status)
	}
}

type failingGetEffectRepository struct {
	EffectRepository
	err error
}

func (r failingGetEffectRepository) GetByDedupeKey(ctx context.Context, dedupeKey string) (*model.Effect, error) {
	return nil, r.err
}
