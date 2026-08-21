package jitter

import (
	"math/rand"
	"time"
)

// Jitter returns d perturbed by up to ±10% (multiplicative, uniformly
// distributed). Applied to externally-visible RetryAfter values so that many
// simultaneously-rejected clients don't all retry at the same instant
// (thundering-herd / synchronized retry storms).
func Jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	delta := float64(d) * 0.1
	perturbed := float64(d) + (rand.Float64()*2-1)*delta
	if perturbed < 0 {
		perturbed = 0
	}
	return time.Duration(perturbed)
}
