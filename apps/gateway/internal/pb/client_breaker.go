package pb

import (
	"context"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/sony/gobreaker"
)

// CreateCircuitBreaker 创建 gateway 下游调用使用的熔断器。
// 这里保留 gateway 自己的保护策略，但把具体参数统一收口到 pb 包内部，
// 避免 provider 层同时承担“命名依赖”和“维护熔断细节”两层职责。
func CreateCircuitBreaker(name string) *gobreaker.CircuitBreaker {
	return gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        name,
		MaxRequests: 3,
		Interval:    15 * time.Second,
		Timeout:     45 * time.Second,
		// 仅把基础设施类故障计入失败率；业务错误、客户端取消不应触发熔断（否则会误熔断健康下游）。
		IsSuccessful: grpcx.BreakerIsSuccessful,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// 至少累计 5 次请求后再按失败率判断，避免低流量时过早熔断。
			if counts.Requests < 5 {
				return false
			}
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return failureRatio >= 0.5
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			logger.Info(context.Background(), "熔断器状态变更",
				logger.String("name", name),
				logger.String("from", from.String()),
				logger.String("to", to.String()),
			)
		},
	})
}
