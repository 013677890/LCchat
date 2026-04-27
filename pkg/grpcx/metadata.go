package grpcx

import (
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// MetadataUnaryInterceptor 将 gRPC incoming metadata 注入到 context 中，
// 使下游业务代码可通过 ctxmeta 包统一读取 trace_id / user_uuid / device_id / client_ip。
func MetadataUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if traceID := firstValue(md.Get(ctxmeta.MetadataTraceID)); traceID != "" {
				ctx = ctxmeta.WithTraceID(ctx, traceID)
			}
			if userUUID := firstValue(md.Get(ctxmeta.MetadataUserUUID)); userUUID != "" {
				ctx = ctxmeta.WithUserUUID(ctx, userUUID)
			}
			if deviceID := firstValue(md.Get(ctxmeta.MetadataDeviceID)); deviceID != "" {
				ctx = ctxmeta.WithDeviceID(ctx, deviceID)
			}
			clientIP := firstValue(md.Get(ctxmeta.MetadataXRealIP))
			if clientIP == "" {
				clientIP = firstValue(md.Get(ctxmeta.MetadataXForwardedFor))
			}
			if clientIP == "" {
				clientIP = firstValue(md.Get(ctxmeta.MetadataClientIP))
			}
			if clientIP != "" {
				ctx = ctxmeta.WithClientIP(ctx, clientIP)
			}
		}
		return handler(ctx, req)
	}
}

func firstValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
