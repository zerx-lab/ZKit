package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	connectcors "connectrpc.com/cors"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/rs/cors"

	"github.com/zerx-lab/zkit/internal/clientip"
)

type requestIDKey struct{}

// requestIDHeader is read from trusted-looking inbound values and always
// echoed on the response so clients and proxies can correlate logs.
const requestIDHeader = "X-Request-Id"

// maxRequestIDLen bounds inbound ids so a client cannot bloat log lines.
const maxRequestIDLen = 64

// withRequestID assigns each request an id (inbound X-Request-Id when it is
// short and printable, else a fresh UUID), stores it in ctx, and echoes it.
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if !validRequestID(id) {
			id = uuid.NewString()
		}
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLen {
		return false
	}
	for i := range len(id) {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
		default:
			return false
		}
	}

	return true
}

// requestIDFrom returns the id assigned by withRequestID, or "" outside it.
func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// defaultCSP locks the SPA down to same-origin scripts. Inline styles stay
// allowed (Radix/Recharts set style attributes); images may come from any
// https/http origin (S3 public URLs, external avatars).
const defaultCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob: https: http:; font-src 'self' data:; connect-src 'self'; " +
	"media-src 'self' https: http:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'; object-src 'none'"

// withSecurityHeaders sets defense-in-depth response headers. HSTS is emitted
// only when the request demonstrably arrived over TLS (direct or via a trusted
// proxy reporting X-Forwarded-Proto=https) so plain-HTTP dev never pins.
func withSecurityHeaders(csp string, proxies *clientip.Resolver, next http.Handler) http.Handler {
	if csp == "" {
		csp = defaultCSP
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if isTLS(r, proxies) {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func isTLS(r *http.Request, proxies *clientip.Resolver) bool {
	if r.TLS != nil {
		return true
	}

	return proxies.TrustedPeer(r.RemoteAddr) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// withCORS enables cross-origin connectRPC calls for the given origins (used
// only for split-domain SPA deployments; empty list = no CORS handling).
func withCORS(origins []string, next http.Handler) http.Handler {
	if len(origins) == 0 {
		return next
	}
	c := cors.New(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   connectcors.AllowedMethods(),
		AllowedHeaders:   append(connectcors.AllowedHeaders(), "Authorization", requestIDHeader),
		ExposedHeaders:   append(connectcors.ExposedHeaders(), requestIDHeader),
		AllowCredentials: false,
		MaxAge:           7200,
	})

	return c.Handler(next)
}

// errInternal is the only CodeInternal message ever sent to clients.
var errInternal = errors.New("internal error")

// NewErrorSanitizerInterceptor replaces the message of CodeInternal / CodeUnknown
// errors with a fixed string so driver/ORM details never reach clients. It sits
// outermost, so the logging and operation-log interceptors inside it still see
// (and persist) the original error.
func NewErrorSanitizerInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			res, err := next(ctx, req)
			if err == nil {
				return res, nil
			}
			switch code := connect.CodeOf(err); code {
			case connect.CodeInternal, connect.CodeUnknown:
				return nil, connect.NewError(code, errInternal)
			default:
				return res, err
			}
		}
	}
}
