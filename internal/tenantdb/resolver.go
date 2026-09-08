package tenantdb

import (
	"context"
	"sync"

	"gorm.io/gorm"
)

// SharedMainResolver 是租户句柄工厂：所有租户解析到同一主库，按 tenantID
// 做 LRU 缓存（失效/淘汰不关库，因为底层是共享主库）。
type SharedMainResolver struct {
	mu       sync.Mutex
	main     *Store
	capacity int
	items    map[uint]struct{}
	order    []uint
}

func NewSharedMainResolver(db *gorm.DB, cacheSize int) *SharedMainResolver {
	if cacheSize <= 0 {
		cacheSize = DefaultCacheSize
	}
	return &SharedMainResolver{
		main:     &Store{DB: db},
		capacity: cacheSize,
		items:    make(map[uint]struct{}),
	}
}

func (resolver *SharedMainResolver) Main() *Store {
	if resolver == nil {
		return nil
	}
	return resolver.main
}

func (resolver *SharedMainResolver) Handle(_ context.Context, tenantID uint) (*Store, error) {
	if tenantID == 0 {
		return nil, ErrInvalidTenantID
	}
	if resolver == nil || resolver.main == nil {
		return nil, ErrInvalidTenantID
	}
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if _, exists := resolver.items[tenantID]; exists {
		resolver.touchLocked(tenantID)
		return resolver.main, nil
	}
	for resolver.capacity > 0 && len(resolver.items) >= resolver.capacity {
		oldest := resolver.order[0]
		resolver.order = resolver.order[1:]
		delete(resolver.items, oldest)
	}
	resolver.items[tenantID] = struct{}{}
	resolver.order = append(resolver.order, tenantID)
	return resolver.main, nil
}

func (resolver *SharedMainResolver) Invalidate(tenantID uint) {
	if resolver == nil || tenantID == 0 {
		return
	}
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if _, exists := resolver.items[tenantID]; !exists {
		return
	}
	delete(resolver.items, tenantID)
	for i, id := range resolver.order {
		if id == tenantID {
			resolver.order = append(resolver.order[:i], resolver.order[i+1:]...)
			return
		}
	}
}

func (resolver *SharedMainResolver) Len() int {
	if resolver == nil {
		return 0
	}
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	return len(resolver.items)
}

func (resolver *SharedMainResolver) Has(tenantID uint) bool {
	if resolver == nil {
		return false
	}
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	_, exists := resolver.items[tenantID]
	return exists
}

func (resolver *SharedMainResolver) touchLocked(tenantID uint) {
	for i, id := range resolver.order {
		if id != tenantID {
			continue
		}
		resolver.order = append(append(resolver.order[:i], resolver.order[i+1:]...), tenantID)
		return
	}
	resolver.order = append(resolver.order, tenantID)
}
