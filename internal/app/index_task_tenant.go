package app

import (
	"context"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
)

// indexTaskTenantResolver 是 TaskTenantResolver 的真实现：
// 任务 → 注册仓行（机器人通道的平台库仓行）→ 控制面路由索引 → 租户。
// StaticTenantResolver（恒无租户）由本实现替换后，派发按真租户过滤
// 池；未指派租户仍回退默认池（未在索引命中的租户组合不在此层处理）。
//
// fail-closed：任务未绑定仓、仓行缺失、索引 miss 一律返回错误拒派发——
// 宁可不派发，不派到错误的租户池（隔离头条）。
type indexTaskTenantResolver struct {
	index *controlplane.Repository
	repos *registeredrepo.Store
}

func (r *indexTaskTenantResolver) ResolveTenant(ctx context.Context, task *model.Task) (uint, error) {
	if task == nil || task.RepositoryID == nil || strings.TrimSpace(*task.RepositoryID) == "" {
		return 0, fmt.Errorf("task %s is not bound to a repository; refusing dispatch", taskIdentity(task))
	}
	repo, err := r.repos.Get(ctx, strings.TrimSpace(*task.RepositoryID))
	if err != nil {
		return 0, fmt.Errorf("lookup repository for dispatch: %w", err)
	}
	routes, err := r.index.GetRepoRouteIndexByPath(ctx, repo.Provider, repo.PathWithNamespace)
	if err != nil {
		return 0, fmt.Errorf("resolve repo route: %w", err)
	}
	// 身份精确匹配（provider+base_url+path）优先；仅 path 命中多行且无一
	// 精确匹配时视为歧义 fail-closed。
	var exact *controlplane.RepoRouteIndex
	for i := range routes {
		if routes[i].BaseURL == repo.BaseURL {
			exact = &routes[i]
			break
		}
	}
	if exact == nil {
		if len(routes) == 1 {
			exact = &routes[0]
		} else {
			return 0, fmt.Errorf("no repo route for %s/%s; refusing dispatch", repo.Provider, repo.PathWithNamespace)
		}
	}
	if exact.TenantID == 0 {
		return 0, fmt.Errorf("repo route for %s/%s has no tenant; refusing dispatch", repo.Provider, repo.PathWithNamespace)
	}
	return exact.TenantID, nil
}

func taskIdentity(task *model.Task) string {
	if task == nil {
		return ""
	}
	return task.TaskID
}
