package auth

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthenticated    = errors.New("unauthenticated")
)

const DefaultSessionTTL = 7 * 24 * time.Hour

type Service struct {
	repository *Repository
	now        func() time.Time
}

type Option func(*Service)

func WithClock(clock func() time.Time) Option {
	return func(service *Service) {
		if clock != nil {
			service.now = clock
		}
	}
}

func NewService(repository *Repository, options ...Option) *Service {
	service := &Service{repository: repository, now: time.Now}
	for _, option := range options {
		option(service)
	}
	return service
}

func (service *Service) Login(ctx context.Context, username, password string) (string, AdminUser, error) {
	user, err := service.repository.GetUser(ctx, username)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return "", AdminUser{}, ErrInvalidCredentials
		}
		return "", AdminUser{}, err
	}
	passwordHash := md5Hex(password)
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(user.PasswordMD5)), []byte(passwordHash)) != 1 {
		return "", AdminUser{}, ErrInvalidCredentials
	}
	token, err := newToken()
	if err != nil {
		return "", AdminUser{}, err
	}
	now := service.now().UTC()
	if err := service.repository.CreateSession(ctx, &AdminSession{
		TokenHash: tokenHash(token), Username: user.Username, ExpiresAt: now.Add(DefaultSessionTTL),
	}); err != nil {
		return "", AdminUser{}, err
	}
	return token, *user, nil
}

func (service *Service) Session(ctx context.Context, token string) (*AdminSession, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ErrUnauthenticated
	}
	session, err := service.repository.GetActiveSession(ctx, tokenHash(token), service.now().UTC())
	if err != nil {
		if errors.Is(err, ErrUnauthenticated) {
			return nil, ErrUnauthenticated
		}
		return nil, err
	}
	return session, nil
}

func md5Hex(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
