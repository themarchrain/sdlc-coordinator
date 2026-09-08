package controlplane

import "context"

// RegisterWithInviteCode 邀请码注册流：规范化输入码 → 哈希解析 →
// 未吊销/未过期/未超次（快查）→ 组织 active → 原子消耗名额（并发兑换唯一
// 裁决点）→ 建号（全局唯一，失败补偿名额）→ 以码内角色入会（恒为
// member）→ 注册即自动登录。
// 用户名冲突与弱密码错误原样透传（ErrUsernameTaken / ErrInvalidInput）。
func (service *Service) RegisterWithInviteCode(ctx context.Context, username, password, inviteCode string) (*LoginResult, error) {
	normalized := normalizeInviteCode(inviteCode)
	if normalized == "" {
		return nil, ErrInviteInvalid
	}
	code, err := service.repo(ctx).GetInviteCodeByHash(ctx, hashInviteCode(normalized))
	if err != nil {
		return nil, err
	}
	if code == nil || code.RevokedAt != nil ||
		!service.now().UTC().Before(code.ExpiresAt) || code.UsedCount >= code.MaxUses {
		return nil, ErrInviteInvalid
	}
	tenant, err := service.repo(ctx).GetTenant(ctx, code.TenantID)
	if err != nil {
		return nil, err
	}
	if tenant.Status != TenantStatusActive {
		return nil, ErrTenantDisabled
	}
	return inTxValue(service, ctx, func(ctx context.Context) (*LoginResult, error) {
		consumed, err := service.repo(ctx).ConsumeInviteCodeUse(ctx, code.ID, service.now().UTC())
		if err != nil {
			return nil, err
		}
		if !consumed {
			return nil, ErrInviteInvalid
		}
		user, err := service.CreateUser(ctx, username, password)
		if err != nil {
			return nil, err
		}
		if _, err := service.AddMembership(ctx, nil, tenant.ID, user.ID, code.Role); err != nil {
			return nil, err
		}
		return service.issueSession(ctx, user, &tenant.ID, ScopeTenant, code.Role)
	})
}

// compensateFailedRegistration 保留给非事务补偿路径；注册主路径已同事务回滚。
func (service *Service) compensateFailedRegistration(ctx context.Context, user *User, inviteCodeID uint) {
	_ = service.repo(ctx).RefundInviteCodeUse(ctx, inviteCodeID)
	_ = service.repo(ctx).DeleteUser(ctx, user)
}
