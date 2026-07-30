package cachex

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestJitterTTLStaysWithinTenPercent(t *testing.T) {
	const base = 10 * time.Minute
	for range 100 {
		actual := JitterTTL(base)
		assert.GreaterOrEqual(t, actual, 9*time.Minute)
		assert.LessOrEqual(t, actual, 11*time.Minute)
	}
	assert.Equal(t, time.Duration(0), JitterTTL(0))
}

func TestChanceHonorsProbabilityBounds(t *testing.T) {
	for range 100 {
		assert.False(t, Chance(0))
		assert.True(t, Chance(1))
	}
}

func TestIsRedisWrongType(t *testing.T) {
	wrongType := errors.New("WRONGTYPE Operation against a key holding the wrong kind of value")

	assert.False(t, IsRedisWrongType(nil))
	assert.True(t, IsRedisWrongType(wrongType))
	assert.True(t, IsRedisWrongType(fmt.Errorf("read cache: %w", wrongType)))
	assert.False(t, IsRedisWrongType(errors.New("wrongtype")))
}
