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

// ClientLoggingConfig 定义 gRPC 出站日志策略。
type ClientLoggingConfig struct {
	// SlowThreshold 定义慢请求阈值；零值表示不做慢请求升级。
	SlowThreshold time.Duration
}

// DefaultClientLoggingConfig 返回统一的出站日志默认值。
func DefaultClientLoggingConfig() ClientLoggingConfig {
	return ClientLoggingConfig{SlowThreshold: time.Second}
}

// LoggingUnaryClientInterceptor 为每次出站 RPC 输出一条聚合日志。
//
// 规则：
//   - 业务错误按 Info 记录；
//   - 服务端错误按 Error 记录；
//   - 成功但超时阈值的请求按 Warn 记录。
func LoggingUnaryClientInterceptor(cfgs ...ClientLoggingConfig) grpc.UnaryClientInterceptor {
	cfg := DefaultClientLoggingConfig()
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}

	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		cost := time.Since(start)

		target := ""
		if cc != nil {
			target = cc.Target()
		}

		fields := []zap.Field{
			logger.String("method", method),
			logger.String("target", target),
			logger.Duration("cost", cost),
			logger.String("grpc_code", status.Code(err).String()),
		}

		if err != nil {
			appErr := apperr.FromStatus(err)
			fields = append(fields,
				logger.Int("business_code", apperr.Code(appErr)),
				logger.ErrorField("error", appErr),
			)
			if apperr.Code(appErr) >= 30000 {
				logger.Error(ctx, "gRPC 出站请求完成", fields...)
			} else {
				logger.Info(ctx, "gRPC 出站请求完成", fields...)
			}
			return err
		}

		if cfg.SlowThreshold > 0 && cost >= cfg.SlowThreshold {
			logger.Warn(ctx, "gRPC 出站慢请求", fields...)
		} else {
			logger.Info(ctx, "gRPC 出站请求完成", fields...)
		}
		return nil
	}
}
