package main

import (
	"testing"
	"time"
)

func TestProvideRelationRedisConfigUsesShortRetryBudget(t *testing.T) {
	cfg := provideRelationRedisConfig()

	if cfg.MaxRetries != 2 {
		t.Fatalf("MaxRetries = %d, want 2", cfg.MaxRetries)
	}
	if cfg.ReadTimeout != 50*time.Millisecond || cfg.WriteTimeout != 50*time.Millisecond {
		t.Fatalf("Redis timeouts = (%s, %s), want both 50ms", cfg.ReadTimeout, cfg.WriteTimeout)
	}
	if cfg.MinRetryBackoff != 10*time.Millisecond || cfg.MaxRetryBackoff != 50*time.Millisecond {
		t.Fatalf("Redis retry backoff = (%s, %s), want (10ms, 50ms)", cfg.MinRetryBackoff, cfg.MaxRetryBackoff)
	}
}
