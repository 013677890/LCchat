package main

import "testing"

// TestParseAuthPoolWorkers 验证 auth 两个旁路 Pool 的默认值、显式值与严格失败规则。
func TestParseAuthPoolWorkers(t *testing.T) {
	for _, envName := range []string{
		"KAFKA_AUTH_PROFILE_DISPLAY_CHANGED_CONSUMER_CONCURRENCY",
		"KAFKA_AUTH_REDIS_RETRY_CONSUMER_CONCURRENCY",
	} {
		t.Run(envName+"_default", func(t *testing.T) {
			t.Setenv(envName, "")
			workers, err := parseAuthPoolWorkers(envName)
			if err != nil || workers != 3 {
				t.Fatalf("默认 workers 不匹配: workers=%d err=%v", workers, err)
			}
		})
		t.Run(envName+"_invalid", func(t *testing.T) {
			t.Setenv(envName, "65")
			if _, err := parseAuthPoolWorkers(envName); err == nil {
				t.Fatal("非法 workers 未阻止启动")
			}
		})
	}
}
