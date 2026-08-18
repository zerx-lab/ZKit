package auth

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/internal/config"
	"github.com/zerx-lab/zkit/internal/database"
)

func newPolicyDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(db, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestPolicyValidate(t *testing.T) {
	p := NewPolicy(config.PasswordPolicyConfig{
		MinLength:    8,
		RequireDigit: true,
	})

	tests := []struct {
		name    string
		pw      string
		wantErr bool
	}{
		{"too short", "abc123", true},
		{"no digit", "abcdefgh", true},
		{"exactly min length with digit", "abcde12a", false},
		{"long with digit", "LongPassword9!", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := p.Validate(tc.pw)
			if tc.wantErr && err == nil {
				t.Errorf("Validate(%q) = nil, want error", tc.pw)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Validate(%q) = %v, want nil", tc.pw, err)
			}
		})
	}
}

func TestPolicyCheckHistory(t *testing.T) {
	db := newPolicyDB(t)
	p := NewPolicy(config.PasswordPolicyConfig{MinLength: 6, HistoryCount: 3})
	ctx := context.Background()
	userID := uint64(999)

	pw1 := "password1"
	hash1, err := Hash(pw1)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	// Record the first password.
	if err := p.RecordHistory(ctx, db, userID, hash1); err != nil {
		t.Fatalf("RecordHistory: %v", err)
	}

	// Reusing pw1 must be rejected.
	if err := p.CheckHistory(ctx, db, userID, pw1); err == nil {
		t.Error("CheckHistory: expected rejection for recently used password, got nil")
	}

	// A different password must pass.
	if err := p.CheckHistory(ctx, db, userID, "different99"); err != nil {
		t.Errorf("CheckHistory: unexpected rejection for new password: %v", err)
	}
}
