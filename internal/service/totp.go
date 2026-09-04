package service

import (
	"context"
	"crypto/subtle"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/zerx-lab/zkit/internal/model"
)

// totpPeriod mirrors totp.Validate defaults (Period 30, Skew 1, 6 digits, SHA1).
const totpPeriod = 30

var totpOpts = totp.ValidateOpts{
	Period:    totpPeriod,
	Skew:      0, // step-by-step matching below handles the skew window
	Digits:    otp.DigitsSix,
	Algorithm: otp.AlgorithmSHA1,
}

// matchTOTPStep searches the skew window (previous, current, next step) for the
// time-step whose code equals the submitted one. Comparison is constant-time.
// The returned step is unix/30 of the matching window, not of `now`.
func matchTOTPStep(secret, code string, now time.Time) (step int64, ok bool) {
	if len(code) != otp.DigitsSix.Length() {
		return 0, false
	}
	cur := now.Unix() / totpPeriod
	for _, s := range [3]int64{cur - 1, cur, cur + 1} {
		want, err := totp.GenerateCodeCustom(secret, time.Unix(s*totpPeriod, 0), totpOpts)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

// consumeTOTP validates code against secret and atomically advances
// user_totps.last_used_step to the matching step. A code whose step is <= the
// stored value (replay, or concurrent double-submit losing the race) is
// rejected: the conditional UPDATE affects zero rows.
func (s *AuthService) consumeTOTP(ctx context.Context, userID uint64, secret, code string) bool {
	step, ok := matchTOTPStep(secret, code, time.Now())
	if !ok {
		return false
	}
	res := s.db.WithContext(ctx).Model(&model.UserTOTP{}).
		Where("user_id = ? AND last_used_step < ?", userID, step).
		Update("last_used_step", step)
	return res.Error == nil && res.RowsAffected == 1
}
