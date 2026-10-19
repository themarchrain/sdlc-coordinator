package store

import (
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// TestParseDSNSQLiteForms 锁定既有 SQLite DSN 形态全部原样透传（不迁移存量
// 开发/测试用法）。
func TestParseDSNSQLiteForms(t *testing.T) {
	cases := []string{
		"file:sdlc-coordinator.db?_foreign_keys=on&_busy_timeout=5000",
		"file:x?mode=memory&cache=shared",
		"control-plane.db",
		"data/tenants/1/db.sqlite",
	}
	for _, raw := range cases {
		spec, err := ParseDSN(raw)
		if err != nil {
			t.Fatalf("ParseDSN(%q): %v", raw, err)
		}
		if spec.Driver != DriverSQLite {
			t.Fatalf("ParseDSN(%q).Driver = %q, want sqlite", raw, spec.Driver)
		}
		if spec.DriverDSN != raw {
			t.Fatalf("ParseDSN(%q).DriverDSN = %q, want passthrough", raw, spec.DriverDSN)
		}
	}
}

// TestParseDSNMySQLForcedSemantics 锁定 MySQL 打开语义：parseTime、UTC、
// 二元排序规则与 UTC 会话时区是打开层强制的，不依赖部署参数。
func TestParseDSNMySQLForcedSemantics(t *testing.T) {
	spec, err := ParseDSN("mysql://root:secret@db.example:3307/sdlc_platform?timeout=5s")
	if err != nil {
		t.Fatalf("ParseDSN: %v", err)
	}
	if spec.Driver != DriverMySQL {
		t.Fatalf("Driver = %q, want mysql", spec.Driver)
	}
	cfg, err := mysql.ParseDSN(spec.DriverDSN)
	if err != nil {
		t.Fatalf("driver DSN not parseable: %v", err)
	}
	if cfg.User != "root" || cfg.Passwd != "secret" {
		t.Fatalf("credentials lost: user=%q", cfg.User)
	}
	if cfg.Addr != "db.example:3307" || cfg.Net != "tcp" {
		t.Fatalf("addr = %q net = %q", cfg.Addr, cfg.Net)
	}
	if cfg.DBName != "sdlc_platform" {
		t.Fatalf("dbname = %q", cfg.DBName)
	}
	if !cfg.ParseTime {
		t.Fatal("parseTime must be forced on")
	}
	if cfg.Loc != time.UTC {
		t.Fatalf("loc = %v, want UTC", cfg.Loc)
	}
	if cfg.Collation != "utf8mb4_bin" {
		t.Fatalf("collation = %q, want utf8mb4_bin (binary comparison semantics)", cfg.Collation)
	}
	if got := cfg.Params["time_zone"]; got != "'+00:00'" {
		t.Fatalf("time_zone = %q, want '+00:00'", got)
	}
	if cfg.Timeout != 5*time.Second {
		t.Fatalf("timeout = %v, want 5s", cfg.Timeout)
	}
}

// TestParseDSNMySQLPoolBudget 锁定连接池预算的解析与默认值。
func TestParseDSNMySQLPoolBudget(t *testing.T) {
	spec, err := ParseDSN("mysql://root:pw@127.0.0.1:3306/sdlc_x")
	if err != nil {
		t.Fatalf("ParseDSN: %v", err)
	}
	if spec.Pool.MaxOpen != DefaultMySQLMaxOpenConns || spec.Pool.MaxIdle != DefaultMySQLMaxIdleConns {
		t.Fatalf("defaults = %+v", spec.Pool)
	}
	spec, err = ParseDSN("mysql://root:pw@127.0.0.1:3306/sdlc_x?max_open_conns=4&max_idle_conns=2&conn_max_lifetime=10m")
	if err != nil {
		t.Fatalf("ParseDSN: %v", err)
	}
	if spec.Pool.MaxOpen != 4 || spec.Pool.MaxIdle != 2 || spec.Pool.ConnMaxLifetime != 10*time.Minute {
		t.Fatalf("override = %+v", spec.Pool)
	}
}

// TestParseDSNMySQLAdminForm 锁定无库名管理连接（开通租户库用）与 WithDatabase。
func TestParseDSNMySQLAdminForm(t *testing.T) {
	spec, err := ParseDSN("mysql://root:pw@127.0.0.1:3306/")
	if err != nil {
		t.Fatalf("admin DSN rejected: %v", err)
	}
	if spec.Database != "" {
		t.Fatalf("admin Database = %q, want empty", spec.Database)
	}
	tenant := spec.WithDatabase("sdlc_tenant_7")
	cfg, err := mysql.ParseDSN(tenant.DriverDSN)
	if err != nil {
		t.Fatalf("tenant DSN: %v", err)
	}
	if cfg.DBName != "sdlc_tenant_7" {
		t.Fatalf("tenant DBName = %q", cfg.DBName)
	}
	if cfg.User != "root" || cfg.Addr != "127.0.0.1:3306" {
		t.Fatalf("WithDatabase lost connection identity: %+v", cfg)
	}
}

// TestParseDSNMySQLRejects 非法配置必须显式失败，不静默降级：未知参数、
// 缺 user、池预算越界。
func TestParseDSNMySQLRejects(t *testing.T) {
	cases := []string{
		"mysql://127.0.0.1:3306/db",                          // 缺 user
		"mysql://root:pw@127.0.0.1:3306/db?bogus=1",          // 未知参数
		"mysql://root:pw@127.0.0.1:3306/db?max_open_conns=1", // 迁移锁需要 >=2 连接
		"mysql://root:pw@127.0.0.1:3306/db?max_open_conns=0",
		"mysql://root:pw@127.0.0.1:3306/db?max_idle_conns=8&max_open_conns=4", // idle > open
		"mysql://[::1/db", // 非法 URL
	}
	for _, raw := range cases {
		if _, err := ParseDSN(raw); err == nil {
			t.Fatalf("ParseDSN(%q) succeeded, want error", raw)
		}
	}
}

// TestParseDSNDatabaseIdentifier 租户库名进入 DDL 前必须过标识符白名单，
// 防止库名注入与非受控命名。
func TestParseDSNDatabaseIdentifier(t *testing.T) {
	if err := ValidateDatabaseName("sdlc_tenant_12"); err != nil {
		t.Fatalf("valid name rejected: %v", err)
	}
	for _, bad := range []string{"", "1abc", "has space", "has-dash", "back`tick", "删库", strings.Repeat("a", 65)} {
		if err := ValidateDatabaseName(bad); err == nil {
			t.Fatalf("ValidateDatabaseName(%q) succeeded, want error", bad)
		}
	}
}
