// Package clientip derives the real client IP for a request. Proxy headers
// (X-Forwarded-For, X-Real-Ip) are honored only when the immediate peer is in
// the configured trusted-proxy list; otherwise the TCP peer address is used, so
// an untrusted client cannot spoof its IP to bypass rate limiting or lockout.
package clientip

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"connectrpc.com/connect"
)

type ctxKey struct{}

// Resolver resolves client IPs given a set of trusted proxy prefixes.
type Resolver struct {
	trusted []netip.Prefix
}

// NewResolver parses trusted proxies as CIDRs or bare IPs (host prefixes).
// Blank entries are ignored. An empty list disables proxy-header trust.
func NewResolver(entries []string) (*Resolver, error) {
	r := &Resolver{}
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if p, err := netip.ParsePrefix(e); err == nil {
			r.trusted = append(r.trusted, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(e)
		if err != nil {
			return nil, err
		}
		r.trusted = append(r.trusted, netip.PrefixFrom(a, a.BitLen()))
	}

	return r, nil
}

// Trusted reports whether ip (bare, no port) belongs to a trusted proxy.
func (r *Resolver) Trusted(ip string) bool {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	a = a.Unmap()
	for _, p := range r.trusted {
		if p.Contains(a) {
			return true
		}
	}

	return false
}

// TrustedPeer reports whether the TCP peer (host:port or bare host) is a
// trusted proxy.
func (r *Resolver) TrustedPeer(remoteAddr string) bool {
	return r.Trusted(hostOf(remoteAddr))
}

// Resolve returns the client IP for a connection from remoteAddr carrying
// header. When the peer is trusted, X-Forwarded-For is walked right-to-left and
// the first untrusted hop wins (falling back to the leftmost entry, then
// X-Real-Ip). Otherwise the peer address itself is returned.
func (r *Resolver) Resolve(remoteAddr string, header http.Header) string {
	peer := hostOf(remoteAddr)
	if len(r.trusted) == 0 || !r.Trusted(peer) {
		return peer
	}

	if xff := header.Get("X-Forwarded-For"); xff != "" {
		hops := strings.Split(xff, ",")
		for i := len(hops) - 1; i >= 0; i-- {
			h := hostOf(strings.TrimSpace(hops[i]))
			if h == "" {
				continue
			}
			if !r.Trusted(h) {
				return h
			}
		}
		if h := hostOf(strings.TrimSpace(hops[0])); h != "" {
			return h
		}
	}
	if real := hostOf(strings.TrimSpace(header.Get("X-Real-Ip"))); real != "" {
		return real
	}

	return peer
}

// Middleware resolves the client IP once per request and stores it in the
// request context for interceptors and handlers (see Of / FromContext).
func (r *Resolver) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ip := r.Resolve(req.RemoteAddr, req.Header)
		next.ServeHTTP(w, req.WithContext(WithIP(req.Context(), ip)))
	})
}

// WithIP stores a resolved client IP in ctx (exported for tests).
func WithIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, ctxKey{}, ip)
}

// FromContext returns the IP stored by Middleware, or "" when absent.
func FromContext(ctx context.Context) string {
	ip, _ := ctx.Value(ctxKey{}).(string)
	return ip
}

// Of returns the resolved client IP for a connect request, falling back to the
// peer address when Middleware did not run (e.g. handlers invoked in tests).
func Of(ctx context.Context, req connect.AnyRequest) string {
	if ip := FromContext(ctx); ip != "" {
		return ip
	}

	return hostOf(req.Peer().Addr)
}

// hostOf strips a port from addr if present; a bare host is returned as-is.
// IPv4-mapped IPv6 addresses are normalized to IPv4.
func hostOf(addr string) string {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	if a, err := netip.ParseAddr(host); err == nil {
		return a.Unmap().String()
	}

	return host
}
