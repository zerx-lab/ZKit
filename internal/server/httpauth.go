package server

import (
	"net/http"
	"strings"

	casbin "github.com/casbin/casbin/v3"

	"github.com/zerx-lab/zkit/internal/auth"
	"github.com/zerx-lab/zkit/internal/model"
)

// authorizeHTTP parses the bearer access token and authorizes the caller against
// a connectRPC procedure using the same rules as the Casbin interceptor (admin
// bypass; otherwise any role granting the procedure). On failure it writes the
// response and returns ok=false.
func authorizeHTTP(w http.ResponseWriter, r *http.Request, issuer *auth.Issuer, enforcer *casbin.SyncedCachedEnforcer, procedure string) (*auth.Claims, bool) {
	raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	claims, err := issuer.ParseAccess(raw)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	for _, role := range claims.Roles {
		if role == model.RoleAdmin {
			return claims, true
		}
	}
	for _, role := range claims.Roles {
		if allowed, err := enforcer.Enforce(role, procedure); err == nil && allowed {
			return claims, true
		}
	}
	http.Error(w, "forbidden", http.StatusForbidden)
	return nil, false
}
