package tenantdb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

const (
	DefaultTenantDatabasePrefix = "sdlc_tenant_"
	// 单租户库连接预算默认值：总量按平台库 + 控制面库 + 租户数 × 本值 ×
	// 协调器副本数估算，避免每租户无限长持大连接池。
	DefaultTenantMaxOpenConns = 8
	DefaultTenantMaxIdleConns = 4
	// DefaultTenantTotalMaxOpenConns 是单个 Coordinator 进程内全部租户池的
	// 物理连接总预算。部署样例最多两个 Coordinator，每进程预留 48 条非租户连接。
	DefaultTenantTotalMaxOpenConns = 16

	provisionLockPrefix = "sdlc:provision:"
	provisionLockWait   = 30
)

type MySQLDatabaseResolver struct {
	mu       sync.Mutex
	main     *Store
	admin    *gorm.DB
	spec     store.Spec
	pool     store.PoolSettings
	prefix   string
	capacity int
	items    map[uint]*Store
	order    []uint
	budget   *store.ConnectionBudget
}

// MySQLResolverOption 调整解析器形态（库名前缀用于同一 MySQL 服务上的
// 多环境隔离，如并行测试环境）。
type MySQLResolverOption func(*MySQLDatabaseResolver)

// WithDatabasePrefix 替换租户库名前缀（默认 sdlc_tenant_）。
func WithDatabasePrefix(prefix string) MySQLResolverOption {
	return func(resolver *MySQLDatabaseResolver) {
		if strings.TrimSpace(prefix) != "" {
			resolver.prefix = prefix
		}
	}
}

// NewMySQLDatabaseResolver 打开开通连接（平台 DSN 的库名保留，DDL 与库名
// 无关）；platformSpec 必须已解析为 MySQL Spec。
func NewMySQLDatabaseResolver(main *Store, platformSpec store.Spec, cacheSize int, opts ...MySQLResolverOption) (*MySQLDatabaseResolver, error) {
	if platformSpec.Driver != store.DriverMySQL {
		return nil, fmt.Errorf("mysql tenant resolver requires a mysql platform dsn")
	}
	if cacheSize <= 0 {
		cacheSize = DefaultCacheSize
	}
	admin, err := store.OpenSpec(platformSpec)
	if err != nil {
		return nil, fmt.Errorf("open tenant provisioning connection: %w", err)
	}
	budget, err := store.NewConnectionBudget(DefaultTenantTotalMaxOpenConns)
	if err != nil {
		closeStore(&Store{DB: admin})
		return nil, fmt.Errorf("create tenant mysql connection budget: %w", err)
	}
	resolver := &MySQLDatabaseResolver{
		main:     main,
		admin:    admin,
		spec:     platformSpec,
		pool:     store.PoolSettings{MaxOpen: DefaultTenantMaxOpenConns, MaxIdle: DefaultTenantMaxIdleConns, ConnMaxLifetime: 30 * time.Minute, ConnMaxIdleTime: 5 * time.Minute},
		prefix:   DefaultTenantDatabasePrefix,
		capacity: cacheSize,
		items:    make(map[uint]*Store),
		budget:   budget,
	}
	for _, opt := range opts {
		opt(resolver)
	}
	return resolver, nil
}

func (resolver *MySQLDatabaseResolver) Main() *Store {
	if resolver == nil {
		return nil
	}
	return resolver.main
}

// DatabaseName 返回租户 ID 对应的库名（确定性派生，白名单校验）。
func (resolver *MySQLDatabaseResolver) DatabaseName(tenantID uint) (string, error) {
	name := fmt.Sprintf("%s%d", resolver.prefix, tenantID)
	if err := store.ValidateDatabaseName(name); err != nil {
		return "", err
	}
	return name, nil
}

// Locator 返回规范 locator（mysql://<host>/<db>，不含凭据）。
func (resolver *MySQLDatabaseResolver) Locator(tenantID uint) string {
	name, err := resolver.DatabaseName(tenantID)
	if err != nil {
		return ""
	}
	return "mysql://" + resolver.spec.Host + "/" + name
}

// IsProvisioned 只认 mysql:// 前缀：空值与历史 sqlite 前缀都视为未开通，
// 由启动遍历/开通入口补开通后回写。
func (resolver *MySQLDatabaseResolver) IsProvisioned(locator string) bool {
	return strings.HasPrefix(strings.TrimSpace(locator), "mysql://")
}

// EnsureDatabase 幂等开通租户库：咨询锁串行的 CREATE DATABASE IF NOT EXISTS；
// 中断（建库成功但后续失败）后重试直接命中已存在的库继续，不产生第二个库。
func (resolver *MySQLDatabaseResolver) EnsureDatabase(ctx context.Context, tenantID uint) (string, error) {
	if tenantID == 0 {
		return "", ErrInvalidTenantID
	}
	name, err := resolver.DatabaseName(tenantID)
	if err != nil {
		return "", err
	}
	sqlDB, err := resolver.admin.DB()
	if err != nil {
		return "", fmt.Errorf("tenant provisioning handle: %w", err)
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return "", fmt.Errorf("tenant provisioning connection: %w", err)
	}
	defer conn.Close()

	// GET_LOCK 名上限 64 字符；库名前缀可配置，锁名用库名的 SHA-256 短串
	// 派生，长度恒定且空间上可区分。
	digest := sha256.Sum256([]byte(name))
	lockName := fmt.Sprintf("%s%x", provisionLockPrefix, digest[:12])
	var acquired sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", lockName, provisionLockWait).Scan(&acquired); err != nil {
		return "", fmt.Errorf("acquire provision lock %s: %w", lockName, err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return "", fmt.Errorf("provision lock %s not acquired within %ds", lockName, provisionLockWait)
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var released sql.NullInt64
		_ = conn.QueryRowContext(releaseCtx, "SELECT RELEASE_LOCK(?)", lockName).Scan(&released)
	}()

	ddl := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin", name)
	if _, err := conn.ExecContext(ctx, ddl); err != nil {
		return "", fmt.Errorf("create tenant database %s: %w", name, err)
	}
	return resolver.Locator(tenantID), nil
}

// DiscardFailedProvision 只失效句柄缓存：库保留给幂等重试接管（中断补偿
// 不依赖删除回滚），未提交的迁移在下次尝试重新收敛。
func (resolver *MySQLDatabaseResolver) DiscardFailedProvision(tenantID uint) {
	resolver.Invalidate(tenantID)
}

func (resolver *MySQLDatabaseResolver) Handle(ctx context.Context, tenantID uint) (*Store, error) {
	if tenantID == 0 {
		return nil, ErrInvalidTenantID
	}
	resolver.mu.Lock()
	if store, exists := resolver.items[tenantID]; exists {
		resolver.touchLocked(tenantID)
		resolver.mu.Unlock()
		return store, nil
	}
	var evicting []*Store
	for len(resolver.items) >= resolver.capacity {
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

	opened, err := resolver.open(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	resolver.mu.Lock()
	if existing, exists := resolver.items[tenantID]; exists {
		// 双检：并发首开同租户时保留先到句柄，关闭本调用多开的连接。
		resolver.touchLocked(tenantID)
		resolver.mu.Unlock()
		closeStore(opened)
		return existing, nil
	}
	resolver.items[tenantID] = opened
	resolver.order = append(resolver.order, tenantID)
	resolver.mu.Unlock()
	return opened, nil
}

// OpenUnmanaged 打开不参与 LRU 缓存与淘汰的独立租户句柄（引擎池长持）。
func (resolver *MySQLDatabaseResolver) OpenUnmanaged(tenantID uint) (*Store, error) {
	if tenantID == 0 {
		return nil, ErrInvalidTenantID
	}
	return resolver.open(context.Background(), tenantID)
}

func (resolver *MySQLDatabaseResolver) open(ctx context.Context, tenantID uint) (*Store, error) {
	name, err := resolver.DatabaseName(tenantID)
	if err != nil {
		return nil, err
	}
	rows, err := resolver.admin.WithContext(ctx).Raw(fmt.Sprintf("SHOW TABLES FROM `%s`", name)).Rows()
	if err != nil {
		return nil, fmt.Errorf("check tenant database %s: %w", name, err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close tenant database check %s: %w", name, err)
	}
	db, err := store.OpenSpec(resolver.spec.WithDatabase(name).WithPool(resolver.pool).WithConnectionBudget(resolver.budget))
	if err != nil {
		return nil, fmt.Errorf("open tenant database %s: %w", name, err)
	}
	return &Store{DB: db, budget: resolver.budget}, nil
}

func (resolver *MySQLDatabaseResolver) Invalidate(tenantID uint) {
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

func (resolver *MySQLDatabaseResolver) Len() int {
	if resolver == nil {
		return 0
	}
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	return len(resolver.items)
}

func (resolver *MySQLDatabaseResolver) Has(tenantID uint) bool {
	if resolver == nil {
		return false
	}
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	_, exists := resolver.items[tenantID]
	return exists
}

// Close 关闭缓存句柄与开通连接；引擎池独立句柄由 enginePool 自管。
func (resolver *MySQLDatabaseResolver) Close() {
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
	if resolver.admin != nil {
		closeStore(&Store{DB: resolver.admin})
	}
}

func (resolver *MySQLDatabaseResolver) popLocked(tenantID uint) (*Store, bool) {
	store, exists := resolver.items[tenantID]
	if exists {
		delete(resolver.items, tenantID)
	}
	return store, exists
}

func (resolver *MySQLDatabaseResolver) touchLocked(tenantID uint) {
	for i, id := range resolver.order {
		if id != tenantID {
			continue
		}
		resolver.order = append(append(resolver.order[:i], resolver.order[i+1:]...), tenantID)
		return
	}
	resolver.order = append(resolver.order, tenantID)
}
