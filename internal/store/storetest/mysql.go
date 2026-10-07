package storetest

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

// MySQLDSNEnv 管理连接 DSN 的环境变量名。
const MySQLDSNEnv = "SDLC_TEST_MYSQL_DSN"

var databaseSeq atomic.Int64

// RequireMySQL 返回管理连接 DSN；未配置时跳过当前测试。
func RequireMySQL(t *testing.T) string {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(MySQLDSNEnv))
	if dsn == "" {
		t.Skipf("set %s to run real-MySQL integration tests", MySQLDSNEnv)
	}
	if !strings.HasPrefix(dsn, "mysql://") {
		t.Fatalf("%s must be a mysql:// admin DSN without a database name", MySQLDSNEnv)
	}
	return dsn
}

// NewDatabase 在管理连接上创建一个独立数据库并返回其 DSN；测试结束自动
// DROP。库名带测试名与序号，避免并行/重复执行碰撞。
func NewDatabase(t *testing.T, adminDSN, prefix string) string {
	t.Helper()
	admin, err := store.Open(adminDSN)
	if err != nil {
		t.Fatalf("open mysql admin connection: %v", err)
	}
	sqlDB, err := admin.DB()
	if err != nil {
		t.Fatalf("admin sql handle: %v", err)
	}

	name := fmt.Sprintf("sdlc_it_%s_%d_%d", sanitizeIdentifier(prefix), os.Getpid(), databaseSeq.Add(1))
	if err := store.ValidateDatabaseName(name); err != nil {
		t.Fatalf("generated database name invalid: %v", err)
	}
	if err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin").Error; err != nil {
		t.Fatalf("create test database %s: %v", name, err)
	}
	t.Cleanup(func() {
		// 先断开本测试持有的句柄再删库由各用例自理；这里只负责最终清库。
		deadline := time.Now().Add(10 * time.Second)
		for {
			dropErr := admin.Exec("DROP DATABASE IF EXISTS `" + name + "`").Error
			if dropErr == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Logf("drop test database %s failed (left for manual cleanup): %v", name, dropErr)
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		_ = sqlDB.Close()
	})

	tenantDSN, err := databaseDSN(adminDSN, name)
	if err != nil {
		t.Fatalf("derive database dsn: %v", err)
	}
	return tenantDSN
}

// OpenDatabase 创建独立数据库并打开句柄；句柄随测试结束关闭。
func OpenDatabase(t *testing.T, adminDSN, prefix string) *gorm.DB {
	t.Helper()
	dsn := NewDatabase(t, adminDSN, prefix)
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// databaseDSN 把管理连接 DSN 换成指定库名的 mysql:// DSN（保留凭据与参数）。
func databaseDSN(adminDSN, name string) (string, error) {
	if err := store.ValidateDatabaseName(name); err != nil {
		return "", err
	}
	parsed, err := url.Parse(adminDSN)
	if err != nil {
		return "", fmt.Errorf("parse admin dsn: %w", err)
	}
	parsed.Path = "/" + name
	return parsed.String(), nil
}

func sanitizeIdentifier(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(raw) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		out = "test"
	}
	if len(out) > 40 {
		out = out[:40]
	}
	if out[0] >= '0' && out[0] <= '9' {
		out = "t" + out
	}
	return out
}
