package store_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"
)

// TestMySQLForcedSessionSemantics 锁定打开层强制的会话语义：UTC 时钟、
// datetime(6) 往返不被时区改写、连接排序规则为二元（业务键大小写敏感）。
func TestMySQLForcedSessionSemantics(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	db := storetest.OpenDatabase(t, admin, "semantics")

	var sessionTZ string
	if err := db.Raw("SELECT @@session.time_zone").Scan(&sessionTZ).Error; err != nil {
		t.Fatalf("read session time_zone: %v", err)
	}
	if sessionTZ != "+00:00" {
		t.Fatalf("session time_zone = %q, want +00:00", sessionTZ)
	}
	var collation string
	if err := db.Raw("SELECT @@collation_connection").Scan(&collation).Error; err != nil {
		t.Fatalf("read collation_connection: %v", err)
	}
	if collation != "utf8mb4_bin" {
		t.Fatalf("collation_connection = %q, want utf8mb4_bin", collation)
	}

	var dbNow time.Time
	if err := db.Raw("SELECT NOW(6)").Scan(&dbNow).Error; err != nil {
		t.Fatalf("select now: %v", err)
	}
	if dbNow.Location() != time.UTC {
		t.Fatalf("NOW(6) decoded with location %v, want UTC", dbNow.Location())
	}
	if delta := time.Since(dbNow); delta > 2*time.Minute || delta < -2*time.Minute {
		t.Fatalf("database clock vs host clock drift too large: %v", delta)
	}

	if err := db.Exec("CREATE TABLE time_probe(id int primary key, created_at datetime(6) not null)").Error; err != nil {
		t.Fatalf("create time_probe: %v", err)
	}
	written := time.Now().Truncate(time.Microsecond)
	if err := db.Exec("INSERT INTO time_probe(id, created_at) VALUES (1, ?)", written).Error; err != nil {
		t.Fatalf("insert time_probe: %v", err)
	}
	var readBack time.Time
	if err := db.Raw("SELECT created_at FROM time_probe WHERE id = 1").Scan(&readBack).Error; err != nil {
		t.Fatalf("select time_probe: %v", err)
	}
	if !readBack.Equal(written) {
		t.Fatalf("time roundtrip mismatch: wrote %s read %s (loc %v)", written, readBack, readBack.Location())
	}
}

// TestMySQLCaseSensitiveBusinessKeys 锁定业务键的大小写语义：库级 utf8mb4_bin
// 下，仅大小写不同的键是不同行（MySQL 默认 ai_ci 排序规则会把它们折叠成同一行）。
func TestMySQLCaseSensitiveBusinessKeys(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	db := storetest.OpenDatabase(t, admin, "case_keys")

	if err := db.Exec("CREATE TABLE case_probe(business_key varchar(64) not null, primary key(business_key))").Error; err != nil {
		t.Fatalf("create case_probe: %v", err)
	}
	if err := db.Exec("INSERT INTO case_probe(business_key) VALUES ('Repo-Alpha')").Error; err != nil {
		t.Fatalf("insert mixed case: %v", err)
	}
	if err := db.Exec("INSERT INTO case_probe(business_key) VALUES ('repo-alpha')").Error; err != nil {
		t.Fatalf("lowercase twin must coexist, got: %v", err)
	}

	var upperCount int64
	if err := db.Raw("SELECT COUNT(*) FROM case_probe WHERE business_key = 'Repo-Alpha'").Scan(&upperCount).Error; err != nil {
		t.Fatalf("count exact case: %v", err)
	}
	if upperCount != 1 {
		t.Fatalf("exact-case lookup matched %d rows, want 1", upperCount)
	}

	err := db.Exec("INSERT INTO case_probe(business_key) VALUES ('Repo-Alpha')").Error
	if err == nil {
		t.Fatal("exact duplicate accepted")
	}
	if !store.IsDuplicateKey(err) {
		t.Fatalf("duplicate key not classified by driver code: %v", err)
	}

	// SQL 表达式层同样保持二元语义（LOWER 兜底查询的等价实现依赖它）。
	var folded int64
	if err := db.Raw("SELECT COUNT(*) FROM case_probe WHERE LOWER(business_key) = LOWER('REPO-ALPHA')").Scan(&folded).Error; err != nil {
		t.Fatalf("lower fallback query: %v", err)
	}
	if folded != 2 {
		t.Fatalf("lower() fallback matched %d rows, want 2", folded)
	}
}

// TestMySQLPoolBudgetApplied 锁定连接池预算落到 sql.DB。
func TestMySQLPoolBudgetApplied(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "pool")

	db, err := store.Open(dsn + "?max_open_conns=4&max_idle_conns=2")
	if err != nil {
		t.Fatalf("open with budget: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql handle: %v", err)
	}
	defer sqlDB.Close()
	if got := sqlDB.Stats().MaxOpenConnections; got != 4 {
		t.Fatalf("MaxOpenConnections = %d, want 4", got)
	}

	defaultDB, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open default: %v", err)
	}
	defaultSQL, err := defaultDB.DB()
	if err != nil {
		t.Fatalf("default sql handle: %v", err)
	}
	defer defaultSQL.Close()
	if got := defaultSQL.Stats().MaxOpenConnections; got != store.DefaultMySQLMaxOpenConns {
		t.Fatalf("default MaxOpenConnections = %d, want %d", got, store.DefaultMySQLMaxOpenConns)
	}
}

// TestMigrateGuardedRecordsVersionAndRefusesNewer 锁定受控迁移入口：记录
// schema 版本、幂等重入、库内版本高于二进制时拒绝执行。
func TestMigrateGuardedRecordsVersionAndRefusesNewer(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	db := storetest.OpenDatabase(t, admin, "migrate_version")
	ctx := context.Background()

	if v, err := store.CurrentSchemaVersion(ctx, db, "biz"); err != nil || v != 0 {
		t.Fatalf("initial version = %d (%v), want 0", v, err)
	}

	runs := 0
	fn := func() error {
		runs++
		return db.Exec("CREATE TABLE IF NOT EXISTS biz_probe(id int primary key)").Error
	}
	if err := store.MigrateGuarded(ctx, db, "biz", fn); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if v, err := store.CurrentSchemaVersion(ctx, db, "biz"); err != nil || v != store.SchemaVersion {
		t.Fatalf("recorded version = %d (%v), want %d", v, err, store.SchemaVersion)
	}
	// 版本一致即当前 schema：MySQL 快速路径跳过迁移函数（受控迁移是发布
	// 步骤，变更须递增 SchemaVersion）；SQLite 开发形态保持每次重放幂等。
	if err := store.MigrateGuarded(ctx, db, "biz", fn); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if runs != 1 {
		t.Fatalf("migration fn ran %d times, want 1 (skip when recorded version matches)", runs)
	}
	if err := db.Exec("UPDATE schema_migrations SET version = ? WHERE component = 'biz'", store.SchemaVersion-1).Error; err != nil {
		t.Fatalf("set previous version: %v", err)
	}
	if err := store.MigrateGuarded(ctx, db, "biz", fn); err != nil {
		t.Fatalf("upgrade from previous version: %v", err)
	}
	if runs != 2 {
		t.Fatalf("migration fn ran %d times, want 2 after upgrade", runs)
	}

	if err := db.Exec("UPDATE schema_migrations SET version = ? WHERE component = 'biz'", store.SchemaVersion+1).Error; err != nil {
		t.Fatalf("bump recorded version: %v", err)
	}
	err := store.MigrateGuarded(ctx, db, "biz", fn)
	if !errors.Is(err, store.ErrSchemaNewerThanBinary) {
		t.Fatalf("newer-schema migrate err = %v, want ErrSchemaNewerThanBinary", err)
	}
	if runs != 2 {
		t.Fatalf("migration fn must not run against newer schema (ran %d times)", runs)
	}
}

func TestMigrateGuardedCrossProcessExclusion(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "migrate_race")

	setup, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("setup open: %v", err)
	}
	setupSQL, _ := setup.DB()
	for _, ddl := range []string{
		"CREATE TABLE migrate_ready(holder varchar(16) primary key)",
		"CREATE TABLE migrate_witness(holder varchar(16) primary key, started_at datetime(6) not null, finished_at datetime(6) null)",
	} {
		if err := setup.Exec(ddl).Error; err != nil {
			t.Fatalf("setup ddl: %v", err)
		}
	}
	_ = setupSQL.Close()

	type childResult struct {
		holder string
		output []byte
		err    error
	}
	results := make(chan childResult, 2)
	var wg sync.WaitGroup
	for _, holder := range []string{"A", "B"} {
		wg.Add(1)
		go func(holder string) {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestMigrateGuardedChildProcess$", "-test.v")
			cmd.Env = append(os.Environ(),
				"SDLC_TEST_MIGRATE_CHILD_DSN="+dsn,
				"SDLC_TEST_MIGRATE_CHILD_HOLDER="+holder,
			)
			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			results <- childResult{holder: holder, output: out.Bytes(), err: cmd.Run()}
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
	verifySQL, _ := verify.DB()
	defer verifySQL.Close()

	var completed int64
	if err := verify.Raw("SELECT COUNT(*) FROM migrate_witness WHERE finished_at IS NOT NULL").Scan(&completed).Error; err != nil {
		t.Fatalf("count witness: %v", err)
	}
	if completed != 2 {
		t.Fatalf("completed critical sections = %d, want 2", completed)
	}
	var overlap int64
	if err := verify.Raw(`SELECT COUNT(*) FROM migrate_witness a JOIN migrate_witness b
		ON a.holder < b.holder
		WHERE a.started_at < b.finished_at AND b.started_at < a.finished_at`).Scan(&overlap).Error; err != nil {
		t.Fatalf("overlap query: %v", err)
	}
	if overlap != 0 {
		t.Fatalf("migration critical sections overlapped (%d), cross-process lock ineffective", overlap)
	}
	if v, err := store.CurrentSchemaVersion(context.Background(), verify, "itest_race"); err != nil || v != store.SchemaVersion {
		t.Fatalf("recorded version = %d (%v), want %d", v, err, store.SchemaVersion)
	}
}

// TestMigrateGuardedChildProcess 是被父测试用 -test.run 重执行调用的子进程
// 入口：先经 MySQL 汇合表与另一子进程对齐，再进入受锁迁移临界区。
func TestMigrateGuardedChildProcess(t *testing.T) {
	dsn := os.Getenv("SDLC_TEST_MIGRATE_CHILD_DSN")
	holder := os.Getenv("SDLC_TEST_MIGRATE_CHILD_HOLDER")
	if dsn == "" || holder == "" {
		t.Skip("child helper for TestMigrateGuardedCrossProcessExclusion")
	}
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("child open: %v", err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	if err := db.Exec("INSERT INTO migrate_ready(holder) VALUES (?)", holder).Error; err != nil {
		t.Fatalf("child rendezvous insert: %v", err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		var ready int64
		if err := db.Raw("SELECT COUNT(*) FROM migrate_ready").Scan(&ready).Error; err != nil {
			t.Fatalf("child rendezvous poll: %v", err)
		}
		if ready >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("child %s rendezvous timed out", holder)
		}
		time.Sleep(25 * time.Millisecond)
	}

	// Force 重放：版本一致时默认跳过迁移函数，两个进程都要真实进入临界区
	// 才能见证互斥；受控迁移锁本身与是否重放无关。
	err = store.MigrateGuarded(context.Background(), db, "itest_race", func() error {
		if err := db.Exec("INSERT INTO migrate_witness(holder, started_at) VALUES (?, NOW(6))", holder).Error; err != nil {
			return fmt.Errorf("witness start: %w", err)
		}
		// 临界区保持窗口：无锁实现下两进程必然重叠（汇合后同时进入）。
		time.Sleep(300 * time.Millisecond)
		return db.Exec("UPDATE migrate_witness SET finished_at = NOW(6) WHERE holder = ?", holder).Error
	}, store.Force())
	if err != nil {
		t.Fatalf("child %s migrate: %v", holder, err)
	}
}
