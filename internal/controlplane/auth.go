package controlplane

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ErrNoMemberships 登录账号未加入任何组织且非超管。
var ErrNoMemberships = errors.New("no memberships")

// dummyPasswordHash 是登录时序对齐用的哑 bcrypt 串：用户不存在时
// 也执行一次同代价哈希比较，抹平「用户名枚举」的响应时序差。
var dummyPasswordHash = func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("control-plane-dummy-account"), bcrypt.DefaultCost)
	if err != nil {
		panic(fmt.Sprintf("controlplane: precompute dummy password hash: %v", err))
	}
	return hash
}()

// DefaultSessionTTL 是控制面会话有效期（与存量管理台会话一致）。
const DefaultSessionTTL = 7 * 24 * time.Hour

// SessionScope 描述会话当前作用域：超管独立作用域 / 组织作用域 /
// 待选择（一人多组织）。
type SessionScope struct {
	Kind   string
	Tenant *Tenant
	Role   string
}

// TenantOption 是登录/会话查询返回的可选组织项。
type TenantOption struct {
	ID          uint   `json:"id"`
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// LoginResult 是登录/选择组织动作的结果：token 走 HttpOnly cookie 下发。
type LoginResult struct {
	Token   string
	User    *User
	Scope   SessionScope
	Tenants []TenantOption
}

// SessionContext 是会话校验（含 memberships 回查）后的完整上下文，
// 中间件据此装配 gin 上下文。
type SessionContext struct {
	Session         *Session
	User            *User
	EffectiveTenant *Tenant
	EffectiveRole   string
	SuperAdminMode  bool
	SupportMode     bool
}

// Login 校验用户名+密码（bcrypt），按用户名+IP 限速；成功建会话：
// 超管独立作用域；单组织直绑；多组织建无作用域会话等待选择。
func (service *Service) Login(ctx context.Context, rawUsername, password, clientIP string) (*LoginResult, error) {
	username := strings.ToLower(strings.TrimSpace(rawUsername))
	if username == "" || password == "" {
		return nil, ErrInvalidInput
	}
	rateKey := username + "|" + strings.TrimSpace(clientIP)
	// 双层限速：主键（用户名+IP）在直连形态受信任代理配置保护，
	// 兜底键（纯用户名）不依赖网络形态，IP 轮换无法绕过。
	if service.rateLimiter.blocked(rateKey) || service.usernameLimiter.blocked(username) {
		return nil, ErrRateLimited
	}
	user, err := service.repository.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	// 无条件求值比较：比较不得与 user==nil 短路合并，否则
	// 「不存在的用户名只付 SQL 代价」构成时序枚举通道。seam 默认实现为
	// bcrypt.CompareHashAndPassword，测试注入计数实现锁定该结构事实。
	hash := dummyPasswordHash
	if user != nil {
		hash = []byte(user.PasswordHash)
	}
	compareErr := service.comparePasswords(hash, []byte(password))
	if user == nil || compareErr != nil {
		service.rateLimiter.record(rateKey)
		service.usernameLimiter.record(username)
		return nil, ErrInvalidCredentials
	}
	service.rateLimiter.reset(rateKey)
	service.usernameLimiter.reset(username)

	if user.Role == UserRoleSuperAdmin {
		return service.issueSession(ctx, user, nil, ScopeSuperAdmin, RoleSuperAdmin)
	}
	memberships, err := service.repository.ListMembershipsByUser(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	options, err := service.tenantOptionsFor(ctx, memberships)
	if err != nil {
		return nil, err
	}
	if len(options) == 0 {
		return nil, ErrNoMemberships
	}
	if len(options) == 1 {
		// 直绑 options[0]（过滤停用后唯一活跃组织），不得回退 memberships[0]：
		// 先加入的组织可能已被停用，绑错会话必 403 且前端无法自救。
		return service.issueSession(ctx, user, &options[0].ID, ScopeTenant, options[0].Role)
	}
	return service.issueSession(ctx, user, nil, ScopeUnscoped, "")
}

// SelectTenant 为无作用域会话绑定组织（换发新会话作用域，旧会话删除）；
// 非成员与禁用组织均拒绝。
func (service *Service) SelectTenant(ctx context.Context, token string, tenantID uint) (*LoginResult, error) {
	session, err := service.activeSession(ctx, token)
	if err != nil {
		return nil, err
	}
	user, err := service.repository.GetUser(ctx, session.UserID)
	if err != nil {
		return nil, ErrSessionRevoked
	}
	membership, err := service.repository.GetMembership(ctx, user.ID, tenantID)
	if err != nil {
		if errors.Is(err, ErrMembershipNotFound) {
			return nil, ErrForbidden
		}
		return nil, err
	}
	tenant, err := service.repository.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if tenant.Status != TenantStatusActive {
		return nil, ErrTenantDisabled
	}
	if err := service.repository.DeleteSession(ctx, session); err != nil {
		return nil, err
	}
	return service.issueSession(ctx, user, &tenantID, ScopeTenant, membership.Role)
}

// ValidateSession 是会话校验的单一入口：token 有效性 → 用户存在 → 回查
// memberships（成员移除/角色降级即时失效）→ 组织状态（禁用拒绝）。
// 超管会话不携带组织作用域；支持访问期间临时挂载组织上下文。
func (service *Service) ValidateSession(ctx context.Context, token string) (*SessionContext, error) {
	session, err := service.activeSession(ctx, token)
	if err != nil {
		return nil, err
	}
	user, err := service.repository.GetUser(ctx, session.UserID)
	if err != nil {
		return nil, ErrSessionRevoked
	}
	sessionCtx := &SessionContext{Session: session, User: user}
	if user.Role == UserRoleSuperAdmin || session.Role == RoleSuperAdmin {
		sessionCtx.SuperAdminMode = true
		if session.SupportTenantID != nil {
			tenant, err := service.repository.GetTenant(ctx, *session.SupportTenantID)
			if err != nil {
				return nil, ErrSessionRevoked
			}
			sessionCtx.SupportMode = true
			sessionCtx.EffectiveTenant = tenant
			sessionCtx.EffectiveRole = RoleTenantAdmin
		}
		return sessionCtx, nil
	}
	if session.ActiveTenantID == nil {
		return nil, ErrSessionRevoked
	}
	membership, err := service.repository.GetMembership(ctx, user.ID, *session.ActiveTenantID)
	if err != nil {
		return nil, ErrSessionRevoked
	}
	if membership.Role != session.Role {
		return nil, ErrSessionRevoked
	}
	tenant, err := service.repository.GetTenant(ctx, *session.ActiveTenantID)
	if err != nil {
		return nil, ErrSessionRevoked
	}
	if tenant.Status != TenantStatusActive {
		return nil, ErrTenantDisabled
	}
	sessionCtx.EffectiveTenant = tenant
	sessionCtx.EffectiveRole = membership.Role
	return sessionCtx, nil
}

// Logout 删除会话行（幂等：不存在也视作已登出）。
func (service *Service) Logout(ctx context.Context, token string) error {
	session, err := service.activeSession(ctx, token)
	if err != nil {
		if errors.Is(err, ErrUnauthenticated) {
			return nil
		}
		return err
	}
	return service.repository.DeleteSession(ctx, session)
}

// ChangePassword 校验当前密码后改密并清除强制改密标记；改密后吊销该用户全部
// 会话（同源安全语义：与密码重置一致）。
func (service *Service) ChangePassword(ctx context.Context, userID uint, currentPassword, newPassword string) error {
	user, err := service.repository.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)) != nil {
		return ErrInvalidCredentials
	}
	if len([]rune(newPassword)) < minPasswordLength || len(newPassword) > maxPasswordLength {
		return ErrInvalidInput
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	user.PasswordHash = string(hash)
	user.MustChangePassword = false
	if err := service.repository.SaveUser(ctx, user); err != nil {
		return err
	}
	return service.repository.DeleteSessionsForUser(ctx, userID)
}

// CreateSuperAdminUser 建全局超管账号（env 种子与测试共用；永不入会）。
func (service *Service) CreateSuperAdminUser(ctx context.Context, username, password string) (uint, error) {
	user, err := service.createUserWithRole(ctx, username, password, UserRoleSuperAdmin, false)
	if err != nil {
		return 0, err
	}
	return user.ID, nil
}

// issueSession 建会话行并返回登录结果；scope 为组织作用域时 token 绑定该组织。
func (service *Service) issueSession(ctx context.Context, user *User, tenantID *uint, scopeKind, role string) (*LoginResult, error) {
	token, err := newSessionToken()
	if err != nil {
		return nil, err
	}
	session := &Session{
		TokenHash:      sessionTokenHash(token),
		UserID:         user.ID,
		ActiveTenantID: tenantID,
		Role:           role,
		ExpiresAt:      service.now().UTC().Add(DefaultSessionTTL),
	}
	if err := service.repository.CreateSession(ctx, session); err != nil {
		return nil, err
	}
	result := &LoginResult{Token: token, User: user, Scope: SessionScope{Kind: scopeKind}}
	if tenantID != nil {
		tenant, err := service.repository.GetTenant(ctx, *tenantID)
		if err != nil {
			return nil, err
		}
		result.Scope.Tenant = tenant
		result.Scope.Role = session.Role
	}
	if scopeKind == ScopeUnscoped {
		memberships, err := service.repository.ListMembershipsByUser(ctx, user.ID)
		if err != nil {
			return nil, err
		}
		options, err := service.tenantOptionsFor(ctx, memberships)
		if err != nil {
			return nil, err
		}
		result.Tenants = options
	}
	return result, nil
}

func (service *Service) tenantOptionsFor(ctx context.Context, memberships []Membership) ([]TenantOption, error) {
	options := make([]TenantOption, 0, len(memberships))
	for _, membership := range memberships {
		tenant, err := service.repository.GetTenant(ctx, membership.TenantID)
		if err != nil {
			return nil, err
		}
		if tenant.Status != TenantStatusActive {
			continue
		}
		options = append(options, TenantOption{
			ID: tenant.ID, Slug: tenant.Slug, DisplayName: tenant.DisplayName, Role: membership.Role,
		})
	}
	return options, nil
}

// activeSession 按 token 哈希取未过期会话。
func (service *Service) activeSession(ctx context.Context, token string) (*Session, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return nil, ErrUnauthenticated
	}
	session, err := service.repository.GetActiveSessionByHash(ctx, sessionTokenHash(trimmed), service.now().UTC())
	if err != nil {
		return nil, err
	}
	return session, nil
}

func newSessionToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

func sessionTokenHash(token string) string {
	return hashInviteCode("session:" + token)
}
