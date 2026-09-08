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
	code, err := service.repository.GetInviteCodeByHash(ctx, hashInviteCode(normalized))
	if err != nil {
		return nil, err
	}
	if code == nil || code.RevokedAt != nil ||
		!service.now().UTC().Before(code.ExpiresAt) || code.UsedCount >= code.MaxUses {
		return nil, ErrInviteInvalid
	}
	tenant, err := service.repository.GetTenant(ctx, code.TenantID)
	if err != nil {
		return nil, err
	}
	if tenant.Status != TenantStatusActive {
		return nil, ErrTenantDisabled
	}
	consumed, err := service.repository.ConsumeInviteCodeUse(ctx, code.ID, service.now().UTC())
	if err != nil {
		return nil, err
	}
	if !consumed {
		return nil, ErrInviteInvalid
	}
	user, err := service.CreateUser(ctx, username, password)
	if err != nil {
		// 建号失败（如用户名冲突）须退还名额，避免白烧一次使用机会。
		if refundErr := service.repository.RefundInviteCodeUse(ctx, code.ID); refundErr != nil {
			return nil, refundErr
		}
		return nil, err
	}
	if _, err := service.AddMembership(ctx, nil, tenant.ID, user.ID, code.Role); err != nil {
		// 入会失败：退名额并删除刚建的孤儿账号，避免用户名被
		// 永久烧毁、名额白损。
		service.compensateFailedRegistration(ctx, user, code.ID)
		return nil, err
	}
	return service.issueSession(ctx, user, &tenant.ID, ScopeTenant, code.Role)
}

// compensateFailedRegistration 注册后半程失败的补偿：退邀请码名额 + 删除
// 刚创建的孤儿账号。两项补偿互不阻断、尽力而为（主错误已由调用方返回；
// 补偿与业务动作的事务化收敛留后续批次）。
func (service *Service) compensateFailedRegistration(ctx context.Context, user *User, inviteCodeID uint) {
	_ = service.repository.RefundInviteCodeUse(ctx, inviteCodeID)
	_ = service.repository.DeleteUser(ctx, user)
}
