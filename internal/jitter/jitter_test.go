package jitter

import (
	"testing"
	"time"
)

func TestNonPositiveDurationsPassThrough(t *testing.T) {
	if got := Jitter(0); got != 0 {
		t.Errorf("Jitter(0) = %v, want 0", got)
	}
	if got := Jitter(-time.Second); got != -time.Second {
		t.Errorf("Jitter(-1s) = %v, want -1s", got)
	}
}

func TestJitterStaysWithinTenPercent(t *testing.T) {
	d := 10 * time.Second
	bound := time.Second // 10% of d

	for i := 0; i < 500; i++ {
		got := Jitter(d)
		delta := got - d
		if delta < -bound || delta > bound {
			t.Fatalf("iteration %d: %v outside ±10%% of %v", i, got, d)
		}
	}
}

func TestJitterProducesVariance(t *testing.T) {
	d := time.Second
	first := Jitter(d)

	same := true
	for i := 0; i < 100; i++ {
		if Jitter(d) != first {
			same = false
			break
		}
	}
	if same {
		t.Fatal("jitter produced identical values 100 times; not random")
	}
}

func TestJitterSmallDuration(t *testing.T) {
	for i := 0; i < 100; i++ {
		got := Jitter(time.Millisecond)
		if got < 900*time.Microsecond || got > 1100*time.Microsecond {
			t.Fatalf("%v outside ±10%% of 1ms", got)
		}
	}
}
