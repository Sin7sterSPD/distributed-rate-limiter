package backend

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/Sin7sterSPD/distributed-rate-limiter/internal/circuitbreaker"
	"github.com/Sin7sterSPD/distributed-rate-limiter/metrics"
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

	// MaxFailures is the number of consecutive failures before the circuit
	// breaker opens. Default: 5.
	MaxFailures int
	// BreakerTimeout is how long the breaker stays open before probing.
	// Default: 10s.
	BreakerTimeout time.Duration
}

type RedisBackend struct {
	client  goredis.UniversalClient
	script  *goredis.Script
	cfg     RedisConfig
	breaker *circuitbreaker.Breaker
	metrics *metrics.Metrics
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

	breakerCfg := circuitbreaker.Config{
		MaxFailures: cfg.MaxFailures,
		Timeout:     cfg.BreakerTimeout,
	}

	rb := &RedisBackend{
		client:  client,
		script:  script,
		cfg:     cfg,
		breaker: circuitbreaker.New(breakerCfg),
		metrics: metrics.NewMetrics("app"),
	}
	rb.observeBreaker()

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
		return nil, fmt.Errorf("%w: circuit breaker open", ErrBackendUnavailable)
	}

	callCtx, cancel := context.WithTimeout(ctx, rb.cfg.Timeout)
	defer cancel()

	fullKey := fmt.Sprintf("%s:%s", rb.cfg.KeyPrefix, key)
	nowMs := time.Now().UnixMilli()
	ttlSecs := int64(rb.cfg.Window.Seconds()) * 2

	var args []interface{}

	if rb.cfg.Algorithm == 0 {
		refillRate := float64(rb.cfg.Limit) / rb.cfg.Window.Seconds()
		args = []interface{}{
			strconv.Itoa(rb.cfg.Burst),      // ARGV[1] capacity
			fmt.Sprintf("%.6f", refillRate), // ARGV[2] refill rate
			strconv.FormatInt(nowMs, 10),    // ARGV[3] now ms
			strconv.Itoa(cost),              // ARGV[4] cost
			strconv.FormatInt(ttlSecs, 10),  // ARGV[5] TTL
		}
	} else { // Sliding Window Counter
		windowMs := rb.cfg.Window.Milliseconds()
		args = []interface{}{
			strconv.FormatInt(windowMs, 10), // ARGV[1] window ms
			strconv.FormatInt(nowMs, 10),    // ARGV[2] now ms
			strconv.Itoa(rb.cfg.Limit),      // ARGV[3] limit
			strconv.Itoa(cost),              // ARGV[4] cost
			strconv.FormatInt(ttlSecs, 10),  // ARGV[5] TTL
		}
	}

	res, err := rb.script.Run(callCtx, rb.client, []string{fullKey}, args...).Int64Slice()
	if err != nil {
		rb.breaker.RecordFailure()
		rb.observeBreaker()
		rb.metrics.RedisErrors.WithLabelValues("script_run").Inc()
		slog.Warn("ratelimiter redis script run failed", "key", key, "error", err)
		return nil, fmt.Errorf("%w: %v", ErrBackendUnavailable, err)
	}

	rb.breaker.RecordSuccess()
	rb.observeBreaker()

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

// observeBreaker exports the current breaker state as a gauge value
// (0=closed, 1=half-open, 2=open).
func (rb *RedisBackend) observeBreaker() {
	var v float64
	switch rb.breaker.State() {
	case circuitbreaker.StateHalfOpen:
		v = 1
	case circuitbreaker.StateOpen:
		v = 2
	default:
		v = 0
	}
	rb.metrics.BreakerState.WithLabelValues("redis").Set(v)
}
