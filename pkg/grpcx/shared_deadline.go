package grpcx

import (
	"context"
	"time"
)

// withEarlierDeadline 是客户端和服务端共同遵守的 deadline 规则：使用配置预算
// 收紧 parent，但绝不放大已有的更短 deadline。调用方应先过滤 timeout <= 0；
// 返回值中的 duration 是创建 context 时实际可用的近似预算，主要用于超时日志。
func withEarlierDeadline(
	parent context.Context,
	timeout time.Duration,
) (context.Context, context.CancelFunc, time.Duration) {
	if deadline, ok := parent.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < 0 {
			remaining = 0
		}
		if remaining <= timeout {
			// 直接继承父 deadline，避免为同一个更早截止时间再创建一套 timer。
			ctx, cancel := context.WithCancel(parent)
			return ctx, cancel, remaining
		}
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	return ctx, cancel, timeout
}
