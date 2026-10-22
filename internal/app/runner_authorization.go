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

// developerDeliveryLeaseScopeForAuthorization 必须与 orchestrator 的交付执行权
// 作用域常量一致（"developer-delivery:"+runID）；本包不 import 编排器，字面
// 重复由两侧的集成测试锁定。
const developerDeliveryLeaseScopeForAuthorization = "developer-delivery:"

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
		// 执行权身份校验（接管隔离）：请求必须携带当前交付租约的 owner+
		// generation 且与库内一致——B 接管同一 run 后，A 的新 commit/push
		// 请求业务字段仍满足但执行权已失效，必须拒绝。未携带身份（旧形态
		// 请求）同样拒绝：状态改变类操作不接受无执行权的匿名请求。
		if strings.TrimSpace(request.Owner) == "" {
			return deny("delivery execution right is required for commit and push")
		}
		valid, err := repository.NewWorkItemLeaseRepository(db).Verify(ctx, developerDeliveryLeaseScopeForAuthorization+run.RunID, request.Owner, request.Generation)
		if err != nil {
			return workspaceproto.AuthorizationDecision{}, fmt.Errorf("verify delivery execution right: %w", err)
		}
		if !valid {
			return deny("delivery execution right is missing, expired, or held by another owner")
		}
		return workspaceproto.AuthorizationDecision{Authorized: true}, nil
	case workspaceproto.KindCleanup:
		stops := repository.NewStopRepository(db)
		var stopCount int64
		if err := db.WithContext(ctx).Model(&model.StopRequest{}).Where("run_id = ?", run.RunID).Count(&stopCount).Error; err != nil {
			return workspaceproto.AuthorizationDecision{}, err
		}
		if stopCount != 0 || run.Status == model.RunStatusInterrupted || (current && task.Status == model.TaskStatusInterrupted) {
			ready, err := stops.CleanupReady(ctx, run.RunID)
			if err != nil {
				return workspaceproto.AuthorizationDecision{}, err
			}
			if !ready {
				return deny("stopped run has unresolved execution or external operations")
			}
			return workspaceproto.AuthorizationDecision{Authorized: true}, nil
		}
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
