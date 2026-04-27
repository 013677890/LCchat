package grpcx

import (
	"context"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// LoggingConfig 日志拦截器配置。
type LoggingConfig struct {
	// SlowThreshold 慢请求阈值，超过此值的请求日志级别从 Info 升为 Warn。
	// 零值表示禁用慢请求检测（所有正常请求都记 Info）。
	SlowThreshold time.Duration
	// IgnoreMethods 不记录日志的方法全路径列表（如健康检查）。
	// 示例：[]string{"/grpc.health.v1.Health/Check"}
	IgnoreMethods []string
}

// DefaultLoggingConfig 返回默认日志配置：500ms 慢请求阈值，忽略健康检查方法。
func DefaultLoggingConfig() LoggingConfig {
	return LoggingConfig{
		SlowThreshold: 500 * time.Millisecond,
		IgnoreMethods: []string{"/grpc.health.v1.Health/Check"},
	}
}

// LoggingUnaryInterceptor 记录每次 Unary 请求的 method、耗时、状态码。
// 错误请求按业务码区分：业务错误记 Info，服务端错误记 Error 并附带堆栈。
// 正常请求根据 SlowThreshold 决定 Info 或 Warn。
func LoggingUnaryInterceptor(cfgs ...LoggingConfig) grpc.UnaryServerInterceptor {
	cfg := DefaultLoggingConfig()
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}

	ignoreSet := make(map[string]struct{}, len(cfg.IgnoreMethods))
	for _, m := range cfg.IgnoreMethods {
		ignoreSet[m] = struct{}{}
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		if _, skip := ignoreSet[info.FullMethod]; skip {
			return handler(ctx, req)
		}

		start := time.Now()
		resp, err = handler(ctx, req)
		cost := time.Since(start)

		fields := []zap.Field{
			logger.String("method", info.FullMethod),
			logger.Duration("cost", cost),
			logger.String("grpc_code", status.Code(err).String()),
		}
		if err != nil {
			appErr := apperr.WithStack(err)
			code := apperr.Code(appErr)
			fields = append(fields,
				logger.Int("business_code", code),
				logger.ErrorField("error", appErr),
			)
			if code >= 30000 {
				fields = append(fields,
					logger.String("top_frame", apperr.TopFrame(appErr)),
					logger.StackFrames("stack", apperr.Frames(appErr)),
				)
				logger.Error(ctx, "gRPC 请求完成", fields...)
				apperr.MarkLogged(appErr)
			} else {
				logger.Info(ctx, "gRPC 请求完成", fields...)
			}
			return resp, appErr
		}

		if cfg.SlowThreshold > 0 && cost >= cfg.SlowThreshold {
			logger.Warn(ctx, "gRPC 慢请求", fields...)
		} else {
			logger.Info(ctx, "gRPC 请求完成", fields...)
		}
		return resp, nil
	}
}
