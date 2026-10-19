package tenantdb_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"sdlc-coordinator/internal/tenantdb"
)

func TestMySQLTenantConnectionBudgetCapsActiveManagedAndUnmanagedPools(t *testing.T) {
	ctx := context.Background()
	resolver, _, _ := newMySQLResolver(t)
	for _, tenantID := range []uint{1001, 1002, 1003, 1004} {
		if _, err := resolver.EnsureDatabase(ctx, tenantID); err != nil {
			t.Fatalf("ensure tenant %d: %v", tenantID, err)
		}
	}
	managedA, err := resolver.Handle(ctx, 1001)
	if err != nil {
		t.Fatalf("managed tenant handle: %v", err)
	}
	managedB, err := resolver.Handle(ctx, 1002)
	if err != nil {
		t.Fatalf("second managed tenant handle: %v", err)
	}
	unmanaged, err := resolver.OpenUnmanaged(1003)
	if err != nil {
		t.Fatalf("unmanaged tenant handle: %v", err)
	}
	t.Cleanup(func() { _ = unmanaged.Close() })
	first := tenantSQLDB(t, managedA)
	second := tenantSQLDB(t, managedB)
	third := tenantSQLDB(t, unmanaged)

	held := make([]*sql.Conn, 0, tenantdb.DefaultTenantTotalMaxOpenConns)
	for _, db := range []*sql.DB{first, second} {
		for i := 0; i < tenantdb.DefaultTenantMaxOpenConns; i++ {
			conn, err := db.Conn(ctx)
			if err != nil {
				t.Fatalf("hold tenant connection: %v", err)
			}
			held = append(held, conn)
		}
	}
	t.Cleanup(func() {
		for _, conn := range held {
			_ = conn.Close()
		}
	})
	opened := make(chan error, 1)
	go func() {
		_, err := resolver.Handle(ctx, 1004)
		opened <- err
	}()
	select {
	case err := <-opened:
		if err != nil {
			t.Fatalf("open tenant handle at active capacity: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("opening tenant handle blocked at active connection capacity")
	}

	waitCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	var value int
	err = third.QueryRowContext(waitCtx, "SELECT 1").Scan(&value)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("query with all %d slots held = %v, want deadline exceeded", len(held), err)
	}
	if err := held[0].Close(); err != nil {
		t.Fatalf("release one tenant connection: %v", err)
	}
	queryCtx, queryCancel := context.WithTimeout(ctx, time.Second)
	defer queryCancel()
	if err := third.QueryRowContext(queryCtx, "SELECT 1").Scan(&value); err != nil || value != 1 {
		t.Fatalf("unmanaged tenant query after one slot released: value=%d error=%v", value, err)
	}
}

func TestMySQLTenantConnectionBudgetReclaimsIdleAndCancelsWaiters(t *testing.T) {
	ctx := context.Background()
	resolver, _, _ := newMySQLResolver(t)
	for _, tenantID := range []uint{1004, 1005, 1006, 1007, 1008} {
		if _, err := resolver.EnsureDatabase(ctx, tenantID); err != nil {
			t.Fatalf("ensure tenant %d: %v", tenantID, err)
		}
	}
	managedA, err := resolver.Handle(ctx, 1004)
	if err != nil {
		t.Fatalf("managed tenant handle: %v", err)
	}
	managedB, err := resolver.Handle(ctx, 1005)
	if err != nil {
		t.Fatalf("second managed tenant handle: %v", err)
	}
	unmanagedA, err := resolver.OpenUnmanaged(1006)
	if err != nil {
		t.Fatalf("first unmanaged tenant handle: %v", err)
	}
	t.Cleanup(func() { _ = unmanagedA.Close() })
	unmanagedB, err := resolver.OpenUnmanaged(1007)
	if err != nil {
		t.Fatalf("second unmanaged tenant handle: %v", err)
	}
	t.Cleanup(func() { _ = unmanagedB.Close() })
	oldPools := []*sql.DB{
		tenantSQLDB(t, managedA), tenantSQLDB(t, managedB),
		tenantSQLDB(t, unmanagedA), tenantSQLDB(t, unmanagedB),
	}
	for _, db := range oldPools {
		conns := make([]*sql.Conn, 0, tenantdb.DefaultTenantMaxIdleConns)
		for i := 0; i < tenantdb.DefaultTenantMaxIdleConns; i++ {
			conn, err := db.Conn(ctx)
			if err != nil {
				t.Fatalf("prime idle tenant pool: %v", err)
			}
			conns = append(conns, conn)
		}
		for _, conn := range conns {
			if err := conn.Close(); err != nil {
				t.Fatalf("return idle tenant connection: %v", err)
			}
		}
	}
	if got := sumOpenConnections(oldPools); got != tenantdb.DefaultTenantTotalMaxOpenConns {
		t.Fatalf("idle connections before pressure = %d, want %d", got, tenantdb.DefaultTenantTotalMaxOpenConns)
	}

	newTenant, err := resolver.Handle(ctx, 1008)
	if err != nil {
		t.Fatalf("open new tenant handle at capacity: %v", err)
	}
	newDB := tenantSQLDB(t, newTenant)
	queryCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var value int
	if err := newDB.QueryRowContext(queryCtx, "SELECT 1").Scan(&value); err != nil || value != 1 {
		t.Fatalf("new tenant query after idle reclamation: value=%d error=%v", value, err)
	}
	if got := sumOpenConnections(oldPools); got >= tenantdb.DefaultTenantTotalMaxOpenConns {
		t.Fatalf("old tenant pools retained %d connections under pressure", got)
	}
}

func tenantSQLDB(t *testing.T, handle *tenantdb.Store) *sql.DB {
	t.Helper()
	db, err := handle.DB.DB()
	if err != nil {
		t.Fatalf("get tenant sql db: %v", err)
	}
	return db
}

func sumOpenConnections(pools []*sql.DB) int {
	total := 0
	for _, pool := range pools {
		total += pool.Stats().OpenConnections
	}
	return total
}
