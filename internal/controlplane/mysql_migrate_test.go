package controlplane_test

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"
)

func TestControlPlaneMigrateIdempotentOnMySQL(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	db := storetest.OpenDatabase(t, admin, "cp_migrate")
	repository := controlplane.NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := repository.Migrate(); err != nil {
		t.Fatalf("second migrate must be idempotent on mysql: %v", err)
	}
}

// v4→v5 升级：v4 的 (tenant_id, endpoint_url) 复合唯一索引收敛为
// endpoint 单列唯一（端点跨租户独占），存量合法数据（每租户各占端点）
// 保留，旧复合索引清除不残留。
func TestControlPlaneV4SchemaUpgradesToEndpointExclusivityOnMySQL(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	db := storetest.OpenDatabase(t, admin, "cp_migrate_v4")
	if err := db.AutoMigrate(&store.SchemaMigration{}); err != nil {
		t.Fatalf("create migration table: %v", err)
	}
	if err := db.Create(&store.SchemaMigration{Component: "controlplane", Version: 4, AppliedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatalf("seed v4 schema version: %v", err)
	}
	if err := db.Exec("CREATE TABLE runner_assignments (id bigint unsigned NOT NULL AUTO_INCREMENT, tenant_id bigint unsigned NOT NULL, endpoint_url varchar(512) NOT NULL, created_at datetime(3) NULL, PRIMARY KEY (id), UNIQUE KEY idx_runner_assignments_tenant_endpoint (tenant_id, endpoint_url)) ENGINE=InnoDB").Error; err != nil {
		t.Fatalf("create v4 runner_assignments table: %v", err)
	}
	for _, row := range []struct{ tenant, endpoint string }{{"1", "http://runner-a:8081"}, {"2", "http://runner-b:8081"}} {
		if err := db.Exec("INSERT INTO runner_assignments (tenant_id, endpoint_url) VALUES (?, ?)", row.tenant, row.endpoint).Error; err != nil {
			t.Fatalf("seed v4 assignment: %v", err)
		}
	}
	repository := controlplane.NewRepository(db)
	if err := store.MigrateGuarded(context.Background(), db, "controlplane", repository.Migrate); err != nil {
		t.Fatalf("upgrade controlplane schema v4: %v", err)
	}
	if version, err := store.CurrentSchemaVersion(context.Background(), db, "controlplane"); err != nil || version != store.SchemaVersion {
		t.Fatalf("schema version = %d (%v), want %d", version, err, store.SchemaVersion)
	}
	if !db.Migrator().HasIndex(&controlplane.RunnerAssignment{}, "idx_runner_assignments_endpoint") {
		t.Fatal("v4 upgrade did not create endpoint exclusivity index")
	}
	if db.Migrator().HasIndex(&controlplane.RunnerAssignment{}, "idx_runner_assignments_tenant_endpoint") {
		t.Fatal("v4 upgrade left legacy tenant-endpoint index behind")
	}
	var rows int64
	if err := db.Table("runner_assignments").Count(&rows).Error; err != nil {
		t.Fatalf("count upgraded assignments: %v", err)
	}
	if rows != 2 {
		t.Fatalf("upgraded assignment rows = %d, want 2", rows)
	}
	// 独占语义生效：另一租户抢注已占用端点必须被唯一索引拒绝。
	if err := db.Exec("INSERT INTO runner_assignments (tenant_id, endpoint_url) VALUES (?, ?)", "3", "http://runner-a:8081").Error; err == nil {
		t.Fatal("cross-tenant endpoint insert must fail on unique index")
	}
}

func TestControlPlaneV3SchemaUpgradesOnMySQL(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	db := storetest.OpenDatabase(t, admin, "cp_migrate_v3")
	if err := db.AutoMigrate(&store.SchemaMigration{}); err != nil {
		t.Fatalf("create migration table: %v", err)
	}
	if err := db.Create(&store.SchemaMigration{Component: "controlplane", Version: 3, AppliedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatalf("seed v3 schema version: %v", err)
	}
	if err := db.Exec("CREATE TABLE login_failures (id bigint unsigned NOT NULL AUTO_INCREMENT, `key` varchar(255) NOT NULL, failed_at datetime(3) NOT NULL, PRIMARY KEY (id), KEY idx_login_failures_key_time (`key`, failed_at), KEY idx_login_failures_failed_at (failed_at)) ENGINE=InnoDB").Error; err != nil {
		t.Fatalf("create v3 login_failures table: %v", err)
	}
	if err := db.Exec("INSERT INTO login_failures (`key`, failed_at) VALUES ('v3-user|192.0.2.20', NOW(6))").Error; err != nil {
		t.Fatalf("seed v3 active failure: %v", err)
	}
	repository := controlplane.NewRepository(db)
	if err := store.MigrateGuarded(context.Background(), db, "controlplane", repository.Migrate); err != nil {
		t.Fatalf("upgrade controlplane schema v3: %v", err)
	}
	if version, err := store.CurrentSchemaVersion(context.Background(), db, "controlplane"); err != nil || version != store.SchemaVersion {
		t.Fatalf("schema version = %d (%v), want %d", version, err, store.SchemaVersion)
	}
	if !db.Migrator().HasColumn(&controlplane.LoginFailure{}, "Pending") {
		t.Fatal("v3 upgrade did not add login_failures.pending")
	}
	if !db.Migrator().HasTable("login_failure_capacity_guards") {
		t.Fatal("v3 upgrade did not create login failure capacity guard table")
	}
	var guards int64
	if err := db.Table("login_failure_capacity_guards").Count(&guards).Error; err != nil {
		t.Fatalf("count capacity guard rows: %v", err)
	}
	if guards != 1 {
		t.Fatalf("capacity guard rows = %d, want 1", guards)
	}
	count, err := repository.CountRecentLoginFailures(context.Background(), "v3-user|192.0.2.20", time.Hour)
	if err != nil || count != 1 {
		t.Fatalf("v3 active failure count = %d (%v), want 1", count, err)
	}
}

func TestLoginFailureReservationUsesMySQLClock(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	db := storetest.OpenDatabase(t, admin, "cp_reservation_clock")
	repository := controlplane.NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	ids, err := repository.ReserveLoginFailureSlots(ctx, time.Hour, 1)
	if err != nil {
		t.Fatalf("reserve slot: %v", err)
	}
	var reserved controlplane.LoginFailure
	if err := db.First(&reserved, ids[0]).Error; err != nil {
		t.Fatalf("load reservation: %v", err)
	}
	var databaseNow time.Time
	if err := db.Raw("SELECT NOW(6)").Scan(&databaseNow).Error; err != nil {
		t.Fatalf("read database time: %v", err)
	}
	if delta := reserved.FailedAt.Sub(databaseNow); delta < -5*time.Second || delta > 5*time.Second {
		t.Fatalf("reservation time differs from MySQL clock by %s", delta)
	}
	if err := repository.CompleteLoginFailureReservation(ctx, ids, []string{"clock-user|192.0.2.21"}, time.Now().Add(-24*time.Hour)); err != nil {
		t.Fatalf("complete reservation: %v", err)
	}
	if err := db.First(&reserved, ids[0]).Error; err != nil {
		t.Fatalf("load completed failure: %v", err)
	}
	if err := db.Raw("SELECT NOW(6)").Scan(&databaseNow).Error; err != nil {
		t.Fatalf("read database time after completion: %v", err)
	}
	if delta := reserved.FailedAt.Sub(databaseNow); delta < -5*time.Second || delta > 5*time.Second {
		t.Fatalf("completed failure time followed caller clock instead of MySQL clock: delta %s", delta)
	}
}
