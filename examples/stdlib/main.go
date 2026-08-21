package main

import (
	"log"
	"net/http"
	"time"

	rl "github.com/Sin7sterSPD/distributed-rate-limiter"
	"github.com/Sin7sterSPD/distributed-rate-limiter/middleware"
)

func main() {
	limiter, err := rl.New(rl.Config{
		Algorithm:    rl.TokenBucket,
		Limit:        10,
		Burst:        10,
		Window:       time.Minute,
		Backend:      "redis",
		RedisAddr:    "localhost:6379",
		RedisTimeout: 100 * time.Millisecond,
		Fallback:     rl.FallbackMemory,
		KeyPrefix:    "myapp",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer limiter.Close()
	// IP-based limiting for public endpoints
	ipMiddleware := middleware.New(middleware.HTTPConfig{
		Limiter: limiter,
		KeyFunc: middleware.DefaultKeyFunc,
	})

	// API-key based limiting for authenticated routes
	apiMiddleware := middleware.New(middleware.HTTPConfig{
		Limiter: limiter,
		KeyFunc: middleware.APIKeyFunc,
	})

	mux := http.NewServeMux()
	mux.Handle("/public/", ipMiddleware(http.HandlerFunc(publicHandler)))
	mux.Handle("/api/", apiMiddleware(http.HandlerFunc(apiHandler)))

	log.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func publicHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`{"status":"ok"}`))
}

func apiHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`{"data":"your data here"}`))
}
