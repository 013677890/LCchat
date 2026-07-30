package grpcx

import (
	"context"
	"time"

	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TimeoutConfig 定义 gRPC 服务端兜底 deadline。
// 默认建议按服务类型配置在 1500-2500ms，避免突破 gateway 侧 1-3s 的入口预算。
type TimeoutConfig struct {
	// DefaultTimeout 作为服务端兜底预算；<=0 表示不启用 timeout interceptor。
	DefaultTimeout time.Duration
	// MethodTimeouts 可按 full method 覆盖预算，例如 /user.AuthService/Login。
	MethodTimeouts map[string]time.Duration
}

// TimeoutUnaryInterceptor 为 gRPC server 补齐请求级 deadline。
// 规则与 gateway timeout 一致：实际生效 deadline = min(父 deadline, 服务端配置 timeout)。
func TimeoutUnaryInterceptor(cfg TimeoutConfig) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		timeout := resolveGRPCTimeout(info.FullMethod, cfg)
		if timeout <= 0 {
			return handler(ctx, req)
		}

		ctx, cancel, effectiveTimeout := newGRPCTimeoutContext(ctx, timeout)
		defer cancel()

		resp, err := handler(ctx, req)
		if err != nil {
			if !isDeadlineExceeded(err) {
				return resp, err
			}
			logger.Warn(ctx, "gRPC 服务端请求超时",
				logger.String("method", info.FullMethod),
				logger.Duration("timeout", effectiveTimeout),
			)
			return resp, apperr.New(consts.CodeTimeoutError)
		}

		if ctx.Err() != context.DeadlineExceeded {
			return resp, nil
		}

		logger.Warn(ctx, "gRPC 服务端请求超时",
			logger.String("method", info.FullMethod),
			logger.Duration("timeout", effectiveTimeout),
		)
		return resp, apperr.New(consts.CodeTimeoutError)
	}
}

func resolveGRPCTimeout(fullMethod string, cfg TimeoutConfig) time.Duration {
	if cfg.MethodTimeouts != nil {
		if timeout, ok := cfg.MethodTimeouts[fullMethod]; ok {
			return timeout
		}
	}
	return cfg.DefaultTimeout
}

func newGRPCTimeoutContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc, time.Duration) {
	if deadline, ok := parent.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < 0 {
			remaining = 0
		}
		if remaining <= timeout {
			ctx, cancel := context.WithCancel(parent)
			return ctx, cancel, remaining
		}
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	return ctx, cancel, timeout
}

func isDeadlineExceeded(err error) bool {
	if err == nil {
		return false
	}
	if err == context.DeadlineExceeded {
		return true
	}
	return status.Code(err) == codes.DeadlineExceeded
}
