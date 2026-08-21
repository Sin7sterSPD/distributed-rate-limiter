package middleware

import (
	"encoding/json"
	"log/slog"
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
				// Even the limiter's own fallback failed. Fail open: availability
				// beats enforcement (design notes, Flow 4). The fail-open vs
				// fail-closed policy is owned by the limiter via Config.Fallback;
				// this is the last resort when even that path errored.
				slog.Warn("ratelimiter unavailable, failing open", "error", err)
				next.ServeHTTP(w, r)
				return
			}

			// Informational headers: de-facto X-RateLimit-* plus the
			// IETF draft RateLimit-* fields.
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
			w.Header().Set("RateLimit-Limit", strconv.Itoa(res.Limit))
			w.Header().Set("RateLimit-Remaining", strconv.Itoa(res.Remaining))
			if res.RetryAfter > 0 {
				secs := int(res.RetryAfter.Seconds())
				if secs < 1 {
					secs = 1
				}
				w.Header().Set("RateLimit-Reset", strconv.Itoa(secs))
			}

			if !res.Allowed {
				cfg.OnReject(w, r, res)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// DefaultKeyFunc extracts the client IP from the request, trusting no proxy
// headers: it always uses RemoteAddr. This is the safe default — XFF-based
// extraction is spoofable unless you know your proxy topology.
func DefaultKeyFunc(r *http.Request) string {
	return remoteIP(r)
}

// NewTrustedProxyKeyFunc returns a KeyFunc that walks X-Forwarded-For from
// rightmost (the proxy closest to your server) leftwards by proxyHops entries,
// which cannot be spoofed when every entry in the chain is a proxy you control.
//
//	proxyHops = 0 → ignore XFF entirely (same as DefaultKeyFunc)
//	proxyHops = 1 → trust one proxy (LB) in front of this server
//	proxyHops = 2 → trust LB + CDN chain
//
// Falls back to RemoteAddr if the header is missing/shorter than expected.
func NewTrustedProxyKeyFunc(proxyHops int) rl.KeyFunc {
	if proxyHops <= 0 {
		return DefaultKeyFunc
	}
	return func(r *http.Request) string {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			// Walk from rightmost: index len-1 is the nearest proxy.
			idx := len(parts) - proxyHops
			if idx >= 0 {
				if ip := strings.TrimSpace(parts[idx]); ip != "" {
					return ip
				}
			}
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" && proxyHops > 0 {
			return strings.TrimSpace(xri)
		}
		return remoteIP(r)
	}
}

// remoteIP extracts the host portion of RemoteAddr.
func remoteIP(r *http.Request) string {
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

// setRateLimitHeaders writes the de-facto X-RateLimit-* headers plus the
// IETF draft RateLimit-* fields onto an http.Header.
func setRateLimitHeaders(h http.Header, res *rl.Result) {
	h.Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
	h.Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
	h.Set("RateLimit-Limit", strconv.Itoa(res.Limit))
	h.Set("RateLimit-Remaining", strconv.Itoa(res.Remaining))
	if res.RetryAfter > 0 {
		secs := int(res.RetryAfter.Seconds())
		if secs < 1 {
			secs = 1
		}
		h.Set("RateLimit-Reset", strconv.Itoa(secs))
	}
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
