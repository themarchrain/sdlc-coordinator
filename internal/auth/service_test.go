package auth

import (
	"context"
	"testing"
	"time"

	"sdlc-coordinator/internal/store"
)

func TestMigrateEnsuresDefaultAdminWithoutOverwritingExistingPassword(t *testing.T) {
	db, err := store.Open("file:auth-default-admin?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repository := NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate auth repository: %v", err)
	}

	admin, err := repository.GetUser(context.Background(), "admin")
	if err != nil {
		t.Fatalf("get default admin: %v", err)
	}
	if admin.PasswordMD5 != DefaultAdminPasswordMD5 {
		t.Fatalf("default password hash = %q, want %q", admin.PasswordMD5, DefaultAdminPasswordMD5)
	}

	admin.PasswordMD5 = "custom-password-hash"
	if err := db.Save(admin).Error; err != nil {
		t.Fatalf("save custom password: %v", err)
	}
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate existing auth repository: %v", err)
	}
	admin, err = repository.GetUser(context.Background(), "admin")
	if err != nil {
		t.Fatalf("get existing admin: %v", err)
	}
	if admin.PasswordMD5 != "custom-password-hash" {
		t.Fatalf("existing password was overwritten: %q", admin.PasswordMD5)
	}
}

func TestServiceCreatesAndValidatesPersistentSession(t *testing.T) {
	db, err := store.Open("file:auth-session?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repository := NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate auth repository: %v", err)
	}
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	service := NewService(repository, WithClock(func() time.Time { return now }))

	token, user, err := service.Login(context.Background(), "admin", "admin123")
	if err != nil {
		t.Fatalf("login default admin: %v", err)
	}
	if user.Username != "admin" || token == "" {
		t.Fatalf("unexpected login result: user=%#v token=%q", user, token)
	}
	if _, _, err := service.Login(context.Background(), "admin", "wrong"); err != ErrInvalidCredentials {
		t.Fatalf("wrong password error = %v, want ErrInvalidCredentials", err)
	}

	session, err := service.Session(context.Background(), token)
	if err != nil {
		t.Fatalf("validate created session: %v", err)
	}
	if session.Username != "admin" || !session.ExpiresAt.Equal(now.Add(DefaultSessionTTL)) {
		t.Fatalf("unexpected session: %#v", session)
	}
	if err := db.Where("token_hash = ?", token).First(&AdminSession{}).Error; err == nil {
		t.Fatal("raw session token must not be persisted")
	}
}

func TestServiceRejectsExpiredSession(t *testing.T) {
	db, err := store.Open("file:auth-expired-session?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repository := NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate auth repository: %v", err)
	}
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	service := NewService(repository, WithClock(func() time.Time { return now }))
	token, _, err := service.Login(context.Background(), "admin", "admin123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	now = now.Add(DefaultSessionTTL + time.Second)
	if _, err := service.Session(context.Background(), token); err != ErrUnauthenticated {
		t.Fatalf("expired session error = %v, want ErrUnauthenticated", err)
	}
}
