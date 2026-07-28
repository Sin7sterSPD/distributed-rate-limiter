package backend

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"math/rand"
	"strconv"
	"time"

	distributedratelimiter "github.com/Sin7sterSPD/distributed-rate-limiter"
	"github.com/Sin7sterSPD/distributed-rate-limiter/internal/circuitbreaker"
	goredis "github.com/redis/go-redis/v9"
)

//go:embed lua/token_bucket.lua
var tokenBucketLua string

//go:embed lua/sliding_window.lua
var slidingWindowLua string

type RedisConfig struct {
	Algorithm int
	Limit     int
	Window    time.Duration
	Burst     int
	Addr      string
	Password  string
	DB        int
	Timeout   time.Duration
	KeyPrefix string
}

type RedisBackend struct {
	client  goredis.UniversalClient
	script  *goredis.Script
	cfg     RedisConfig
	breaker *circuitbreaker.Breaker
}

func NewRedisBackend(cfg RedisConfig) (*RedisBackend, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  cfg.Timeout,
		ReadTimeout:  cfg.Timeout,
		WriteTimeout: cfg.Timeout,
	})

	luaSrc := tokenBucketLua
	if cfg.Algorithm == 1 {
		luaSrc = slidingWindowLua
	}
	script := goredis.NewScript(luaSrc)

	rb := &RedisBackend{
		client:  client,
		script:  script,
		cfg:     cfg,
		breaker: circuitbreaker.New(circuitbreaker.Config{MaxFailures: 5, Timeout: 10 * time.Second}),
	}

	pingCtx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {

		slog.Warn("ratelimiter redis ping failed at startup", "error", err)
	} else {
		_ = script.Load(pingCtx, client).Err()
	}

	return rb, nil
}

func (rb *RedisBackend) GetAndUpdate(ctx context.Context, key string, cost int) (*BackendResult, error) {

	if err := rb.breaker.Allow(); err != nil {
		return nil, fmt.Errorf("%w: circuit breaker open", distributedratelimiter.ErrBackendUnavailable)
	}

	callCtx, cancel := context.WithTimeout(ctx, rb.cfg.Timeout)
	defer cancel()

	fullKey := fmt.Sprintf("%s:%s", rb.cfg.KeyPrefix, key)
	nowMs := time.Now().UnixMilli()
	ttlSecs := int64(rb.cfg.Window.Seconds()) * 2

	var args []interface{}

	if rb.cfg.Algorithm == 0 {
		capacity := float64(rb.cfg.Burst)
		refillRate := float64(rb.cfg.Limit) / rb.cfg.Window.Seconds()
		args = []interface{}{
			strconv.Itoa(rb.cfg.Burst),      // ARGV[1] capacity
			fmt.Sprintf("%.6f", refillRate), // ARGV[2] refill rate
			strconv.FormatInt(nowMs, 10),    // ARGV[3] now ms
			strconv.Itoa(cost),              // ARGV[4] cost
			strconv.FormatInt(ttlSecs, 10),  // ARGV[5] TTL
		}
		_ = capacity // used above
	} else { // Sliding Window
		windowMs := rb.cfg.Window.Milliseconds()
		nonce := fmt.Sprintf("%d-%d", nowMs, rand.Int63()) // unique member for ZADD
		args = []interface{}{
			strconv.FormatInt(windowMs, 10), // ARGV[1] window ms
			strconv.FormatInt(nowMs, 10),    // ARGV[2] now ms
			strconv.Itoa(rb.cfg.Limit),      // ARGV[3] limit
			nonce,                           // ARGV[4] member
			strconv.FormatInt(ttlSecs, 10),  // ARGV[5] TTL
		}
	}

	res, err := rb.script.Run(callCtx, rb.client, []string{fullKey}, args...).Int64Slice()
	if err != nil {
		rb.breaker.RecordFailure()
		slog.Warn("ratelimiter redis script run failed", "key", key, "error", err)
		return nil, fmt.Errorf("%w: %v", distributedratelimiter.ErrBackendUnavailable, err)
	}

	rb.breaker.RecordSuccess()

	allowed := res[0] == 1
	remaining := int(res[1])
	retryAfterSecs := time.Duration(res[2]) * time.Second

	return &BackendResult{
		Allowed:    allowed,
		Remaining:  remaining,
		RetryAfter: retryAfterSecs,
	}, nil
}

func (rb *RedisBackend) Close() error {
	return rb.client.Close()
}
