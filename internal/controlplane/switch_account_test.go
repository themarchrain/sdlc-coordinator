package controlplane

import (
	"context"
	"errors"
	"testing"
	"time"
)

// seedTwoUsersWithSessions 造两个各持一条有效会话的成员，返回两条会话 token。
func seedTwoUsersWithSessions(t *testing.T, service *Service, slug string) (string, string) {
	t.Helper()
	tenantID, _ := seedTenantWithAdmin(t, service, slug)
	ctx := context.Background()
	userA, err := service.CreateUser(ctx, "member-a-"+slug, "test-password-001")
	if err != nil {
		t.Fatalf("seed member a: %v", err)
	}
	if _, err := service.AddMembership(ctx, &testActor, tenantID, userA.ID, RoleMember); err != nil {
		t.Fatalf("seed member a membership: %v", err)
	}
	userB, err := service.CreateUser(ctx, "member-b-"+slug, "test-password-001")
	if err != nil {
		t.Fatalf("seed member b: %v", err)
	}
	if _, err := service.AddMembership(ctx, &testActor, tenantID, userB.ID, RoleMember); err != nil {
		t.Fatalf("seed member b membership: %v", err)
	}
	loginA, err := service.Login(ctx, "member-a-"+slug, "test-password-001", "10.8.0.1")
	if err != nil {
		t.Fatalf("login member a: %v", err)
	}
	loginB, err := service.Login(ctx, "member-b-"+slug, "test-password-001", "10.8.0.2")
	if err != nil {
		t.Fatalf("login member b: %v", err)
	}
	return loginA.Token, loginB.Token
}

// ===== SwitchAccount =====

func TestSwitchAccountAcceptsValidSessionToken(t *testing.T) {
	service := newTestService(t)
	tokenA, tokenB := seedTwoUsersWithSessions(t, service, "switch")
	ctx := context.Background()

	if err := service.SwitchAccount(ctx, tokenB); err != nil {
		t.Fatalf("switch to token b: %v", err)
	}
	// 切换不吊销任何一侧会话：两个 token 在切换后都仍有效。
	if err := service.SwitchAccount(ctx, tokenA); err != nil {
		t.Fatalf("switch back to token a: %v", err)
	}
	if _, err := service.activeSession(ctx, tokenB); err != nil {
		t.Fatalf("token b must survive switching away: %v", err)
	}
}

func TestSwitchAccountRejectsInvalidExpiredAndRevokedTokens(t *testing.T) {
	service := newTestService(t)
	tokenA, tokenB := seedTwoUsersWithSessions(t, service, "switch-guard")
	ctx := context.Background()

	if err := service.SwitchAccount(ctx, ""); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("empty token: expected ErrUnauthenticated, got %v", err)
	}
	if err := service.SwitchAccount(ctx, "not-a-real-token"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("unknown token: expected ErrUnauthenticated, got %v", err)
	}
	// 撤销后不可再作为切换目标。
	if err := service.Logout(ctx, tokenB); err != nil {
		t.Fatalf("logout token b: %v", err)
	}
	if err := service.SwitchAccount(ctx, tokenB); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("revoked token: expected ErrUnauthenticated, got %v", err)
	}
	if err := service.SwitchAccount(ctx, tokenA); err != nil {
		t.Fatalf("live token must still switch: %v", err)
	}
}

func TestSwitchAccountRejectsExpiredSessionToken(t *testing.T) {
	repository := openTestControlPlane(t)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	service := NewService(repository)
	tokenA, _ := seedTwoUsersWithSessions(t, service, "expiry")

	// 把会话过期时间拨回过去：activeSession 按当前时钟判定必须拒绝。
	session, err := service.activeSession(context.Background(), tokenA)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.ExpiresAt = service.now().UTC().Add(-time.Minute)
	if err := repository.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("expire session: %v", err)
	}
	if err := service.SwitchAccount(context.Background(), tokenA); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expired token: expected ErrUnauthenticated, got %v", err)
	}
}

// ===== RevokeSavedSessions =====

func TestRevokeSavedSessionsDeletesEachToken(t *testing.T) {
	service := newTestService(t)
	tokenA, tokenB := seedTwoUsersWithSessions(t, service, "revoke")
	ctx := context.Background()

	if err := service.RevokeSavedSessions(ctx, []string{tokenA, tokenB}); err != nil {
		t.Fatalf("revoke saved: %v", err)
	}
	if _, err := service.activeSession(ctx, tokenA); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("token a must be revoked, got %v", err)
	}
	if _, err := service.activeSession(ctx, tokenB); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("token b must be revoked, got %v", err)
	}
}

func TestRevokeSavedSessionsIsIdempotentOnUnknownTokens(t *testing.T) {
	service := newTestService(t)
	tokenA, _ := seedTwoUsersWithSessions(t, service, "revoke-idem")
	ctx := context.Background()

	// 不存在/空白/重复 token 全部静默跳过，已知 token 照常撤销。
	if err := service.RevokeSavedSessions(ctx, []string{"ghost-token", "", "  ", tokenA, tokenA}); err != nil {
		t.Fatalf("revoke with unknown tokens: %v", err)
	}
	if _, err := service.activeSession(ctx, tokenA); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("token a must be revoked, got %v", err)
	}
	// 再跑一次全未知列表仍成功（幂等）。
	if err := service.RevokeSavedSessions(ctx, []string{tokenA}); err != nil {
		t.Fatalf("revoke already-revoked token: %v", err)
	}
}

func TestRevokeSavedSessionsRejectsOverlongList(t *testing.T) {
	service := newTestService(t)
	tokens := make([]string, maxRevokeTokens+1)
	for i := range tokens {
		tokens[i] = "padding-token"
	}
	if err := service.RevokeSavedSessions(context.Background(), tokens); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("overlong list: expected ErrInvalidInput, got %v", err)
	}
}
