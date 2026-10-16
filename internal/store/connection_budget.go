package store

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

type ConnectionBudget struct {
	mu      sync.Mutex
	limit   int
	slots   chan struct{}
	waiters int
	purging bool
	network string
	pools   map[*sql.DB]int
}

type budgetNetConn struct {
	net.Conn
	once    sync.Once
	release func()
}

var nextConnectionBudgetID atomic.Uint64

func NewConnectionBudget(limit int) (*ConnectionBudget, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("connection budget must be positive")
	}
	budget := &ConnectionBudget{
		limit:   limit,
		slots:   make(chan struct{}, limit),
		network: fmt.Sprintf("sdlc-budget-%d", nextConnectionBudgetID.Add(1)),
		pools:   make(map[*sql.DB]int),
	}
	mysqlDriver.RegisterDialContext(budget.network, budget.dialContext)
	return budget, nil
}

func (budget *ConnectionBudget) DialectConfig(cfg *mysqlDriver.Config) *mysqlDriver.Config {
	clone := cfg.Clone()
	clone.Net = budget.network
	return clone
}

func (budget *ConnectionBudget) registerDB(db *sql.DB, maxIdle int) {
	budget.mu.Lock()
	defer budget.mu.Unlock()
	budget.pools[db] = maxIdle
	if budget.purging {
		db.SetMaxIdleConns(0)
	}
}

func (budget *ConnectionBudget) unregisterDB(db *sql.DB) {
	budget.mu.Lock()
	delete(budget.pools, db)
	budget.mu.Unlock()
}

func (budget *ConnectionBudget) UnregisterDB(db *sql.DB) {
	budget.unregisterDB(db)
}

func (budget *ConnectionBudget) acquire(ctx context.Context) error {
	budget.mu.Lock()
	select {
	case budget.slots <- struct{}{}:
		budget.mu.Unlock()
		return nil
	default:
		budget.waiters++
		if !budget.purging {
			budget.setIdleModeLocked(0)
			budget.purging = true
		}
	}
	budget.mu.Unlock()

	select {
	case budget.slots <- struct{}{}:
		budget.finishWaiter()
		return nil
	case <-ctx.Done():
		budget.finishWaiter()
		return ctx.Err()
	}
}

func (budget *ConnectionBudget) release() {
	<-budget.slots
}

func (budget *ConnectionBudget) finishWaiter() {
	budget.mu.Lock()
	defer budget.mu.Unlock()
	budget.waiters--
	if budget.waiters == 0 && budget.purging {
		budget.setIdleModeLocked(-1)
		budget.purging = false
	}
}

func (budget *ConnectionBudget) setIdleModeLocked(mode int) {
	for db, maxIdle := range budget.pools {
		if mode < 0 {
			db.SetMaxIdleConns(maxIdle)
		} else {
			db.SetMaxIdleConns(mode)
		}
	}
}

func (budget *ConnectionBudget) dialContext(ctx context.Context, address string) (net.Conn, error) {
	if err := budget.acquire(ctx); err != nil {
		return nil, err
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		budget.release()
		return nil, err
	}
	return &budgetNetConn{Conn: conn, release: budget.release}, nil
}

func (conn *budgetNetConn) Close() error {
	err := conn.Conn.Close()
	conn.once.Do(conn.release)
	return err
}
