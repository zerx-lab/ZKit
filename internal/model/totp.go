package model

import "time"

// UserTOTP holds a user's TOTP secret and activation state. Secret is stored in
// plaintext in this scaffold; production deployments should encrypt it at rest.
type UserTOTP struct {
	UserID  uint64 `gorm:"primaryKey"`
	Secret  string `gorm:"not null"`
	Enabled bool   `gorm:"not null;default:false"`
	// LastUsedStep is the TOTP time-step (unix/30) of the most recently
	// accepted code; a code for a step <= this value is rejected (replay guard).
	LastUsedStep int64 `gorm:"not null;default:0"`
	ConfirmedAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TOTPRecoveryCode is a single-use 2FA recovery code (bcrypt-hashed).
type TOTPRecoveryCode struct {
	ID       uint64 `gorm:"primaryKey"`
	UserID   uint64 `gorm:"index;not null"`
	CodeHash string `gorm:"not null"`
	UsedAt   *time.Time
}
