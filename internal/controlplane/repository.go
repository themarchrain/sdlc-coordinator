package controlplane

import (
	"context"
	"errors"
	"fmt"

	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 封装控制面库全部 gorm 访问；控制面库是独立 sqlite 单文件。
type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (repository *Repository) Migrate() error {
	if err := repository.db.AutoMigrate(
		&User{}, &Tenant{}, &Membership{}, &InviteCode{},
		&RunnerAssignment{}, &AuditLog{}, &Session{}, &RepoRouteIndex{},
		&LoginFailure{}, &LoginFailureCapacityGuard{},
	); err != nil {
		return fmt.Errorf("migrate control plane: %w", err)
	}
	// v5 收敛：AutoMigrate 只增不删，v4 复合唯一索引会残留并伪装成仍
	// 有效的约束面；端点独占约束以 idx_runner_assignments_endpoint 单一
	// 承载，旧索引显式清除（幂等：不存在即跳过）。
	if repository.db.Migrator().HasIndex(&RunnerAssignment{}, "idx_runner_assignments_tenant_endpoint") {
		if err := repository.db.Migrator().DropIndex(&RunnerAssignment{}, "idx_runner_assignments_tenant_endpoint"); err != nil {
			return fmt.Errorf("drop legacy runner assignment index: %w", err)
		}
	}
	if err := repository.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&LoginFailureCapacityGuard{ID: 1}).Error; err != nil {
		return fmt.Errorf("initialize login failure capacity guard: %w", err)
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

// CreateUserIfAbsent 建号但不与并发赢家冲突：唯一键命中时静默让位并返回
// created=false（多实例同时首启种子场景）。
func (repository *Repository) CreateUserIfAbsent(ctx context.Context, user *User) (bool, error) {
	result := repository.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(user)
	if result.Error != nil {
		return false, fmt.Errorf("create user if absent: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

func (repository *Repository) GetUserByUsernameForUpdate(ctx context.Context, username string) (*User, error) {
	var user User
	query := repository.db.WithContext(ctx)
	if repository.db.Dialector.Name() == "mysql" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get user by username for update: %w", err)
	}
	return &user, nil
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
		Select("tenants.id AS id, tenants.slug AS slug, tenants.display_name AS display_name, tenants.status AS status, tenants.db_locator AS dm_locator, tenants.created_at AS created_at, COUNT(memberships.id) AS member_count").
		Joins("LEFT JOIN memberships ON memberships.tenant_id = tenants.id").
		Group("tenants.id").
		Order("tenants.id ASC").
		Scan(&summaries).Error; err != nil {
		return nil, fmt.Errorf("list tenant summaries: %w", err)
	}
	return summaries, nil
}

// SetTenantDMLocator 回写数据面定位串（开通/自愈完成标记）：条件更新按
// 主键，仅此列；不做整行覆盖。
func (repository *Repository) SetTenantDMLocator(ctx context.Context, tenantID uint, locator string) error {
	if err := repository.db.WithContext(ctx).Model(&Tenant{}).
		Where("id = ?", tenantID).
		UpdateColumn("db_locator", locator).Error; err != nil {
		return fmt.Errorf("set tenant db locator: %w", err)
	}
	return nil
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

// ListAllRunnerAssignments 跨租户全量列举指派行（启动校验消费端：需要
// 全表形态判断，租户池路由按租户走 ListRunnerAssignments）。
func (repository *Repository) ListAllRunnerAssignments(ctx context.Context) ([]RunnerAssignment, error) {
	var assignments []RunnerAssignment
	if err := repository.db.WithContext(ctx).
		Order("tenant_id ASC, endpoint_url ASC").Find(&assignments).Error; err != nil {
		return nil, fmt.Errorf("list all runner assignments: %w", err)
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

// DeleteExpiredSessions 删除 expires_at 不晚于 now 的会话。
func (repository *Repository) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	result := repository.db.WithContext(ctx).Where("expires_at <= ?", now).Delete(&Session{})
	if result.Error != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// CountRecentLoginFailures 统计键在窗口内的失败行数；MySQL 上窗口边界按
// 数据库时钟（NOW(6)）判定，跨实例时钟偏移免疫；SQLite 按参数时间判定。
func (repository *Repository) CountRecentLoginFailures(ctx context.Context, key string, window time.Duration) (int64, error) {
	var count int64
	query := repository.db.WithContext(ctx).Model(&LoginFailure{}).Where("`key` = ? AND `pending` = ?", key, false)
	if repository.db.Dialector.Name() == "mysql" {
		query = query.Where("failed_at > DATE_SUB(NOW(6), INTERVAL ? MICROSECOND)", window.Microseconds())
	} else {
		query = query.Where("failed_at > ?", time.Now().Add(-window))
	}
	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count recent login failures: %w", err)
	}
	return count, nil
}

var ErrLoginFailureCapacity = errors.New("login failure capacity reached")

func (repository *Repository) ReserveLoginFailureSlots(ctx context.Context, window time.Duration, slots int) ([]uint, error) {
	if slots <= 0 || slots > loginFailureCapacityLimit {
		return nil, ErrLoginFailureCapacity
	}
	var ids []uint
	capacityReached := false
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var guard LoginFailureCapacityGuard
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&guard, 1).Error; err != nil {
			return fmt.Errorf("lock login failure capacity guard: %w", err)
		}
		for batch := 0; batch < (loginFailureCapacityLimit+loginFailureBatchLimit-1)/loginFailureBatchLimit; batch++ {
			deleted, err := deleteExpiredLoginFailuresBounded(tx, window, loginFailureBatchLimit)
			if err != nil {
				return err
			}
			if deleted < loginFailureBatchLimit {
				break
			}
		}
		var count int64
		if err := tx.Model(&LoginFailure{}).Count(&count).Error; err != nil {
			return fmt.Errorf("count login failures for capacity: %w", err)
		}
		if count+int64(slots) > loginFailureCapacityLimit {
			capacityReached = true
			return nil
		}
		reservedAt := time.Now()
		if tx.Dialector.Name() == "mysql" {
			if err := tx.Raw("SELECT NOW(6)").Scan(&reservedAt).Error; err != nil {
				return fmt.Errorf("read database time for login failure reservation: %w", err)
			}
		}
		rows := make([]LoginFailure, slots)
		for i := range rows {
			rows[i] = LoginFailure{Key: "", FailedAt: reservedAt, Pending: true}
		}
		if err := tx.Create(&rows).Error; err != nil {
			return fmt.Errorf("reserve login failure capacity: %w", err)
		}
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if capacityReached {
		return nil, ErrLoginFailureCapacity
	}
	return ids, nil
}

func (repository *Repository) CompleteLoginFailureReservation(ctx context.Context, ids []uint, keys []string, failedAt time.Time) error {
	if len(ids) != len(keys) {
		return fmt.Errorf("complete login failure reservation: slot count mismatch")
	}
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		persistedAt := any(failedAt)
		if tx.Dialector.Name() == "mysql" {
			persistedAt = gorm.Expr("NOW(6)")
		}
		for i, id := range ids {
			result := tx.Model(&LoginFailure{}).Where("id = ? AND pending = ?", id, true).
				Updates(map[string]any{"key": keys[i], "failed_at": persistedAt, "pending": false})
			if result.Error != nil {
				return fmt.Errorf("complete login failure reservation: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("complete login failure reservation: reservation missing")
			}
		}
		return nil
	})
	return err
}

func (repository *Repository) ReleaseLoginFailureReservation(ctx context.Context, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	if err := repository.db.WithContext(ctx).Where("id IN ? AND pending = ?", ids, true).Delete(&LoginFailure{}).Error; err != nil {
		return fmt.Errorf("release login failure reservation: %w", err)
	}
	return nil
}

// RecordLoginFailure 追加一条失败记录。
func (repository *Repository) RecordLoginFailure(ctx context.Context, key string, failedAt time.Time, window time.Duration) error {
	ids, err := repository.ReserveLoginFailureSlots(ctx, window, 1)
	if err != nil {
		return fmt.Errorf("record login failure: %w", err)
	}
	if err := repository.CompleteLoginFailureReservation(ctx, ids, []string{key}, failedAt); err != nil {
		_ = repository.ReleaseLoginFailureReservation(context.Background(), ids)
		return fmt.Errorf("record login failure: %w", err)
	}
	return nil
}

// DeleteLoginFailures 清空键的全部失败记录（成功登录重置）。
func (repository *Repository) DeleteLoginFailures(ctx context.Context, key string) error {
	if err := repository.db.WithContext(ctx).Where("`key` = ?", key).Delete(&LoginFailure{}).Error; err != nil {
		return fmt.Errorf("delete login failures: %w", err)
	}
	return nil
}

// DeleteExpiredLoginFailuresBounded 每次最多删除 limit 条过期行并返回删除数。
// MySQL 使用库内时钟，与限流窗口判定保持一致；每批是独立 DELETE。
func (repository *Repository) DeleteExpiredLoginFailuresBounded(ctx context.Context, window time.Duration, limit int) (int64, error) {
	if limit <= 0 {
		limit = loginFailureBatchLimit
	}
	return deleteExpiredLoginFailuresBounded(repository.db.WithContext(ctx), window, limit)
}

func deleteExpiredLoginFailuresBounded(db *gorm.DB, window time.Duration, limit int) (int64, error) {
	if db.Dialector.Name() == "mysql" {
		result := db.Exec("DELETE FROM `login_failures` WHERE `failed_at` <= DATE_SUB(NOW(6), INTERVAL ? MICROSECOND) ORDER BY `failed_at` ASC, `id` ASC LIMIT ?", window.Microseconds(), limit)
		if result.Error != nil {
			return 0, fmt.Errorf("delete expired login failures bounded: %w", result.Error)
		}
		return result.RowsAffected, nil
	}
	subquery := db.Model(&LoginFailure{}).
		Select("id").
		Where("failed_at <= ?", time.Now().Add(-window)).
		Order("failed_at ASC").
		Order("id ASC").
		Limit(limit)
	result := db.
		Where("id IN (?)", subquery).
		Delete(&LoginFailure{})
	if result.Error != nil {
		return 0, fmt.Errorf("delete expired login failures bounded: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// CapKeyLoginFailures 把单个键的失败行裁剪到上限（保留最新 limit 行）：
// blocked 判定只需"窗口内行数是否达到上限"，多余历史行对判定无影响；
// 单键上限让匿名来源用不同用户名放大总行数时，任一键不无界堆积。
func (repository *Repository) CapKeyLoginFailures(ctx context.Context, key string, limit int) error {
	if limit <= 0 {
		return nil
	}
	var count int64
	if err := repository.db.WithContext(ctx).Model(&LoginFailure{}).
		Where("`key` = ?", key).Count(&count).Error; err != nil {
		return fmt.Errorf("count login failures for key cap: %w", err)
	}
	if count <= int64(limit) {
		return nil
	}
	// 保留最新 limit 行：删除更早的行。
	subquery := repository.db.Model(&LoginFailure{}).
		Select("id").
		Where("`key` = ?", key).
		Order("failed_at DESC").
		Limit(limit)
	if err := repository.db.WithContext(ctx).
		Where("`key` = ? AND id NOT IN (?)", key, subquery).
		Delete(&LoginFailure{}).Error; err != nil {
		return fmt.Errorf("cap login failures for key: %w", err)
	}
	return nil
}
