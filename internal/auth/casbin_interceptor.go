package auth

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	casbin "github.com/casbin/casbin/v3"

	"github.com/zerx-lab/zkit/internal/model"
)

// NewCasbinInterceptor authorizes procedures using Casbin (subject = role code,
// object = procedure). Order is fixed to avoid a nil claims dereference:
//
//	disabled plugin -> deny (applies to everyone, incl. admin)
//	public          -> allow (no claims required)
//	selfServe       -> allow for any authenticated caller
//	admin role      -> allow (bypass)
//	otherwise       -> enforce(role, procedure)
//
// procEnabled reports whether a procedure's owning plugin is enabled; it returns
// true for core (non-plugin) procedures. A nil procEnabled disables the gate
// (all procedures treated as enabled).
//
// The auth interceptor must run before this one so non-public procedures carry
// claims.
func NewCasbinInterceptor(enforcer *casbin.SyncedCachedEnforcer, public, selfServe map[string]bool, procEnabled func(string) bool) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			proc := req.Spec().Procedure
			// Disabled plugins are fully off: deny before any allow path, including
			// public/self-serve/admin, so a disabled plugin exposes nothing.
			if procEnabled != nil && !procEnabled(proc) {
				return nil, connect.NewError(connect.CodeUnavailable, errors.New("plugin disabled"))
			}
			if public[proc] {
				return next(ctx, req)
			}

			claims, ok := ClaimsFromContext(ctx)
			if !ok || claims == nil {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
			}

			if selfServe[proc] {
				return next(ctx, req)
			}
			for _, r := range claims.Roles {
				if r == model.RoleAdmin {
					return next(ctx, req)
				}
			}

			for _, r := range claims.Roles {
				allowed, err := enforcer.Enforce(r, proc)
				if err != nil {
					return nil, connect.NewError(connect.CodeInternal, err)
				}
				if allowed {
					return next(ctx, req)
				}
			}
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
		}
	}
}
