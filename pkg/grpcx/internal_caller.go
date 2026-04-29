package grpcx

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	// MetadataInternalCaller 是内部服务调用时必须携带的 metadata key。
	// value 为调用方服务名（如 "gateway"、"relation-service"）。
	MetadataInternalCaller = "x-internal-caller"
)

// InternalCallerInterceptor 返回一个 gRPC 一元拦截器，用于区分内部接口和对外接口的鉴权：
//
//   - 对 internalMethods 中的方法：要求 metadata 中携带 x-internal-caller 且值在白名单内；
//   - 对其他方法（对外接口）：如果出现 x-internal-caller header，直接拒绝，避免通过该 header 绕过业务鉴权。
//
// internalMethods 的 key 是 gRPC full method（如 "/auth.InternalAuthService/FindAccountByEmail"），
// value 是允许的 caller 名称列表（如 ["gateway", "relation-service"]）。
//
// 使用示例:
//
//	whitelist := map[string][]string{
//	    "/auth.InternalAuthService/FindAccountByEmail":     {"gateway", "relation-service"},
//	    "/auth.InternalAuthService/UpdateLoginDisplay":     {"user-service"},
//	    "/auth.InternalAuthService/BatchCheckAccountStatus": {"relation-service", "group-service"},
//	}
//	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
//	    grpcx.InternalCallerInterceptor(whitelist),
//	    // ... other interceptors
//	))
func InternalCallerInterceptor(internalMethods map[string][]string) grpc.UnaryServerInterceptor {
	// 预计算 lookup set 加速匹配
	type callerSet map[string]struct{}
	lookup := make(map[string]callerSet, len(internalMethods))
	for method, callers := range internalMethods {
		s := make(callerSet, len(callers))
		for _, c := range callers {
			s[c] = struct{}{}
		}
		lookup[method] = s
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		callerHeader := extractInternalCaller(ctx)
		allowedCallers, isInternal := lookup[info.FullMethod]

		if isInternal {
			// 内部方法：必须携带 x-internal-caller 且在白名单内
			if callerHeader == "" {
				return nil, status.Errorf(codes.PermissionDenied,
					"internal method %s requires %s metadata", info.FullMethod, MetadataInternalCaller)
			}
			if _, ok := allowedCallers[callerHeader]; !ok {
				return nil, status.Errorf(codes.PermissionDenied,
					"caller %q is not allowed to invoke %s", callerHeader, info.FullMethod)
			}
		} else {
			// 对外方法：拒绝携带 x-internal-caller 的请求
			if callerHeader != "" {
				return nil, status.Errorf(codes.PermissionDenied,
					"external method %s must not carry %s header", info.FullMethod, MetadataInternalCaller)
			}
		}

		return handler(ctx, req)
	}
}

// extractInternalCaller 从 incoming metadata 中提取 x-internal-caller 的值。
func extractInternalCaller(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(MetadataInternalCaller)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
