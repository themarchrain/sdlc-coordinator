package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"

	"gorm.io/gorm"
)

func TestExecutePendingEffectsMarksSucceeded(t *testing.T) {
	ctx := context.Background()
	effectRepo := newEffectExecutionRepository(t)
	if err := effectRepo.Create(ctx, &model.Effect{
		TaskID:      "task-1",
		RunID:       "run-1",
		DedupeKey:   "effect-1",
		Kind:        "send_im_message",
		Target:      "team-review-group",
		Status:      model.EffectStatusPending,
		RequestJSON: `{"body":"done"}`,
	}); err != nil {
		t.Fatalf("create effect: %v", err)
	}

	executor := &fakeEffectExecutor{responses: map[string]string{
		"effect-1": `{"message_id":"msg-1"}`,
	}}
	orch := New(nil, nil, WithEffectRepository(effectRepo), WithEffectExecutor(executor))

	results, err := orch.ExecutePendingEffects(ctx, 10)
	if err != nil {
		t.Fatalf("execute pending effects: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].Status != model.EffectStatusSucceeded {
		t.Fatalf("expected succeeded result, got %s", results[0].Status)
	}
	if len(executor.calls) != 1 || executor.calls[0] != "effect-1" {
		t.Fatalf("unexpected executor calls: %#v", executor.calls)
	}

	effect, err := effectRepo.GetByDedupeKey(ctx, "effect-1")
	if err != nil {
		t.Fatalf("get effect: %v", err)
	}
	if effect.Status != model.EffectStatusSucceeded {
		t.Fatalf("expected succeeded effect, got %s", effect.Status)
	}
	if effect.AttemptCount != 1 {
		t.Fatalf("expected attempt count 1, got %d", effect.AttemptCount)
	}
	if effect.ResponseJSON != `{"message_id":"msg-1"}` {
		t.Fatalf("unexpected response json: %s", effect.ResponseJSON)
	}
	if effect.ErrorMessage != "" {
		t.Fatalf("expected empty error message, got %s", effect.ErrorMessage)
	}
}

func TestExecutePendingEffectsMarksFailedAndContinues(t *testing.T) {
	ctx := context.Background()
	effectRepo := newEffectExecutionRepository(t)
	for _, effect := range []model.Effect{
		{
			TaskID:      "task-1",
			RunID:       "run-1",
			DedupeKey:   "effect-failed",
			Kind:        "send_im_message",
			Target:      "team-review-group",
			Status:      model.EffectStatusPending,
			RequestJSON: `{"body":"failed"}`,
		},
		{
			TaskID:      "task-1",
			RunID:       "run-1",
			DedupeKey:   "effect-succeeded",
			Kind:        "send_im_message",
			Target:      "team-review-group",
			Status:      model.EffectStatusPending,
			RequestJSON: `{"body":"succeeded"}`,
		},
	} {
		if err := effectRepo.Create(ctx, &effect); err != nil {
			t.Fatalf("create effect %s: %v", effect.DedupeKey, err)
		}
	}

	executor := &fakeEffectExecutor{
		responses: map[string]string{
			"effect-succeeded": `{"message_id":"msg-2"}`,
		},
		errors: map[string]error{
			"effect-failed": errors.New("webhook unavailable"),
		},
	}
	orch := New(nil, nil, WithEffectRepository(effectRepo), WithEffectExecutor(executor))

	results, err := orch.ExecutePendingEffects(ctx, 10)
	if err != nil {
		t.Fatalf("execute pending effects: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected two results, got %d", len(results))
	}
	if results[0].Status != model.EffectStatusFailed {
		t.Fatalf("expected first result failed, got %s", results[0].Status)
	}
	if !strings.Contains(results[0].ErrorMessage, "webhook unavailable") {
		t.Fatalf("unexpected first error: %s", results[0].ErrorMessage)
	}
	if results[1].Status != model.EffectStatusSucceeded {
		t.Fatalf("expected second result succeeded, got %s", results[1].Status)
	}

	failed, err := effectRepo.GetByDedupeKey(ctx, "effect-failed")
	if err != nil {
		t.Fatalf("get failed effect: %v", err)
	}
	if failed.Status != model.EffectStatusFailed {
		t.Fatalf("expected failed effect, got %s", failed.Status)
	}
	if failed.AttemptCount != 1 {
		t.Fatalf("expected failed attempt count 1, got %d", failed.AttemptCount)
	}

	succeeded, err := effectRepo.GetByDedupeKey(ctx, "effect-succeeded")
	if err != nil {
		t.Fatalf("get succeeded effect: %v", err)
	}
	if succeeded.Status != model.EffectStatusSucceeded {
		t.Fatalf("expected succeeded effect, got %s", succeeded.Status)
	}
}

// IM 未配置时没有 effect executor：队列空则正常空转；有暂存效果则跳过执行、
// 保持 pending（等渠道配置后自然重放），不得让 tick 循环报错。
func TestExecutePendingEffectsSkipsWhenExecutorMissing(t *testing.T) {
	ctx := context.Background()
	effectRepo := newEffectExecutionRepository(t)
	orch := New(nil, nil, WithEffectRepository(effectRepo))

	results, err := orch.ExecutePendingEffects(ctx, 10)
	if err != nil {
		t.Fatalf("empty queue without executor should not fail: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results, got %#v", results)
	}

	if err := effectRepo.Create(ctx, &model.Effect{
		TaskID:      "task-1",
		RunID:       "run-1",
		DedupeKey:   "effect-stashed",
		Kind:        "send_im_message",
		Target:      "default",
		Status:      model.EffectStatusPending,
		RequestJSON: `{"body":"done"}`,
	}); err != nil {
		t.Fatalf("create stashed effect: %v", err)
	}

	results, err = orch.ExecutePendingEffects(ctx, 10)
	if err != nil {
		t.Fatalf("missing executor should skip instead of failing: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results without executor, got %#v", results)
	}
	stashed, err := effectRepo.GetByDedupeKey(ctx, "effect-stashed")
	if err != nil {
		t.Fatalf("get stashed effect: %v", err)
	}
	if stashed.Status != model.EffectStatusPending || stashed.AttemptCount != 0 {
		t.Fatalf("stashed effect should stay pending untouched: %#v", stashed)
	}
}

type fakeEffectExecutor struct {
	responses map[string]string
	errors    map[string]error
	calls     []string
}

func (f *fakeEffectExecutor) ExecuteEffect(ctx context.Context, effect *model.Effect) (string, error) {
	f.calls = append(f.calls, effect.DedupeKey)
	if err := f.errors[effect.DedupeKey]; err != nil {
		return "", err
	}
	return f.responses[effect.DedupeKey], nil
}

func newEffectExecutionRepository(t *testing.T) *repository.EffectRepository {
	t.Helper()

	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Effect{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return repository.NewEffectRepository(db)
}

// 停留 running 超过阈值的效果由 RecoverStaleEffects 置 failed 供人工重试;
// 未超时的 running 效果不动。
func TestRecoverStaleEffectsExpiresOnlyStaleRunning(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Effect{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	effectRepo := repository.NewEffectRepository(db)
	stale := model.Effect{TaskID: "task-1", RunID: "run-1", DedupeKey: "effect-stale", Kind: "send_im_message", Target: "default", Status: model.EffectStatusRunning, RequestJSON: `{"body":"stale"}`}
	fresh := model.Effect{TaskID: "task-1", RunID: "run-1", DedupeKey: "effect-fresh", Kind: "send_im_message", Target: "default", Status: model.EffectStatusRunning, RequestJSON: `{"body":"fresh"}`}
	for _, effect := range []*model.Effect{&stale, &fresh} {
		if err := effectRepo.Create(ctx, effect); err != nil {
			t.Fatalf("create effect: %v", err)
		}
	}
	backdateEffectUpdatedAt(t, db, "effect-stale")

	orch := New(nil, nil, WithEffectRepository(effectRepo))

	results, err := orch.RecoverStaleEffects(ctx, 10)
	if err != nil {
		t.Fatalf("recover stale effects: %v", err)
	}
	if len(results) != 1 || results[0].DedupeKey != "effect-stale" {
		t.Fatalf("expected only stale effect recovered, got %#v", results)
	}

	recovered, err := effectRepo.GetByDedupeKey(ctx, "effect-stale")
	if err != nil {
		t.Fatalf("get recovered effect: %v", err)
	}
	if recovered.Status != model.EffectStatusFailed || recovered.ErrorMessage == "" {
		t.Fatalf("expected failed effect with error message, got %#v", recovered)
	}

	freshAfter, err := effectRepo.GetByDedupeKey(ctx, "effect-fresh")
	if err != nil {
		t.Fatalf("get fresh effect: %v", err)
	}
	if freshAfter.Status != model.EffectStatusRunning {
		t.Fatalf("fresh running effect must stay untouched, got %s", freshAfter.Status)
	}
}

func backdateEffectUpdatedAt(t *testing.T, db *gorm.DB, dedupeKey string) {
	t.Helper()
	if err := db.Exec("UPDATE effects SET updated_at = ? WHERE dedupe_key = ?", time.Now().Add(-2*staleEffectRunningTimeout), dedupeKey).Error; err != nil {
		t.Fatalf("backdate effect %s: %v", dedupeKey, err)
	}
}
