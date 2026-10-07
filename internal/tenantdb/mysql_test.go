package tenantdb_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"
	"sdlc-coordinator/internal/tenantdb"
)

func newMySQLResolver(t *testing.T) (*tenantdb.MySQLDatabaseResolver, *store.Spec, string) {
	t.Helper()
	admin := storetest.RequireMySQL(t)
	platformDSN := storetest.NewDatabase(t, admin, "resolver_platform")
	spec, err := store.ParseDSN(platformDSN)
	if err != nil {
		t.Fatalf("parse platform dsn: %v", err)
	}
	platformDB, err := store.OpenSpec(spec)
	if err != nil {
		t.Fatalf("open platform db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := platformDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	// 租户库名按租户 ID 确定性派生，跨测试同名会互相干扰：按测试名分配独立
	// 前缀并在清理阶段删除残留库。
	prefix := storetest.IdentifierPrefix(t, "sdlc_it_res_")
	resolver, err := tenantdb.NewMySQLDatabaseResolver(&tenantdb.Store{DB: platformDB}, spec, 0, tenantdb.WithDatabasePrefix(prefix))
	if err != nil {
		t.Fatalf("new mysql resolver: %v", err)
	}
	t.Cleanup(func() {
		resolver.Close()
		storetest.DropDatabasesWithPrefix(t, admin, prefix)
	})
	return resolver, &spec, prefix
}

// TestMySQLResolverProvisionIsolateAndShare 锁定：开通幂等；同租户两实例句柄
// 读同一库；不同租户库互不可见。
func TestMySQLResolverProvisionIsolateAndShare(t *testing.T) {
	ctx := context.Background()
	resolver, _, _ := newMySQLResolver(t)

	locatorA, err := resolver.EnsureDatabase(ctx, 1)
	if err != nil {
		t.Fatalf("ensure tenant 1: %v", err)
	}
	if !resolver.IsProvisioned(locatorA) || !strings.HasPrefix(locatorA, "mysql://") {
		t.Fatalf("locator = %q, want provisioned mysql:// locator", locatorA)
	}
	if locatorAgain, err := resolver.EnsureDatabase(ctx, 1); err != nil || locatorAgain != locatorA {
		t.Fatalf("ensure idempotent: %q err=%v", locatorAgain, err)
	}
	if _, err := resolver.EnsureDatabase(ctx, 2); err != nil {
		t.Fatalf("ensure tenant 2: %v", err)
	}

	handleA, err := resolver.Handle(ctx, 1)
	if err != nil {
		t.Fatalf("handle tenant 1: %v", err)
	}
	handleB, err := resolver.Handle(ctx, 2)
	if err != nil {
		t.Fatalf("handle tenant 2: %v", err)
	}
	if handleA.DB == handleB.DB {
		t.Fatal("two tenants resolved to the same handle")
	}
	ddl := "CREATE TABLE isolation_probe(id int primary key, business_key varchar(64) not null)"
	if err := handleA.DB.Exec(ddl).Error; err != nil {
		t.Fatalf("create probe on tenant 1: %v", err)
	}
	if handleB.DB.Migrator().HasTable("isolation_probe") {
		t.Fatal("tenant 2 sees tenant 1 tables (isolation broken)")
	}
	if !handleA.DB.Migrator().HasTable("isolation_probe") {
		t.Fatal("tenant 1 lost its own table")
	}
}

// TestMySQLResolverTwoInstancesShareTenantData 模拟两个 Coordinator 实例各自的
// 解析器：同租户读写同一库。
func TestMySQLResolverTwoInstancesShareTenantData(t *testing.T) {
	ctx := context.Background()
	resolverOne, spec, prefix := newMySQLResolver(t)
	if _, err := resolverOne.EnsureDatabase(ctx, 7); err != nil {
		t.Fatalf("ensure tenant: %v", err)
	}
	handleOne, err := resolverOne.Handle(ctx, 7)
	if err != nil {
		t.Fatalf("instance one handle: %v", err)
	}
	if err := handleOne.DB.Exec("CREATE TABLE shared_probe(id int primary key, value varchar(32) not null)").Error; err != nil {
		t.Fatalf("create shared probe: %v", err)
	}
	if err := handleOne.DB.Exec("INSERT INTO shared_probe(id, value) VALUES (1, 'from-one')").Error; err != nil {
		t.Fatalf("write from instance one: %v", err)
	}

	platformDB, err := store.OpenSpec(*spec)
	if err != nil {
		t.Fatalf("open second platform handle: %v", err)
	}
	defer func() {
		if sqlDB, err := platformDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	resolverTwo, err := tenantdb.NewMySQLDatabaseResolver(&tenantdb.Store{DB: platformDB}, *spec, 0, tenantdb.WithDatabasePrefix(prefix))
	if err != nil {
		t.Fatalf("second resolver: %v", err)
	}
	defer resolverTwo.Close()
	handleTwo, err := resolverTwo.Handle(ctx, 7)
	if err != nil {
		t.Fatalf("instance two handle: %v", err)
	}
	var value string
	if err := handleTwo.DB.Raw("SELECT value FROM shared_probe WHERE id = 1").Scan(&value).Error; err != nil {
		t.Fatalf("read from instance two: %v", err)
	}
	if value != "from-one" {
		t.Fatalf("instance two read %q, want from-one", value)
	}
}

// TestMySQLEnsureDatabaseConcurrent 两个并发开通请求（不同句柄）只产生一个
// 可用库且都成功返回（咨询锁串行；库名确定性派生不存在第二份）。
func TestMySQLEnsureDatabaseConcurrent(t *testing.T) {
	ctx := context.Background()
	resolver, spec, prefix := newMySQLResolver(t)

	platformDB, err := store.OpenSpec(*spec)
	if err != nil {
		t.Fatalf("open second platform handle: %v", err)
	}
	defer func() {
		if sqlDB, err := platformDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	resolverTwo, err := tenantdb.NewMySQLDatabaseResolver(&tenantdb.Store{DB: platformDB}, *spec, 0, tenantdb.WithDatabasePrefix(prefix))
	if err != nil {
		t.Fatalf("second resolver: %v", err)
	}
	defer resolverTwo.Close()

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, r := range []*tenantdb.MySQLDatabaseResolver{resolver, resolverTwo} {
		wg.Add(1)
		go func(r *tenantdb.MySQLDatabaseResolver) {
			defer wg.Done()
			<-start
			_, err := r.EnsureDatabase(ctx, 11)
			errs <- err
		}(r)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent ensure: %v", err)
		}
	}

	handle, err := resolver.Handle(ctx, 11)
	if err != nil {
		t.Fatalf("open concurrently provisioned tenant: %v", err)
	}
	if err := handle.DB.Exec("CREATE TABLE concurrent_probe(id int primary key)").Error; err != nil {
		t.Fatalf("use concurrently provisioned tenant: %v", err)
	}
}

// TestMySQLResolverHandleUnprovisioned 未开通租户的句柄解析按驱动码分类为
// 库不存在，供启动遍历区分补偿与故障。
func TestMySQLResolverHandleUnprovisioned(t *testing.T) {
	ctx := context.Background()
	resolver, _, _ := newMySQLResolver(t)
	if _, err := resolver.Handle(ctx, 999); err == nil {
		t.Fatal("handle on unprovisioned tenant succeeded")
	} else if !store.IsUnknownDatabase(err) {
		t.Fatalf("unprovisioned handle error not classified: %v", err)
	}
	if _, err := resolver.OpenUnmanaged(999); err == nil {
		t.Fatal("unmanaged handle on unprovisioned tenant succeeded")
	}
	// DiscardFailedProvision 只失效缓存，不删库：随后 EnsureDatabase 仍成功。
	if locator, err := resolver.EnsureDatabase(ctx, 999); err != nil {
		t.Fatalf("ensure after discard: %v", err)
	} else if !resolver.IsProvisioned(locator) {
		t.Fatalf("locator not provisioned: %q", locator)
	}
	resolver.DiscardFailedProvision(999)
	if resolver.Has(999) {
		t.Fatal("discard did not invalidate cached handle")
	}
	if _, err := resolver.Handle(ctx, 999); err != nil {
		t.Fatalf("handle after discard: %v", err)
	}
}

// TestMySQLResolverIsProvisionedSemantics 锁定就绪判定：空值与历史 sqlite
// 前缀视为未开通（需补开通），mysql:// 前缀视为已开通。
func TestMySQLResolverIsProvisionedSemantics(t *testing.T) {
	resolver, _, _ := newMySQLResolver(t)
	if resolver.IsProvisioned("") {
		t.Fatal("empty locator must be treated as unprovisioned")
	}
	if resolver.IsProvisioned("sqlite://data/tenants/acme/db.sqlite") {
		t.Fatal("legacy sqlite locator must be treated as unprovisioned on mysql deployments")
	}
	if !resolver.IsProvisioned(resolver.Locator(3)) {
		t.Fatalf("own locator %q must be provisioned", resolver.Locator(3))
	}
}
