package main

import (
	"time"

	rl "github.com/Sin7sterSPD/distributed-rate-limiter"
	rlmw "github.com/Sin7sterSPD/distributed-rate-limiter/middleware"
	"github.com/gin-gonic/gin"
)

func main() {
	limiter, _ := rl.New(rl.Config{
		Algorithm: rl.SlidingWindow,
		Limit:     60,
		Window:    time.Minute,
		Backend:   "redis",
		RedisAddr: "localhost:6379",
		Fallback:  rl.FallbackAllow, // non-critical path; fail open
	})
	defer limiter.Close()

	r := gin.Default()

	// Apply rate limiting to all /api/* routes
	api := r.Group("/api")
	api.Use(rlmw.GinMiddleware(limiter, rlmw.APIKeyFunc))
	{
		api.GET("/users", func(c *gin.Context) {
			c.JSON(200, gin.H{"users": []string{"alice", "bob"}})
		})
	}

	r.Run(":8080")
}
