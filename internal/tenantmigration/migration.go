// Package tenantmigration 把存量单租户部署迁移为「默认组织」。
//
// 两条迁移路径：
//   - 复制不移动：平台主库**复制**为租户库（VACUUM INTO，零数据改写），
//     主库原位保留为平台库（runtime_config_snapshots 继续用它，业务表留存
//     为死数据不读不删）；回滚 = 删租户目录后可重跑。
//   - 新建迁移专属组织：目标租户=控制面**新建** slug `tenant-one` 的
//     「默认组织」，不并入既有组织（存量平台时代数据语义上不属于它们）。
//
// 幂等：租户库文件即迁移标记；目录存在 = 已迁移，重跑为 no-op。
// 配套：存量 runner 端点写入该组织 runner_assignments（派发行为不变）；
// 存量注册仓写穿 repo_route_index（写穿不等对账）。
package tenantmigration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"
)

// DefaultTenantSlug / DefaultTenantDisplayName 是迁移目标组织。
const (
	DefaultTenantSlug        = "tenant-one"
	DefaultTenantDisplayName = "默认组织"
)

type Deps struct {
	// MainDB 是平台主库（只读源；迁移不改写其中任何数据）。
	MainDB *gorm.DB
	// ControlPlane 承接组织登记、runner 指派与路由索引。
	ControlPlane *controlplane.Repository
	// TenantsRoot 是租户数据根（与装配的 TENANT_DATA_DIR 同值）。
	TenantsRoot string
	// LegacyRunnerEndpoint 是存量 agent_runner.service_url（空 = 不种指派）。
	LegacyRunnerEndpoint string
}

// Run 执行迁移。返回 migrated=true 表示本次发生了迁移；false 表示跳过
// （空库新装）或已迁移（幂等重入）。
func Run(ctx context.Context, deps Deps) (bool, error) {
	if deps.MainDB == nil || deps.ControlPlane == nil || strings.TrimSpace(deps.TenantsRoot) == "" {
		return false, errors.New("tenant migration dependencies are required")
	}
	if !hasMigratableBusinessData(ctx, deps.MainDB) {
		return false, nil
	}
	tenant, err := ensureDefaultTenant(ctx, deps.ControlPlane)
	if err != nil {
		return false, err
	}
	tenantDir := filepath.Join(deps.TenantsRoot, strconv.FormatUint(uint64(tenant.ID), 10))
	tenantFile := filepath.Join(tenantDir, "db.sqlite")
	if _, err := os.Stat(tenantFile); err == nil {
		// 目录即标记：已迁移（含「建了组织但复制中断后重跑」的恢复路径）。
		return false, nil
	}
	if err := copyPlatformDatabase(deps.MainDB, tenantFile); err != nil {
		return false, fmt.Errorf("copy platform database: %w", err)
	}
	// 对副本补齐业务域 schema（AutoMigrate 幂等；平台库与租户库清单一致，
	// 与装配层 migrateTenantStores 同口径）。
	if err := migrateCopiedDatabase(tenantFile); err != nil {
		_ = os.RemoveAll(tenantDir)
		return false, fmt.Errorf("migrate copied tenant database: %w", err)
	}
	if deps.LegacyRunnerEndpoint != "" {
		if err := deps.ControlPlane.ReplaceRunnerAssignments(ctx, tenant.ID, []string{deps.LegacyRunnerEndpoint}); err != nil {
			return false, fmt.Errorf("seed legacy runner assignment: %w", err)
		}
	}
	if err := backfillRepoRouteIndex(ctx, deps, tenant.ID); err != nil {
		return false, err
	}
	_ = deps.ControlPlane.CreateAuditLog(ctx, &controlplane.AuditLog{
		Action: "legacy_migration.completed",
		Detail: "slug=" + DefaultTenantSlug + " tenant_id=" + strconv.FormatUint(uint64(tenant.ID), 10),
	})
	return true, nil
}

func hasMigratableBusinessData(ctx context.Context, mainDB *gorm.DB) bool {
	if !mainDB.Migrator().HasTable("tasks") {
		return false
	}
	var count int64
	if err := mainDB.WithContext(ctx).Raw("SELECT COUNT(*) FROM tasks").Scan(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func ensureDefaultTenant(ctx context.Context, repository *controlplane.Repository) (*controlplane.Tenant, error) {
	existing, err := repository.GetTenantBySlug(ctx, DefaultTenantSlug)
	if err != nil {
		return nil, fmt.Errorf("query default tenant: %w", err)
	}
	if existing != nil {
		return existing, nil
	}
	tenant := &controlplane.Tenant{
		Slug: DefaultTenantSlug, DisplayName: DefaultTenantDisplayName,
		Status: controlplane.TenantStatusActive,
	}
	if err := repository.CreateTenant(ctx, tenant); err != nil {
		return nil, fmt.Errorf("create default tenant: %w", err)
	}
	return tenant, nil
}

// copyPlatformDatabase 用 VACUUM INTO 做一致性快照复制（零数据改写、含全部
// 已提交内容；比文件复制更稳，不受残留 -wal/-shm 影响）。
func copyPlatformDatabase(mainDB *gorm.DB, targetFile string) error {
	if err := os.MkdirAll(filepath.Dir(targetFile), 0o755); err != nil {
		return err
	}
	_ = os.Remove(targetFile)
	literal := "file:" + filepath.ToSlash(targetFile)
	literal = strings.ReplaceAll(literal, "'", "''")
	return mainDB.Exec("VACUUM INTO '" + literal + "'").Error
}

func migrateCopiedDatabase(tenantFile string) error {
	db, err := store.Open("file:" + filepath.ToSlash(tenantFile) + "?_busy_timeout=5000")
	if err != nil {
		return err
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	if err := store.Migrate(db, &model.Task{}, &model.Run{}, &model.AgentExecution{}, &model.AgentExecutionEvent{}, &model.Artifact{}, &model.Dependency{}, &model.Writeback{}, &model.Effect{}, &model.InboundEvent{}, &model.ResultSubmission{}, &model.ResultSubmissionRejection{}, &model.ConnectorInstance{}); err != nil {
		return err
	}
	if err := registeredrepo.NewStore(db).Migrate(); err != nil {
		return err
	}
	return nil
}

func backfillRepoRouteIndex(ctx context.Context, deps Deps, tenantID uint) error {
	rows, err := registeredrepo.NewStore(deps.MainDB).List(ctx)
	if err != nil {
		return fmt.Errorf("list legacy repositories: %w", err)
	}
	for i := range rows {
		row := &rows[i]
		if err := deps.ControlPlane.UpsertRepoRouteIndex(ctx, controlplane.RepoRouteIndex{
			Provider: row.Provider, BaseURL: row.BaseURL, PathWithNamespace: row.PathWithNamespace,
			TenantID: tenantID, RepoFingerprint: row.RepositoryID,
		}); err != nil {
			return fmt.Errorf("backfill route index for %s: %w", row.RepositoryID, err)
		}
	}
	return nil
}
