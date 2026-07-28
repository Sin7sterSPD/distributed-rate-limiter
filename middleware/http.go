package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"

	rl "github.com/Sin7sterSPD/distributed-rate-limiter"
)

// HTTPConfig configures the net/http middleware.
type HTTPConfig struct {
	// Limiter is the rate limiter instance (required).
	Limiter rl.Limiter

	// KeyFunc extracts the rate-limit key from each request.
	// Defaults to DefaultKeyFunc (client IP extraction).
	KeyFunc rl.KeyFunc

	// OnReject is called when a request is rate-limited.
	// Defaults to DefaultRejectHandler.
	OnReject func(http.ResponseWriter, *http.Request, *rl.Result)
}

// New wraps an http.Handler with rate limiting.
// It is compatible with any framework that accepts
// func(http.Handler) http.Handler middleware (Chi, stdlib mux, etc.).
func New(cfg HTTPConfig) func(http.Handler) http.Handler {
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = DefaultKeyFunc
	}
	if cfg.OnReject == nil {
		cfg.OnReject = DefaultRejectHandler
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := cfg.KeyFunc(r)

			res, err := cfg.Limiter.Allow(r.Context(), key)
			if err != nil {
				// Backend error (not a limit exceeded) — treat as internal error.
				// The limiter's own fallback logic should prevent this from being common.
				http.Error(w, `{"error":"rate limiter internal error"}`, http.StatusInternalServerError)
				return
			}

			// Always set informational headers (RFC 6585 + de-facto standards)
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
			w.Header().Set("X-RateLimit-Window", res.Window.String())

			if !res.Allowed {
				cfg.OnReject(w, r, res)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// DefaultKeyFunc extracts the client IP from the request.
// Trust chain: X-Forwarded-For (leftmost) → X-Real-IP → RemoteAddr.
// Note: Only trust X-Forwarded-For if your infrastructure sets it reliably.
func DefaultKeyFunc(r *http.Request) string {
	// X-Forwarded-For can contain a comma-separated chain; use leftmost (client IP)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// RemoteAddr is "host:port"; extract just the host
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// APIKeyFunc extracts the API key from the Authorization header (Bearer token)
// or a custom X-API-Key header. Use this for per-API-key rate limiting.
func APIKeyFunc(r *http.Request) string {
	if key := r.Header.Get("X-API-Key"); key != "" {
		return key
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return DefaultKeyFunc(r) // fallback to IP
}

// DefaultRejectHandler sends a 429 Too Many Requests response
// with standard rate-limit headers and a JSON body.
func DefaultRejectHandler(w http.ResponseWriter, r *http.Request, res *rl.Result) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", strconv.Itoa(int(res.RetryAfter.Seconds())))
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error":       "rate limit exceeded",
		"retry_after": res.RetryAfter.String(),
		"limit":       res.Limit,
	})
}
