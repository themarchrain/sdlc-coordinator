package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (repository *Repository) Migrate() error {
	if err := repository.db.AutoMigrate(&AdminUser{}, &AdminSession{}); err != nil {
		return fmt.Errorf("migrate admin auth: %w", err)
	}
	result := repository.db.Where("username = ?", DefaultAdminUsername).FirstOrCreate(&AdminUser{
		Username: DefaultAdminUsername, PasswordMD5: DefaultAdminPasswordMD5,
	})
	if result.Error != nil {
		return fmt.Errorf("ensure default admin: %w", result.Error)
	}
	return nil
}

func (repository *Repository) GetUser(ctx context.Context, username string) (*AdminUser, error) {
	var user AdminUser
	if err := repository.db.WithContext(ctx).Where("username = ?", strings.TrimSpace(username)).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("get admin user: %w", err)
	}
	return &user, nil
}

func (repository *Repository) CreateSession(ctx context.Context, session *AdminSession) error {
	if err := repository.db.WithContext(ctx).Create(session).Error; err != nil {
		return fmt.Errorf("create admin session: %w", err)
	}
	return nil
}

func (repository *Repository) GetActiveSession(ctx context.Context, tokenHash string, now time.Time) (*AdminSession, error) {
	var session AdminSession
	if err := repository.db.WithContext(ctx).Where("token_hash = ? AND expires_at > ?", tokenHash, now).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUnauthenticated
		}
		return nil, fmt.Errorf("get admin session: %w", err)
	}
	return &session, nil
}
