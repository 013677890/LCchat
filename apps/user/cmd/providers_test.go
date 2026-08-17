package main

import "testing"

// TestParseUserPoolWorkers 验证 user 三个旁路 Pool 共享严格 workers 契约且配置彼此独立。
func TestParseUserPoolWorkers(t *testing.T) {
	for _, envName := range []string{
		"KAFKA_USER_CREATED_CONSUMER_CONCURRENCY",
		"KAFKA_USER_ACCOUNT_DELETED_CONSUMER_CONCURRENCY",
		"KAFKA_USER_REDIS_RETRY_CONSUMER_CONCURRENCY",
	} {
		t.Run(envName+"_configured", func(t *testing.T) {
			t.Setenv(envName, "4")
			workers, err := parseUserPoolWorkers(envName)
			if err != nil || workers != 4 {
				t.Fatalf("显式 workers 不匹配: workers=%d err=%v", workers, err)
			}
		})
		t.Run(envName+"_invalid", func(t *testing.T) {
			t.Setenv(envName, "not-a-number")
			if _, err := parseUserPoolWorkers(envName); err == nil {
				t.Fatal("非法 workers 未阻止启动")
			}
		})
	}
}
