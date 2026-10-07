package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/controlplane"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store/repository"
	"sdlc-coordinator/internal/tenantdb"
	"sdlc-coordinator/internal/workspaceproto"

	"gorm.io/gorm"
)

type tenantRunnerAuthorizer struct {
	control  *controlplane.Service
	resolver tenantdb.Resolver
}

func (a *tenantRunnerAuthorizer) AuthorizeRunnerWorkspaceOperation(ctx context.Context, request workspaceproto.AuthorizationRequest) (workspaceproto.AuthorizationDecision, error) {
	deny := func(reason string) (workspaceproto.AuthorizationDecision, error) {
		return workspaceproto.AuthorizationDecision{Authorized: false, Reason: reason}, nil
	}
	if request.TenantID == 0 || strings.TrimSpace(request.TaskID) == "" || strings.TrimSpace(request.RunID) == "" {
		return deny("authorization request identity is incomplete")
	}
	tenant, err := a.control.Repository().GetTenant(ctx, request.TenantID)
	if err != nil {
		if errors.Is(err, controlplane.ErrTenantNotFound) {
			return deny("tenant is unknown")
		}
		return workspaceproto.AuthorizationDecision{}, fmt.Errorf("load tenant for authorization: %w", err)
	}
	if tenant.Status != controlplane.TenantStatusActive {
		return deny("tenant is disabled")
	}
	handle, err := a.resolver.Handle(ctx, request.TenantID)
	if err != nil {
		return workspaceproto.AuthorizationDecision{}, fmt.Errorf("resolve tenant store for authorization: %w", err)
	}
	return authorizeRunnerOperation(ctx, handle.DB, request)
}

// authorizeRunnerOperation 是与存储装配解耦的裁决核心（按租户库查询）。
func authorizeRunnerOperation(ctx context.Context, db *gorm.DB, request workspaceproto.AuthorizationRequest) (workspaceproto.AuthorizationDecision, error) {
	deny := func(reason string) (workspaceproto.AuthorizationDecision, error) {
		return workspaceproto.AuthorizationDecision{Authorized: false, Reason: reason}, nil
	}
	tasks := repository.NewTaskRepository(db)
	runs := repository.NewRunRepository(db)
	task, err := tasks.GetByTaskID(ctx, strings.TrimSpace(request.TaskID))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return deny("task is unknown")
		}
		return workspaceproto.AuthorizationDecision{}, fmt.Errorf("load task for authorization: %w", err)
	}
	run, err := runs.GetByRunID(ctx, strings.TrimSpace(request.RunID))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return deny("run is unknown")
		}
		return workspaceproto.AuthorizationDecision{}, fmt.Errorf("load run for authorization: %w", err)
	}
	if run.TaskID != task.TaskID {
		return deny("run does not belong to the task")
	}
	current := task.CurrentRunID != nil && *task.CurrentRunID == run.RunID

	switch request.Kind {
	case workspaceproto.KindDeveloperCommit, workspaceproto.KindDeveloperPush:
		if !current {
			return deny("run is no longer the current run of the task")
		}
		if task.Status != model.TaskStatusRunning && task.Status != model.TaskStatusFinalizing {
			return deny("task is not in a deliverable state")
		}
		return workspaceproto.AuthorizationDecision{Authorized: true}, nil
	case workspaceproto.KindCleanup:
		// 清理只允许非在途工作区：run 已终态，或 run 已不再是当前 run
		//（被接管/重试取代）。当前 run 仍在途时拒绝删除。
		terminal := run.Status == model.RunStatusSucceeded || run.Status == model.RunStatusFailed || run.Status == model.RunStatusBlocked
		if current && !terminal {
			return deny("run is still in flight and cannot be cleaned up")
		}
		return workspaceproto.AuthorizationDecision{Authorized: true}, nil
	default:
		return deny("operation kind is not eligible for authorization")
	}
}
