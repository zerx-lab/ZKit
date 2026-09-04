// Package model holds GORM data models and the query-codegen input interfaces.
package model

import (
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Role constants for the minimal RBAC scheme.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// deletedEmailPrefix marks a soft-deleted user's email. Email carries a plain
// unique index (NULL-distinct composite indexes cannot enforce liveness across
// sqlite/postgres/mysql), so DeleteUser rewrites the address to free it for
// re-registration while keeping the row for audit joins.
const deletedEmailPrefix = "deleted:"

// TombstoneEmail returns the rewritten email for a soft-deleted user; the
// original stays recoverable after the second ':'. Idempotent.
func TombstoneEmail(id uint64, email string) string {
	if IsTombstonedEmail(email) {
		return email
	}

	return deletedEmailPrefix + strconv.FormatUint(id, 10) + ":" + email
}

// IsTombstonedEmail reports whether email was rewritten by TombstoneEmail.
func IsTombstonedEmail(email string) bool {
	return strings.HasPrefix(email, deletedEmailPrefix)
}

// User is an account. PasswordHash is never exposed over the API.
type User struct {
	ID           uint64 `gorm:"primaryKey"`
	Email        string `gorm:"uniqueIndex;not null"`
	Name         string
	Nickname     string
	Avatar       string
	Phone        string
	PasswordHash string `gorm:"not null"`
	Status       bool   `gorm:"not null;default:true"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}
