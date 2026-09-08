package tenantmigration

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

// 存量迁移：主库**复制**为「默认组织」租户库
// （slug tenant-one），主库原位保留为平台库；存量 runner 端点写入该组织
// runner_assignments；存量注册仓写穿 repo_route_index。幂等：租户库文件即
// 迁移标记；回滚 = 删租户目录后可重跑。
func newMigrationWorld(t *testing.T) (Deps, *gorm.DB) {
	t.Helper()
	mainDB, err := store.Open("file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "platform.db")))
	if err != nil {
		t.Fatalf("open platform db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := mainDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := store.Migrate(mainDB, &model.Task{}); err != nil {
		t.Fatalf("migrate platform db: %v", err)
	}
	cpDB, err := store.Open("file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "control-plane.db")))
	if err != nil {
		t.Fatalf("open control plane db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := cpDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	repository := controlplane.NewRepository(cpDB)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate control plane: %v", err)
	}
	return Deps{
		MainDB:               mainDB,
		ControlPlane:         repository,
		TenantsRoot:          t.TempDir(),
		LegacyRunnerEndpoint: "http://legacy-runner:8081",
	}, mainDB
}

func seedLegacyBusinessData(t *testing.T, mainDB *gorm.DB) {
	t.Helper()
	if err := mainDB.Create(&model.Task{TaskID: "task-legacy", DedupeKey: "legacy", Type: "review_mr", Role: "ReviewerWorker", Status: model.TaskStatusPending}).Error; err != nil {
		t.Fatalf("seed legacy task: %v", err)
	}
	repoStore := registeredrepo.NewStore(mainDB)
	if err := repoStore.Migrate(); err != nil {
		t.Fatalf("migrate registered repos: %v", err)
	}
	if err := repoStore.Create(context.Background(), &registeredrepo.RegisteredRepository{
		RepositoryID: "repo-legacy", DisplayName: "Legacy Repo", Provider: "gitlab",
		BaseURL: "https://legacy.gitlab.example.com", PathWithNamespace: "acme/app",
	}); err != nil {
		t.Fatalf("seed legacy repo: %v", err)
	}
}

func TestRunCopiesPlatformDatabaseToDefaultTenant(t *testing.T) {
	deps, mainDB := newMigrationWorld(t)
	seedLegacyBusinessData(t, mainDB)

	migrated, err := Run(context.Background(), deps)
	if err != nil || !migrated {
		t.Fatalf("run migration: migrated=%v err=%v", migrated, err)
	}

	repository := deps.ControlPlane
	tenant, err := repository.GetTenantBySlug(context.Background(), "tenant-one")
	if err != nil || tenant == nil {
		t.Fatalf("default tenant must be created: %#v err=%v", tenant, err)
	}
	if tenant.DisplayName != "默认组织" {
		t.Fatalf("default tenant display name drift: %q", tenant.DisplayName)
	}

	tenantDB, err := store.Open("file:" + filepath.ToSlash(filepath.Join(deps.TenantsRoot, strconv.FormatUint(uint64(tenant.ID), 10), "db.sqlite")))
	if err != nil {
		t.Fatalf("open tenant copy: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := tenantDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	var legacy model.Task
	if err := tenantDB.Where("task_id = ?", "task-legacy").First(&legacy).Error; err != nil {
		t.Fatalf("legacy task must be copied into the tenant database: %v", err)
	}

	assignments, err := repository.ListRunnerAssignments(context.Background(), tenant.ID)
	if err != nil || len(assignments) != 1 || assignments[0].EndpointURL != "http://legacy-runner:8081" {
		t.Fatalf("legacy runner endpoint must be seeded: %#v err=%v", assignments, err)
	}

	routes, err := repository.ListRepoRouteIndex(context.Background(), tenant.ID)
	if err != nil || len(routes) != 1 || routes[0].PathWithNamespace != "acme/app" || routes[0].BaseURL != "https://legacy.gitlab.example.com" {
		t.Fatalf("legacy repos must be written into the route index: %#v err=%v", routes, err)
	}
}

func TestRunIsIdempotentOnRerun(t *testing.T) {
	deps, mainDB := newMigrationWorld(t)
	seedLegacyBusinessData(t, mainDB)
	ctx := context.Background()

	if migrated, err := Run(ctx, deps); err != nil || !migrated {
		t.Fatalf("first run: migrated=%v err=%v", migrated, err)
	}
	migrated, err := Run(ctx, deps)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if migrated {
		t.Fatalf("second run must be a no-op")
	}
	repository := deps.ControlPlane
	tenant, _ := repository.GetTenantBySlug(ctx, "tenant-one")
	assignments, err := repository.ListRunnerAssignments(ctx, tenant.ID)
	if err != nil || len(assignments) != 1 {
		t.Fatalf("assignments must not duplicate on rerun: %#v err=%v", assignments, err)
	}
}

func TestRunSkipsFreshInstallWithoutBusinessData(t *testing.T) {
	deps, _ := newMigrationWorld(t)
	migrated, err := Run(context.Background(), deps)
	if err != nil || migrated {
		t.Fatalf("fresh install must be skipped: migrated=%v err=%v", migrated, err)
	}
	tenant, err := deps.ControlPlane.GetTenantBySlug(context.Background(), "tenant-one")
	if err != nil {
		t.Fatalf("query tenant: %v", err)
	}
	if tenant != nil {
		t.Fatalf("fresh install must not create the default tenant")
	}
}
