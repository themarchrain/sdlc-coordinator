package controlplane

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// TenantSummary 是超管组织列表行（含成员计数；只含生命周期量级，不含业务
// 数据，跨租户聚合边界）。
type TenantSummary struct {
	ID          uint
	Slug        string
	DisplayName string
	Status      string
	MemberCount int64
	CreatedAt   time.Time
}

// AuditFilter 是审计查询条件；Page 从 1 起，PageSize 上限 100。
type AuditFilter struct {
	TenantID *uint
	Action   string
	Page     int
	PageSize int
}

// ProvisionedAdmin 是管理员开通结果；InitialPassword 仅新建账号时非空。
type ProvisionedAdmin struct {
	User            *User
	InitialPassword string
}

const (
	maxRunnerAssignments   = 20
	maxSupportReasonLength = 500
	defaultAuditPageSize   = 20
	maxAuditPageSize       = 100
)

// ListTenants 返回全部组织与成员计数（超管后台列表）。
func (service *Service) ListTenants(ctx context.Context) ([]TenantSummary, error) {
	return service.repo(ctx).ListTenantSummaries(ctx)
}

// ProvisionTenantAdmin 开通组织管理员：
// - 新用户名：建号 + 一次性初始密码 + 首登强制改密 + tenant_admin 入会；
// - 已有普通用户：直接授 tenant_admin（不动密码）；
// - 已是管理员：ErrMembershipExists；超管账号：ErrForbidden（永不入会）。
func (service *Service) ProvisionTenantAdmin(ctx context.Context, actor *Actor, tenantID uint, rawUsername string) (*ProvisionedAdmin, error) {
	username := strings.ToLower(strings.TrimSpace(rawUsername))
	if !usernamePattern.MatchString(username) {
		return nil, ErrInvalidInput
	}
	return inTxValue(service, ctx, func(ctx context.Context) (*ProvisionedAdmin, error) {
		if _, err := service.repo(ctx).GetTenant(ctx, tenantID); err != nil {
			return nil, err
		}
		existing, err := service.repo(ctx).GetUserByUsername(ctx, username)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			initial, err := generateInitialPassword()
			if err != nil {
				return nil, err
			}
			user, err := service.createUserWithRole(ctx, username, initial, UserRoleUser, true)
			if err != nil {
				return nil, err
			}
			if _, err := service.AddMembership(ctx, actor, tenantID, user.ID, RoleTenantAdmin); err != nil {
				return nil, err
			}
			if err := service.writeAudit(ctx, actor, tenantID, "tenant_admin.provisioned",
				fmt.Sprintf("mode=created user_id=%d username=%s", user.ID, username)); err != nil {
				return nil, err
			}
			return &ProvisionedAdmin{User: user, InitialPassword: initial}, nil
		}
		if existing.Role == UserRoleSuperAdmin {
			return nil, ErrForbidden
		}
		membership, membershipErr := service.repo(ctx).GetMembership(ctx, existing.ID, tenantID)
		if membershipErr == nil && membership != nil {
			if membership.Role == RoleTenantAdmin {
				return nil, ErrMembershipExists
			}
			membership.Role = RoleTenantAdmin
			if err := service.repo(ctx).SaveMembership(ctx, membership); err != nil {
				return nil, err
			}
			if err := service.repo(ctx).DeleteSessionsForUserTenant(ctx, existing.ID, tenantID); err != nil {
				return nil, err
			}
			if err := service.writeAudit(ctx, actor, tenantID, "tenant_admin.provisioned",
				fmt.Sprintf("mode=promoted user_id=%d username=%s", existing.ID, username)); err != nil {
				return nil, err
			}
			return &ProvisionedAdmin{User: existing}, nil
		} else if membershipErr != nil && !errors.Is(membershipErr, ErrMembershipNotFound) {
			return nil, membershipErr
		}
		if _, err := service.AddMembership(ctx, actor, tenantID, existing.ID, RoleTenantAdmin); err != nil {
			return nil, err
		}
		if err := service.writeAudit(ctx, actor, tenantID, "tenant_admin.provisioned",
			fmt.Sprintf("mode=granted user_id=%d username=%s", existing.ID, username)); err != nil {
			return nil, err
		}
		return &ProvisionedAdmin{User: existing}, nil
	})
}

// ResetTenantAdminPassword 超管重置组织管理员密码（密码重置链）。
// 仅 tenant_admin 角色可经此通道重置。
func (service *Service) ResetTenantAdminPassword(ctx context.Context, actor *Actor, tenantID, targetUserID uint) (string, error) {
	return inTxValue(service, ctx, func(ctx context.Context) (string, error) {
		membership, err := service.repo(ctx).GetMembership(ctx, targetUserID, tenantID)
		if err != nil {
			return "", err
		}
		if membership.Role != RoleTenantAdmin {
			return "", ErrForbidden
		}
		initial, err := generateInitialPassword()
		if err != nil {
			return "", err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(initial), bcrypt.DefaultCost)
		if err != nil {
			return "", fmt.Errorf("hash reset password: %w", err)
		}
		user, err := service.repo(ctx).GetUser(ctx, targetUserID)
		if err != nil {
			return "", err
		}
		user.PasswordHash = string(hash)
		user.MustChangePassword = true
		if err := service.repo(ctx).SaveUser(ctx, user); err != nil {
			return "", err
		}
		if err := service.repo(ctx).DeleteSessionsForUser(ctx, targetUserID); err != nil {
			return "", err
		}
		if err := service.writeAudit(ctx, actor, tenantID, "password.reset",
			fmt.Sprintf("target_user_id=%d by_role=%s", targetUserID, RoleSuperAdmin)); err != nil {
			return "", err
		}
		return initial, nil
	})
}

// EnterSupportAccess 带审计支持访问：理由必填，会话临时挂载组织上下文，
// 进出均写审计。停用组织拒绝建立支持访问。
func (service *Service) EnterSupportAccess(ctx context.Context, token string, tenantID uint, rawReason string) error {
	session, err := service.activeSession(ctx, token)
	if err != nil {
		return err
	}
	user, err := service.repo(ctx).GetUser(ctx, session.UserID)
	if err != nil {
		return ErrSessionRevoked
	}
	if user.Role != UserRoleSuperAdmin && session.Role != RoleSuperAdmin {
		return ErrForbidden
	}
	reason := strings.TrimSpace(rawReason)
	if reason == "" || len([]rune(reason)) > maxSupportReasonLength {
		return ErrReasonRequired
	}
	return service.inTx(ctx, func(ctx context.Context) error {
		tenant, err := service.repo(ctx).GetTenant(ctx, tenantID)
		if err != nil {
			return err
		}
		if tenant.Status != TenantStatusActive {
			return ErrTenantDisabled
		}
		session.SupportTenantID = &tenant.ID
		session.SupportReason = reason
		now := service.now().UTC()
		session.SupportEnteredAt = &now
		if err := service.repo(ctx).SaveSession(ctx, session); err != nil {
			return err
		}
		return service.writeAudit(ctx, &Actor{ID: user.ID, Username: user.Username}, tenant.ID,
			"support_access.entered", "reason="+reason)
	})
}

// ExitSupportAccess 退出支持访问并还原会话作用域（幂等）。
func (service *Service) ExitSupportAccess(ctx context.Context, token string) error {
	session, err := service.activeSession(ctx, token)
	if err != nil {
		return err
	}
	if session.SupportTenantID == nil {
		return nil
	}
	tenantID := *session.SupportTenantID
	user, err := service.repo(ctx).GetUser(ctx, session.UserID)
	if err != nil {
		return ErrSessionRevoked
	}
	return service.inTx(ctx, func(ctx context.Context) error {
		session.SupportTenantID = nil
		session.SupportReason = ""
		session.SupportEnteredAt = nil
		if err := service.repo(ctx).SaveSession(ctx, session); err != nil {
			return err
		}
		return service.writeAudit(ctx, &Actor{ID: user.ID, Username: user.Username}, tenantID,
			"support_access.exited", "")
	})
}

// ListAuditLogs 分页查询审计（超管后台；不自动清理）。
func (service *Service) ListAuditLogs(ctx context.Context, filter AuditFilter) ([]AuditLog, int64, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = defaultAuditPageSize
	}
	if filter.PageSize > maxAuditPageSize {
		filter.PageSize = maxAuditPageSize
	}
	return service.repo(ctx).ListAuditLogs(ctx, filter)
}

// ListRunnerAssignments 返回组织 runner 静态指派（Phase 1 形态）。
func (service *Service) ListRunnerAssignments(ctx context.Context, tenantID uint) ([]RunnerAssignment, error) {
	return service.repo(ctx).ListRunnerAssignments(ctx, tenantID)
}

// ListAllRunnerAssignments 跨租户全量列举指派行（启动校验消费端）。
func (service *Service) ListAllRunnerAssignments(ctx context.Context) ([]RunnerAssignment, error) {
	return service.repo(ctx).ListAllRunnerAssignments(ctx)
}

// ReplaceRunnerAssignments 整表替换组织 runner 端点列表：仅接受绝对
// http/https URL（含 host，与消费端启动校验同口径），去重、上限 20 条；
// 写 runner_assignments.updated 审计。
func (service *Service) ReplaceRunnerAssignments(ctx context.Context, actor *Actor, tenantID uint, endpoints []string) error {
	cleaned := make([]string, 0, len(endpoints))
	seen := map[string]struct{}{}
	for _, endpoint := range endpoints {
		endpoint = strings.TrimSpace(endpoint)
		parsed, err := url.Parse(endpoint)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return ErrInvalidInput
		}
		if _, duplicate := seen[endpoint]; duplicate {
			continue
		}
		seen[endpoint] = struct{}{}
		cleaned = append(cleaned, endpoint)
	}
	if len(cleaned) > maxRunnerAssignments {
		return ErrInvalidInput
	}
	return service.inTx(ctx, func(ctx context.Context) error {
		if _, err := service.repo(ctx).GetTenant(ctx, tenantID); err != nil {
			return err
		}
		if err := service.repo(ctx).ReplaceRunnerAssignments(ctx, tenantID, cleaned); err != nil {
			return err
		}
		return service.writeAudit(ctx, actor, tenantID, "runner_assignments.updated",
			fmt.Sprintf("count=%d endpoints=%s", len(cleaned), strings.Join(cleaned, ",")))
	})
}

// ListTenantInviteCodes 超管视角的组织邀请码列表（租户级吊销管理）。
func (service *Service) ListTenantInviteCodes(ctx context.Context, tenantID uint) ([]InviteCode, error) {
	if _, err := service.repo(ctx).GetTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	return service.repo(ctx).ListInviteCodesByTenant(ctx, tenantID)
}
