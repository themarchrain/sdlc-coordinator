package store

import (
	"fmt"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Open 打开存储句柄。DSN 以 mysql:// 开头时使用 MySQL（分布式部署的共享
// 状态），其余按既有 SQLite 语义打开（单机开发与纯业务逻辑测试）。
func Open(dsn string) (*gorm.DB, error) {
	spec, err := ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	return OpenSpec(spec)
}

// OpenSpec 按已解析的 Spec 打开句柄。
func OpenSpec(spec Spec) (*gorm.DB, error) {
	switch spec.Driver {
	case DriverMySQL:
		return openMySQL(spec)
	default:
		return openSQLite(spec.DriverDSN)
	}
}

func openSQLite(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("obtain sql db handle: %w", err)
	}

	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	sqlDB.SetMaxOpenConns(1)

	return db, nil
}

func openMySQL(spec Spec) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(spec.DriverDSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open mysql database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("obtain sql db handle: %w", err)
	}

	sqlDB.SetMaxOpenConns(spec.Pool.MaxOpen)
	sqlDB.SetMaxIdleConns(spec.Pool.MaxIdle)
	sqlDB.SetConnMaxLifetime(spec.Pool.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(spec.Pool.ConnMaxIdleTime)

	return db, nil
}

func Migrate(db *gorm.DB, models ...any) error {
	if err := db.AutoMigrate(models...); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	return nil
}
