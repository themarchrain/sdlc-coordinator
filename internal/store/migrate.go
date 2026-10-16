package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SchemaVersion 是本二进制期望的 schema 版本。变更 schema 的发布必须递增，
// 并保证迁移函数幂等（多实例任意顺序启动都能收敛到同版本）。
const SchemaVersion uint64 = 5

// migrationLockWaitSeconds 是等待跨实例迁移锁的上限；超过即报错退出，
// 不允许无界等待（迁移是受控维护步骤）。
const migrationLockWaitSeconds = 60

// migrationLockPrefix 是迁移咨询锁名前缀。
const migrationLockPrefix = "sdlc:migrate:"

// SchemaMigration 记录每个组件最近一次应用的 schema 版本（每个数据库一份）。
type SchemaMigration struct {
	Component string    `gorm:"column:component;primaryKey;size:64"`
	Version   uint64    `gorm:"column:version;not null"`
	AppliedAt time.Time `gorm:"column:applied_at;not null"`
}

func (SchemaMigration) TableName() string { return "schema_migrations" }

// ErrSchemaNewerThanBinary 表示库内记录的 schema 版本高于当前二进制支持的
// 版本：绝不允许旧代码对新 schema 继续写入，启动即失败。
var ErrSchemaNewerThanBinary = errors.New("database schema is newer than this binary supports")

func MigrateGuarded(ctx context.Context, db *gorm.DB, component string, fn func() error, opts ...MigrateOption) error {
	options := migrateOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	if db == nil {
		return fmt.Errorf("migrate %s: nil database handle", component)
	}
	if db.Dialector.Name() != DriverMySQL {
		return runGuardedMigration(ctx, db, component, fn, options)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("migrate %s: obtain sql handle: %w", component, err)
	}
	// 锁必须绑定在固定连接上：GET_LOCK/RELEASE_LOCK 是连接作用域的。
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migrate %s: acquire lock connection: %w", component, err)
	}
	defer conn.Close()

	var database sql.NullString
	if err := conn.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&database); err != nil {
		return fmt.Errorf("migrate %s: read current database: %w", component, err)
	}
	lockName := migrationLockName(database.String)
	var acquired sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", lockName, migrationLockWaitSeconds).Scan(&acquired); err != nil {
		return fmt.Errorf("migrate %s: acquire migration lock: %w", component, err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return fmt.Errorf("migrate %s: migration lock %q not acquired within %ds (another instance is migrating)", component, lockName, migrationLockWaitSeconds)
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var released sql.NullInt64
		_ = conn.QueryRowContext(releaseCtx, "SELECT RELEASE_LOCK(?)", lockName).Scan(&released)
	}()

	return runGuardedMigration(ctx, db, component, fn, options)
}

// MigrateOption 调整受控迁移行为。
type MigrateOption func(*migrateOptions)

type migrateOptions struct {
	force bool
}

// migrationLockName 由数据库名派生咨询锁名（GET_LOCK 上限 64 字符）。
func migrationLockName(database string) string {
	digest := sha256.Sum256([]byte(database))
	return fmt.Sprintf("%s%x", migrationLockPrefix, digest[:12])
}

// Force 强制重放迁移函数（默认在版本一致时跳过重复 DDL；人工修复或验证
// 场景需要显式重放时使用）。
func Force() MigrateOption {
	return func(options *migrateOptions) { options.force = true }
}

func runGuardedMigration(ctx context.Context, db *gorm.DB, component string, fn func() error, options migrateOptions) error {
	if err := db.WithContext(ctx).AutoMigrate(&SchemaMigration{}); err != nil {
		return fmt.Errorf("migrate %s: ensure schema_migrations: %w", component, err)
	}
	var recorded SchemaMigration
	err := db.WithContext(ctx).Where("component = ?", component).First(&recorded).Error
	switch {
	case err == nil:
		if recorded.Version > SchemaVersion {
			return fmt.Errorf("%w: component %s recorded v%d, binary supports v%d", ErrSchemaNewerThanBinary, component, recorded.Version, SchemaVersion)
		}
		if recorded.Version == SchemaVersion && db.Dialector.Name() == DriverMySQL && !options.force {
			return nil
		}
	case errors.Is(err, gorm.ErrRecordNotFound):
	default:
		return fmt.Errorf("migrate %s: read schema version: %w", component, err)
	}

	if fn != nil {
		if err := fn(); err != nil {
			return err
		}
	}

	row := SchemaMigration{Component: component, Version: SchemaVersion, AppliedAt: time.Now().UTC()}
	if err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "component"}},
		DoUpdates: clause.AssignmentColumns([]string{"version", "applied_at"}),
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("migrate %s: record schema version: %w", component, err)
	}
	return nil
}

// CurrentSchemaVersion 返回组件已记录的 schema 版本（未记录或迁移表尚不
// 存在均为 0）。
func CurrentSchemaVersion(ctx context.Context, db *gorm.DB, component string) (uint64, error) {
	if !db.WithContext(ctx).Migrator().HasTable(&SchemaMigration{}) {
		return 0, nil
	}
	var recorded SchemaMigration
	err := db.WithContext(ctx).Where("component = ?", component).First(&recorded).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return recorded.Version, nil
}
