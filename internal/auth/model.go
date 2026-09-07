package auth

import "time"

const (
	DefaultAdminUsername    = "admin"
	DefaultAdminPasswordMD5 = "0192023a7bbd73250516f069df18b500"
	SessionCookieName       = "coordinator_admin_session"
)

type AdminUser struct {
	ID          uint   `gorm:"primaryKey"`
	Username    string `gorm:"not null;uniqueIndex"`
	PasswordMD5 string `gorm:"not null"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (AdminUser) TableName() string { return "admin_users" }

type AdminSession struct {
	ID        uint      `gorm:"primaryKey"`
	TokenHash string    `gorm:"not null;uniqueIndex"`
	Username  string    `gorm:"not null;index"`
	ExpiresAt time.Time `gorm:"not null;index"`
	CreatedAt time.Time
}

func (AdminSession) TableName() string { return "admin_sessions" }
