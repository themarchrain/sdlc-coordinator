package app

import (
	"context"
	"strings"
	"testing"

	"sdlc-coordinator/internal/config"
	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"
	"sdlc-coordinator/internal/tenantdb"

	"go.uber.org/zap"
)

// mysqlTenantWorld 搭一套控制面 + 平台库 + MySQL 租户解析器的最小装配，
// 租户库名按测试前缀隔离并在清理阶段删除。
func mysqlTenantWorld(t *testing.T) (*controlplane.Service, *tenantdb.MySQLDatabaseResolver, string) {
	t.Helper()
	admin := storetest.RequireMySQL(t)
	controlDSN := storetest.NewDatabase(t, admin, "app_control")
	control, err := newControlPlane(config.ControlPlaneSettings{
		DSN: controlDSN, SuperAdminUsername: "seed-admin", SuperAdminPassword: "seed-password-1",
	}, nil)
	if err != nil {
		t.Fatalf("control plane: %v", err)
	}
	platformDSN := storetest.NewDatabase(t, admin, "app_platform")
	platformSpec, err := store.ParseDSN(platformDSN)
	if err != nil {
		t.Fatalf("parse platform dsn: %v", err)
	}
	platformDB, err := store.OpenSpec(platformSpec)
	if err != nil {
		t.Fatalf("open platform db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := platformDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	prefix := storetest.IdentifierPrefix(t, "sdlc_it_app_")
	resolver, err := tenantdb.NewMySQLDatabaseResolver(&tenantdb.Store{DB: platformDB}, platformSpec, 0, tenantdb.WithDatabasePrefix(prefix))
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}
	t.Cleanup(func() {
		resolver.Close()
		storetest.DropDatabasesWithPrefix(t, admin, prefix)
	})
	return control, resolver, prefix
}

// TestCreateTenantProvisionsMySQLDataPlane 建组织同事务开通数据面：locator
// 回写为 mysql://，业务表已建，重复开通幂等。
func TestCreateTenantProvisionsMySQLDataPlane(t *testing.T) {
	control, resolver, _ := mysqlTenantWorld(t)
	control.SetTenantProvisioner(&tenantStoreProvisioner{resolver: resolver})
	ctx := context.Background()

	tenant, err := control.CreateTenant(ctx, nil, controlplane.TenantInput{Slug: "acme", DisplayName: "Acme"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if !resolver.IsProvisioned(tenant.DMLocator) {
		t.Fatalf("locator after provisioning = %q, want mysql:// locator", tenant.DMLocator)
	}
	handle, err := resolver.Handle(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("open provisioned tenant: %v", err)
	}
	for _, table := range []string{"tasks", "runs", "effects", "people", "registered_repositories"} {
		if !handle.DB.Migrator().HasTable(table) {
			t.Fatalf("tenant business table %s missing after provisioning", table)
		}
	}

	second, err := control.CreateTenant(ctx, nil, controlplane.TenantInput{Slug: "beta", DisplayName: "Beta"})
	if err != nil {
		t.Fatalf("create second tenant: %v", err)
	}
	if second.DMLocator == tenant.DMLocator {
		t.Fatal("two tenants share one database")
	}
	secondHandle, err := resolver.Handle(ctx, second.ID)
	if err != nil {
		t.Fatalf("open second tenant: %v", err)
	}
	if err := secondHandle.DB.Exec("CREATE TABLE beta_probe(id int primary key)").Error; err != nil {
		t.Fatalf("second tenant not writable: %v", err)
	}
	if handle.DB.Migrator().HasTable("beta_probe") {
		t.Fatal("tenant databases are not isolated")
	}
}

// TestSweepProvisionsUnprovisionedTenant 开通中断补偿：租户行存在但 locator
// 未就绪（占位/空）时，启动遍历补开通并回写；重复遍历幂等。
func TestSweepProvisionsUnprovisionedTenant(t *testing.T) {
	control, resolver, _ := mysqlTenantWorld(t)
	ctx := context.Background()

	// 无 provisioner 建组织：行落库、locator 为占位串，数据面未开通。
	tenant, err := control.CreateTenant(ctx, nil, controlplane.TenantInput{Slug: "acme", DisplayName: "Acme"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if resolver.IsProvisioned(tenant.DMLocator) {
		t.Fatalf("placeholder locator %q read as provisioned", tenant.DMLocator)
	}
	if _, err := resolver.Handle(ctx, tenant.ID); err == nil {
		t.Fatal("handle succeeded before provisioning")
	}

	if err := migrateTenantStores(ctx, control, resolver); err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	summaries, err := control.ListTenants(ctx)
	if err != nil || len(summaries) != 1 {
		t.Fatalf("list tenants: %v (%d)", err, len(summaries))
	}
	if !resolver.IsProvisioned(summaries[0].DMLocator) {
		t.Fatalf("locator after sweep = %q, want provisioned", summaries[0].DMLocator)
	}
	handle, err := resolver.Handle(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("open after sweep: %v", err)
	}
	if !handle.DB.Migrator().HasTable("tasks") {
		t.Fatal("business tables missing after sweep provisioning")
	}

	if err := migrateTenantStores(ctx, control, resolver); err != nil {
		t.Fatalf("second sweep must be idempotent: %v", err)
	}
}

// TestSweepFailsLoudlyWhenProvisionedDatabaseMissing 已标记开通但库被删除：
// 不静默重建，启动失败并给出可判断原因（人工处置）。
func TestSweepFailsLoudlyWhenProvisionedDatabaseMissing(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	control, resolver, _ := mysqlTenantWorld(t)
	control.SetTenantProvisioner(&tenantStoreProvisioner{resolver: resolver})
	ctx := context.Background()

	tenant, err := control.CreateTenant(ctx, nil, controlplane.TenantInput{Slug: "acme", DisplayName: "Acme"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	name, err := resolver.DatabaseName(tenant.ID)
	if err != nil {
		t.Fatalf("database name: %v", err)
	}
	adminDB, err := store.Open(admin)
	if err != nil {
		t.Fatalf("open admin: %v", err)
	}
	defer func() {
		if sqlDB, err := adminDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	disabled, err := control.CreateTenant(ctx, nil, controlplane.TenantInput{Slug: "disabled", DisplayName: "Disabled"})
	if err != nil {
		t.Fatalf("create disabled tenant: %v", err)
	}
	if _, err := control.DisableTenant(ctx, nil, disabled.ID); err != nil {
		t.Fatalf("disable tenant: %v", err)
	}
	disabledName, err := resolver.DatabaseName(disabled.ID)
	if err != nil {
		t.Fatalf("disabled database name: %v", err)
	}
	if err := adminDB.Exec("DROP DATABASE `" + disabledName + "`").Error; err != nil {
		t.Fatalf("drop disabled tenant database: %v", err)
	}
	resolver.Invalidate(disabled.ID)
	if err := migrateTenantStores(ctx, control, resolver); err != nil {
		t.Fatalf("disabled tenant database must not block active tenants: %v", err)
	}
	if err := adminDB.Exec("DROP DATABASE `" + name + "`").Error; err != nil {
		t.Fatalf("drop tenant database: %v", err)
	}
	// 模拟新实例（无进程内句柄缓存）。
	resolver.Invalidate(tenant.ID)

	err = migrateTenantStores(ctx, control, resolver)
	if err == nil {
		t.Fatal("sweep succeeded although the provisioned database is missing")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("sweep error does not identify the missing database: %v", err)
	}
}

func TestEnginePoolTracksTenantStatusAcrossInstances(t *testing.T) {
	control, resolver, _ := mysqlTenantWorld(t)
	control.SetTenantProvisioner(&tenantStoreProvisioner{resolver: resolver})
	ctx := context.Background()

	tenant, err := control.CreateTenant(ctx, nil, controlplane.TenantInput{Slug: "acme", DisplayName: "Acme"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	logger := zap.NewNop()
	shared := engineShared{logger: logger}
	poolOne := newEnginePool(resolver, control, shared, logger)
	defer poolOne.CloseTenantEngines()
	if engine := poolOne.forTenantID(ctx, tenant.ID); engine == nil {
		t.Fatal("active tenant must resolve an engine on mysql")
	}

	if _, err := control.DisableTenant(ctx, nil, tenant.ID); err != nil {
		t.Fatalf("disable tenant: %v", err)
	}
	if engine := poolOne.forTenantID(ctx, tenant.ID); engine != nil {
		t.Fatal("disabled tenant must not resolve an engine")
	}
	poolTwo := newEnginePool(resolver, control, shared, logger)
	defer poolTwo.CloseTenantEngines()
	if engine := poolTwo.forTenantID(ctx, tenant.ID); engine != nil {
		t.Fatal("second instance must reject the disabled tenant immediately")
	}

	if _, err := control.RestoreTenant(ctx, nil, tenant.ID); err != nil {
		t.Fatalf("restore tenant: %v", err)
	}
	if engine := poolOne.forTenantID(ctx, tenant.ID); engine == nil {
		t.Fatal("restored tenant must resolve an engine again")
	}
}
