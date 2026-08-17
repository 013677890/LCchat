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

// TestParseRelationPoolWorkers 验证 relation 两个旁路 Pool 的默认值与严格失败规则。
func TestParseRelationPoolWorkers(t *testing.T) {
	for _, envName := range []string{
		"KAFKA_RELATION_ACCOUNT_DELETED_CONSUMER_CONCURRENCY",
		"KAFKA_RELATION_REDIS_RETRY_CONSUMER_CONCURRENCY",
	} {
		t.Run(envName+"_default", func(t *testing.T) {
			t.Setenv(envName, "")
			workers, err := parseRelationPoolWorkers(envName)
			if err != nil || workers != 3 {
				t.Fatalf("默认 workers 不匹配: workers=%d err=%v", workers, err)
			}
		})
		t.Run(envName+"_invalid", func(t *testing.T) {
			t.Setenv(envName, "0")
			if _, err := parseRelationPoolWorkers(envName); err == nil {
				t.Fatal("非法 workers 未阻止启动")
			}
		})
	}
}
