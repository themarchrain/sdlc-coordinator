package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"sync"
	"testing"
	"time"
)

func TestConnectionBudgetReclaimsIdleAndPreservesActiveConnections(t *testing.T) {
	budget, err := NewConnectionBudget(1)
	if err != nil {
		t.Fatalf("create connection budget: %v", err)
	}
	first := openBudgetTestDB(t, budget)
	second := openBudgetTestDB(t, budget)

	rows, err := first.QueryContext(context.Background(), "hold")
	if err != nil {
		t.Fatalf("open first query: %v", err)
	}
	if first.Stats().InUse != 1 {
		t.Fatalf("first pool in-use connections = %d, want 1", first.Stats().InUse)
	}

	result := make(chan error, 1)
	go func() {
		rows, err := second.QueryContext(context.Background(), "wait")
		if err == nil {
			err = rows.Close()
		}
		result <- err
	}()
	waitForBudgetWaiters(t, budget, 1)
	if err := rows.Close(); err != nil {
		t.Fatalf("release first active query: %v", err)
	}
	if err := <-result; err != nil {
		t.Fatalf("second query after first connection returned: %v", err)
	}
	if first.Stats().OpenConnections != 0 {
		t.Fatalf("first pool retained %d connections after pressure, want 0", first.Stats().OpenConnections)
	}
	if second.Stats().OpenConnections != 1 {
		t.Fatalf("second pool open connections = %d, want 1", second.Stats().OpenConnections)
	}
}

func TestConnectionBudgetWaitCancellationRestoresIdleCaps(t *testing.T) {
	budget, err := NewConnectionBudget(1)
	if err != nil {
		t.Fatalf("create connection budget: %v", err)
	}
	first := openBudgetTestDB(t, budget)
	second := openBudgetTestDB(t, budget)

	rows, err := first.QueryContext(context.Background(), "hold")
	if err != nil {
		t.Fatalf("open first query: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := second.QueryContext(ctx, "cancel"); err == nil {
		t.Fatal("second query succeeded while the only budget slot was held")
	}
	waitForBudgetWaiters(t, budget, 0)
	if err := rows.Close(); err != nil {
		t.Fatalf("release first active query: %v", err)
	}

	rows, err = second.QueryContext(context.Background(), "after-cancel")
	if err != nil {
		t.Fatalf("second query after cancelled waiter: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close second query: %v", err)
	}
	if first.Stats().Idle != 0 {
		t.Fatalf("first pool idle connections = %d, want 0 after pressure", first.Stats().Idle)
	}
}

func openBudgetTestDB(t *testing.T, budget *ConnectionBudget) *sql.DB {
	t.Helper()
	db := sql.OpenDB(&budgetTestConnector{budget: budget})
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	budget.registerDB(db, 1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func waitForBudgetWaiters(t *testing.T, budget *ConnectionBudget, expected int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		budget.mu.Lock()
		waiters := budget.waiters
		budget.mu.Unlock()
		if waiters == expected {
			return
		}
		time.Sleep(time.Millisecond)
	}
	budget.mu.Lock()
	waiters := budget.waiters
	budget.mu.Unlock()
	t.Fatalf("budget waiters = %d, want %d", waiters, expected)
}

type budgetTestConnector struct {
	budget *ConnectionBudget
}

func (connector *budgetTestConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := connector.budget.acquire(ctx); err != nil {
		return nil, err
	}
	return &budgetTestConn{budget: connector.budget}, nil
}

func (connector *budgetTestConnector) Driver() driver.Driver { return budgetTestDriver{} }

type budgetTestDriver struct{}

func (budgetTestDriver) Open(string) (driver.Conn, error) { return nil, driver.ErrSkip }

type budgetTestConn struct {
	budget *ConnectionBudget
	once   sync.Once
}

func (conn *budgetTestConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (conn *budgetTestConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (conn *budgetTestConn) Close() error {
	conn.once.Do(conn.budget.release)
	return nil
}
func (conn *budgetTestConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &budgetTestRows{}, nil
}

type budgetTestRows struct {
	read bool
}

func (rows *budgetTestRows) Columns() []string { return []string{"value"} }
func (rows *budgetTestRows) Close() error      { return nil }
func (rows *budgetTestRows) Next(dest []driver.Value) error {
	if rows.read {
		return io.EOF
	}
	rows.read = true
	dest[0] = int64(1)
	return nil
}
