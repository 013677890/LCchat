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
