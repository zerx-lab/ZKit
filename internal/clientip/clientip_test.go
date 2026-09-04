package clientip

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveIgnoresHeadersFromUntrustedPeer(t *testing.T) {
	r, err := NewResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	h := http.Header{"X-Forwarded-For": {"1.2.3.4"}, "X-Real-Ip": {"5.6.7.8"}}
	if got := r.Resolve("9.9.9.9:1234", h); got != "9.9.9.9" {
		t.Fatalf("got %q, want peer", got)
	}
}

func TestResolveWalksForwardedChainFromTrustedPeer(t *testing.T) {
	r, err := NewResolver([]string{"10.0.0.0/8", "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		remote string
		header http.Header
		want   string
	}{
		{"single hop", "10.1.1.1:80", http.Header{"X-Forwarded-For": {"203.0.113.9"}}, "203.0.113.9"},
		{"skips trusted hops", "10.1.1.1:80", http.Header{"X-Forwarded-For": {"203.0.113.9, 10.2.2.2"}}, "203.0.113.9"},
		{"client-spoofed prefix is ignored", "10.1.1.1:80", http.Header{"X-Forwarded-For": {"1.1.1.1, 203.0.113.9"}}, "203.0.113.9"},
		{"all trusted falls back to leftmost", "10.1.1.1:80", http.Header{"X-Forwarded-For": {"10.5.5.5, 10.2.2.2"}}, "10.5.5.5"},
		{"x-real-ip fallback", "127.0.0.1:80", http.Header{"X-Real-Ip": {"198.51.100.3"}}, "198.51.100.3"},
		{"no headers returns peer", "127.0.0.1:80", http.Header{}, "127.0.0.1"},
		{"ipv6 mapped normalized", "[::ffff:10.1.1.1]:80", http.Header{"X-Forwarded-For": {"[2001:db8::1]:443"}}, "2001:db8::1"},
		{"untrusted peer despite list", "8.8.8.8:80", http.Header{"X-Forwarded-For": {"1.1.1.1"}}, "8.8.8.8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := r.Resolve(tc.remote, tc.header); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewResolverRejectsGarbage(t *testing.T) {
	if _, err := NewResolver([]string{"not-an-ip"}); err == nil {
		t.Fatal("expected parse error")
	}
	r, err := NewResolver([]string{" ", ""})
	if err != nil || len(r.trusted) != 0 {
		t.Fatalf("blank entries should be ignored: %v %v", err, r.trusted)
	}
}

func TestMiddlewareStoresIPInContext(t *testing.T) {
	r, _ := NewResolver([]string{"127.0.0.1"})
	var got string
	h := r.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		got = FromContext(req.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "203.0.113.7" {
		t.Fatalf("got %q", got)
	}
	if FromContext(context.Background()) != "" {
		t.Fatal("empty context should yield empty ip")
	}
}
