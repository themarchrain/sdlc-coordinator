package controlplane

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// F1（审查发现）：used_count 读-查-写非原子，并发兑换同一次数上限的邀请码
// 会超发成员。修复后并发成功数不得超过 max_uses。
func TestInviteCodeUseLimitAtomicUnderConcurrency(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	tenantID, adminID := seedTenantWithAdmin(t, service, "acme")
	admin := &Actor{ID: adminID, Username: "admin-acme"}
	code, err := service.IssueInviteCode(ctx, admin, tenantID, InviteCodeInput{ExpiresInHours: 24, MaxUses: 2})
	if err != nil {
		t.Fatalf("issue invite: %v", err)
	}

	const attempts = 8
	const maxUses = 2
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := service.RegisterWithInviteCode(
				ctx,
				fmt.Sprintf("race-user-%d", n),
				"test-race-password",
				code.Plaintext,
			)
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes > maxUses {
		t.Fatalf("invite code overbooked: %d registrations succeeded with max_uses=%d", successes, maxUses)
	}
	reloaded, err := service.repository.GetInviteCode(ctx, code.Record.ID)
	if err != nil {
		t.Fatalf("reload invite: %v", err)
	}
	if reloaded.UsedCount > maxUses {
		t.Fatalf("used_count exceeded max_uses: %d > %d", reloaded.UsedCount, maxUses)
	}
	if reloaded.UsedCount != successes {
		t.Fatalf("used_count %d does not match successful registrations %d", reloaded.UsedCount, successes)
	}
}

// F2（审查发现）：组织停用不应删除支持访问中的超管会话本体，只应清除其
// 支持上下文；组织成员会话仍须删除。
func TestDisableTenantKeepsSuperAdminSessionButClearsSupport(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	tenantID, _ := seedTenantWithAdmin(t, service, "acme")

	// 超管登录并进入支持访问
	superID, err := service.CreateSuperAdminUser(ctx, "root", "test-super-password")
	if err != nil {
		t.Fatalf("seed super admin: %v", err)
	}
	superLogin, err := service.Login(ctx, "root", "test-super-password", "10.0.0.2")
	if err != nil {
		t.Fatalf("super login: %v", err)
	}
	if err := service.EnterSupportAccess(ctx, superLogin.Token, tenantID, "停用前排障"); err != nil {
		t.Fatalf("enter support: %v", err)
	}

	// 普通成员登录
	user, err := service.CreateUser(ctx, "member-x", "test-member-password")
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	if _, err := service.AddMembership(ctx, &Actor{ID: superID, Username: "root"}, tenantID, user.ID, RoleMember); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	memberLogin, err := service.Login(ctx, "member-x", "test-member-password", "10.0.0.1")
	if err != nil {
		t.Fatalf("member login: %v", err)
	}

	if _, err := service.DisableTenant(ctx, &Actor{ID: superID, Username: "root"}, tenantID); err != nil {
		t.Fatalf("disable tenant: %v", err)
	}

	// 成员会话删除
	if _, err := service.ValidateSession(ctx, memberLogin.Token); err == nil {
		t.Fatal("member session should be revoked on tenant disable")
	}
	// 超管会话本体保留，支持上下文清除
	superCtx, err := service.ValidateSession(ctx, superLogin.Token)
	if err != nil {
		t.Fatalf("super admin session must survive tenant disable, got %v", err)
	}
	if superCtx.SupportMode {
		t.Fatal("support context must be cleared on tenant disable")
	}
	if superCtx.EffectiveTenant != nil {
		t.Fatalf("support tenant must be detached, got %+v", superCtx.EffectiveTenant)
	}
}

// F4（审查发现）：邀请码签发参数负值在服务层应拒绝，而非签出「已过期」废码。
func TestIssueInviteCodeRejectsNegativeParameters(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	tenantID, adminID := seedTenantWithAdmin(t, service, "acme")
	admin := &Actor{ID: adminID, Username: "admin-acme"}

	if _, err := service.IssueInviteCode(ctx, admin, tenantID, InviteCodeInput{ExpiresInHours: -5, MaxUses: 5}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("negative expires_in_hours: expected ErrInvalidInput, got %v", err)
	}
	if _, err := service.IssueInviteCode(ctx, admin, tenantID, InviteCodeInput{ExpiresInHours: 24, MaxUses: -1}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("negative max_uses: expected ErrInvalidInput, got %v", err)
	}
}

// F6（审查发现）：限速失败表按用户名+IP 组合无上限增长，可被伪造请求撑爆内存。
// 修复后表容量有上限，超限时淘汰最旧条目且限速语义不变。
func TestLoginRateLimiterCapsTrackedKeys(t *testing.T) {
	limiter := newLoginRateLimiter(time.Hour, 3, 4)
	for i := 0; i < 16; i++ {
		limiter.record("key-" + string(rune('a'+i%26)) + string(rune('0'+i)))
	}
	limiter.mu.Lock()
	size := len(limiter.failures)
	limiter.mu.Unlock()
	if size > 4 {
		t.Fatalf("rate limiter tracked %d keys, want capped at 4", size)
	}
}

// 停用组织须连带删除其路由索引行：webhook 立即失去租户路由，引擎层的
// 停用状态门是第二道防线。
func TestDisableTenantRemovesRepoRouteIndexRows(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	tenantID, _ := seedTenantWithAdmin(t, service, "route-freeze")

	identity := RepoRouteIdentity{Provider: "gitlab", BaseURL: "https://gitlab.example.com", PathWithNamespace: "acme/frozen"}
	if err := service.Repository().UpsertRepoRouteIndex(ctx, RepoRouteIndex{
		Provider: identity.Provider, BaseURL: identity.BaseURL, PathWithNamespace: identity.PathWithNamespace,
		TenantID: tenantID, RepoFingerprint: "repo-frozen",
	}); err != nil {
		t.Fatalf("seed route index: %v", err)
	}

	if _, err := service.DisableTenant(ctx, &Actor{ID: 1, Username: "root"}, tenantID); err != nil {
		t.Fatalf("disable tenant: %v", err)
	}
	if _, err := service.Repository().GetRepoRouteIndex(ctx, identity); err == nil {
		t.Fatal("disable must remove the tenant's route index rows")
	}
}
