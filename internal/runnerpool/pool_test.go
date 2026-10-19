package runnerpool

import (
	"context"
	"errors"
	"sync"
	"testing"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
	"sdlc-coordinator/internal/runner"
)

// 租户 runner 池：任务 → 租户 → 池 → 池内轮询。
// 头条断言：租户 A 的池不含租户 B 的端点（派发隔离）；未指派
// 租户回退默认池（单 runner 存量部署行为等价）。

type fakePoolLookup struct {
	mu    sync.Mutex
	pools map[uint][]controlplane.RunnerAssignment
	slots map[uint]uint64
	err   error
}

func (f *fakePoolLookup) ClaimRunnerPool(_ context.Context, tenantID uint) ([]controlplane.RunnerAssignment, uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, 0, f.err
	}
	if f.slots == nil {
		f.slots = make(map[uint]uint64)
	}
	slot := f.slots[tenantID]
	f.slots[tenantID]++
	return append([]controlplane.RunnerAssignment(nil), f.pools[tenantID]...), slot, nil
}

type fakePoolSource struct {
	mu    sync.Mutex
	pools map[uint][]string
	slots map[uint]uint64
	err   error
}

func (f *fakePoolSource) ClaimPool(_ context.Context, tenantID uint) ([]string, uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, 0, f.err
	}
	if f.slots == nil {
		f.slots = make(map[uint]uint64)
	}
	slot := f.slots[tenantID]
	f.slots[tenantID]++
	return append([]string(nil), f.pools[tenantID]...), slot, nil
}

type fakeTaskTenants struct {
	tenant uint
	err    error
}

func (f *fakeTaskTenants) ResolveTenant(context.Context, *model.Task) (uint, error) {
	return f.tenant, f.err
}

type recordingPoolClient struct{ endpoint string }

func (c *recordingPoolClient) Create(context.Context, runner.CreateExecutionRequest) (*runner.Execution, error) {
	return &runner.Execution{ExecutionID: "exec"}, nil
}

func (c *recordingPoolClient) Get(_ context.Context, executionID string) (*runner.Execution, error) {
	return &runner.Execution{ExecutionID: executionID}, nil
}

func (c *recordingPoolClient) Events(_ context.Context, executionID string, _ int64) ([]runner.Event, error) {
	return nil, nil
}

func (c *recordingPoolClient) Result(_ context.Context, executionID string) (*runner.Result, error) {
	return &runner.Result{ExecutionID: executionID}, nil
}

func (c *recordingPoolClient) Cancel(_ context.Context, executionID string) (*runner.Execution, error) {
	return &runner.Execution{ExecutionID: executionID}, nil
}

func newTestRouter(tenants TaskTenantResolver, source PoolSource) *Router {
	return NewRouter(tenants, source, func(endpoint string) orchestrator.ReviewerRunnerClient {
		return &recordingPoolClient{endpoint: endpoint}
	})
}

func TestStaticTenantResolverAlwaysYieldsNoTenant(t *testing.T) {
	// 占位语义=恒「无租户」（tenantID=0），派发回退默认池；
	// 真实现（仓→租户）归后续批次。
	tenant, err := StaticTenantResolver{}.ResolveTenant(context.Background(), &model.Task{TaskID: "task-1"})
	if err != nil || tenant != 0 {
		t.Fatalf("static resolver must yield no tenant, got tenant=%d err=%v", tenant, err)
	}
}

func TestRouterRoundRobinsWithinTenantPool(t *testing.T) {
	source := &fakePoolSource{pools: map[uint][]string{7: {"http://runner-a", "http://runner-b"}}}
	router := newTestRouter(&fakeTaskTenants{tenant: 7}, source)
	seen := make([]string, 0, 4)
	for i := 0; i < 4; i++ {
		attempt, err := router.AttemptForTask(context.Background(), &model.Task{TaskID: "task"})
		if err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
		seen = append(seen, attempt.Endpoint)
	}
	want := []string{"http://runner-a", "http://runner-b", "http://runner-a", "http://runner-b"}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("round robin drifted at %d: got %v", i, seen)
		}
	}
}

func TestRoutersShareTenantPoolCursor(t *testing.T) {
	source := &fakePoolSource{pools: map[uint][]string{7: {"http://runner-a", "http://runner-b"}}}
	first := newTestRouter(&fakeTaskTenants{tenant: 7}, source)
	second := newTestRouter(&fakeTaskTenants{tenant: 7}, source)
	for _, item := range []struct {
		router *Router
		want   string
	}{{first, "http://runner-a"}, {second, "http://runner-b"}} {
		attempt, err := item.router.AttemptForTask(context.Background(), &model.Task{TaskID: "task"})
		if err != nil || attempt.Endpoint != item.want {
			t.Fatalf("shared pool slot selected %q, want %q, err=%v", attempt.Endpoint, item.want, err)
		}
	}
}

// 默认池是单端点常量：派发不该为它开游标事务。
func TestDefaultPoolDoesNotClaimCursor(t *testing.T) {
	lookup := &fakePoolLookup{}
	source := NewControlPlanePoolSource(lookup, []string{"http://default-runner"})
	if _, slot, err := source.ClaimPool(context.Background(), 0); err != nil || slot != 0 {
		t.Fatalf("no-tenant claim = slot %d, err %v", slot, err)
	}
	lookup.mu.Lock()
	defer lookup.mu.Unlock()
	if len(lookup.slots) != 0 {
		t.Fatalf("default pool dispatch consumed cursor slots: %v", lookup.slots)
	}
}

func TestRouterKeepsTenantPoolsDisjoint(t *testing.T) {
	source := &fakePoolSource{pools: map[uint][]string{
		7: {"http://runner-a7", "http://runner-b7"},
		8: {"http://runner-a8", "http://runner-b8"},
	}}
	router := newTestRouter(&fakeTaskTenants{tenant: 7}, source)
	tenant8 := newTestRouter(&fakeTaskTenants{tenant: 8}, source)
	for i := 0; i < 6; i++ {
		attempt, err := router.AttemptForTask(context.Background(), &model.Task{TaskID: "task-7"})
		if err != nil {
			t.Fatalf("tenant 7 attempt: %v", err)
		}
		if attempt.Endpoint != "http://runner-a7" && attempt.Endpoint != "http://runner-b7" {
			t.Fatalf("tenant 7 pool leaked endpoint %q", attempt.Endpoint)
		}
		other, err := tenant8.AttemptForTask(context.Background(), &model.Task{TaskID: "task-8"})
		if err != nil {
			t.Fatalf("tenant 8 attempt: %v", err)
		}
		if other.Endpoint != "http://runner-a8" && other.Endpoint != "http://runner-b8" {
			t.Fatalf("tenant 8 pool leaked endpoint %q", other.Endpoint)
		}
	}
}

func TestRouterAttemptNextWalksRemainingEndpoints(t *testing.T) {
	source := &fakePoolSource{pools: map[uint][]string{7: {"http://a", "http://b", "http://c"}}}
	router := newTestRouter(&fakeTaskTenants{tenant: 7}, source)
	attempt, err := router.AttemptForTask(context.Background(), &model.Task{TaskID: "task"})
	if err != nil {
		t.Fatalf("first attempt: %v", err)
	}
	if attempt.Endpoint != "http://a" {
		t.Fatalf("unexpected first endpoint %q", attempt.Endpoint)
	}
	second, ok := attempt.Next()
	if !ok || second.Endpoint != "http://b" {
		t.Fatalf("unexpected second attempt: %q ok=%v", second.Endpoint, ok)
	}
	third, ok := second.Next()
	if !ok || third.Endpoint != "http://c" {
		t.Fatalf("unexpected third attempt: %q ok=%v", third.Endpoint, ok)
	}
	if _, ok := third.Next(); ok {
		t.Fatalf("pool should be exhausted after all endpoints")
	}
}

func TestRouterAttemptWrapsAfterLastEndpoint(t *testing.T) {
	source := &fakePoolSource{pools: map[uint][]string{7: {"http://a", "http://b"}}, slots: map[uint]uint64{7: 1}}
	router := newTestRouter(&fakeTaskTenants{tenant: 7}, source)
	attempt, err := router.AttemptForTask(context.Background(), &model.Task{TaskID: "task"})
	if err != nil || attempt.Endpoint != "http://b" {
		t.Fatalf("first attempt = %q, err=%v", attempt.Endpoint, err)
	}
	next, ok := attempt.Next()
	if !ok || next.Endpoint != "http://a" {
		t.Fatalf("fallback attempt = %q, ok=%v", next.Endpoint, ok)
	}
	if _, ok := next.Next(); ok {
		t.Fatal("pool traversal must stop after each endpoint is tried once")
	}
}

func TestRouterRejectsEmptyPool(t *testing.T) {
	router := newTestRouter(&fakeTaskTenants{tenant: 7}, &fakePoolSource{pools: map[uint][]string{7: {}}})
	if _, err := router.AttemptForTask(context.Background(), &model.Task{TaskID: "task"}); err == nil {
		t.Fatalf("empty pool must fail dispatch")
	}
}

func TestRouterSurfacesResolutionFailures(t *testing.T) {
	router := newTestRouter(&fakeTaskTenants{err: errors.New("tenant lookup failed")}, &fakePoolSource{})
	if _, err := router.AttemptForTask(context.Background(), &model.Task{TaskID: "task"}); err == nil {
		t.Fatalf("tenant resolution failure must abort dispatch")
	}
	router = newTestRouter(&fakeTaskTenants{tenant: 7}, &fakePoolSource{err: errors.New("control plane unavailable")})
	if _, err := router.AttemptForTask(context.Background(), &model.Task{TaskID: "task"}); err == nil {
		t.Fatalf("pool resolution failure must abort dispatch")
	}
}

func TestRouterPinsClientForEndpoint(t *testing.T) {
	router := newTestRouter(&fakeTaskTenants{tenant: 7}, &fakePoolSource{pools: map[uint][]string{7: {"http://runner-a"}}})
	if _, err := router.AttemptForTask(context.Background(), &model.Task{TaskID: "task"}); err != nil {
		t.Fatalf("attempt: %v", err)
	}
	pinned, ok := router.ClientForEndpoint("http://runner-a")
	if !ok {
		t.Fatalf("known endpoint should pin a client")
	}
	if pinned.(*recordingPoolClient).endpoint != "http://runner-a" {
		t.Fatalf("pinned client bound to wrong endpoint")
	}
	if _, ok := router.ClientForEndpoint(""); ok {
		t.Fatalf("empty endpoint must not pin a client")
	}
}

func TestControlPlanePoolSourceFallsBackToDefaultPool(t *testing.T) {
	// A1 行为等价锚点：无租户（0）或未指派租户 → 默认池；指派租户 → 控制面
	// 行（保序）；查询失败透传。
	source := NewControlPlanePoolSource(&fakePoolLookup{
		pools: map[uint][]controlplane.RunnerAssignment{
			7: {
				{TenantID: 7, EndpointURL: "http://second"},
				{TenantID: 7, EndpointURL: "http://first"},
			},
		},
	}, []string{"http://default-runner"})

	endpoints, slot, err := source.ClaimPool(context.Background(), 0)
	if err != nil || slot != 0 || len(endpoints) != 1 || endpoints[0] != "http://default-runner" {
		t.Fatalf("no-tenant dispatch must use default pool: %v %d %v", endpoints, slot, err)
	}
	endpoints, slot, err = source.ClaimPool(context.Background(), 9)
	if err != nil || slot != 0 || len(endpoints) != 1 || endpoints[0] != "http://default-runner" {
		t.Fatalf("unassigned tenant must use default pool: %v %d %v", endpoints, slot, err)
	}
	endpoints, _, err = source.ClaimPool(context.Background(), 7)
	if err != nil || len(endpoints) != 2 || endpoints[0] != "http://second" || endpoints[1] != "http://first" {
		t.Fatalf("assigned tenant must use control plane rows in order: %v %v", endpoints, err)
	}
	endpoints[0] = "mutated"
	again, _, err := source.ClaimPool(context.Background(), 7)
	if err != nil || again[0] != "http://second" {
		t.Fatalf("pool source must hand out copies: %v %v", again, err)
	}
	source = NewControlPlanePoolSource(&fakePoolLookup{err: errors.New("db down")}, []string{"http://default-runner"})
	if _, _, err := source.ClaimPool(context.Background(), 7); err == nil {
		t.Fatalf("lookup failure must surface")
	}
}
