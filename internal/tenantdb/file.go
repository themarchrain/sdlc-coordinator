package tenantdb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"sdlc-coordinator/internal/store"
)

// SqliteFileResolver 是真分库句柄工厂：DSN=
// file:<tenants 根>/<tenantID>/db.sqlite。与 SharedMainResolver 的差异点：
// 分库句柄持有文件，LRU 淘汰与 Invalidate 必须 Close 释放文件句柄；
// Main() 仍返回平台库句柄（无组织作用域的请求与平台配置链使用）。
type SqliteFileResolver struct {
	mu       sync.Mutex
	main     *Store
	root     string
	capacity int
	items    map[uint]*Store
	order    []uint
}

func NewSqliteFileResolver(main *Store, tenantsRoot string, cacheSize int) *SqliteFileResolver {
	if cacheSize <= 0 {
		cacheSize = DefaultCacheSize
	}
	return &SqliteFileResolver{
		main:     main,
		root:     strings.TrimSpace(tenantsRoot),
		capacity: cacheSize,
		items:    make(map[uint]*Store),
	}
}

func (resolver *SqliteFileResolver) Main() *Store {
	if resolver == nil {
		return nil
	}
	return resolver.main
}

func (resolver *SqliteFileResolver) Handle(_ context.Context, tenantID uint) (*Store, error) {
	if tenantID == 0 {
		return nil, ErrInvalidTenantID
	}
	if resolver == nil || resolver.root == "" {
		return nil, fmt.Errorf("tenant data dir is not configured: %w", ErrInvalidTenantID)
	}
	resolver.mu.Lock()
	if store, exists := resolver.items[tenantID]; exists {
		resolver.touchLocked(tenantID)
		resolver.mu.Unlock()
		return store, nil
	}
	// 先在锁内完成淘汰记账，解锁后再 Close 被淘汰句柄，避免持锁做 IO。
	var evicting []*Store
	for resolver.lenLocked() >= resolver.capacity {
		oldest := resolver.order[0]
		resolver.order = resolver.order[1:]
		if store, ok := resolver.popLocked(oldest); ok {
			evicting = append(evicting, store)
		}
	}
	resolver.mu.Unlock()
	defer func() {
		for _, store := range evicting {
			closeStore(store)
		}
	}()

	dir := filepath.Join(resolver.root, strconv.FormatUint(uint64(tenantID), 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create tenant data dir: %w", err)
	}
	db, err := store.Open("file:" + filepath.ToSlash(filepath.Join(dir, "db.sqlite")) + "?_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open tenant database: %w", err)
	}
	store := &Store{DB: db}
	resolver.mu.Lock()
	// 双检：并发首开同租户时可能已由另一调用方放入缓存——保留先到句柄，
	// 关闭本调用多开的连接，避免文件句柄泄漏。
	if existing, exists := resolver.items[tenantID]; exists {
		resolver.touchLocked(tenantID)
		resolver.mu.Unlock()
		closeStore(store)
		return existing, nil
	}
	resolver.items[tenantID] = store
	resolver.order = append(resolver.order, tenantID)
	resolver.mu.Unlock()
	return store, nil
}

// OpenUnmanaged 打开一个不参与 LRU 缓存与淘汰的独立租户句柄：生命周期
// 归调用方（引擎池长持句柄专用——缓存通道的淘汰/Invalidate/Close 会
// 截断长持者的连接，独立句柄不受影响；调用方自行管理关闭，随进程退出
// 由 OS 释放亦安全）。目录与 DSN 语义与 Handle 一致。
func (resolver *SqliteFileResolver) OpenUnmanaged(tenantID uint) (*Store, error) {
	if tenantID == 0 {
		return nil, ErrInvalidTenantID
	}
	if resolver == nil || resolver.root == "" {
		return nil, fmt.Errorf("tenant data dir is not configured: %w", ErrInvalidTenantID)
	}
	dir := filepath.Join(resolver.root, strconv.FormatUint(uint64(tenantID), 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create tenant data dir: %w", err)
	}
	db, err := store.Open("file:" + filepath.ToSlash(filepath.Join(dir, "db.sqlite")) + "?_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open tenant database: %w", err)
	}
	return &Store{DB: db}, nil
}

func (resolver *SqliteFileResolver) Invalidate(tenantID uint) {
	if resolver == nil || tenantID == 0 {
		return
	}
	resolver.mu.Lock()
	store, ok := resolver.popLocked(tenantID)
	if ok {
		for i, id := range resolver.order {
			if id == tenantID {
				resolver.order = append(resolver.order[:i], resolver.order[i+1:]...)
				break
			}
		}
	}
	resolver.mu.Unlock()
	if ok {
		closeStore(store)
	}
}

func (resolver *SqliteFileResolver) Len() int {
	if resolver == nil {
		return 0
	}
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	return resolver.lenLocked()
}

func (resolver *SqliteFileResolver) Has(tenantID uint) bool {
	if resolver == nil {
		return false
	}
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	_, exists := resolver.items[tenantID]
	return exists
}

// Close 关闭全部缓存的租户句柄并清空缓存（进程优雅退出用；平台库 Main
// 归属装配层，不在本方法范围内）。
func (resolver *SqliteFileResolver) Close() {
	if resolver == nil {
		return
	}
	resolver.mu.Lock()
	stores := make([]*Store, 0, len(resolver.items))
	for tenantID, store := range resolver.items {
		stores = append(stores, store)
		delete(resolver.items, tenantID)
	}
	resolver.order = nil
	resolver.mu.Unlock()
	for _, store := range stores {
		closeStore(store)
	}
}

func (resolver *SqliteFileResolver) lenLocked() int {
	return len(resolver.items)
}

func (resolver *SqliteFileResolver) popLocked(tenantID uint) (*Store, bool) {
	store, exists := resolver.items[tenantID]
	if exists {
		delete(resolver.items, tenantID)
	}
	return store, exists
}

func (resolver *SqliteFileResolver) touchLocked(tenantID uint) {
	for i, id := range resolver.order {
		if id != tenantID {
			continue
		}
		resolver.order = append(append(resolver.order[:i], resolver.order[i+1:]...), tenantID)
		return
	}
	resolver.order = append(resolver.order, tenantID)
}

func closeStore(store *Store) {
	if store == nil || store.DB == nil {
		return
	}
	if sqlDB, err := store.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
}
