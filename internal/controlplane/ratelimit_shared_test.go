package controlplane

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"
)

// newSharedRateLimitServices 打开同一控制面库的两个 Service（模拟两台协调器）。
func newSharedRateLimitServices(t *testing.T, dsn string, maxFailures int) []*Service {
	t.Helper()
	services := make([]*Service, 2)
	for i := range services {
		db, err := store.Open(dsn)
		if err != nil {
			t.Fatalf("open instance %d: %v", i, err)
		}
		t.Cleanup(func() {
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
		})
		repository := NewRepository(db)
		if err := repository.Migrate(); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		services[i] = NewService(repository, WithRateLimit(time.Hour, maxFailures))
	}
	return services
}

// TestLoginRateLimitSharedAcrossServices 两个 Service 打开同一控制面库（模拟
// 两台协调器）：实例 A 累积的失败计数对实例 B 可见,轮流攻击不能扩大原配额。
func TestLoginRateLimitSharedAcrossServices(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "rate_shared")
	services := newSharedRateLimitServices(t, dsn, 3)
	seedRateLimitUser(t, services[0], "ratelimit-user", "rate-shared")

	ctx := context.Background()
	// 实例 A 打满 3 次失败配额。
	for i := 0; i < 3; i++ {
		if _, err := services[0].Login(ctx, "ratelimit-user", "wrong-password", "10.1.1.1"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: expected invalid credentials, got %v", i, err)
		}
	}
	// 实例 A 已被锁。
	if _, err := services[0].Login(ctx, "ratelimit-user", "test-password", "10.1.1.1"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("instance A must be rate limited after its own failures, got %v", err)
	}
	// 实例 B 轮流访问:共享配额已耗尽,同样被锁——不能扩大原失败配额。
	if _, err := services[1].Login(ctx, "ratelimit-user", "test-password", "10.1.1.1"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("instance B must see the shared failure budget, got %v", err)
	}
}

// TestLoginRateLimitResetPropagatesAcrossServices 实例 A 成功登录的重置对
// 实例 B 生效:重置后 B 可正常登录。
func TestLoginRateLimitResetPropagatesAcrossServices(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "rate_reset")
	services := newSharedRateLimitServices(t, dsn, 3)
	seedRateLimitUser(t, services[0], "ratelimit-user", "rate-reset")

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := services[0].Login(ctx, "ratelimit-user", "wrong-password", "10.2.2.2"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("failure %d: %v", i, err)
		}
	}
	// 实例 A 成功登录:重置共享计数。
	if _, err := services[0].Login(ctx, "ratelimit-user", "test-password", "10.2.2.2"); err != nil {
		t.Fatalf("successful login: %v", err)
	}
	// 实例 B 视角:计数已清,正常登录不再受历史失败拖累。
	if _, err := services[1].Login(ctx, "ratelimit-user", "test-password", "10.2.2.2"); err != nil {
		t.Fatalf("instance B login after A's reset: %v", err)
	}
}

// TestLoginRateLimitWindowExpiresViaDBTime 滑动窗口按库内时间过期:窗口外的
// 失败记录不再计入配额(与租约同款库时钟语义,跨实例时钟偏移免疫)。
func TestLoginRateLimitWindowExpiresViaDBTime(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, admin, "rate_window")
	services := newSharedRateLimitServices(t, dsn, 2)
	seedRateLimitUser(t, services[0], "ratelimit-user", "rate-window")
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	service := services[0]

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := service.Login(ctx, "ratelimit-user", "wrong-password", "10.3.3.3"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("failure %d: %v", i, err)
		}
	}
	if _, err := service.Login(ctx, "ratelimit-user", "test-password", "10.3.3.3"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("must be locked at quota, got %v", err)
	}
	// 用数据库时钟把失败记录拨出窗口:锁定解除。
	if err := db.Exec("UPDATE login_failures SET failed_at = DATE_SUB(NOW(6), INTERVAL 2 HOUR)").Error; err != nil {
		t.Fatalf("backdate failures: %v", err)
	}
	if _, err := service.Login(ctx, "ratelimit-user", "test-password", "10.3.3.3"); err != nil {
		t.Fatalf("login must succeed after window expiry, got %v", err)
	}
}

// TestLoginRateLimitSQLiteSemanticsMatchMySQL SQLite 后端(单机形态)与 MySQL
// 的共享限流语义一致:配额/锁定/键隔离行为不漂移。
func TestLoginRateLimitSQLiteSemanticsMatchMySQL(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "rate-sqlite.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	repository := NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	service := NewService(repository, WithRateLimit(time.Hour, 2))
	seedRateLimitUser(t, service, "ratelimit-user", "rate-sqlite")
	seedRateLimitUser(t, service, "other-ratelimit-user", "rate-sqlite-2")

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := service.Login(ctx, "ratelimit-user", "wrong-password", "10.4.4.4"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("failure %d: %v", i, err)
		}
	}
	if _, err := service.Login(ctx, "ratelimit-user", "test-password", "10.4.4.4"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("must be locked at quota, got %v", err)
	}
	// 键隔离:其他用户名不受影响。
	if _, err := service.Login(ctx, "other-ratelimit-user", "test-password", "10.4.4.4"); err != nil {
		t.Fatalf("other key must be unaffected: %v", err)
	}
}

// seedRateLimitUser 建一个带组织 membership 的登录可用账号；slug 每测试
// 唯一避免撞名。
func seedRateLimitUser(t *testing.T, service *Service, username, slug string) {
	t.Helper()
	tenantID, _ := seedTenantWithAdmin(t, service, slug)
	user, err := service.CreateUser(context.Background(), username, "test-password")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := service.AddMembership(context.Background(), &testActor, tenantID, user.ID, RoleMember); err != nil {
		t.Fatalf("add membership: %v", err)
	}
}
