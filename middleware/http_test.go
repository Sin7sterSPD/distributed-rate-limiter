package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	rl "github.com/Sin7sterSPD/distributed-rate-limiter"
)

// fakeLimiter returns canned results/errors without touching any backend.
type fakeLimiter struct {
	res *rl.Result
	err error
}

func (f *fakeLimiter) Allow(ctx context.Context, key string) (*rl.Result, error) {
	return f.res, f.err
}
func (f *fakeLimiter) AllowN(ctx context.Context, key string, n int) (*rl.Result, error) {
	return f.res, f.err
}
func (f *fakeLimiter) Close() error { return nil }

func allowRes() *rl.Result {
	return &rl.Result{Allowed: true, Remaining: 7, Limit: 10, Window: time.Minute}
}

func denyRes() *rl.Result {
	return &rl.Result{Allowed: false, Remaining: 0, RetryAfter: 30 * time.Second, Limit: 10, Window: time.Minute}
}

func newReq(t *testing.T) *http.Request {
	t.Helper()
	r := httptest.NewRequest("GET", "/x", nil)
	r.RemoteAddr = "9.9.9.9:12345"
	return r
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func TestHTTPAllowedSetsHeaders(t *testing.T) {
	lim := &fakeLimiter{res: allowRes()}
	h := New(HTTPConfig{Limiter: lim})(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq(t))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	for name, want := range map[string]string{
		"X-RateLimit-Limit":     "10",
		"X-RateLimit-Remaining": "7",
		"RateLimit-Limit":       "10",
		"RateLimit-Remaining":   "7",
	} {
		if got := rec.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if body := rec.Body.String(); body != "ok" {
		t.Errorf("body = %q", body)
	}
}

func TestHTTPRejectedReturns429(t *testing.T) {
	lim := &fakeLimiter{res: denyRes()}
	h := New(HTTPConfig{Limiter: lim})(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq(t))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "30" {
		t.Errorf("Retry-After = %q, want 30", got)
	}
	if got := rec.Header().Get("RateLimit-Reset"); got == "" {
		t.Error("RateLimit-Reset should be set on rejection")
	}
	if !strings.Contains(rec.Body.String(), "rate limit exceeded") {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestHTTPErrFailsOpen(t *testing.T) {
	lim := &fakeLimiter{err: errors.New("backend exploded")}
	h := New(HTTPConfig{Limiter: lim})(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq(t))

	if rec.Code != http.StatusOK {
		t.Fatalf("middleware must fail open (200), got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("next handler should run, body=%q", rec.Body.String())
	}
}

func TestDefaultKeyFuncUsesRemoteAddrOnly(t *testing.T) {
	got := DefaultKeyFunc(newReq(t))
	if got != "9.9.9.9" {
		t.Errorf("expected 9.9.9.9, got %q", got)
	}
}

func TestTrustedProxyKeyFunc(t *testing.T) {
	r := newReq(t)
	r.Header.Set("X-Forwarded-For", "spoofed, 6.6.6.6")

	if got := NewTrustedProxyKeyFunc(1)(r); got != "6.6.6.6" {
		t.Errorf("hops=1: expected rightmost-minus-1 (6.6.6.6), got %q", got)
	}
	if got := NewTrustedProxyKeyFunc(2)(r); got != "spoofed" {
		t.Errorf("hops=2: expected 'spoofed', got %q", got)
	}
	if got := NewTrustedProxyKeyFunc(0)(r); got != "9.9.9.9" {
		t.Errorf("hops=0 must ignore XFF, got %q", got)
	}

	r2 := newReq(t) // no XFF header
	if got := NewTrustedProxyKeyFunc(1)(r2); got != "9.9.9.9" {
		t.Errorf("missing XFF should fall back to RemoteAddr, got %q", got)
	}
}

func TestAPIKeyFunc(t *testing.T) {
	r := newReq(t)
	r.Header.Set("X-API-Key", "key-123")
	if got := APIKeyFunc(r); got != "key-123" {
		t.Errorf("X-API-Key should win, got %q", got)
	}

	r2 := newReq(t)
	r2.Header.Set("Authorization", "Bearer tok-456")
	if got := APIKeyFunc(r2); got != "tok-456" {
		t.Errorf("Bearer token expected, got %q", got)
	}

	if got := APIKeyFunc(newReq(t)); got != "9.9.9.9" {
		t.Errorf("no headers should fall back to IP, got %q", got)
	}
}
