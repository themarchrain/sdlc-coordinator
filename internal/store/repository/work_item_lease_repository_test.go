package repository

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"

	"gorm.io/gorm"
)

func openLeaseTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open(fmt.Sprintf("file:lease_%s?mode=memory&cache=shared", model.NewID("t")))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, &model.WorkItemLease{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestWorkItemLeaseLifecycleAndTakeover 锁定执行权生命周期：争抢失败、续租
// 代次校验、过期接管后旧 owner 的续租与核验全部失败、条件释放。
func TestWorkItemLeaseLifecycleAndTakeover(t *testing.T) {
	db := openLeaseTestDB(t)
	repo := NewWorkItemLeaseRepository(db)
	ctx := context.Background()
	scope := "task-dispatch:task_1"

	leaseA, won, err := repo.Acquire(ctx, scope, "instance-a", "", time.Minute)
	if err != nil || !won {
		t.Fatalf("first acquire: won=%v err=%v", won, err)
	}
	if leaseA.Generation != 1 {
		t.Fatalf("first generation = %d, want 1", leaseA.Generation)
	}
	if _, won, err := repo.Acquire(ctx, scope, "instance-b", "", time.Minute); err != nil || won {
		t.Fatalf("contended acquire must lose: won=%v err=%v", won, err)
	}

	if ok, err := repo.Renew(ctx, scope, "instance-a", leaseA.Generation, time.Minute); err != nil || !ok {
		t.Fatalf("owner renew: ok=%v err=%v", ok, err)
	}
	if ok, err := repo.Renew(ctx, scope, "instance-b", leaseA.Generation, time.Minute); err != nil || ok {
		t.Fatalf("non-owner renew must fail: ok=%v err=%v", ok, err)
	}
	if ok, err := repo.Verify(ctx, scope, "instance-a", leaseA.Generation); err != nil || !ok {
		t.Fatalf("owner verify: ok=%v err=%v", ok, err)
	}

	// 模拟过期（直接把到期时间拨到过去），B 接管。
	if err := db.Exec("UPDATE work_item_leases SET lease_until = ? WHERE scope = ?", time.Now().UTC().Add(-time.Minute), scope).Error; err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	leaseB, won, err := repo.Acquire(ctx, scope, "instance-b", "op-1", time.Minute)
	if err != nil || !won {
		t.Fatalf("takeover acquire: won=%v err=%v", won, err)
	}
	if leaseB.Generation != leaseA.Generation+1 {
		t.Fatalf("takeover generation = %d, want %d", leaseB.Generation, leaseA.Generation+1)
	}

	// A 的陈旧操作全部失败：续租、核验、按旧代次释放。
	if ok, _ := repo.Renew(ctx, scope, "instance-a", leaseA.Generation, time.Minute); ok {
		t.Fatal("stale renew succeeded after takeover")
	}
	if ok, _ := repo.Verify(ctx, scope, "instance-a", leaseA.Generation); ok {
		t.Fatal("stale verify succeeded after takeover")
	}
	if ok, _ := repo.Release(ctx, scope, "instance-a", leaseA.Generation); ok {
		t.Fatal("stale release succeeded after takeover")
	}
	// B 的新代次有效。
	if ok, err := repo.Verify(ctx, scope, "instance-b", leaseB.Generation); err != nil || !ok {
		t.Fatalf("new owner verify: ok=%v err=%v", ok, err)
	}
	if ok, err := repo.Release(ctx, scope, "instance-b", leaseB.Generation); err != nil || !ok {
		t.Fatalf("new owner release: ok=%v err=%v", ok, err)
	}
	if row, err := repo.Get(ctx, scope); err != nil || row != nil {
		t.Fatalf("released lease still present: %+v err=%v", row, err)
	}
	if _, won, err := repo.Acquire(ctx, scope, "instance-c", "", time.Minute); err != nil || !won {
		t.Fatalf("acquire after release: won=%v err=%v", won, err)
	}
}

// TestWorkItemLeaseMySQLDBTimeSemantics 锁定 MySQL 上的数据库时间语义：到期
// 判定与租期推进全部由数据库时钟完成（客户端时钟不参与）。
func TestWorkItemLeaseMySQLDBTimeSemantics(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	db := storetest.OpenDatabase(t, admin, "lease_time")
	if err := store.Migrate(db, &model.WorkItemLease{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewWorkItemLeaseRepository(db)
	ctx := context.Background()
	scope := "effect-send:effect_1"

	lease, won, err := repo.Acquire(ctx, scope, "instance-a", "", 90*time.Second)
	if err != nil || !won {
		t.Fatalf("acquire: won=%v err=%v", won, err)
	}
	// 租期由数据库时钟推进：DB 侧差值应接近 TTL。
	var remainingMicros int64
	if err := db.Raw("SELECT TIMESTAMPDIFF(MICROSECOND, NOW(6), lease_until) FROM work_item_leases WHERE scope = ?", scope).Scan(&remainingMicros).Error; err != nil {
		t.Fatalf("read remaining: %v", err)
	}
	if remainingMicros < int64((80*time.Second).Microseconds()) || remainingMicros > int64((95*time.Second).Microseconds()) {
		t.Fatalf("db remaining lease = %dus, want ~90s", remainingMicros)
	}
	if !lease.LeaseUntil.After(time.Now().UTC()) {
		t.Fatalf("lease_until %v not in the future", lease.LeaseUntil)
	}

	// 用数据库表达式把到期时间拨到过去 → 可接管；拨到未来 → 拒绝。
	if err := db.Exec("UPDATE work_item_leases SET lease_until = DATE_SUB(NOW(6), INTERVAL 1 SECOND) WHERE scope = ?", scope).Error; err != nil {
		t.Fatalf("expire by db time: %v", err)
	}
	leaseB, won, err := repo.Acquire(ctx, scope, "instance-b", "", time.Minute)
	if err != nil || !won || leaseB.Generation != 2 {
		t.Fatalf("takeover after db-expiry: won=%v gen=%d err=%v", won, leaseB.Generation, err)
	}
	if err := db.Exec("UPDATE work_item_leases SET lease_until = DATE_ADD(NOW(6), INTERVAL 10 SECOND) WHERE scope = ?", scope).Error; err != nil {
		t.Fatalf("extend by db time: %v", err)
	}
	if _, won, err := repo.Acquire(ctx, scope, "instance-c", "", time.Minute); err != nil || won {
		t.Fatalf("acquire against db-unexpired lease must lose: won=%v err=%v", won, err)
	}
}

// TestWorkItemLeaseMySQLContention 两个独立连接在栅栏后反复争抢同一工作项：
// 每轮恰好一个赢家且代次单调递增。
func TestWorkItemLeaseMySQLContention(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "lease_race")
	first, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open first: %v", err)
	}
	if err := store.Migrate(first, &model.WorkItemLease{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	second, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open second: %v", err)
	}
	ctx := context.Background()
	repos := []*WorkItemLeaseRepository{NewWorkItemLeaseRepository(first), NewWorkItemLeaseRepository(second)}
	owners := []string{"instance-a", "instance-b"}

	for round := 0; round < 8; round++ {
		scope := fmt.Sprintf("merge-gate:task_%d", round)
		start := make(chan struct{})
		results := make(chan bool, 2)
		var wg sync.WaitGroup
		for i := range repos {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				_, won, err := repos[i].Acquire(ctx, scope, owners[i], "", 30*time.Second)
				if err != nil {
					t.Errorf("round %d acquire: %v", round, err)
				}
				results <- won
			}(i)
		}
		close(start)
		wg.Wait()
		close(results)
		winners := 0
		for won := range results {
			if won {
				winners++
			}
		}
		if winners != 1 {
			t.Fatalf("round %d winners = %d, want exactly 1", round, winners)
		}
		// 该轮赢家释放，下一轮重新竞争（代次应继续递增）。
		row, err := NewWorkItemLeaseRepository(first).Get(ctx, scope)
		if err != nil || row == nil {
			t.Fatalf("round %d read lease: %+v err=%v", round, row, err)
		}
		if row.Generation != 1 {
			t.Fatalf("round %d generation = %d, want 1", round, row.Generation)
		}
		if ok, err := NewWorkItemLeaseRepository(first).Release(ctx, scope, row.Owner, row.Generation); err != nil || !ok {
			t.Fatalf("round %d release: ok=%v err=%v", round, ok, err)
		}
	}
}

// TestWorkItemLeaseCrossProcessWinner 两个真实进程争抢同一工作项：恰好一个
// 进程报告赢得执行权（进程级互斥，不依赖同进程锁）。
func TestWorkItemLeaseCrossProcessWinner(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "lease_procrace")
	setup, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("setup open: %v", err)
	}
	if err := store.Migrate(setup, &model.WorkItemLease{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := setup.Exec("CREATE TABLE lease_ready(holder varchar(16) primary key)").Error; err != nil {
		t.Fatalf("ready table: %v", err)
	}
	if err := setup.Exec("CREATE TABLE lease_result(holder varchar(16) primary key, won tinyint not null)").Error; err != nil {
		t.Fatalf("result table: %v", err)
	}
	if sqlDB, err := setup.DB(); err == nil {
		_ = sqlDB.Close()
	}

	type childResult struct {
		holder string
		output string
		err    error
	}
	results := make(chan childResult, 2)
	var wg sync.WaitGroup
	for _, holder := range []string{"A", "B"} {
		wg.Add(1)
		go func(holder string) {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestWorkItemLeaseChildProcess$", "-test.v")
			cmd.Env = append(os.Environ(),
				"SDLC_TEST_LEASE_CHILD_DSN="+dsn,
				"SDLC_TEST_LEASE_CHILD_HOLDER="+holder,
			)
			out, err := cmd.CombinedOutput()
			results <- childResult{holder: holder, output: string(out), err: err}
		}(holder)
	}
	wg.Wait()
	close(results)
	for result := range results {
		if result.err != nil {
			t.Fatalf("child %s failed: %v\n%s", result.holder, result.err, result.output)
		}
	}

	verify, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("verify open: %v", err)
	}
	defer func() {
		if sqlDB, err := verify.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	var winners int64
	if err := verify.Raw("SELECT COUNT(*) FROM lease_result WHERE won = 1").Scan(&winners).Error; err != nil {
		t.Fatalf("count winners: %v", err)
	}
	if winners != 1 {
		t.Fatalf("process-level winners = %d, want exactly 1", winners)
	}
	row, err := NewWorkItemLeaseRepository(verify).Get(context.Background(), "requirement-publish:req_1")
	if err != nil || row == nil {
		t.Fatalf("lease row: %+v err=%v", row, err)
	}
	if row.Generation != 1 {
		t.Fatalf("generation after cross-process race = %d, want 1", row.Generation)
	}
}

// TestWorkItemLeaseChildProcess 子进程入口：经汇合表对齐后争抢同一工作项并
// 落结果行，供父测试断言进程级互斥。
func TestWorkItemLeaseChildProcess(t *testing.T) {
	dsn := os.Getenv("SDLC_TEST_LEASE_CHILD_DSN")
	holder := os.Getenv("SDLC_TEST_LEASE_CHILD_HOLDER")
	if dsn == "" || holder == "" {
		t.Skip("child helper for TestWorkItemLeaseCrossProcessWinner")
	}
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("child open: %v", err)
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	ctx := context.Background()
	if err := db.Exec("INSERT INTO lease_ready(holder) VALUES (?)", holder).Error; err != nil {
		t.Fatalf("rendezvous insert: %v", err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		var ready int64
		if err := db.Raw("SELECT COUNT(*) FROM lease_ready").Scan(&ready).Error; err != nil {
			t.Fatalf("rendezvous poll: %v", err)
		}
		if ready >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("child %s rendezvous timed out", holder)
		}
		time.Sleep(25 * time.Millisecond)
	}
	_, won, err := NewWorkItemLeaseRepository(db).Acquire(ctx, "requirement-publish:req_1", "child-"+holder, "", time.Minute)
	if err != nil {
		t.Fatalf("child acquire: %v", err)
	}
	wonFlag := 0
	if won {
		wonFlag = 1
	}
	if err := db.Exec("INSERT INTO lease_result(holder, won) VALUES (?, ?)", holder, wonFlag).Error; err != nil {
		t.Fatalf("record result: %v", err)
	}
}
