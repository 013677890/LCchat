// Package cachex contains small cache policies shared by repository packages.
package cachex

import (
	"math/rand"
	"strings"
	"time"
)

// JitterTTL adds a uniformly distributed offset of up to ten percent in either
// direction, reducing synchronized expiration of keys written together.
func JitterTTL(base time.Duration) time.Duration {
	jitterRange := float64(base) * 0.1
	jitter := time.Duration(rand.Float64()*jitterRange*2 - jitterRange)
	return base + jitter
}

// Chance reports whether a random sample falls within probability.
func Chance(probability float64) bool {
	return rand.Float64() < probability
}

// IsRedisWrongType identifies Redis key-type mismatch errors without coupling
// callers to a concrete Redis error type.
func IsRedisWrongType(err error) bool {
	return err != nil && strings.Contains(err.Error(), "WRONGTYPE")
}
