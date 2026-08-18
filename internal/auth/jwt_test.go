package auth

import (
	"slices"
	"testing"
	"time"

	"github.com/zerx-lab/zkit/internal/config"
)

func newIssuer(secret string, access, refresh time.Duration) *Issuer {
	return NewIssuer(config.JWTConfig{Secret: secret, AccessTTL: access, RefreshTTL: refresh})
}

func TestAccessTokenRoundTrip(t *testing.T) {
	issuer := newIssuer("test-secret", 15*time.Minute, time.Hour)

	tok, err := issuer.IssueAccess(42, []string{"admin"})
	if err != nil {
		t.Fatalf("IssueAccess: %v", err)
	}

	claims, err := issuer.ParseAccess(tok)
	if err != nil {
		t.Fatalf("ParseAccess: %v", err)
	}
	if claims.UserID != 42 {
		t.Errorf("UserID = %d, want 42", claims.UserID)
	}
	if !slices.Contains(claims.Roles, "admin") {
		t.Errorf("Roles = %v, want to contain admin", claims.Roles)
	}
	if claims.TokenType != TokenTypeAccess {
		t.Errorf("TokenType = %q, want %q", claims.TokenType, TokenTypeAccess)
	}
}

func TestRefreshTokenCannotBeUsedAsAccess(t *testing.T) {
	issuer := newIssuer("test-secret", 15*time.Minute, time.Hour)

	refresh, err := issuer.IssueRefresh(7, "sess-1")
	if err != nil {
		t.Fatalf("IssueRefresh: %v", err)
	}

	if _, err := issuer.ParseAccess(refresh); err == nil {
		t.Fatal("ParseAccess must reject a refresh token")
	}

	claims, err := issuer.ParseRefresh(refresh)
	if err != nil {
		t.Fatalf("ParseRefresh: %v", err)
	}
	if claims.UserID != 7 {
		t.Errorf("UserID = %d, want 7", claims.UserID)
	}
	if claims.ID != "sess-1" {
		t.Errorf("ID = %q, want sess-1", claims.ID)
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	issuer := newIssuer("test-secret", -time.Minute, time.Hour) // already expired

	tok, err := issuer.IssueAccess(1, []string{"user"})
	if err != nil {
		t.Fatalf("IssueAccess: %v", err)
	}
	if _, err := issuer.ParseAccess(tok); err == nil {
		t.Fatal("ParseAccess must reject an expired token")
	}
}

func TestTokenSignedWithOtherSecretRejected(t *testing.T) {
	issuer := newIssuer("test-secret", 15*time.Minute, time.Hour)
	other := newIssuer("different-secret", 15*time.Minute, time.Hour)

	tok, err := issuer.IssueAccess(1, []string{"user"})
	if err != nil {
		t.Fatalf("IssueAccess: %v", err)
	}
	if _, err := other.ParseAccess(tok); err == nil {
		t.Fatal("ParseAccess must reject a token signed with another secret")
	}
}
