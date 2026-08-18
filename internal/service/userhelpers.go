package service

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/zerx-lab/zkit/internal/model"
)

// auditJSON marshals an audit-detail map; on error returns an empty object.
func auditJSON(v map[string]any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// userRoleCodes returns the role codes assigned to a user.
func userRoleCodes(ctx context.Context, db *gorm.DB, userID uint64) ([]string, error) {
	rows, err := gorm.G[model.UserRole](db).Where("user_id = ?", userID).Find(ctx)
	if err != nil {
		return nil, err
	}
	codes := make([]string, 0, len(rows))
	for i := range rows {
		codes = append(codes, rows[i].RoleCode)
	}

	return codes, nil
}

// rolesByUserIDs returns a map of user ID to role codes for the given users
// (single query, avoiding N+1).
func rolesByUserIDs(ctx context.Context, db *gorm.DB, ids []uint64) (map[uint64][]string, error) {
	out := make(map[uint64][]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := gorm.G[model.UserRole](db).Where("user_id IN ?", ids).Find(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		out[rows[i].UserID] = append(out[rows[i].UserID], rows[i].RoleCode)
	}

	return out, nil
}

// totpEnabledByUserIDs returns a map of user ID to TOTP-enabled state.
func totpEnabledByUserIDs(ctx context.Context, db *gorm.DB, ids []uint64) (map[uint64]bool, error) {
	out := make(map[uint64]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := gorm.G[model.UserTOTP](db).Where("user_id IN ? AND enabled = ?", ids, true).Find(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		out[rows[i].UserID] = true
	}

	return out, nil
}

// userTOTPEnabled reports whether a single user has 2FA enabled.
func userTOTPEnabled(ctx context.Context, db *gorm.DB, userID uint64) (bool, error) {
	t, err := gorm.G[model.UserTOTP](db).Where("user_id = ?", userID).First(ctx)
	if err != nil {
		return false, nil //nolint:nilerr // absent row means disabled
	}

	return t.Enabled, nil
}

// rolesExist reports whether every code is present in the roles table.
func rolesExist(ctx context.Context, db *gorm.DB, codes []string) (bool, error) {
	if len(codes) == 0 {
		return false, nil
	}
	n, err := gorm.G[model.Role](db).Where("code IN ?", codes).Count(ctx, "code")
	if err != nil {
		return false, err
	}

	return int(n) == len(unique(codes)), nil
}

func unique(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}

	return out
}

// onConflictUserTOTP upserts a UserTOTP row on its user_id primary key.
func onConflictUserTOTP() clause.OnConflict {
	return clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"secret", "enabled", "updated_at"}),
	}
}

// encodePNG encodes an image to PNG bytes.
func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
