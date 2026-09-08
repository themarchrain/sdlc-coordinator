package tenantdb

import (
	"context"
	"path/filepath"
	"testing"

	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

func TestSharedMainResolverReturnsSameStoreForEveryTenant(t *testing.T) {
	resolver := newTestResolver(t, 8)
	first, err := resolver.Handle(context.Background(), 1)
	if err != nil {
		t.Fatalf("handle 1: %v", err)
	}
	second, err := resolver.Handle(context.Background(), 2)
	if err != nil {
		t.Fatalf("handle 2: %v", err)
	}
	if first == nil || first.DB == nil || first.DB != second.DB || first.DB != resolver.Main().DB {
		t.Fatalf("shared-main resolver must return the shared main store, got %#v vs %#v", first, second)
	}
}

func TestSharedMainResolverRejectsZeroTenantID(t *testing.T) {
	resolver := newTestResolver(t, 8)
	if _, err := resolver.Handle(context.Background(), 0); err == nil {
		t.Fatal("expected error for tenant id 0")
	}
}

func TestSharedMainResolverEvictsLeastRecentlyUsed(t *testing.T) {
	resolver := newTestResolver(t, 2)
	if _, err := resolver.Handle(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Handle(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	if resolver.Len() != 2 {
		t.Fatalf("len=%d want 2", resolver.Len())
	}
	if _, err := resolver.Handle(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	if resolver.Has(1) {
		t.Fatal("tenant 1 should have been evicted")
	}
	if !resolver.Has(2) || !resolver.Has(3) {
		t.Fatalf("expected tenants 2 and 3 cached, len=%d", resolver.Len())
	}
	if _, err := resolver.Handle(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if resolver.Has(2) {
		t.Fatal("tenant 2 should have been evicted after reinserting 1")
	}
}

func TestSharedMainResolverInvalidateDropsCacheEntry(t *testing.T) {
	resolver := newTestResolver(t, 8)
	if _, err := resolver.Handle(context.Background(), 9); err != nil {
		t.Fatal(err)
	}
	resolver.Invalidate(9)
	if resolver.Has(9) {
		t.Fatal("invalidated tenant should not stay cached")
	}
	if _, err := resolver.Handle(context.Background(), 9); err != nil {
		t.Fatal(err)
	}
	if !resolver.Has(9) {
		t.Fatal("handle after invalidate should recache")
	}
}

func newTestResolver(t *testing.T, capacity int) *SharedMainResolver {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "tenant.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
	return NewSharedMainResolver(db, capacity)
}

func TestStoreWrapsGormDB(t *testing.T) {
	var db *gorm.DB
	storeHandle := &Store{DB: db}
	if storeHandle.DB != db {
		t.Fatal("Store must expose the tenant-scoped gorm handle")
	}
}
