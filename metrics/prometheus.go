package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	RequestsTotal  *prometheus.CounterVec
	Remaining      *prometheus.GaugeVec
	LatencySeconds *prometheus.HistogramVec
	RedisErrors    *prometheus.CounterVec
	FallbackTotal  *prometheus.CounterVec
}

func NewMetrics(namespace string) *Metrics {
	return &Metrics{
		RequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "ratelimiter_requests_total",
				Help:      "Total number of rate limiter evaluations.",
			},
			[]string{"algorithm", "backend", "result"},
		),

		Remaining: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "ratelimiter_remaining",
				Help:      "Remaining request quota in the current window (sampled).",
			},
			[]string{"algorithm"},
		),

		LatencySeconds: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "ratelimiter_latency_seconds",
				Help:      "Time spent inside Allow() including backend call.",
				Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5},
			},
			[]string{"algorithm", "backend"},
		),

		RedisErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "ratelimiter_redis_errors_total",
				Help:      "Total Redis operation errors.",
			},
			[]string{"operation"},
		),

		FallbackTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "ratelimiter_fallback_total",
				Help:      "Number of times fallback mode was activated.",
			},
			[]string{"mode"},
		),
	}
}

func (m *Metrics) ObserveAllow(algo, backendName string, allowed bool, remaining int, latency time.Duration) {
	result := "allowed"
	if !allowed {
		result = "rejected"
	}
	m.RequestsTotal.WithLabelValues(algo, backendName, result).Inc()
	m.Remaining.WithLabelValues(algo).Set(float64(remaining))
	m.LatencySeconds.WithLabelValues(algo, backendName).Observe(latency.Seconds())
}
