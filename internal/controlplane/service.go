package controlplane

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidInput           = errors.New("invalid input")
	ErrInvalidCredentials     = errors.New("invalid credentials")
	ErrUnauthenticated        = errors.New("unauthenticated")
	ErrSessionRevoked         = errors.New("session revoked")
	ErrRateLimited            = errors.New("rate limited")
	ErrSlugTaken              = errors.New("slug taken")
	ErrTenantNotFound         = errors.New("tenant not found")
	ErrTenantAlreadyDisabled  = errors.New("tenant already disabled")
	ErrTenantNotDisabled      = errors.New("tenant not disabled")
	ErrTenantDisabled         = errors.New("tenant disabled")
	ErrUsernameTaken          = errors.New("username taken")
	ErrUserNotFound           = errors.New("user not found")
	ErrInviteInvalid          = errors.New("invite code invalid")
	ErrCannotRemoveSelf       = errors.New("cannot remove self")
	ErrReasonRequired         = errors.New("reason required")
	ErrPasswordChangeRequired = errors.New("password change required")
	ErrForbidden              = errors.New("forbidden")
)

// tenantSlugPattern：3-32 位小写数字连字符，首尾非连字符。
var tenantSlugPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{1,30}[a-z0-9])$`)

// Actor 是控制面动作发起者（写审计用）；系统级种子动作传 nil。
type Actor struct {
	ID       uint
	Username string
}

// TenantInput 是组织创建入参。
type TenantInput struct {
	Slug        string
	DisplayName string
}

// Service 是控制面门面：认证/会话（auth.go）、组织与成员管理（tenant.go）、
// 超管动作（superadmin.go）共用同一 repository 与时钟。
type Service struct {
	repository  *Repository
	now         func() time.Time
	rateLimiter *loginRateLimiter
	// usernameLimiter 是按纯用户名计数的兜底限速：IP 维度键可被
	// 伪造 XFF 轮换打穿，用户名维度不依赖网络形态。阈值为主限速的 3 倍。
	usernameLimiter *loginRateLimiter
	// comparePasswords 是密码比较的唯一 seam：登录路径无条件
	// 经此执行一次比较，「不存在的用户名也付满额比较代价」成为结构性事实
	// 而非调用点顺序约定；注入实现供测试计数断言。
	comparePasswords func(hashedPassword, password []byte) error
}

type Option func(*Service)

func WithClock(clock func() time.Time) Option {
	return func(service *Service) {
		if clock != nil {
			service.now = clock
		}
	}
}

// WithRateLimit 覆盖登录限速参数（默认 15 分钟窗口 10 次失败）。
func WithRateLimit(window time.Duration, maxFailures int) Option {
	return func(service *Service) {
		if window > 0 && maxFailures > 0 {
			service.rateLimiter = newLoginRateLimiter(window, maxFailures, defaultRateLimitMaxKeys)
			service.usernameLimiter = newLoginRateLimiter(window, maxFailures*3, defaultRateLimitMaxKeys)
		}
	}
}

func NewService(repository *Repository, options ...Option) *Service {
	service := &Service{
		repository:       repository,
		now:              time.Now,
		rateLimiter:      newLoginRateLimiter(defaultRateLimitWindow, defaultRateLimitMaxFailures, defaultRateLimitMaxKeys),
		usernameLimiter:  newLoginRateLimiter(defaultRateLimitWindow, defaultRateLimitMaxFailures*3, defaultRateLimitMaxKeys),
		comparePasswords: bcrypt.CompareHashAndPassword,
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// validTenantInput 校验组织入参：slug 3-32 位小写数字连字符，
// display_name 1-64 字符。
func validTenantInput(slug, displayName string) (string, string, bool) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	displayName = strings.TrimSpace(displayName)
	if !tenantSlugPattern.MatchString(slug) {
		return "", "", false
	}
	if length := len([]rune(displayName)); length < 1 || length > 64 {
		return "", "", false
	}
	return slug, displayName, true
}

func tenantDMLocator(slug string) string {
	return "sqlite://data/tenants/" + slug + "/db.sqlite"
}

// CreateTenant 建组织：slug 全局唯一、状态 active、落 db_locator 串，
// 写 tenant.created 审计。
func (service *Service) CreateTenant(ctx context.Context, actor *Actor, input TenantInput) (*Tenant, error) {
	slug, displayName, ok := validTenantInput(input.Slug, input.DisplayName)
	if !ok {
		return nil, ErrInvalidInput
	}
	existing, err := service.repository.GetTenantBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrSlugTaken
	}
	tenant := &Tenant{
		Slug: slug, DisplayName: displayName,
		Status: TenantStatusActive, DMLocator: tenantDMLocator(slug),
	}
	if err := service.repository.CreateTenant(ctx, tenant); err != nil {
		return nil, err
	}
	if err := service.writeAudit(ctx, actor, tenant.ID, "tenant.created", "slug="+slug+" display_name="+displayName); err != nil {
		return nil, err
	}
	return tenant, nil
}

// UpdateTenant 修改 display_name，写 tenant.updated 审计。
func (service *Service) UpdateTenant(ctx context.Context, actor *Actor, tenantID uint, displayName string) (*Tenant, error) {
	tenant, err := service.repository.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	_, validName, ok := validTenantInput(tenant.Slug, displayName)
	if !ok {
		return nil, ErrInvalidInput
	}
	tenant.DisplayName = validName
	if err := service.repository.SaveTenant(ctx, tenant); err != nil {
		return nil, err
	}
	if err := service.writeAudit(ctx, actor, tenant.ID, "tenant.updated", "display_name="+validName); err != nil {
		return nil, err
	}
	return tenant, nil
}

// DisableTenant 停用组织：成员作用域会话即时吊销，写 tenant.disabled
// 审计；重复停用报错防审计噪音。
func (service *Service) DisableTenant(ctx context.Context, actor *Actor, tenantID uint) (*Tenant, error) {
	tenant, err := service.repository.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if tenant.Status == TenantStatusDisabled {
		return nil, ErrTenantAlreadyDisabled
	}
	tenant.Status = TenantStatusDisabled
	if err := service.repository.SaveTenant(ctx, tenant); err != nil {
		return nil, err
	}
	if err := service.repository.DeleteSessionsForTenant(ctx, tenant.ID); err != nil {
		return nil, err
	}
	// 支持访问中的超管会话只清上下文不删本体。
	if err := service.repository.ClearSupportForTenant(ctx, tenant.ID); err != nil {
		return nil, err
	}
	if err := service.writeAudit(ctx, actor, tenant.ID, "tenant.disabled", "slug="+tenant.Slug); err != nil {
		return nil, err
	}
	return tenant, nil
}

// RestoreTenant 恢复组织并写 tenant.restored 审计。
func (service *Service) RestoreTenant(ctx context.Context, actor *Actor, tenantID uint) (*Tenant, error) {
	tenant, err := service.repository.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if tenant.Status != TenantStatusDisabled {
		return nil, ErrTenantNotDisabled
	}
	tenant.Status = TenantStatusActive
	if err := service.repository.SaveTenant(ctx, tenant); err != nil {
		return nil, err
	}
	if err := service.writeAudit(ctx, actor, tenant.ID, "tenant.restored", "slug="+tenant.Slug); err != nil {
		return nil, err
	}
	return tenant, nil
}

// GetUserByUsername 按用户名取控制面账号（不存在返回 nil, nil）。
func (service *Service) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	return service.repository.GetUserByUsername(ctx, username)
}

// writeAudit 落一条审计日志；actor 为 nil 时记系统动作。如实声明（已知后置
// 项）：本调用发生在业务动作生效之后，写入失败时动作已提交、调用方收到
// 错误但重试将遇状态冲突——真正的「审计与动作同事务」留后续批次统一处理。
func (service *Service) writeAudit(ctx context.Context, actor *Actor, tenantID uint, action, detail string) error {
	entry := &AuditLog{Action: action, Detail: detail}
	if tenantID != 0 {
		id := tenantID
		entry.TenantID = &id
	}
	if actor != nil {
		id := actor.ID
		entry.ActorUserID = &id
		entry.ActorUsername = actor.Username
	}
	return service.repository.CreateAuditLog(ctx, entry)
}
