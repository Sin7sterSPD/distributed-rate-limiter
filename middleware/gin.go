package middleware

import (
	"net/http"
	"strconv"

	rl "github.com/Sin7sterSPD/distributed-rate-limiter"
	"github.com/gin-gonic/gin"
)

// GinMiddleware returns a Gin middleware function for rate limiting.
// keyFunc can be nil to fall back to DefaultKeyFunc (IP-based).
func GinMiddleware(limiter rl.Limiter, keyFunc rl.KeyFunc) gin.HandlerFunc {
	if keyFunc == nil {
		keyFunc = DefaultKeyFunc
	}
	return func(c *gin.Context) {
		key := keyFunc(c.Request)
		res, err := limiter.Allow(c.Request.Context(), key)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "rate limiter error"})
			return
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(res.Limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))

		if !res.Allowed {
			c.Header("Retry-After", strconv.Itoa(int(res.RetryAfter.Seconds())))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate limit exceeded",
				"retry_after": res.RetryAfter.String(),
			})
			return
		}
		c.Next()
	}
}
