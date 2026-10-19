package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/store/storetest"
)

// concurrentEffectExecutor 线程安全的执行器桩：记录跨实例总调用次数。
type concurrentEffectExecutor struct {
	mu    sync.Mutex
	calls []string
}

func (f *concurrentEffectExecutor) ExecuteEffect(_ context.Context, effect *model.Effect) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, effect.DedupeKey)
	return `{"message_id":"msg-race"}`, nil
}

func (f *concurrentEffectExecutor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// TestExecutePendingEffectsDualInstanceSingleWinner 两实例共享 MySQL 状态、
// 同时消费同一待发效果：恰好一个实例取得执行权并进入有效执行，执行器总
// 调用恰好一次，落库一笔成功终态。
func TestExecutePendingEffectsDualInstanceSingleWinner(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "effect_dual")

	repos := make([]*repository.EffectRepository, 2)
	orches := make([]*Orchestrator, 2)
	for i := range repos {
		db, err := store.Open(dsn)
		if err != nil {
			t.Fatalf("open instance %d: %v", i, err)
		}
		defer func() {
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}()
		if err := store.Migrate(db, &model.Effect{}); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		repos[i] = repository.NewEffectRepository(db)
		orches[i] = New(nil, nil, WithEffectRepository(repos[i]))
	}

	ctx := context.Background()
	if err := repos[0].Create(ctx, &model.Effect{
		TaskID: "task-dual", RunID: "run-dual", DedupeKey: "effect-dual",
		Kind: "send_im_message", Target: "default",
		Status:      model.EffectStatusPending,
		RequestJSON: `{"body":"once"}`,
	}); err != nil {
		t.Fatalf("create effect: %v", err)
	}

	executor := &concurrentEffectExecutor{}
	for _, orch := range orches {
		orch.effectExecutor = executor
	}

	start := make(chan struct{})
	outcomes := make([][]EffectExecutionResult, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range orches {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			outcomes[i], errs[i] = orches[i].ExecutePendingEffects(ctx, 10)
		}(i)
	}
	close(start)
	wg.Wait()

	for i := range orches {
		if errs[i] != nil {
			t.Fatalf("instance %d: %v", i, errs[i])
		}
	}
	if executor.callCount() != 1 {
		t.Fatalf("executor called %d times, want exactly 1", executor.callCount())
	}
	totalResults := 0
	for i := range outcomes {
		totalResults += len(outcomes[i])
	}
	if totalResults != 1 {
		t.Fatalf("total results = %d across instances, want exactly 1 (%+v / %+v)", totalResults, outcomes[0], outcomes[1])
	}
	final, err := repos[0].GetByDedupeKey(ctx, "effect-dual")
	if err != nil {
		t.Fatalf("reload effect: %v", err)
	}
	if final.Status != model.EffectStatusSucceeded || final.AttemptCount != 1 {
		t.Fatalf("row = %s/%d, want succeeded/1", final.Status, final.AttemptCount)
	}
}

// TestResponseLossNotBlindlyResent 远端发送成功但响应丢失(执行方在落终态前
// 中断,行停留 running):恢复入口把停留超阈值的行置 failed 供人工裁决,而不是
// 被另一实例直接重发;人工 RetryEffect 后才重新排队。
func TestResponseLossNotBlindlyResent(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Effect{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	effectRepo := repository.NewEffectRepository(db)

	row := model.Effect{
		TaskID: "task-loss", RunID: "run-loss", DedupeKey: "effect-loss",
		Kind: "send_im_message", Target: "default",
		Status:       model.EffectStatusRunning,
		AttemptCount: 1,
		RequestJSON:  `{"body":"maybe delivered"}`,
	}
	if err := effectRepo.Create(ctx, &row); err != nil {
		t.Fatalf("create effect: %v", err)
	}
	if err := db.Exec("UPDATE effects SET updated_at = ? WHERE dedupe_key = ?", time.Now().Add(-2*staleEffectRunningTimeout), "effect-loss").Error; err != nil {
		t.Fatalf("backdate effect: %v", err)
	}

	orchA := New(nil, nil, WithEffectRepository(effectRepo))
	// 另一实例的执行器一旦被调用即失败:证明恢复路径没有重发。
	neverCalled := &concurrentEffectExecutor{}
	orchB := New(nil, nil, WithEffectRepository(effectRepo))
	orchB.effectExecutor = neverCalled

	recovered, err := orchA.RecoverStaleEffects(ctx, 10)
	if err != nil {
		t.Fatalf("recover stale effects: %v", err)
	}
	if len(recovered) != 1 || recovered[0].Status != model.EffectStatusFailed {
		t.Fatalf("expected one failed recovery, got %#v", recovered)
	}

	// 未知结果的行不进入常规执行队列:另一实例的消费批次不得重发。
	consumed, err := orchB.ExecutePendingEffects(ctx, 10)
	if err != nil {
		t.Fatalf("execute pending: %v", err)
	}
	if len(consumed) != 0 {
		t.Fatalf("unknown-result effect must not be re-sent, got %#v", consumed)
	}
	if neverCalled.callCount() != 0 {
		t.Fatalf("executor must not fire for unknown-result effect, got %v", neverCalled.calls)
	}

	stored, err := effectRepo.GetByDedupeKey(ctx, "effect-loss")
	if err != nil {
		t.Fatalf("reload effect: %v", err)
	}
	if stored.Status != model.EffectStatusFailed || stored.ErrorMessage == "" {
		t.Fatalf("row = %s/%q, want failed with reason", stored.Status, stored.ErrorMessage)
	}

	// 人工裁决后重新排队,才可再次执行。
	if _, err := orchA.RetryEffect(ctx, "effect-loss"); err != nil {
		t.Fatalf("human retry: %v", err)
	}
	requeuedExecutor := &concurrentEffectExecutor{}
	orchA.effectExecutor = requeuedExecutor
	if _, err := orchA.ExecutePendingEffects(ctx, 10); err != nil {
		t.Fatalf("execute after human retry: %v", err)
	}
	if requeuedExecutor.callCount() != 1 {
		t.Fatalf("executor called %d times after explicit retry, want 1", requeuedExecutor.callCount())
	}
}

// TestStaleRecoveryRaceDualInstance 双实例同时回收同一 stale 效果:恰好一个
// 落 failed 终态,败者静默,无双写。
func TestStaleRecoveryRaceDualInstance(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(testDatabaseDSN(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Effect{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	effectRepo := repository.NewEffectRepository(db)
	row := model.Effect{
		TaskID: "task-stale", RunID: "run-stale", DedupeKey: "effect-stale-race",
		Kind: "send_im_message", Target: "default",
		Status:       model.EffectStatusRunning,
		AttemptCount: 1,
		RequestJSON:  `{"body":"stale"}`,
	}
	if err := effectRepo.Create(ctx, &row); err != nil {
		t.Fatalf("create effect: %v", err)
	}
	if err := db.Exec("UPDATE effects SET updated_at = ? WHERE dedupe_key = ?", time.Now().Add(-2*staleEffectRunningTimeout), "effect-stale-race").Error; err != nil {
		t.Fatalf("backdate effect: %v", err)
	}

	orchA := New(nil, nil, WithEffectRepository(effectRepo))
	orchB := New(nil, nil, WithEffectRepository(effectRepo))

	var recoveredA, recoveredB []EffectExecutionResult
	var errA, errB error
	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); recoveredA, errA = orchA.RecoverStaleEffects(ctx, 10) }()
		go func() { defer wg.Done(); recoveredB, errB = orchB.RecoverStaleEffects(ctx, 10) }()
		wg.Wait()
	}()
	<-done
	if errA != nil || errB != nil {
		t.Fatalf("recover errors: %v / %v", errA, errB)
	}
	total := len(recoveredA) + len(recoveredB)
	if total != 1 {
		t.Fatalf("total recoveries = %d (%+v / %+v), want exactly 1", total, recoveredA, recoveredB)
	}
	final, err := effectRepo.GetByDedupeKey(ctx, "effect-stale-race")
	if err != nil {
		t.Fatalf("reload effect: %v", err)
	}
	if final.Status != model.EffectStatusFailed {
		t.Fatalf("status = %s, want failed", final.Status)
	}
	if final.AttemptCount != 1 {
		t.Fatalf("attempt = %d, want unchanged 1 (recovery must not increment)", final.AttemptCount)
	}
}
