// Package controlplane 实现多租户控制面：用户、组织（租户）、成员关系、
// 邀请码、runner 指派与审计日志。控制面持有独立 sqlite 单文件，
// 与租户面业务库物理分离；本包不含任何租户业务数据。
package controlplane

import "time"

// 角色三级 RBAC：super_admin 为控制面全局角色，
// tenant_admin/member 为 memberships 内的组织级角色。
const (
	RoleSuperAdmin  = "super_admin"
	RoleTenantAdmin = "tenant_admin"
	RoleMember      = "member"
)

// 组织状态：disabled 拒绝该组织作用域的会话校验与登录选择。
const (
	TenantStatusActive   = "active"
	TenantStatusDisabled = "disabled"
)

// 账号级角色：全局超管 vs 普通账号（超管不持有 memberships）。
const (
	UserRoleUser       = "user"
	UserRoleSuperAdmin = "super_admin"
)

// 会话作用域类别。
const (
	ScopeSuperAdmin = "super_admin"
	ScopeTenant     = "tenant"
	ScopeUnscoped   = "unscoped"
)

// User 是控制面登录账号，只回答 RBAC 问题，与租户面平台身份
// （people 三元组）正交，禁止合并。Role 区分全局超管与普通账号。
type User struct {
	ID                 uint   `gorm:"primaryKey"`
	Username           string `gorm:"not null;uniqueIndex;size:128"`
	PasswordHash       string `gorm:"not null"`
	Role               string `gorm:"not null;default:user"`
	MustChangePassword bool   `gorm:"not null;default:false"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (User) TableName() string { return "users" }

// Tenant 是组织（租户）控制面行；DMLocator 声明租户库定位（后续由
// TenantStoreResolver 消费，本阶段仅落串不建文件）。
type Tenant struct {
	ID          uint   `gorm:"primaryKey"`
	Slug        string `gorm:"not null;uniqueIndex;size:128"`
	DisplayName string `gorm:"not null"`
	Status      string `gorm:"not null;default:active;index;size:32"`
	DMLocator   string `gorm:"column:db_locator"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Tenant) TableName() string { return "tenants" }

// Membership 把用户挂进组织并授予组织级角色；一人可属多个组织，
// 同一组织内唯一。超管账号不持有 memberships。
type Membership struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"not null;uniqueIndex:idx_memberships_user_tenant"`
	TenantID  uint   `gorm:"not null;uniqueIndex:idx_memberships_user_tenant;index"`
	Role      string `gorm:"not null"`
	CreatedAt time.Time
}

func (Membership) TableName() string { return "memberships" }

// InviteCode 以哈希存储（不存明文）；明文只在签发响应中出现一次。
type InviteCode struct {
	ID        uint      `gorm:"primaryKey"`
	TenantID  uint      `gorm:"not null;index"`
	CodeHash  string    `gorm:"not null;uniqueIndex;size:128"`
	Role      string    `gorm:"not null;default:member"`
	ExpiresAt time.Time `gorm:"not null;index"`
	MaxUses   int       `gorm:"not null"`
	UsedCount int       `gorm:"not null;default:0"`
	RevokedAt *time.Time
	CreatedBy uint `gorm:"not null"`
	CreatedAt time.Time
}

func (InviteCode) TableName() string { return "invite_codes" }

// RunnerAssignment 是租户 runner 池的 Phase 1 静态配置形态；
// 本阶段仅提供模型与超管 list/replace 端点，派发消费归后续租户化改造。
type RunnerAssignment struct {
	ID          uint   `gorm:"primaryKey"`
	TenantID    uint   `gorm:"not null;index"`
	EndpointURL string `gorm:"not null;uniqueIndex:idx_runner_assignments_endpoint;size:512"`
	CreatedAt   time.Time
}

func (RunnerAssignment) TableName() string { return "runner_assignments" }

// AuditLog 记录超管动作与支持访问等控制面留痕。
type AuditLog struct {
	ID            uint `gorm:"primaryKey"`
	ActorUserID   *uint
	ActorUsername string    `gorm:"not null;default:''"`
	TenantID      *uint     `gorm:"index"`
	Action        string    `gorm:"not null;index;size:64"`
	Detail        string    `gorm:"not null;default:''"`
	SourceIP      string    `gorm:"not null;default:''"`
	CreatedAt     time.Time `gorm:"index"`
}

func (AuditLog) TableName() string { return "audit_log" }

// Session 携带 (user, active_tenant, role)；超管会话不带
// active_tenant，进入组织上下文走支持访问（SupportTenantID + 理由，进退均写审计）。
type Session struct {
	ID               uint   `gorm:"primaryKey"`
	TokenHash        string `gorm:"not null;uniqueIndex;size:128"`
	UserID           uint   `gorm:"not null;index"`
	ActiveTenantID   *uint
	Role             string `gorm:"not null"`
	SupportTenantID  *uint
	SupportReason    string `gorm:"not null;default:''"`
	SupportEnteredAt *time.Time
	ExpiresAt        time.Time `gorm:"not null;index"`
	CreatedAt        time.Time
}

func (Session) TableName() string { return "sessions" }
