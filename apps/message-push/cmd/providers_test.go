package main

import "testing"

// TestProvideMessagePushMaxFanoutConcurrency 验证默认值、显式配置与严格失败规则。
// 显式写入非法值时必须返回错误，不能静默回退后让服务继续启动。
func TestProvideMessagePushMaxFanoutConcurrency(t *testing.T) {
	testCases := []struct {
		name      string
		value     string
		want      messagePushMaxFanoutConcurrency
		wantError bool
	}{
		{name: "default", value: "", want: 32},
		{name: "configured", value: "8", want: 8},
		{name: "not_integer", value: "invalid", wantError: true},
		{name: "zero", value: "0", wantError: true},
		{name: "negative", value: "-1", wantError: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("MESSAGE_PUSH_MAX_FANOUT_CONCURRENCY", testCase.value)
			got, err := provideMessagePushMaxFanoutConcurrency()
			if testCase.wantError {
				if err == nil {
					t.Fatalf("非法并发配置 %q 未返回错误", testCase.value)
				}
				if got != 0 {
					t.Fatalf("非法并发配置不应返回可用值: got=%d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("合法并发配置返回错误: %v", err)
			}
			if got != testCase.want {
				t.Fatalf("并发配置不匹配: got=%d want=%d", got, testCase.want)
			}
		})
	}
}

// TestParseMessagePushPoolWorkers 验证两个 topic 共用严格 workers 契约但读取独立环境变量。
func TestParseMessagePushPoolWorkers(t *testing.T) {
	for _, envName := range []string{
		"KAFKA_MSG_PUSH_CONSUMER_CONCURRENCY",
		"KAFKA_REALTIME_PUSH_CONSUMER_CONCURRENCY",
	} {
		t.Run(envName+"_default", func(t *testing.T) {
			t.Setenv(envName, "")
			workers, err := parseMessagePushPoolWorkers(envName)
			if err != nil || workers != 3 {
				t.Fatalf("默认 workers 不匹配: workers=%d err=%v", workers, err)
			}
		})
		t.Run(envName+"_configured", func(t *testing.T) {
			t.Setenv(envName, "7")
			workers, err := parseMessagePushPoolWorkers(envName)
			if err != nil || workers != 7 {
				t.Fatalf("显式 workers 不匹配: workers=%d err=%v", workers, err)
			}
		})
		t.Run(envName+"_invalid", func(t *testing.T) {
			t.Setenv(envName, "0")
			if _, err := parseMessagePushPoolWorkers(envName); err == nil {
				t.Fatal("非法 workers 未阻止启动")
			}
		})
	}
}
