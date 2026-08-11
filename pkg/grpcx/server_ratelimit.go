package grpcx

import (
	"context"

	"github.com/013677890/LCchat-Backend/pkg/logger"

	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RateLimitConfig 令牌桶限流配置。
type RateLimitConfig struct {
	// RequestsPerSecond 每秒允许的请求数（令牌填充速率）。
	RequestsPerSecond float64
	// Burst 允许的突发请求数（令牌桶容量）。
	Burst int
}

// DefaultRateLimitConfig 返回默认限流配置。
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerSecond: 1500,
		Burst:             2500,
	}
}

// RateLimitUnaryInterceptor 创建令牌桶限流拦截器。
// 每次调用都会创建一个独立的限流器实例，不同服务可使用不同阈值。
func RateLimitUnaryInterceptor(cfgs ...RateLimitConfig) grpc.UnaryServerInterceptor {
	cfg := DefaultRateLimitConfig()
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}

	limiter := rate.NewLimiter(rate.Limit(cfg.RequestsPerSecond), cfg.Burst)

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		if !limiter.Allow() {
			logger.Warn(ctx, "请求被限流拦截",
				logger.String("method", info.FullMethod),
				logger.Float64("limit_rate", cfg.RequestsPerSecond),
				logger.Int("burst", cfg.Burst),
			)
			return nil, status.Error(codes.ResourceExhausted, "服务繁忙，请稍后重试")
		}
		return handler(ctx, req)
	}
}
