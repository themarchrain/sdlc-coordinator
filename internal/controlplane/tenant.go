package controlplane

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ErrMembershipExists / ErrMembershipNotFound 服务于成员管理动作。
var (
	ErrMembershipExists   = errors.New("membership exists")
	ErrMembershipNotFound = errors.New("membership not found")
)

// usernamePattern：3-32 位小写字母数字下划线连字符。
var usernamePattern = regexp.MustCompile(`^[a-z0-9_-]{3,32}$`)

const (
	minPasswordLength = 8
	// maxPasswordLength 对齐 bcrypt 72 字节硬限（按字节计）：超长
	// 密码在用户路径表现为 500、在种子路径拒绝启动，统一在服务层夹紧为
	// 输入校验。最小长度保留 rune 语义（中文密码至少 8 个字符）。
	maxPasswordLength = 72
	defaultInviteTTL  = 72
	defaultInviteUses = 10
)

// MemberView 是成员列表行：仅控制面身份，不涉平台身份。
type MemberView struct {
	UserID    uint      `json:"user_id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// InviteCodeInput 签发参数；0 值取默认（72h / 10 次）。
type InviteCodeInput struct {
	ExpiresInHours int
	MaxUses        int
}

// IssuedInviteCode 携带一次性明文与落库记录。
type IssuedInviteCode struct {
	Plaintext string
	Record    *InviteCode
}

// CreateUser 建控制面普通账号：用户名规范化小写全局唯一，bcrypt
// 存储。密码最短 8 位。
func (service *Service) CreateUser(ctx context.Context, username, password string) (*User, error) {
	return service.createUserWithRole(ctx, username, password, UserRoleUser, false)
}

// createUserWithRole 是账号创建的公共路径。
func (service *Service) createUserWithRole(ctx context.Context, username, password, role string, mustChange bool) (*User, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if !usernamePattern.MatchString(username) {
		return nil, ErrInvalidInput
	}
	if len([]rune(password)) < minPasswordLength || len(password) > maxPasswordLength {
		return nil, ErrInvalidInput
	}
	existing, err := service.repo(ctx).GetUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrUsernameTaken
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	user := &User{Username: username, PasswordHash: string(hash), Role: role, MustChangePassword: mustChange}
	if err := service.repo(ctx).CreateUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// AddMembership 把已有用户挂进组织并授角色（tenant_admin/member）；同组织唯一。
func (service *Service) AddMembership(ctx context.Context, actor *Actor, tenantID, userID uint, role string) (*Membership, error) {
	if role != RoleTenantAdmin && role != RoleMember {
		return nil, ErrInvalidInput
	}
	return inTxValue(service, ctx, func(ctx context.Context) (*Membership, error) {
		if _, err := service.repo(ctx).GetTenant(ctx, tenantID); err != nil {
			return nil, err
		}
		// 超管账号不持有任何 memberships。
		target, err := service.repo(ctx).GetUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		if target.Role == UserRoleSuperAdmin {
			return nil, ErrForbidden
		}
		existing, err := service.repo(ctx).GetMembership(ctx, userID, tenantID)
		if err != nil && !errors.Is(err, ErrMembershipNotFound) {
			return nil, err
		}
		if existing != nil {
			return nil, ErrMembershipExists
		}
		membership := &Membership{UserID: userID, TenantID: tenantID, Role: role}
		if err := service.repo(ctx).CreateMembership(ctx, membership); err != nil {
			return nil, err
		}
		if err := service.writeAudit(ctx, actor, tenantID, "member.added",
			fmt.Sprintf("user_id=%d role=%s", userID, role)); err != nil {
			return nil, err
		}
		return membership, nil
	})
}

// RemoveMember 移出成员并即时吊销其在该组织的会话；
// 组织管理员不能移除自己（防误操作锁死组织）。
func (service *Service) RemoveMember(ctx context.Context, actor *Actor, tenantID, userID uint) (*Membership, error) {
	if actor != nil && actor.ID == userID {
		return nil, ErrCannotRemoveSelf
	}
	return inTxValue(service, ctx, func(ctx context.Context) (*Membership, error) {
		membership, err := service.repo(ctx).GetMembership(ctx, userID, tenantID)
		if err != nil {
			return nil, err
		}
		if err := service.repo(ctx).DeleteMembership(ctx, membership); err != nil {
			return nil, err
		}
		if err := service.repo(ctx).DeleteSessionsForUserTenant(ctx, userID, tenantID); err != nil {
			return nil, err
		}
		if err := service.writeAudit(ctx, actor, tenantID, "member.removed", fmt.Sprintf("user_id=%d", userID)); err != nil {
			return nil, err
		}
		return membership, nil
	})
}

// ListMembers 返回组织成员列表（含角色），按入会时间排序。
func (service *Service) ListMembers(ctx context.Context, tenantID uint) ([]MemberView, error) {
	return service.repo(ctx).ListMembersByTenant(ctx, tenantID)
}

// IssueInviteCode 签发组织邀请码：明文一次性返回，库内仅哈希；
// 一律 member 角色。
func (service *Service) IssueInviteCode(ctx context.Context, actor *Actor, tenantID uint, input InviteCodeInput) (*IssuedInviteCode, error) {
	hours := input.ExpiresInHours
	if hours == 0 {
		hours = defaultInviteTTL
	}
	maxUses := input.MaxUses
	if maxUses == 0 {
		maxUses = defaultInviteUses
	}
	if hours < 0 || maxUses < 1 {
		return nil, ErrInvalidInput
	}
	plaintext, err := generateInviteCode()
	if err != nil {
		return nil, err
	}
	return inTxValue(service, ctx, func(ctx context.Context) (*IssuedInviteCode, error) {
		if _, err := service.repo(ctx).GetTenant(ctx, tenantID); err != nil {
			return nil, err
		}
		record := &InviteCode{
			TenantID:  tenantID,
			CodeHash:  hashInviteCode(normalizeInviteCode(plaintext)),
			Role:      RoleMember,
			ExpiresAt: service.now().UTC().Add(time.Duration(hours) * time.Hour),
			MaxUses:   maxUses,
			CreatedBy: actorIDOf(actor),
		}
		if err := service.repo(ctx).CreateInviteCode(ctx, record); err != nil {
			return nil, err
		}
		if err := service.writeAudit(ctx, actor, tenantID, "invite.issued",
			fmt.Sprintf("invite_id=%d max_uses=%d expires_at=%s", record.ID, record.MaxUses, record.ExpiresAt.Format(time.RFC3339))); err != nil {
			return nil, err
		}
		return &IssuedInviteCode{Plaintext: plaintext, Record: record}, nil
	})
}

// RevokeInviteCode 吊销邀请码（幂等）；跨组织吊销按组织不存在拒绝，防越权探测。
func (service *Service) RevokeInviteCode(ctx context.Context, actor *Actor, tenantID, codeID uint) error {
	return service.inTx(ctx, func(ctx context.Context) error {
		code, err := service.repo(ctx).GetInviteCode(ctx, codeID)
		if err != nil {
			return err
		}
		if code.TenantID != tenantID {
			return ErrTenantNotFound
		}
		if code.RevokedAt != nil {
			return nil
		}
		now := service.now().UTC()
		code.RevokedAt = &now
		if err := service.repo(ctx).SaveInviteCode(ctx, code); err != nil {
			return err
		}
		return service.writeAudit(ctx, actor, tenantID, "invite.revoked", fmt.Sprintf("invite_id=%d", code.ID))
	})
}

// ResetMemberPassword 组织管理员重置本组织成员密码（密码重置链）：
// 生成一次性初始密码、置强制改密、吊销目标全部会话并写审计。仅 member 角色
// 可被组织管理员重置。
func (service *Service) ResetMemberPassword(ctx context.Context, actor *Actor, tenantID, targetUserID uint) (string, error) {
	return inTxValue(service, ctx, func(ctx context.Context) (string, error) {
		membership, err := service.repo(ctx).GetMembership(ctx, targetUserID, tenantID)
		if err != nil {
			return "", err
		}
		if membership.Role != RoleMember {
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
			fmt.Sprintf("target_user_id=%d by_role=%s", targetUserID, RoleTenantAdmin)); err != nil {
			return "", err
		}
		return initial, nil
	})
}

// ListInviteCodes 返回组织邀请码列表（不含哈希，防枚举）。
func (service *Service) ListInviteCodes(ctx context.Context, tenantID uint) ([]InviteCode, error) {
	return service.repo(ctx).ListInviteCodesByTenant(ctx, tenantID)
}

func actorIDOf(actor *Actor) uint {
	if actor == nil {
		return 0
	}
	return actor.ID
}
