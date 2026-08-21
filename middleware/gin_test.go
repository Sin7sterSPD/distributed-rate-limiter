package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	rl "github.com/Sin7sterSPD/distributed-rate-limiter"

	"github.com/gin-gonic/gin"
)

func newGinEngine(limiter rl.Limiter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinMiddleware(limiter, nil))
	r.GET("/x", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return r
}

func TestGinAllowed(t *testing.T) {
	r := newGinEngine(&fakeLimiter{res: allowRes()})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "7" {
		t.Errorf("X-RateLimit-Remaining = %q", got)
	}
}

func TestGinRejected429Aborts(t *testing.T) {
	r := newGinEngine(&fakeLimiter{res: denyRes()})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

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

func TestGinErrFailsOpen(t *testing.T) {
	r := newGinEngine(&fakeLimiter{err: context.DeadlineExceeded})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("middleware must fail open, got %d %q", rec.Code, rec.Body.String())
	}
}
