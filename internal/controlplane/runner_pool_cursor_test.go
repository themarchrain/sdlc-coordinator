package controlplane

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func assertRunnerPoolSlots(t *testing.T, slots []uint64) {
	t.Helper()
	seen := make(map[uint64]bool, len(slots))
	for _, slot := range slots {
		if seen[slot] {
			t.Fatalf("runner pool slot %d was claimed more than once", slot)
		}
		seen[slot] = true
	}
	for slot := uint64(0); slot < uint64(len(slots)); slot++ {
		if !seen[slot] {
			t.Fatalf("runner pool slot %d was not claimed", slot)
		}
	}
}

func TestServiceClaimRunnerPoolConcurrentSQLite(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "control-plane.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	repository := NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatal(err)
	}
	service := NewService(repository)

	const claims = 32
	start := make(chan struct{})
	slots := make([]uint64, claims)
	errs := make([]error, claims)
	var wg sync.WaitGroup
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, slots[i], errs[i] = service.ClaimRunnerPool(context.Background(), 7)
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("claim %d: %v", i, err)
		}
	}
	assertRunnerPoolSlots(t, slots)

	_, slot, err := service.ClaimRunnerPool(context.Background(), 0)
	if err != nil || slot != 0 {
		t.Fatalf("tenant 0 first slot = %d, error = %v; want 0, nil", slot, err)
	}
}

func TestServiceClaimRunnerPoolReturnsOrderedSnapshotSQLite(t *testing.T) {
	repository := openTestControlPlane(t)
	if err := repository.Migrate(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := repository.ReplaceRunnerAssignments(ctx, 7, []string{"http://runner-b", "http://runner-a"}); err != nil {
		t.Fatal(err)
	}
	service := NewService(repository)

	assignments, slot, err := service.ClaimRunnerPool(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if slot != 0 {
		t.Fatalf("first claimed slot = %d, want 0", slot)
	}
	got := make([]string, len(assignments))
	for i := range assignments {
		got[i] = assignments[i].EndpointURL
	}
	want := []string{"http://runner-a", "http://runner-b"}
	if !equalStrings(got, want) {
		t.Fatalf("claimed runner snapshot = %v, want %v", got, want)
	}
	if _, slot, err := service.ClaimRunnerPool(ctx, 7); err != nil || slot != 1 {
		t.Fatalf("second claimed slot = %d, error = %v; want 1, nil", slot, err)
	}
}

func TestServiceClaimRunnerPoolConcurrentMySQL(t *testing.T) {
	adminDSN := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, adminDSN, "cp_runner_pool_cursor")
	services := make([]*Service, 2)
	for i := range services {
		db, err := store.Open(dsn)
		if err != nil {
			t.Fatalf("open connection %d: %v", i, err)
		}
		t.Cleanup(func() {
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
		})
		services[i] = NewService(NewRepository(db))
	}
	if err := services[0].repository.Migrate(); err != nil {
		t.Fatal(err)
	}

	const claims = 40
	start := make(chan struct{})
	slots := make([]uint64, claims)
	errs := make([]error, claims)
	var wg sync.WaitGroup
	for i := range slots {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			service := services[i%len(services)]
			_, slots[i], errs[i] = service.ClaimRunnerPool(context.Background(), 7)
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("claim %d: %v", i, err)
		}
	}
	assertRunnerPoolSlots(t, slots)

	_, slot, err := services[0].ClaimRunnerPool(context.Background(), 0)
	if err != nil || slot != 0 {
		t.Fatalf("tenant 0 first slot = %d, error = %v; want 0, nil", slot, err)
	}
	if _, slot, err = services[1].ClaimRunnerPool(context.Background(), 0); err != nil || slot != 1 {
		t.Fatalf("tenant 0 second slot = %d, error = %v; want 1, nil", slot, err)
	}
}

func TestClaimRunnerPoolSerializesWithReplaceOnMySQL(t *testing.T) {
	adminDSN := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, adminDSN, "cp_runner_pool_replace")
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	repository := NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := repository.CreateTenant(ctx, &Tenant{ID: 7, Slug: "runner-pool", DisplayName: "Runner Pool"}); err != nil {
		t.Fatal(err)
	}
	service := NewService(repository)
	if err := service.ReplaceRunnerAssignments(ctx, nil, 7, []string{"http://runner-a", "http://runner-b"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.ClaimRunnerPool(ctx, 7); err != nil {
		t.Fatal(err)
	}

	holder := db.Begin()
	if holder.Error != nil {
		t.Fatal(holder.Error)
	}
	defer func() { _ = holder.Rollback().Error }()
	var cursor RunnerPoolCursor
	if err := holder.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ?", 7).Take(&cursor).Error; err != nil {
		_ = holder.Rollback().Error
		t.Fatalf("hold runner pool cursor: %v", err)
	}

	type claimResult struct {
		assignments []RunnerAssignment
		slot        uint64
		err         error
	}
	claimed := make(chan claimResult, 1)
	claimStarted := make(chan struct{})
	go func() {
		close(claimStarted)
		assignments, slot, err := service.ClaimRunnerPool(ctx, 7)
		claimed <- claimResult{assignments: assignments, slot: slot, err: err}
	}()
	<-claimStarted
	waitForRunnerPoolLockWaits(t, db, 1)

	replaced := make(chan error, 1)
	replaceStarted := make(chan struct{})
	go func() {
		close(replaceStarted)
		replaced <- service.ReplaceRunnerAssignments(ctx, nil, 7, []string{"http://runner-c"})
	}()
	<-replaceStarted
	waitForRunnerPoolLockWaits(t, db, 2)
	if err := holder.Commit().Error; err != nil {
		t.Fatal(err)
	}

	claim := <-claimed
	if claim.err != nil {
		t.Fatalf("claim pool: %v", claim.err)
	}
	if claim.slot != 1 || len(claim.assignments) != 2 {
		t.Fatalf("claim before replace = slot %d, assignments %#v; want slot 1 and old two-member snapshot", claim.slot, claim.assignments)
	}
	if err := <-replaced; err != nil {
		t.Fatalf("replace assignments: %v", err)
	}
	assignments, _, err := service.ClaimRunnerPool(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 1 || assignments[0].EndpointURL != "http://runner-c" {
		t.Fatalf("claim after replace = %#v, want only runner-c", assignments)
	}
}

func waitForRunnerPoolLockWaits(t *testing.T, db *gorm.DB, want int64) {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting int64
		if err := db.Raw(`SELECT COUNT(*)
FROM performance_schema.data_lock_waits w
JOIN performance_schema.data_locks l
  ON l.ENGINE = w.ENGINE AND l.ENGINE_LOCK_ID = w.REQUESTING_ENGINE_LOCK_ID
WHERE l.OBJECT_SCHEMA = DATABASE() AND l.OBJECT_NAME = 'runner_pool_cursors'`).Scan(&waiting).Error; err != nil {
			t.Fatalf("read MySQL runner pool lock waits: %v", err)
		}
		if waiting >= want {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("observed %d runner pool lock waiters, want at least %d", waiting, want)
		case <-ticker.C:
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
