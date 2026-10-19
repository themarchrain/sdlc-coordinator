package controlplane

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ===== 登录对不存在的用户名也必须无条件执行一次密码比较 =====

// TestLoginComparesPasswordForUnknownUsername 用可计数 seam 锁定「哑比较无条件
// 执行」这一结构性事实：seam 不存在则测试无法编译（红），seam 存在但调用点与
// user==nil 短路合并则计数为 0（红）。
func TestLoginComparesPasswordForUnknownUsername(t *testing.T) {
	repository := openTestControlPlane(t)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	service := NewService(repository)
	comparisons := 0
	service.comparePasswords = func(hashedPassword, password []byte) error {
		comparisons++
		return bcrypt.ErrMismatchedHashAndPassword
	}
	_, err := service.Login(context.Background(), "ghost-user", "whatever-pass", "10.9.9.9")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
	if comparisons != 1 {
		t.Fatalf("unknown username must still perform exactly 1 comparison (anti-enumeration), got %d", comparisons)
	}
}

// ===== 服务层按纯用户名的兜底限速（IP 轮换无效） =====

func TestLoginRateLimitBackstopPerUsername(t *testing.T) {
	service := newTestServiceWithRateLimit(t, time.Hour, 3)
	ctx := context.Background()
	tenantID, _ := seedTenantWithAdmin(t, service, "acme")
	user, err := service.CreateUser(ctx, "backstop-user", "test-member-password")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := service.AddMembership(ctx, &testActor, tenantID, user.ID, RoleMember); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	// 主限速（用户名+IP，阈值 3）用不同 IP 各打 3 次，共 9 次失败，
	// 任何单一 (用户名,IP) 组合都不触顶。
	for i := 0; i < 9; i++ {
		ip := "10.1." + string(rune('a'+i/4)) + "." + string(rune('0'+i%4+1))
		if _, err := service.Login(ctx, "backstop-user", "wrong-password-"+strings.Repeat("x", i%5), ip); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: expected ErrInvalidCredentials, got %v", i+1, err)
		}
	}
	// 兜底限速（纯用户名，阈值 3×3=9）必须顶住：换新 IP + 正确密码也拒绝。
	if _, err := service.Login(ctx, "backstop-user", "test-member-password", "10.200.200.200"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected per-username backstop ErrRateLimited, got %v", err)
	}
	// 其他用户不受波及（键隔离）
	other, err := service.CreateUser(ctx, "other-user", "test-member-password")
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	if _, err := service.AddMembership(ctx, &testActor, tenantID, other.ID, RoleMember); err != nil {
		t.Fatalf("add other membership: %v", err)
	}
	if _, err := service.Login(ctx, "other-user", "test-member-password", "10.200.200.200"); err != nil {
		t.Fatalf("other user must not inherit backstop block, got %v", err)
	}
}

// ===== 注册链补偿（入会失败 → 退名额 + 删孤儿账号） =====

func TestRegistrationCompensationRemovesUserAndRefundsInvite(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	tenantID, adminID := seedTenantWithAdmin(t, service, "acme")
	admin := &Actor{ID: adminID, Username: "admin-acme"}
	code, err := service.IssueInviteCode(ctx, admin, tenantID, InviteCodeInput{ExpiresInHours: 24, MaxUses: 5})
	if err != nil {
		t.Fatalf("issue invite: %v", err)
	}
	user, err := service.CreateUser(ctx, "orphan-user", "test-member-password")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if consumed, err := service.repository.ConsumeInviteCodeUse(ctx, code.Record.ID, service.now().UTC()); err != nil || !consumed {
		t.Fatalf("consume invite: consumed=%v err=%v", consumed, err)
	}

	service.compensateFailedRegistration(ctx, user, code.Record.ID)

	gone, err := service.repository.GetUserByUsername(ctx, "orphan-user")
	if err != nil {
		t.Fatalf("load orphan: %v", err)
	}
	if gone != nil {
		t.Fatal("orphan user must be removed on registration compensation")
	}
	reloaded, err := service.repository.GetInviteCode(ctx, code.Record.ID)
	if err != nil {
		t.Fatalf("reload invite: %v", err)
	}
	if reloaded.UsedCount != 0 {
		t.Fatalf("invite use must be refunded, got used_count=%d", reloaded.UsedCount)
	}
}

// ===== 密码 72 字节上限（bcrypt 硬限，统一在服务层夹紧为 400） =====

func TestPasswordLengthUpperBoundClamped(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	tooLong := strings.Repeat("x", 73)
	if _, err := service.CreateUser(ctx, "longpw-user", tooLong); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("create user with 73-byte password: expected ErrInvalidInput, got %v", err)
	}
	user, err := service.CreateUser(ctx, "clamp-user", "test-member-password")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := service.ChangePassword(ctx, user.ID, "test-member-password", tooLong); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("change password to 73-byte value: expected ErrInvalidInput, got %v", err)
	}
	if err := service.SeedSuperAdmin(ctx, SeedSuperAdminInput{Username: "longseed", Password: tooLong}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("seed with 73-byte password: expected ErrInvalidInput, got %v", err)
	}
}

// ===== 密码上限必须按字节计（bcrypt 72 字节硬限） =====

// TestPasswordLengthClampsByBytes 锁定字节语义：25 个汉字（75 字节/25 rune）
// 必须被服务层拒绝为 400，而不是放过校验后撞 bcrypt 报错（用户路径 500、
// 种子路径拒启动）；边界内的多字节密码（8 个汉字 24 字节）合法。
func TestPasswordLengthClampsByBytes(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	multibyte := strings.Repeat("汉", 25)
	if _, err := service.CreateUser(ctx, "hanzi-user", multibyte); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("25-hanzi (75 bytes) password: expected ErrInvalidInput, got %v", err)
	}
	user, err := service.CreateUser(ctx, "hanzi-ok", strings.Repeat("密", 8))
	if err != nil {
		t.Fatalf("8-hanzi (24 bytes) password should be valid, got %v", err)
	}
	if err := service.ChangePassword(ctx, user.ID, strings.Repeat("密", 8), multibyte); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("change password to 75-byte value: expected ErrInvalidInput, got %v", err)
	}
	if err := service.SeedSuperAdmin(ctx, SeedSuperAdminInput{Username: "hanzi-seed", Password: multibyte}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("seed with 75-byte password: expected ErrInvalidInput, got %v", err)
	}
}

func newTestServiceWithRateLimit(t *testing.T, window time.Duration, maxFailures int) *Service {
	t.Helper()
	repository := openTestControlPlane(t)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewService(repository, WithRateLimit(window, maxFailures))
}
