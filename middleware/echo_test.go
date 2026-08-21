package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	rl "github.com/Sin7sterSPD/distributed-rate-limiter"

	"github.com/labstack/echo/v5"
)

func newEchoServer(limiter rl.Limiter) *echo.Echo {
	e := echo.New()
	e.Use(EchoMiddleware(limiter, nil))
	e.GET("/x", func(c *echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	return e
}

func TestEchoAllowed(t *testing.T) {
	e := newEchoServer(&fakeLimiter{res: allowRes()})

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "7" {
		t.Errorf("X-RateLimit-Remaining = %q", got)
	}
}

func TestEchoRejected429Aborts(t *testing.T) {
	e := newEchoServer(&fakeLimiter{res: denyRes()})

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "30" {
		t.Errorf("Retry-After = %q, want 30", got)
	}
	if rec.Body.String() == "ok" {
		t.Error("handler must not run on rejection")
	}
}

func TestEchoErrFailsOpen(t *testing.T) {
	e := newEchoServer(&fakeLimiter{err: context.DeadlineExceeded})

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("middleware must fail open, got %d %q", rec.Code, rec.Body.String())
	}
}
