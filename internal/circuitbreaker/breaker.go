package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

type Config struct {
	MaxFailures int

	Timeout time.Duration
}

type Breaker struct {
	mu          sync.Mutex
	cfg         Config
	state       State
	failures    int
	lastFailure time.Time
}

func New(cfg Config) *Breaker {
	if cfg.MaxFailures == 0 {
		cfg.MaxFailures = 5
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &Breaker{cfg: cfg, state: StateClosed}
}

// State returns the current breaker state (closed, open, or half-open).
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// Allow returns nil if the request should proceed, ErrCircuitOpen if blocked.
func (b *Breaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateOpen:
		if time.Since(b.lastFailure) >= b.cfg.Timeout {
			b.state = StateHalfOpen
			return nil // allow the probe request
		}
		return ErrCircuitOpen

	case StateHalfOpen:
		// Only one probe passes; subsequent calls are blocked until result known.
		return ErrCircuitOpen

	default: // StateClosed
		return nil
	}
}

func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.state = StateClosed
}

func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	b.lastFailure = time.Now()
	if b.failures >= b.cfg.MaxFailures {
		b.state = StateOpen
	}
}
