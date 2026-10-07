package controlplane

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// ErrSeedUsernameConflict env 种子超管用户名与既有非超管账号同名：fail-fast
// 拒绝启动，改 env 用户名或清理同名账号后重启；绝不静默改写。
var ErrSeedUsernameConflict = errors.New("seed super admin username conflicts with existing non-super-admin account")

// SeedSuperAdminInput 是 env 种子超管入参。
// Password 兼容两种形态：明文（建号时 bcrypt）或 `$2a$/$2b$/$2y$` 开头的
// 60 位 bcrypt 串（直接落库）。Reset=true 时走密码丢失重置通道：覆盖密码、
// 吊销既有会话并写审计。
type SeedSuperAdminInput struct {
	Username string
	Password string
	Reset    bool
}

// SeedSuperAdmin 首次启动创建超管；账号已存在且未开 Reset 时为 no-op。
// 多实例同时首启用 insert-if-absent 收敛：重复键不再让任一
// 实例启动失败，输家接管赢家已建的行并按其角色/Reset 语义继续核验。
func (service *Service) SeedSuperAdmin(ctx context.Context, input SeedSuperAdminInput) error {
	username := strings.ToLower(strings.TrimSpace(input.Username))
	if !usernamePattern.MatchString(username) {
		return ErrInvalidInput
	}
	passwordHash, err := resolveSeedPasswordHash(input.Password)
	if err != nil {
		return err
	}
	existing, err := service.repo(ctx).GetUserByUsername(ctx, username)
	if err != nil {
		return err
	}
	if existing == nil {
		return service.seedCreateOrAdopt(ctx, username, passwordHash, input.Reset)
	}
	return service.seedApplyExisting(ctx, existing, username, passwordHash, input.Reset)
}

// seedCreateOrAdopt 在单一事务内建号；唯一键命中（并发竞态的输家）时按锁
// 定读接管赢家行，走与既有账号一致的核验路径。建号与首启审计同事务。
func (service *Service) seedCreateOrAdopt(ctx context.Context, username, passwordHash string, reset bool) error {
	return service.inTx(ctx, func(ctx context.Context) error {
		user := &User{Username: username, PasswordHash: passwordHash, Role: UserRoleSuperAdmin}
		created, err := service.repo(ctx).CreateUserIfAbsent(ctx, user)
		if err != nil {
			return err
		}
		if created {
			return service.writeAudit(ctx, nil, 0, "superadmin.seeded",
				"username="+username+" user_id="+fmt.Sprint(user.ID))
		}
		winner, err := service.repo(ctx).GetUserByUsernameForUpdate(ctx, username)
		if err != nil {
			return err
		}
		if winner == nil {
			return fmt.Errorf("seed super admin %q: concurrent insert lost but no committed row found", username)
		}
		return service.seedApplyExisting(ctx, winner, username, passwordHash, reset)
	})
}

// seedApplyExisting 对既有账号执行种子语义：非超管 fail-fast 拒绝；未开
// Reset 是 no-op（绝不覆盖已有密码）；Reset 覆盖密码并吊销会话。
func (service *Service) seedApplyExisting(ctx context.Context, existing *User, username, passwordHash string, reset bool) error {
	if existing.Role != UserRoleSuperAdmin {
		// fail-fast：env 声称的超管用户名被普通账号占用属于配置
		// 冲突，静默 no-op 或改写其密码都会让「super admin ensured」失实。
		return fmt.Errorf("seed super admin %q exists as %s account: %w",
			username, existing.Role, ErrSeedUsernameConflict)
	}
	if !reset {
		return nil
	}
	existing.PasswordHash = passwordHash
	existing.MustChangePassword = false
	if err := service.repo(ctx).SaveUser(ctx, existing); err != nil {
		return err
	}
	if err := service.repo(ctx).DeleteSessionsForUser(ctx, existing.ID); err != nil {
		return err
	}
	return service.writeAudit(ctx, nil, 0, "superadmin.password_reset", "username="+username)
}

// resolveSeedPasswordHash 明文形态做 bcrypt；bcrypt 串形态校验长度与前缀后原样返回。
func resolveSeedPasswordHash(password string) (string, error) {
	trimmed := strings.TrimSpace(password)
	if trimmed == "" {
		return "", ErrInvalidInput
	}
	if isBcryptHash(trimmed) {
		return trimmed, nil
	}
	if len([]rune(trimmed)) < minPasswordLength || len(trimmed) > maxPasswordLength {
		return "", ErrInvalidInput
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(trimmed), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash seed password: %w", err)
	}
	return string(hash), nil
}

func isBcryptHash(value string) bool {
	return len(value) == 60 &&
		(strings.HasPrefix(value, "$2a$") || strings.HasPrefix(value, "$2b$") || strings.HasPrefix(value, "$2y$"))
}
