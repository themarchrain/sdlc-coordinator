package store

import (
	"database/sql/driver"
	"errors"
	"io"
	"net"
	"strings"

	sqlite "github.com/glebarez/go-sqlite"
	"github.com/go-sql-driver/mysql"
)

// 数据库错误分类：按驱动错误码判定，不依赖错误文本。
// MySQL 服务端码与 go-sql-driver 客户端码（CR_* 2xxx 段）分开登记。
const (
	mysqlErrDupEntry           = 1062 // ER_DUP_ENTRY
	mysqlErrDupEntryWithKey    = 1586 // ER_DUP_ENTRY_WITH_KEY_NAME
	mysqlErrLockDeadlock       = 1213 // ER_LOCK_DEADLOCK
	mysqlErrLockWaitTimeout    = 1205 // ER_LOCK_WAIT_TIMEOUT
	mysqlErrTooManyConnections = 1040 // ER_CON_COUNT_ERROR
	mysqlErrBadHost            = 1042 // ER_BAD_HOST_ERROR
	mysqlErrUnknownDatabase    = 1049 // ER_BAD_DB_ERROR
	mysqlErrServerShutdown     = 1053 // ER_SERVER_SHUTDOWN
	mysqlErrClientConn         = 2002 // CR_CONNECTION_ERROR
	mysqlErrClientHost         = 2003 // CR_CONN_HOST_ERROR
	mysqlErrClientLostConn     = 2006 // CR_SERVER_GONE_ERROR
	mysqlErrClientLostConn2    = 2013 // CR_SERVER_LOST

	sqliteErrConstraintPrimaryKey = 1555 // SQLITE_CONSTRAINT_PRIMARYKEY
	sqliteErrConstraintUnique     = 2067 // SQLITE_CONSTRAINT_UNIQUE
)

// IsDuplicateKey 判定唯一约束冲突（重复键）。判真才可用于输给并发方
// 之类的语义分支；判错方向必须保守。
func IsDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case mysqlErrDupEntry, mysqlErrDupEntryWithKey:
			return true
		}
	}
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		// 只认扩展码（唯一/主键冲突）；基础码 19 覆盖外键等全部约束类型，
		// 误判成重复键会走错语义分支。
		switch sqliteErr.Code() {
		case sqliteErrConstraintPrimaryKey, sqliteErrConstraintUnique:
			return true
		}
	}
	return false
}

// IsDeadlock 判定死锁（可安全重试：MySQL 已回滚整个事务）。
func IsDeadlock(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == mysqlErrLockDeadlock
	}
	return false
}

// IsLockWaitTimeout 判定行锁等待超时（可重试，但需有界退避）。
func IsLockWaitTimeout(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == mysqlErrLockWaitTimeout
	}
	return false
}

// IsUnknownDatabase 判定目标库不存在（MySQL 1049）：租户库解析据此区分
// 库未开通/被删除与其他连接或权限故障。
func IsUnknownDatabase(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == mysqlErrUnknownDatabase
	}
	return false
}

// IsConnectionError 判定连接层故障（服务端不可达/连接池耗尽/链路中断）。
// 共享库不可用期间不得取得新执行权或派发新副作用。
func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, driver.ErrBadConn) || errors.Is(err, mysql.ErrInvalidConn) {
		return true
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case mysqlErrTooManyConnections, mysqlErrBadHost, mysqlErrServerShutdown,
			mysqlErrClientConn, mysqlErrClientHost, mysqlErrClientLostConn, mysqlErrClientLostConn2:
			return true
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}
	return false
}

// IsRetryable 判定可重试的存储层瞬时错误（死锁、锁等待超时、连接故障）。
func IsRetryable(err error) bool {
	return IsDeadlock(err) || IsLockWaitTimeout(err) || IsConnectionError(err)
}

func RedactError(message string, cause error, secrets ...string) error {
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret == "" || !strings.Contains(message, secret) {
			continue
		}
		message = strings.ReplaceAll(message, secret, "***")
	}
	return &redactedError{message: message, cause: cause}
}

type redactedError struct {
	message string
	cause   error
}

func (e *redactedError) Error() string { return e.message }
func (e *redactedError) Unwrap() error { return e.cause }
