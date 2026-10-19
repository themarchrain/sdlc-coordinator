package store

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Driver 名。SQLite 仅保留给单机开发与纯业务逻辑测试；分布式部署的共享
// 状态一律 MySQL（不承诺 SQLite 双后端兼容）。
const (
	DriverSQLite = "sqlite"
	DriverMySQL  = "mysql"
)

const (
	DefaultMySQLMaxOpenConns = 16
	DefaultMySQLMaxIdleConns = 8
	minimumMySQLMaxOpenConns = 2
)

const mysqlDSNPrefix = "mysql://"

// PoolSettings 是打开层消费的连接池预算（不进入驱动 DSN）。
type PoolSettings struct {
	MaxOpen         int
	MaxIdle         int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// Spec 是存储 DSN 的解析结果：驱动、驱动就绪 DSN、池预算与 MySQL 连接身份。
type Spec struct {
	Driver    string
	DriverDSN string
	// MySQL 身份（SQLite 时为零值）。
	Host     string
	Database string
	Pool     PoolSettings
	budget   *ConnectionBudget

	mysqlCfg *mysql.Config
}

// WithDatabase 返回换库后的同身份 Spec（租户库句柄与开通连接派生用）。
func (s Spec) WithDatabase(name string) Spec {
	if s.Driver != DriverMySQL || s.mysqlCfg == nil {
		return s
	}
	next := s
	cfg := s.mysqlCfg.Clone()
	cfg.DBName = name
	next.mysqlCfg = cfg
	next.DriverDSN = cfg.FormatDSN()
	next.Database = name
	return next
}

// WithPool 返回连接池预算被替换后的 Spec（租户库按总量预算收敛单库连接数）。
func (s Spec) WithPool(pool PoolSettings) Spec {
	next := s
	next.Pool = pool
	return next
}

func (s Spec) WithConnectionBudget(budget *ConnectionBudget) Spec {
	next := s
	next.budget = budget
	return next
}

var databaseNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)

// ValidateDatabaseName 是租户库名进入 DDL/DSN 前的标识符白名单。
func ValidateDatabaseName(name string) error {
	if !databaseNamePattern.MatchString(name) {
		return fmt.Errorf("invalid database name %q: must match %s", name, databaseNamePattern.String())
	}
	return nil
}

func ParseDSN(raw string) (Spec, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Spec{}, fmt.Errorf("empty storage dsn")
	}
	if !strings.HasPrefix(trimmed, mysqlDSNPrefix) {
		return Spec{Driver: DriverSQLite, DriverDSN: raw}, nil
	}
	return parseMySQLDSN(trimmed)
}

func parseMySQLDSN(raw string) (Spec, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return Spec{}, fmt.Errorf("parse mysql dsn: %w", err)
	}
	if parsed.User == nil || parsed.User.Username() == "" {
		return Spec{}, fmt.Errorf("mysql dsn requires a user (mysql://user:pass@host:port/dbname)")
	}
	password, _ := parsed.User.Password()

	host := parsed.Hostname()
	if host == "" {
		return Spec{}, fmt.Errorf("mysql dsn requires a host")
	}
	port := parsed.Port()
	if port == "" {
		port = "3306"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return Spec{}, fmt.Errorf("mysql dsn has an invalid port %q", port)
	}

	database := strings.TrimPrefix(parsed.Path, "/")
	if database != "" {
		if err := ValidateDatabaseName(database); err != nil {
			return Spec{}, err
		}
	}
	if parsed.Fragment != "" {
		return Spec{}, fmt.Errorf("mysql dsn must not carry a fragment")
	}

	cfg := mysql.NewConfig()
	cfg.User = parsed.User.Username()
	cfg.Passwd = password
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(host, port)
	cfg.DBName = database
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Collation = "utf8mb4_bin"
	cfg.Params = map[string]string{"time_zone": "'+00:00'"}

	pool := PoolSettings{
		MaxOpen:         DefaultMySQLMaxOpenConns,
		MaxIdle:         DefaultMySQLMaxIdleConns,
		ConnMaxLifetime: 30 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
	}

	query := parsed.Query()
	for key, values := range query {
		if len(values) != 1 {
			return Spec{}, fmt.Errorf("mysql dsn parameter %q must appear exactly once", key)
		}
		value := values[0]
		switch key {
		case "max_open_conns", "max_idle_conns":
			n, err := strconv.Atoi(value)
			if err != nil {
				return Spec{}, fmt.Errorf("mysql dsn parameter %s=%q is not an integer", key, value)
			}
			if key == "max_open_conns" {
				pool.MaxOpen = n
			} else {
				pool.MaxIdle = n
			}
		case "conn_max_lifetime", "conn_max_idle_time":
			d, err := time.ParseDuration(value)
			if err != nil || d < 0 {
				return Spec{}, fmt.Errorf("mysql dsn parameter %s=%q is not a duration", key, value)
			}
			if key == "conn_max_lifetime" {
				pool.ConnMaxLifetime = d
			} else {
				pool.ConnMaxIdleTime = d
			}
		case "timeout", "read_timeout", "write_timeout":
			d, err := time.ParseDuration(value)
			if err != nil || d <= 0 {
				return Spec{}, fmt.Errorf("mysql dsn parameter %s=%q is not a positive duration", key, value)
			}
			switch key {
			case "timeout":
				cfg.Timeout = d
			case "read_timeout":
				cfg.ReadTimeout = d
			case "write_timeout":
				cfg.WriteTimeout = d
			}
		case "collation":
			if strings.TrimSpace(value) == "" {
				return Spec{}, fmt.Errorf("mysql dsn parameter collation must not be empty")
			}
			cfg.Collation = value
		case "tls":
			cfg.TLSConfig = value
		default:
			return Spec{}, fmt.Errorf("mysql dsn has unknown parameter %q (supported: max_open_conns, max_idle_conns, conn_max_lifetime, conn_max_idle_time, timeout, read_timeout, write_timeout, collation, tls)", key)
		}
	}
	if pool.MaxOpen < minimumMySQLMaxOpenConns {
		return Spec{}, fmt.Errorf("max_open_conns=%d is below the minimum %d required by the cross-instance migration lock", pool.MaxOpen, minimumMySQLMaxOpenConns)
	}
	if pool.MaxIdle < 1 {
		return Spec{}, fmt.Errorf("max_idle_conns=%d must be at least 1", pool.MaxIdle)
	}
	if pool.MaxIdle > pool.MaxOpen {
		return Spec{}, fmt.Errorf("max_idle_conns=%d exceeds max_open_conns=%d", pool.MaxIdle, pool.MaxOpen)
	}

	return Spec{
		Driver:    DriverMySQL,
		DriverDSN: cfg.FormatDSN(),
		Host:      cfg.Addr,
		Database:  database,
		Pool:      pool,
		mysqlCfg:  cfg,
	}, nil
}
