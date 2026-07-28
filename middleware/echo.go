package middleware

import (
	"net/http"
	"strconv"

	rl "github.com/Sin7sterSPD/distributed-rate-limiter"
	"github.com/labstack/echo/v5"
)

// EchoMiddleware returns an Echo middleware for rate limiting.
func EchoMiddleware(limiter rl.Limiter, keyFunc rl.KeyFunc) echo.MiddlewareFunc {
	if keyFunc == nil {
		keyFunc = DefaultKeyFunc
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := keyFunc(c.Request())
			res, err := limiter.Allow(c.Request().Context(), key)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "rate limiter error"})
			}

			c.Response().Header().Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
			c.Response().Header().Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))

			if !res.Allowed {
				c.Response().Header().Set("Retry-After", strconv.Itoa(int(res.RetryAfter.Seconds())))
				return c.JSON(http.StatusTooManyRequests, map[string]interface{}{
					"error":       "rate limit exceeded",
					"retry_after": res.RetryAfter.String(),
				})
			}
			return next(c)
		}
	}
}
