package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"
)

const effectClaimStaleWindow = 15 * time.Minute

func newEffectClaimRow(dedupeKey string) model.Effect {
	return model.Effect{
		TaskID: "task-claim", RunID: "run-claim", DedupeKey: dedupeKey,
		Kind: "send_im_message", Target: "default",
		Status:      model.EffectStatusPending,
		RequestJSON: `{"body":"claim"}`,
	}
}

// TestEffectClaimMySQLExactlyOneWinner 两个独立连接同时抢占同一效果：恰好一个
// 赢家进入 running 且 attempt 递增一步；新鲜 running 与终态行不可抢占；停留
// 超过窗口的 running 可被接管。到期判定全部由数据库时钟完成。
func TestEffectClaimMySQLExactlyOneWinner(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "effect_claim")
	first, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open first: %v", err)
	}
	second, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open second: %v", err)
	}
	defer func() {
		if sqlDB, err := first.DB(); err == nil {
			_ = sqlDB.Close()
		}
		if sqlDB, err := second.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	if err := store.Migrate(first, &model.Effect{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	repoFirst := NewEffectRepository(first)
	repoSecond := NewEffectRepository(second)

	row := newEffectClaimRow("effect-race")
	if err := repoFirst.Create(ctx, &row); err != nil {
		t.Fatalf("create effect: %v", err)
	}

	type claimOutcome struct {
		won     bool
		attempt int
	}
	start := make(chan struct{})
	outcomes := make(chan claimOutcome, 2)
	var wg sync.WaitGroup
	for _, repo := range []*EffectRepository{repoFirst, repoSecond} {
		wg.Add(1)
		go func(repo *EffectRepository) {
			defer wg.Done()
			<-start
			claimed, won, err := repo.Claim(ctx, "effect-race", effectClaimStaleWindow)
			if err != nil {
				t.Errorf("claim: %v", err)
				outcomes <- claimOutcome{}
				return
			}
			attempt := 0
			if claimed != nil {
				attempt = claimed.AttemptCount
			}
			outcomes <- claimOutcome{won: won, attempt: attempt}
		}(repo)
	}
	close(start)
	wg.Wait()
	close(outcomes)

	winners := 0
	for outcome := range outcomes {
		if !outcome.won {
			continue
		}
		winners++
		if outcome.attempt != 1 {
			t.Fatalf("winner attempt = %d, want 1", outcome.attempt)
		}
	}
	if winners != 1 {
		t.Fatalf("winners = %d, want exactly 1", winners)
	}

	stored, err := repoFirst.GetByDedupeKey(ctx, "effect-race")
	if err != nil {
		t.Fatalf("reload effect: %v", err)
	}
	if stored.Status != model.EffectStatusRunning || stored.AttemptCount != 1 {
		t.Fatalf("row = %s/%d, want running/1", stored.Status, stored.AttemptCount)
	}

	// 新鲜 running 不可抢占。
	if err := first.Exec("UPDATE effects SET updated_at = NOW(6) WHERE dedupe_key = ?", "effect-race").Error; err != nil {
		t.Fatalf("refresh running row: %v", err)
	}
	if _, won, err := repoSecond.Claim(ctx, "effect-race", effectClaimStaleWindow); err != nil || won {
		t.Fatalf("claim against fresh running must lose: won=%v err=%v", won, err)
	}

	// 停留超过窗口的 running 可被接管，attempt 递增到 2。
	if err := first.Exec("UPDATE effects SET updated_at = DATE_SUB(NOW(6), INTERVAL 20 MINUTE) WHERE dedupe_key = ?", "effect-race").Error; err != nil {
		t.Fatalf("backdate running row: %v", err)
	}
	claimed, won, err := repoSecond.Claim(ctx, "effect-race", effectClaimStaleWindow)
	if err != nil || !won {
		t.Fatalf("stale takeover: won=%v err=%v", won, err)
	}
	if claimed.AttemptCount != 2 {
		t.Fatalf("takeover attempt = %d, want 2", claimed.AttemptCount)
	}

	// 终态行不可再抢。
	if applied, err := repoSecond.Complete(ctx, "effect-race", 2, model.EffectStatusSucceeded, `{"ok":true}`, ""); err != nil || !applied {
		t.Fatalf("complete claimed effect: applied=%v err=%v", applied, err)
	}
	if _, won, err := repoFirst.Claim(ctx, "effect-race", effectClaimStaleWindow); err != nil || won {
		t.Fatalf("claim against succeeded must lose: won=%v err=%v", won, err)
	}
}

// TestEffectCompleteAndRequeueMySQLGenerationFence 终态回写与退队都带 attempt
// 代次栅栏：被接管的旧持有方影响 0 行且不覆盖新执行方的写入；退队保留
// attempt 计数；陈旧代次的退队同样被拒。
func TestEffectCompleteAndRequeueMySQLGenerationFence(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	db := storetest.OpenDatabase(t, admin, "effect_fence")
	if err := store.Migrate(db, &model.Effect{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	repo := NewEffectRepository(db)

	row := newEffectClaimRow("effect-fence")
	if err := repo.Create(ctx, &row); err != nil {
		t.Fatalf("create effect: %v", err)
	}
	first, won, err := repo.Claim(ctx, "effect-fence", effectClaimStaleWindow)
	if err != nil || !won {
		t.Fatalf("first claim: won=%v err=%v", won, err)
	}
	if err := db.Exec("UPDATE effects SET updated_at = DATE_SUB(NOW(6), INTERVAL 20 MINUTE) WHERE dedupe_key = ?", "effect-fence").Error; err != nil {
		t.Fatalf("backdate running row: %v", err)
	}
	second, won, err := repo.Claim(ctx, "effect-fence", effectClaimStaleWindow)
	if err != nil || !won {
		t.Fatalf("takeover claim: won=%v err=%v", won, err)
	}
	if second.AttemptCount != 2 {
		t.Fatalf("takeover attempt = %d, want 2", second.AttemptCount)
	}

	applied, err := repo.Complete(ctx, "effect-fence", first.AttemptCount, model.EffectStatusSucceeded, `{"stale":true}`, "")
	if err != nil || applied {
		t.Fatalf("stale generation complete must be rejected: applied=%v err=%v", applied, err)
	}
	applied, err = repo.Complete(ctx, "effect-fence", second.AttemptCount, model.EffectStatusSucceeded, `{"ok":true}`, "")
	if err != nil || !applied {
		t.Fatalf("current generation complete: applied=%v err=%v", applied, err)
	}
	final, err := repo.GetByDedupeKey(ctx, "effect-fence")
	if err != nil {
		t.Fatalf("reload effect: %v", err)
	}
	if final.Status != model.EffectStatusSucceeded || final.ResponseJSON != `{"ok":true}` {
		t.Fatalf("row = %s/%q, want succeeded with winner response", final.Status, final.ResponseJSON)
	}

	requeued := newEffectClaimRow("effect-requeue")
	if err := repo.Create(ctx, &requeued); err != nil {
		t.Fatalf("create requeue effect: %v", err)
	}
	claimed, won, err := repo.Claim(ctx, "effect-requeue", effectClaimStaleWindow)
	if err != nil || !won {
		t.Fatalf("claim for requeue: won=%v err=%v", won, err)
	}
	applied, err = repo.RequeueIfRunning(ctx, "effect-requeue", claimed.AttemptCount)
	if err != nil || !applied {
		t.Fatalf("requeue running effect: applied=%v err=%v", applied, err)
	}
	after, err := repo.GetByDedupeKey(ctx, "effect-requeue")
	if err != nil {
		t.Fatalf("reload requeued effect: %v", err)
	}
	if after.Status != model.EffectStatusPending || after.AttemptCount != 1 {
		t.Fatalf("requeued row = %s/%d, want pending/1 (attempt kept)", after.Status, after.AttemptCount)
	}
	if applied, err := repo.RequeueIfRunning(ctx, "effect-requeue", 99); err != nil || applied {
		t.Fatalf("stale generation requeue must be rejected: applied=%v err=%v", applied, err)
	}
}

// TestEffectClaimSQLiteSemanticsMatchMySQL SQLite 上的抢占/代次语义与 MySQL
// 一致；staleness 比较参数与写入同走本机时钟（updated_at 以本地时区串存储），
// 该测试锁存两侧时间语义不漂移。
func TestEffectClaimSQLiteSemanticsMatchMySQL(t *testing.T) {
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.Effect{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	repo := NewEffectRepository(db)

	row := newEffectClaimRow("effect-sqlite")
	if err := repo.Create(ctx, &row); err != nil {
		t.Fatalf("create effect: %v", err)
	}

	claimed, won, err := repo.Claim(ctx, "effect-sqlite", effectClaimStaleWindow)
	if err != nil || !won {
		t.Fatalf("claim pending: won=%v err=%v", won, err)
	}
	if claimed.Status != model.EffectStatusRunning || claimed.AttemptCount != 1 {
		t.Fatalf("claimed = %s/%d, want running/1", claimed.Status, claimed.AttemptCount)
	}

	if _, won, err := repo.Claim(ctx, "effect-sqlite", effectClaimStaleWindow); err != nil || won {
		t.Fatalf("claim against fresh running must lose: won=%v err=%v", won, err)
	}

	if err := db.Exec("UPDATE effects SET updated_at = ? WHERE dedupe_key = ?", time.Now().Add(-20*time.Minute), "effect-sqlite").Error; err != nil {
		t.Fatalf("backdate running row: %v", err)
	}
	again, won, err := repo.Claim(ctx, "effect-sqlite", effectClaimStaleWindow)
	if err != nil || !won {
		t.Fatalf("stale takeover: won=%v err=%v", won, err)
	}
	if again.AttemptCount != 2 {
		t.Fatalf("takeover attempt = %d, want 2", again.AttemptCount)
	}

	if applied, err := repo.Complete(ctx, "effect-sqlite", 1, model.EffectStatusFailed, "", "stale holder write"); err != nil || applied {
		t.Fatalf("stale generation complete must be rejected: applied=%v err=%v", applied, err)
	}
	if applied, err := repo.Complete(ctx, "effect-sqlite", again.AttemptCount, model.EffectStatusSucceeded, `{"ok":true}`, ""); err != nil || !applied {
		t.Fatalf("current generation complete: applied=%v err=%v", applied, err)
	}
	if _, won, err := repo.Claim(ctx, "effect-sqlite", effectClaimStaleWindow); err != nil || won {
		t.Fatalf("claim against succeeded must lose: won=%v err=%v", won, err)
	}
}
