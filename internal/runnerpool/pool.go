// Package runnerpool 实现租户 runner 池：任务 →
// 租户 → 端点池 → 池内轮询。控制面 runner_assignments 是唯一数据源；未指派
// 租户回退默认池（单 runner 存量部署行为等价）。隔离红线：同一租户的池互
// 不交叉，跨租户端点重合由启动校验拒绝。
package runnerpool

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/orchestrator"
)

// TaskTenantResolver 解析任务所属租户（派发路由第一跳）。
type TaskTenantResolver interface {
	ResolveTenant(ctx context.Context, task *model.Task) (uint, error)
}

// StaticTenantResolver 是占位实现：恒返回「无租户」（tenantID=0）→
// 池回退默认池（占位语义=无租户；真实现仓→租户查 repo_route_index 归
// 后续批次）。这是既有 SharedMainResolver 同款「结构先行、行为等价」模式。
type StaticTenantResolver struct{}

func (StaticTenantResolver) ResolveTenant(context.Context, *model.Task) (uint, error) {
	return 0, nil
}

// PoolSource 返回租户的 runner 端点池（保序）。
type PoolSource interface {
	Endpoints(ctx context.Context, tenantID uint) ([]string, error)
}

// PoolLookup 是控制面指派查询的窄端口；*controlplane.Service 天然满足。
type PoolLookup interface {
	ListRunnerAssignments(ctx context.Context, tenantID uint) ([]controlplane.RunnerAssignment, error)
}

// ControlPlanePoolSource 租户池数据源：无租户（0）或未指派租户 → 默认池
// （副本交付，调用方改动不回写）；指派租户 → 控制面行保序。
type ControlPlanePoolSource struct {
	lookup   PoolLookup
	fallback []string
}

func NewControlPlanePoolSource(lookup PoolLookup, fallback []string) *ControlPlanePoolSource {
	return &ControlPlanePoolSource{lookup: lookup, fallback: fallback}
}

func (s *ControlPlanePoolSource) Endpoints(ctx context.Context, tenantID uint) ([]string, error) {
	if tenantID == 0 {
		return s.defaultPool(), nil
	}
	rows, err := s.lookup.ListRunnerAssignments(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return s.defaultPool(), nil
	}
	endpoints := make([]string, 0, len(rows))
	for _, row := range rows {
		endpoints = append(endpoints, row.EndpointURL)
	}
	return endpoints, nil
}

func (s *ControlPlanePoolSource) defaultPool() []string {
	pool := make([]string, len(s.fallback))
	copy(pool, s.fallback)
	return pool
}

// Router 是 orchestrator.TenantRunnerRouter 的池实现：每租户独立 round-robin
// 游标（内存即可，进程重启从头轮询可接受）；端点客户端按 URL 缓存复用。
type Router struct {
	mu      sync.Mutex
	cursors map[uint]int
	clients map[string]orchestrator.ReviewerRunnerClient
	tenants TaskTenantResolver
	source  PoolSource
	factory func(endpoint string) orchestrator.ReviewerRunnerClient
}

func NewRouter(tenants TaskTenantResolver, source PoolSource, factory func(endpoint string) orchestrator.ReviewerRunnerClient) *Router {
	return &Router{cursors: map[uint]int{}, clients: map[string]orchestrator.ReviewerRunnerClient{}, tenants: tenants, source: source, factory: factory}
}

func (r *Router) AttemptForTask(ctx context.Context, task *model.Task) (orchestrator.RunnerAttempt, error) {
	if r == nil {
		return orchestrator.RunnerAttempt{}, errors.New("runner pool router is not configured")
	}
	tenantID, err := r.tenants.ResolveTenant(ctx, task)
	if err != nil {
		return orchestrator.RunnerAttempt{}, fmt.Errorf("resolve task tenant: %w", err)
	}
	endpoints, err := r.source.Endpoints(ctx, tenantID)
	if err != nil {
		return orchestrator.RunnerAttempt{}, fmt.Errorf("resolve runner pool: %w", err)
	}
	if len(endpoints) == 0 {
		return orchestrator.RunnerAttempt{}, errors.New("runner pool is empty")
	}
	return r.attempt(tenantID, endpoints), nil
}

func (r *Router) ClientForEndpoint(endpoint string) (orchestrator.ReviewerRunnerClient, bool) {
	if r == nil {
		return nil, false
	}
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil, false
	}
	return r.client(endpoint), true
}

// attempt 声明游标取首个端点；Next() 在本池内依次推进，不再次声明游标
// （一次派发只消费一个游标位，失败轮换不干扰后续派发的分布）。
func (r *Router) attempt(tenantID uint, endpoints []string) orchestrator.RunnerAttempt {
	index := r.claimCursor(tenantID, len(endpoints))
	var build func(i int) (orchestrator.RunnerAttempt, bool)
	build = func(i int) (orchestrator.RunnerAttempt, bool) {
		if i >= len(endpoints) {
			return orchestrator.RunnerAttempt{}, false
		}
		return orchestrator.NewRunnerAttempt(
			r.client(endpoints[i]),
			endpoints[i],
			func() (orchestrator.RunnerAttempt, bool) { return build(i + 1) },
		), true
	}
	attempt, _ := build(index)
	return attempt
}

func (r *Router) claimCursor(tenantID uint, size int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	index := r.cursors[tenantID] % size
	r.cursors[tenantID] = (r.cursors[tenantID] + 1) % size
	return index
}

func (r *Router) client(endpoint string) orchestrator.ReviewerRunnerClient {
	r.mu.Lock()
	defer r.mu.Unlock()
	if client, ok := r.clients[endpoint]; ok {
		return client
	}
	client := r.factory(endpoint)
	r.clients[endpoint] = client
	return client
}
