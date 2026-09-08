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
		user := &User{Username: username, PasswordHash: passwordHash, Role: UserRoleSuperAdmin}
		return service.inTx(ctx, func(ctx context.Context) error {
			if err := service.repo(ctx).CreateUser(ctx, user); err != nil {
				return err
			}
			return service.writeAudit(ctx, nil, 0, "superadmin.seeded",
				"username="+username+" user_id="+fmt.Sprint(user.ID))
		})
	}
	if existing.Role != UserRoleSuperAdmin {
		// fail-fast：env 声称的超管用户名被普通账号占用属于配置
		// 冲突，静默 no-op 或改写其密码都会让「super admin ensured」失实。
		return fmt.Errorf("seed super admin %q exists as %s account: %w",
			username, existing.Role, ErrSeedUsernameConflict)
	}
	if !input.Reset {
		return nil
	}
	existing.PasswordHash = passwordHash
	existing.MustChangePassword = false
	return service.inTx(ctx, func(ctx context.Context) error {
		if err := service.repo(ctx).SaveUser(ctx, existing); err != nil {
			return err
		}
		if err := service.repo(ctx).DeleteSessionsForUser(ctx, existing.ID); err != nil {
			return err
		}
		return service.writeAudit(ctx, nil, 0, "superadmin.password_reset", "username="+username)
	})
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
