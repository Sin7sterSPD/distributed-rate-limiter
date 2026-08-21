package distributedratelimiter

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Sin7sterSPD/distributed-rate-limiter/backend"
	"github.com/Sin7sterSPD/distributed-rate-limiter/internal/blockedcache"
	"github.com/Sin7sterSPD/distributed-rate-limiter/internal/jitter"
	"github.com/Sin7sterSPD/distributed-rate-limiter/metrics"
)

type limiterImpl struct {
	cfg          Config
	primary      backend.Backend // Redis or Memory (as configured)
	fallback     backend.Backend // Memory backend (used on Redis failure)
	metrics      *metrics.Metrics
	algoName     string
	backendName  string
	blockedCache *blockedcache.BlockedCache
}

func New(cfg Config) (Limiter, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	m := metrics.NewMetrics(cfg.MetricsNamespace)

	algoName := "token_bucket"
	if cfg.Algorithm == SlidingWindow {
		algoName = "sliding_window"
	}

	memCfg := backend.MemoryConfig{
		Algorithm:     int(cfg.Algorithm),
		Limit:         cfg.Limit,
		Window:        cfg.Window,
		Burst:         cfg.Burst,
		MaxKeys:       cfg.MaxMemoryKeys,
		SweepInterval: cfg.MemorySweepInterval,
	}
	memBackend := backend.NewMemoryBackend(memCfg)

	if cfg.Backend == "memory" {
		return &limiterImpl{
			cfg:          cfg,
			primary:      memBackend,
			metrics:      m,
			algoName:     algoName,
			backendName:  "memory",
			blockedCache: blockedcache.New(cfg.MaxMemoryKeys),
		}, nil
	}

	// Redis backend
	redisCfg := backend.RedisConfig{
		Algorithm: int(cfg.Algorithm),
		Limit:     cfg.Limit,
		Window:    cfg.Window,
		Burst:     cfg.Burst,
		Addr:      cfg.RedisAddr,
		Password:  cfg.RedisPassword,
		DB:        cfg.RedisDB,
		Timeout:   cfg.RedisTimeout,
		KeyPrefix: cfg.KeyPrefix,
		Metrics:   m,
	}
	redisBackend, err := backend.NewRedisBackend(redisCfg)
	if err != nil {

		slog.Warn("ratelimiter redis backend init failed", "error", err, "fallback", cfg.Fallback)
	}

	return &limiterImpl{
		cfg:          cfg,
		primary:      redisBackend,
		fallback:     memBackend,
		metrics:      m,
		algoName:     algoName,
		backendName:  "redis",
		blockedCache: blockedcache.New(cfg.MaxMemoryKeys),
	}, nil
}

func (l *limiterImpl) Allow(ctx context.Context, key string) (*Result, error) {
	return l.AllowN(ctx, key, 1)
}

func (l *limiterImpl) AllowN(ctx context.Context, key string, n int) (*Result, error) {
	start := time.Now()

	// Short-circuit: if this key was recently rejected, don't hit the backend
	// again. Only rejections are cached, so this can only over-block briefly.
	if l.blockedCache != nil && l.blockedCache.Get(key) {
		l.metrics.BlockedCacheHits.Inc()
		return &Result{
			Allowed:    false,
			Remaining:  0,
			RetryAfter: jitter.Jitter(time.Second),
			Limit:      l.cfg.Limit,
			Window:     l.cfg.Window,
			Algorithm:  l.cfg.Algorithm,
		}, nil
	}

	br, backendUsed, err := l.evaluate(ctx, key, n)
	if err != nil {
		return nil, err
	}

	retryAfter := br.RetryAfter
	if !br.Allowed {
		if l.blockedCache != nil {
			ttl := retryAfter
			if ttl <= 0 || ttl > time.Second {
				ttl = time.Second
			}
			l.blockedCache.Set(key, ttl)
		}
		// Jitter only the externally-visible value; internal state stays exact.
		retryAfter = jitter.Jitter(retryAfter)
	}

	latency := time.Since(start)
	l.metrics.ObserveAllow(l.algoName, backendUsed, br.Allowed, br.Remaining, latency)

	return &Result{
		Allowed:    br.Allowed,
		Remaining:  br.Remaining,
		RetryAfter: retryAfter,
		Limit:      l.cfg.Limit,
		Window:     l.cfg.Window,
		Algorithm:  l.cfg.Algorithm,
	}, nil
}
func (l *limiterImpl) evaluate(ctx context.Context, key string, cost int) (*backend.BackendResult, string, error) {
	if l.primary == nil {
		return l.handleFallback(ctx, key, cost, errors.New("primary backend not initialized"))
	}

	br, err := l.primary.GetAndUpdate(ctx, key, cost)
	if err == nil {
		return br, l.backendName, nil
	}

	return l.handleFallback(ctx, key, cost, err)
}

func (l *limiterImpl) handleFallback(ctx context.Context, key string, cost int, originalErr error) (*backend.BackendResult, string, error) {
	slog.Warn("ratelimiter primary backend failed, applying fallback",
		"fallback_mode", l.cfg.Fallback,
		"error", originalErr)

	switch l.cfg.Fallback {
	case FallbackAllow:
		l.metrics.FallbackTotal.WithLabelValues("allow").Inc()
		return &backend.BackendResult{Allowed: true, Remaining: l.cfg.Limit}, "fallback_allow", nil

	case FallbackDeny:
		l.metrics.FallbackTotal.WithLabelValues("deny").Inc()
		return &backend.BackendResult{
			Allowed:    false,
			RetryAfter: time.Second,
		}, "fallback_deny", nil

	default: // FallbackMemory
		l.metrics.FallbackTotal.WithLabelValues("memory").Inc()
		if l.fallback != nil {
			br, err := l.fallback.GetAndUpdate(ctx, key, cost)
			if err != nil {
				return nil, "", err
			}
			return br, "fallback_memory", nil
		}
		return nil, "", originalErr
	}
}

func (l *limiterImpl) Close() error {
	var errs []error
	if l.primary != nil {
		if err := l.primary.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if l.fallback != nil && l.fallback != l.primary {
		if err := l.fallback.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
