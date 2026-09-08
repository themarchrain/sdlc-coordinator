package tenantdb

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"sdlc-coordinator/internal/store"
)

// sqlite 一租户一文件。与 SharedMainResolver 的差异点：
// 分库句柄持有文件，淘汰/失效必须 Close，否则文件句柄泄漏。
func TestSqliteFileResolverOpensIsolatedPerTenantFiles(t *testing.T) {
	root := t.TempDir()
	resolver := NewSqliteFileResolver(nil, root, DefaultCacheSize)
	t.Cleanup(resolver.Close)

	tenantStore, err := resolver.Handle(context.Background(), 7)
	if err != nil {
		t.Fatalf("open tenant 7: %v", err)
	}
	if err := tenantStore.DB.Exec("CREATE TABLE marker_t7 (id INTEGER)").Error; err != nil {
		t.Fatalf("seed tenant 7: %v", err)
	}
	other, err := resolver.Handle(context.Background(), 8)
	if err != nil {
		t.Fatalf("open tenant 8: %v", err)
	}
	if other.DB.Migrator().HasTable("marker_t7") {
		t.Fatalf("tenant 8 must not see tenant 7 tables")
	}
	if _, err := resolver.Handle(context.Background(), 0); err == nil {
		t.Fatalf("tenant 0 must be rejected")
	}
	// 句柄按租户缓存：同租户重复解析返回同一句柄。
	again, err := resolver.Handle(context.Background(), 7)
	if err != nil || again != tenantStore {
		t.Fatalf("expected cached handle for tenant 7, err=%v same=%v", err, again == tenantStore)
	}
	for _, tenantDir := range []string{"7", "8"} {
		if _, err := os.Stat(filepath.Join(root, tenantDir, "db.sqlite")); err != nil {
			t.Fatalf("tenant %s sqlite file must exist: %v", tenantDir, err)
		}
	}
}

func TestSqliteFileResolverClosesHandleOnEviction(t *testing.T) {
	resolver := NewSqliteFileResolver(nil, t.TempDir(), 1)
	t.Cleanup(resolver.Close)

	evicted, err := resolver.Handle(context.Background(), 7)
	if err != nil {
		t.Fatalf("open tenant 7: %v", err)
	}
	if _, err := resolver.Handle(context.Background(), 8); err != nil {
		t.Fatalf("open tenant 8: %v", err)
	}
	if resolver.Has(7) {
		t.Fatalf("tenant 7 handle should have been evicted (capacity 1)")
	}
	if err := evicted.DB.Exec("SELECT 1").Error; err == nil {
		t.Fatalf("evicted handle must be closed to release the file")
	}
	reopened, err := resolver.Handle(context.Background(), 7)
	if err != nil {
		t.Fatalf("reopen tenant 7: %v", err)
	}
	if err := reopened.DB.Exec("SELECT 1").Error; err != nil {
		t.Fatalf("reopened handle must be usable: %v", err)
	}
}

func TestSqliteFileResolverInvalidateClosesHandle(t *testing.T) {
	resolver := NewSqliteFileResolver(nil, t.TempDir(), DefaultCacheSize)
	t.Cleanup(resolver.Close)
	handle, err := resolver.Handle(context.Background(), 7)
	if err != nil {
		t.Fatalf("open tenant 7: %v", err)
	}
	resolver.Invalidate(7)
	if err := handle.DB.Exec("SELECT 1").Error; err == nil {
		t.Fatalf("invalidated handle must be closed")
	}
}

func TestSqliteFileResolverExposesPlatformMainStore(t *testing.T) {
	db, err := store.Open("file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "main.db")))
	if err != nil {
		t.Fatalf("open main: %v", err)
	}
	main := &Store{DB: db}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	resolver := NewSqliteFileResolver(main, t.TempDir(), DefaultCacheSize)
	t.Cleanup(resolver.Close)
	if resolver.Main() != main {
		t.Fatalf("Main must return the platform store")
	}
}
