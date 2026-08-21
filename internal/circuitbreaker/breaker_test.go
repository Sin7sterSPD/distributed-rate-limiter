package circuitbreaker

import (
	"errors"
	"testing"
	"time"
)

func TestDefaultsApplied(t *testing.T) {
	b := New(Config{})
	if b.cfg.MaxFailures != 5 {
		t.Errorf("expected default MaxFailures 5, got %d", b.cfg.MaxFailures)
	}
	if b.cfg.Timeout != 10*time.Second {
		t.Errorf("expected default Timeout 10s, got %v", b.cfg.Timeout)
	}
}

func TestClosedAllowsRequests(t *testing.T) {
	b := New(Config{})
	if err := b.Allow(); err != nil {
		t.Fatalf("closed breaker should allow: %v", err)
	}
	if b.State() != StateClosed {
		t.Errorf("expected StateClosed, got %v", b.State())
	}
}

func TestOpensAfterMaxFailures(t *testing.T) {
	b := New(Config{MaxFailures: 3})

	for i := 0; i < 2; i++ {
		b.RecordFailure()
	}
	if b.State() != StateClosed {
		t.Fatal("breaker should still be closed below MaxFailures")
	}
	if err := b.Allow(); err != nil {
		t.Fatalf("should still allow below MaxFailures: %v", err)
	}

	b.RecordFailure()
	if b.State() != StateOpen {
		t.Fatal("breaker should be open at MaxFailures")
	}
	if !errors.Is(b.Allow(), ErrCircuitOpen) {
		t.Fatal("open breaker must return ErrCircuitOpen")
	}
}

func TestSuccessResetsFailureCount(t *testing.T) {
	b := New(Config{MaxFailures: 3})

	b.RecordFailure()
	b.RecordFailure()
	b.RecordSuccess() // resets count

	b.RecordFailure()
	b.RecordFailure()
	if b.State() != StateClosed {
		t.Error("failures after a success should not carry over")
	}
}

func TestHalfOpenAfterTimeout(t *testing.T) {
	b := New(Config{MaxFailures: 1, Timeout: 30 * time.Millisecond})

	b.RecordFailure()
	if b.State() != StateOpen {
		t.Fatal("expected open")
	}

	time.Sleep(50 * time.Millisecond)

	// First call transitions to half-open and lets the probe through.
	if err := b.Allow(); err != nil {
		t.Fatalf("probe should pass: %v", err)
	}
	if b.State() != StateHalfOpen {
		t.Fatalf("expected half-open, got %v", b.State())
	}

	// Subsequent calls are blocked while the probe is in flight.
	if !errors.Is(b.Allow(), ErrCircuitOpen) {
		t.Fatal("second call during half-open must be blocked")
	}
}

func TestProbeSuccessClosesBreaker(t *testing.T) {
	b := New(Config{MaxFailures: 1, Timeout: 30 * time.Millisecond})

	b.RecordFailure()
	time.Sleep(50 * time.Millisecond)

	if err := b.Allow(); err != nil { // probe
		t.Fatalf("probe should pass: %v", err)
	}
	b.RecordSuccess()

	if b.State() != StateClosed {
		t.Fatalf("successful probe should close breaker, got %v", b.State())
	}
	if err := b.Allow(); err != nil {
		t.Fatalf("closed breaker should allow: %v", err)
	}
}

func TestProbeFailureReopensBreaker(t *testing.T) {
	b := New(Config{MaxFailures: 1, Timeout: 30 * time.Millisecond})

	b.RecordFailure()
	time.Sleep(50 * time.Millisecond)

	if err := b.Allow(); err != nil { // probe
		t.Fatalf("probe should pass: %v", err)
	}
	b.RecordFailure()

	if b.State() != StateOpen {
		t.Fatalf("failed probe should reopen breaker, got %v", b.State())
	}
	if !errors.Is(b.Allow(), ErrCircuitOpen) {
		t.Fatal("reopened breaker must block")
	}
}
