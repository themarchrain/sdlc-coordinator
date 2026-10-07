package app

import (
	"context"
	"strings"
	"testing"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/store/storetest"
)

func TestMigrateBusinessStoreOnMySQL(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	db := storetest.OpenDatabase(t, admin, "biz_schema")

	if err := migrateBusinessStore(db); err != nil {
		t.Fatalf("migrate business store on mysql: %v", err)
	}
	// 多实例冷建引擎会重复走迁移：幂等重入必须成功。
	if err := migrateBusinessStore(db); err != nil {
		t.Fatalf("second migrate (idempotency): %v", err)
	}

	repoStore := registeredrepo.NewStore(db)
	ctx := context.Background()

	// 部分唯一索引的 MySQL 等价语义：同 provider 多条空 path 合法共存。
	emptyA := &registeredrepo.RegisteredRepository{RepositoryID: "repo_empty_a", Provider: "github", PathWithNamespace: "", Status: registeredrepo.StatusConfiguring}
	emptyB := &registeredrepo.RegisteredRepository{RepositoryID: "repo_empty_b", Provider: "github", PathWithNamespace: "", Status: registeredrepo.StatusConfiguring}
	if err := repoStore.Create(ctx, emptyA); err != nil {
		t.Fatalf("create first empty-path repo: %v", err)
	}
	if err := repoStore.Create(ctx, emptyB); err != nil {
		t.Fatalf("create second empty-path repo (partial index equivalence broken): %v", err)
	}

	// 同 provider 同 path 拒绝；跨 provider 同 path 合法共存。
	samePath := "acme/widgets"
	first := &registeredrepo.RegisteredRepository{RepositoryID: "repo_path_a", Provider: "github", PathWithNamespace: samePath, Status: registeredrepo.StatusConfiguring}
	if err := repoStore.Create(ctx, first); err != nil {
		t.Fatalf("create path row: %v", err)
	}
	dup := &registeredrepo.RegisteredRepository{RepositoryID: "repo_path_a2", Provider: "github", PathWithNamespace: samePath, Status: registeredrepo.StatusConfiguring}
	err := repoStore.Create(ctx, dup)
	if err == nil {
		t.Fatal("duplicate (provider, path) accepted, identity index not enforced")
	}
	if !store.IsDuplicateKey(err) {
		t.Fatalf("duplicate path not rejected by unique index: %v", err)
	}
	crossProvider := &registeredrepo.RegisteredRepository{RepositoryID: "repo_path_b", Provider: "gitlab", PathWithNamespace: samePath, Status: registeredrepo.StatusConfiguring}
	if err := repoStore.Create(ctx, crossProvider); err != nil {
		t.Fatalf("cross-provider same path must coexist: %v", err)
	}

	// 大小写不同视为不同业务键（二元排序规则）。
	upper := &registeredrepo.RegisteredRepository{RepositoryID: "repo_case_a", Provider: "github", PathWithNamespace: "acme/Widgets", Status: registeredrepo.StatusConfiguring}
	if err := repoStore.Create(ctx, upper); err != nil {
		t.Fatalf("case-different path must coexist on mysql: %v", err)
	}

	// 列改名探测在 MySQL 上不炸（fresh 部署无旧列，应为 no-op）。
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("legacy rename probe on mysql: %v", err)
	}

	// GetByPath 大小写兜底在 MySQL 上用等价实现命中。
	hit, err := repoStore.GetByPath(ctx, "github", strings.ToUpper(samePath))
	if err != nil {
		t.Fatalf("case-insensitive path fallback on mysql: %v", err)
	}
	if hit.RepositoryID != first.RepositoryID {
		t.Fatalf("fallback matched %s, want %s", hit.RepositoryID, first.RepositoryID)
	}

	// effect dedupe_key 唯一约束抽查（幂等通知的数据库防线）。
	effectStore := repository.NewEffectRepository(db)
	effectA := &model.Effect{EffectID: model.NewID("effect"), TaskID: "task_x", Kind: "notify", DedupeKey: "dedupe_probe_1", Status: model.EffectStatusPending}
	if err := effectStore.Create(ctx, effectA); err != nil {
		t.Fatalf("insert effect: %v", err)
	}
	effectDup := &model.Effect{EffectID: model.NewID("effect"), TaskID: "task_x", Kind: "notify", DedupeKey: "dedupe_probe_1", Status: model.EffectStatusPending}
	if err := effectStore.Create(ctx, effectDup); err == nil {
		t.Fatal("duplicate effect dedupe_key accepted")
	}
}

// TestControlPlaneSchemaAndSeedOnMySQL 在真实 MySQL 上锁定控制面 schema 与
// 种子超管语义：首启建号、重复种子不改密码、Reset 通道覆盖。
func TestControlPlaneSchemaAndSeedOnMySQL(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	db := storetest.OpenDatabase(t, admin, "cp_schema")
	ctx := context.Background()

	repo := controlplane.NewRepository(db)
	if err := repo.Migrate(); err != nil {
		t.Fatalf("migrate control plane on mysql: %v", err)
	}
	service := controlplane.NewService(repo)

	if err := service.SeedSuperAdmin(ctx, controlplane.SeedSuperAdminInput{Username: "seed-admin", Password: "seed-password-1"}); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	user, err := repo.GetUserByUsername(ctx, "seed-admin")
	if err != nil || user == nil {
		t.Fatalf("super admin missing after seed: %v", err)
	}
	if user.Role != controlplane.UserRoleSuperAdmin {
		t.Fatalf("seeded user role = %q", user.Role)
	}
	firstHash := user.PasswordHash

	// 重复种子（未开 Reset）是 no-op，不覆盖既有密码。
	if err := service.SeedSuperAdmin(ctx, controlplane.SeedSuperAdminInput{Username: "seed-admin", Password: "different-password-2"}); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	user, err = repo.GetUserByUsername(ctx, "seed-admin")
	if err != nil || user == nil {
		t.Fatalf("super admin vanished after second seed: %v", err)
	}
	if user.PasswordHash != firstHash {
		t.Fatal("second seed overwrote existing password without Reset")
	}

	// Reset 通道覆盖密码并吊销会话。
	if err := service.SeedSuperAdmin(ctx, controlplane.SeedSuperAdminInput{Username: "seed-admin", Password: "reset-password-3", Reset: true}); err != nil {
		t.Fatalf("reset seed: %v", err)
	}
	user, err = repo.GetUserByUsername(ctx, "seed-admin")
	if err != nil || user == nil {
		t.Fatalf("super admin vanished after reset: %v", err)
	}
	if user.PasswordHash == firstHash {
		t.Fatal("reset seed did not change the password hash")
	}
}

func TestSeedSuperAdminConcurrentFirstStartOnMySQL(t *testing.T) {
	admin := storetest.RequireMySQL(t)
	dsns := make([]string, 2)
	for i := range dsns {
		dsns[i] = storetest.NewDatabase(t, admin, "cp_seed_race")
	}
	for iteration := 0; iteration < 6; iteration++ {
		dsn := dsns[iteration%len(dsns)]
		db, err := store.Open(dsn)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		repo := controlplane.NewRepository(db)
		if err := repo.Migrate(); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		if err := db.Exec("DELETE FROM users").Error; err != nil {
			t.Fatalf("reset users: %v", err)
		}

		db2, err := store.Open(dsn)
		if err != nil {
			t.Fatalf("open second: %v", err)
		}
		svc1 := controlplane.NewService(controlplane.NewRepository(db))
		svc2 := controlplane.NewService(controlplane.NewRepository(db2))

		start := make(chan struct{})
		errs := make(chan error, 2)
		for _, svc := range []*controlplane.Service{svc1, svc2} {
			go func(svc *controlplane.Service) {
				<-start
				errs <- svc.SeedSuperAdmin(context.Background(), controlplane.SeedSuperAdminInput{
					Username: "race-admin", Password: "race-password-9",
				})
			}(svc)
		}
		close(start)
		for i := 0; i < 2; i++ {
			if err := <-errs; err != nil {
				t.Fatalf("iteration %d: concurrent seed failed: %v", iteration, err)
			}
		}
		var count int64
		if err := db.Raw("SELECT COUNT(*) FROM users WHERE username = 'race-admin'").Scan(&count).Error; err != nil {
			t.Fatalf("count users: %v", err)
		}
		if count != 1 {
			t.Fatalf("iteration %d: super admin rows = %d, want 1", iteration, count)
		}
		if sqlDB, err := db2.DB(); err == nil {
			_ = sqlDB.Close()
		}
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
}
