package controlplane

import (
	"context"
	"errors"
	"fmt"

	"time"

	"gorm.io/gorm"
)

// Repository 封装控制面库全部 gorm 访问；控制面库是独立 sqlite 单文件。
type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (repository *Repository) Migrate() error {
	if err := repository.db.AutoMigrate(
		&User{}, &Tenant{}, &Membership{}, &InviteCode{},
		&RunnerAssignment{}, &AuditLog{}, &Session{},
	); err != nil {
		return fmt.Errorf("migrate control plane: %w", err)
	}
	return nil
}

// ---- 租户 ----

func (repository *Repository) CreateTenant(ctx context.Context, tenant *Tenant) error {
	if err := repository.db.WithContext(ctx).Create(tenant).Error; err != nil {
		return fmt.Errorf("create tenant: %w", err)
	}
	return nil
}

func (repository *Repository) SaveTenant(ctx context.Context, tenant *Tenant) error {
	if err := repository.db.WithContext(ctx).Save(tenant).Error; err != nil {
		return fmt.Errorf("save tenant: %w", err)
	}
	return nil
}

// GetTenant 按主键取组织；不存在返回 (nil, ErrTenantNotFound)。
func (repository *Repository) GetTenant(ctx context.Context, id uint) (*Tenant, error) {
	var tenant Tenant
	if err := repository.db.WithContext(ctx).First(&tenant, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTenantNotFound
		}
		return nil, fmt.Errorf("get tenant: %w", err)
	}
	return &tenant, nil
}

// GetTenantBySlug 不存在时返回 (nil, nil)。
func (repository *Repository) GetTenantBySlug(ctx context.Context, slug string) (*Tenant, error) {
	var tenant Tenant
	if err := repository.db.WithContext(ctx).Where("slug = ?", slug).First(&tenant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get tenant by slug: %w", err)
	}
	return &tenant, nil
}

// ---- 审计 ----

func (repository *Repository) CreateAuditLog(ctx context.Context, entry *AuditLog) error {
	if err := repository.db.WithContext(ctx).Create(entry).Error; err != nil {
		return fmt.Errorf("create audit log: %w", err)
	}
	return nil
}

func (repository *Repository) listAuditLogsByAction(ctx context.Context, action string) []AuditLog {
	var logs []AuditLog
	_ = repository.db.WithContext(ctx).Where("action = ?", action).Order("id ASC").Find(&logs).Error
	return logs
}

// ---- 用户 ----

// GetUserByUsername 不存在时返回 (nil, nil)。
func (repository *Repository) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	if err := repository.db.WithContext(ctx).Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	return &user, nil
}

func (repository *Repository) GetUser(ctx context.Context, id uint) (*User, error) {
	var user User
	if err := repository.db.WithContext(ctx).First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &user, nil
}

func (repository *Repository) CreateUser(ctx context.Context, user *User) error {
	if err := repository.db.WithContext(ctx).Create(user).Error; err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// DeleteUser 硬删刚创建的孤儿账号（注册补偿专用）。
func (repository *Repository) DeleteUser(ctx context.Context, user *User) error {
	if err := repository.db.WithContext(ctx).Delete(user).Error; err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

func (repository *Repository) SaveUser(ctx context.Context, user *User) error {
	if err := repository.db.WithContext(ctx).Save(user).Error; err != nil {
		return fmt.Errorf("save user: %w", err)
	}
	return nil
}

// ---- 成员关系 ----

// GetMembership 不存在时返回 (nil, ErrMembershipNotFound)。
func (repository *Repository) GetMembership(ctx context.Context, userID, tenantID uint) (*Membership, error) {
	var membership Membership
	if err := repository.db.WithContext(ctx).
		Where("user_id = ? AND tenant_id = ?", userID, tenantID).First(&membership).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMembershipNotFound
		}
		return nil, fmt.Errorf("get membership: %w", err)
	}
	return &membership, nil
}

func (repository *Repository) CreateMembership(ctx context.Context, membership *Membership) error {
	if err := repository.db.WithContext(ctx).Create(membership).Error; err != nil {
		return fmt.Errorf("create membership: %w", err)
	}
	return nil
}

// SaveMembership 保存成员关系变更（如晋升组织管理员）：服务层写路径统一经
// repository，不直接触达 gorm。
func (repository *Repository) SaveMembership(ctx context.Context, membership *Membership) error {
	if err := repository.db.WithContext(ctx).Save(membership).Error; err != nil {
		return fmt.Errorf("save membership: %w", err)
	}
	return nil
}

func (repository *Repository) DeleteMembership(ctx context.Context, membership *Membership) error {
	if err := repository.db.WithContext(ctx).Delete(membership).Error; err != nil {
		return fmt.Errorf("delete membership: %w", err)
	}
	return nil
}

// ListMembersByTenant 联表返回成员视图（控制面身份 + 角色）。
func (repository *Repository) ListMembersByTenant(ctx context.Context, tenantID uint) ([]MemberView, error) {
	var members []MemberView
	if err := repository.db.WithContext(ctx).
		Table("memberships").
		Select("users.id AS user_id, users.username AS username, memberships.role AS role, memberships.created_at AS created_at").
		Joins("JOIN users ON users.id = memberships.user_id").
		Where("memberships.tenant_id = ?", tenantID).
		Order("memberships.created_at ASC, memberships.id ASC").
		Scan(&members).Error; err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return members, nil
}

// ListMembershipsByUser 返回用户在全部组织的成员关系（登录后组织选择用）。
func (repository *Repository) ListMembershipsByUser(ctx context.Context, userID uint) ([]Membership, error) {
	var memberships []Membership
	if err := repository.db.WithContext(ctx).
		Where("user_id = ?", userID).Order("id ASC").Find(&memberships).Error; err != nil {
		return nil, fmt.Errorf("list memberships by user: %w", err)
	}
	return memberships, nil
}

// ---- 邀请码 ----

func (repository *Repository) CreateInviteCode(ctx context.Context, code *InviteCode) error {
	if err := repository.db.WithContext(ctx).Create(code).Error; err != nil {
		return fmt.Errorf("create invite code: %w", err)
	}
	return nil
}

func (repository *Repository) SaveInviteCode(ctx context.Context, code *InviteCode) error {
	if err := repository.db.WithContext(ctx).Save(code).Error; err != nil {
		return fmt.Errorf("save invite code: %w", err)
	}
	return nil
}

// GetInviteCode 不存在时返回 (nil, ErrInviteInvalid)。
func (repository *Repository) GetInviteCode(ctx context.Context, id uint) (*InviteCode, error) {
	var code InviteCode
	if err := repository.db.WithContext(ctx).First(&code, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInviteInvalid
		}
		return nil, fmt.Errorf("get invite code: %w", err)
	}
	return &code, nil
}

// GetInviteCodeByHash 按规范化哈希取码，不存在返回 (nil, nil)。
func (repository *Repository) GetInviteCodeByHash(ctx context.Context, codeHash string) (*InviteCode, error) {
	var code InviteCode
	if err := repository.db.WithContext(ctx).Where("code_hash = ?", codeHash).First(&code).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get invite code by hash: %w", err)
	}
	return &code, nil
}

func (repository *Repository) ListInviteCodesByTenant(ctx context.Context, tenantID uint) ([]InviteCode, error) {
	var codes []InviteCode
	if err := repository.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).Order("id DESC").Find(&codes).Error; err != nil {
		return nil, fmt.Errorf("list invite codes: %w", err)
	}
	return codes, nil
}

// ---- 组织汇总 ----

// ListTenantSummaries 返回全部组织及成员计数（超管列表；不含业务数据）。
func (repository *Repository) ListTenantSummaries(ctx context.Context) ([]TenantSummary, error) {
	var summaries []TenantSummary
	if err := repository.db.WithContext(ctx).
		Table("tenants").
		Select("tenants.id AS id, tenants.slug AS slug, tenants.display_name AS display_name, tenants.status AS status, tenants.created_at AS created_at, COUNT(memberships.id) AS member_count").
		Joins("LEFT JOIN memberships ON memberships.tenant_id = tenants.id").
		Group("tenants.id").
		Order("tenants.id ASC").
		Scan(&summaries).Error; err != nil {
		return nil, fmt.Errorf("list tenant summaries: %w", err)
	}
	return summaries, nil
}

// ListAuditLogs 按过滤条件分页返回审计（id 倒序），并给出总数。
func (repository *Repository) ListAuditLogs(ctx context.Context, filter AuditFilter) ([]AuditLog, int64, error) {
	query := repository.db.WithContext(ctx).Model(&AuditLog{})
	if filter.TenantID != nil {
		query = query.Where("tenant_id = ?", *filter.TenantID)
	}
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count audit logs: %w", err)
	}
	var logs []AuditLog
	if err := query.Order("id DESC").
		Limit(filter.PageSize).Offset((filter.Page - 1) * filter.PageSize).
		Find(&logs).Error; err != nil {
		return nil, 0, fmt.Errorf("list audit logs: %w", err)
	}
	return logs, total, nil
}

// ---- runner 指派 ----

func (repository *Repository) ListRunnerAssignments(ctx context.Context, tenantID uint) ([]RunnerAssignment, error) {
	var assignments []RunnerAssignment
	if err := repository.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).Order("endpoint_url ASC").Find(&assignments).Error; err != nil {
		return nil, fmt.Errorf("list runner assignments: %w", err)
	}
	return assignments, nil
}

// ReplaceRunnerAssignments 事务内整表替换。
func (repository *Repository) ReplaceRunnerAssignments(ctx context.Context, tenantID uint, endpoints []string) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ?", tenantID).Delete(&RunnerAssignment{}).Error; err != nil {
			return fmt.Errorf("clear runner assignments: %w", err)
		}
		for _, endpoint := range endpoints {
			assignment := &RunnerAssignment{TenantID: tenantID, EndpointURL: endpoint}
			if err := tx.Create(assignment).Error; err != nil {
				return fmt.Errorf("create runner assignment: %w", err)
			}
		}
		return nil
	})
}

// ---- 会话 ----

func (repository *Repository) CreateSession(ctx context.Context, session *Session) error {
	if err := repository.db.WithContext(ctx).Create(session).Error; err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// GetActiveSessionByHash 按哈希取未过期会话；不存在/已过期返回 ErrUnauthenticated。
func (repository *Repository) GetActiveSessionByHash(ctx context.Context, tokenHash string, now time.Time) (*Session, error) {
	var session Session
	if err := repository.db.WithContext(ctx).
		Where("token_hash = ? AND expires_at > ?", tokenHash, now).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUnauthenticated
		}
		return nil, fmt.Errorf("get active session: %w", err)
	}
	return &session, nil
}

func (repository *Repository) SaveSession(ctx context.Context, session *Session) error {
	if err := repository.db.WithContext(ctx).Save(session).Error; err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

func (repository *Repository) DeleteSession(ctx context.Context, session *Session) error {
	if err := repository.db.WithContext(ctx).Delete(session).Error; err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// ---- 会话吊销 ----

// DeleteSessionsForTenant 删除作用域绑定该组织的成员会话（组织禁用）；
// 支持访问中的超管会话本体不由本方法删除，改由
// ClearSupportForTenant 清除上下文保留登录态。
func (repository *Repository) DeleteSessionsForTenant(ctx context.Context, tenantID uint) error {
	if err := repository.db.WithContext(ctx).
		Where("active_tenant_id = ?", tenantID).
		Delete(&Session{}).Error; err != nil {
		return fmt.Errorf("delete sessions for tenant: %w", err)
	}
	return nil
}

// ClearSupportForTenant 清除挂载在该组织上的支持访问上下文（会话本体保留）：
// 超管被停用组织打断支持访问后仍是已登录的超管。
func (repository *Repository) ClearSupportForTenant(ctx context.Context, tenantID uint) error {
	if err := repository.db.WithContext(ctx).Model(&Session{}).
		Where("support_tenant_id = ?", tenantID).
		Updates(map[string]any{
			"support_tenant_id":  nil,
			"support_reason":     "",
			"support_entered_at": nil,
		}).Error; err != nil {
		return fmt.Errorf("clear support for tenant: %w", err)
	}
	return nil
}

// ConsumeInviteCodeUse 原子消耗一次邀请码名额：仅当未吊销、
// 未过期、未达上限时 used_count+1，返回是否消耗成功。并发兑换以此为
// 唯一裁决点，杜绝读-查-写窗口内的超发。
func (repository *Repository) ConsumeInviteCodeUse(ctx context.Context, codeID uint, now time.Time) (bool, error) {
	result := repository.db.WithContext(ctx).Model(&InviteCode{}).
		Where("id = ? AND revoked_at IS NULL AND expires_at > ? AND used_count < max_uses", codeID, now).
		UpdateColumn("used_count", gorm.Expr("used_count + 1"))
	if result.Error != nil {
		return false, fmt.Errorf("consume invite code use: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// RefundInviteCodeUse 兑换后建号失败路径的名额补偿。
func (repository *Repository) RefundInviteCodeUse(ctx context.Context, codeID uint) error {
	if err := repository.db.WithContext(ctx).Model(&InviteCode{}).
		Where("id = ? AND used_count > 0", codeID).
		UpdateColumn("used_count", gorm.Expr("used_count - 1")).Error; err != nil {
		return fmt.Errorf("refund invite code use: %w", err)
	}
	return nil
}

// DeleteSessionsForUserTenant 删除某用户绑定该组织的会话；不影响其在其他
// 组织的会话（租户禁用不影响该用户在其他租户的会话）。
func (repository *Repository) DeleteSessionsForUserTenant(ctx context.Context, userID, tenantID uint) error {
	if err := repository.db.WithContext(ctx).
		Where("user_id = ? AND (active_tenant_id = ? OR support_tenant_id = ?)", userID, tenantID, tenantID).
		Delete(&Session{}).Error; err != nil {
		return fmt.Errorf("delete sessions for user tenant: %w", err)
	}
	return nil
}

// DeleteSessionsForUser 删除某用户全部会话（密码重置后的兜底吊销）。
func (repository *Repository) DeleteSessionsForUser(ctx context.Context, userID uint) error {
	if err := repository.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&Session{}).Error; err != nil {
		return fmt.Errorf("delete sessions for user: %w", err)
	}
	return nil
}
