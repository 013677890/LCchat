package middleware

import (
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// GRPCMetadataInterceptor 将上下文信息注入 gRPC metadata（用于透传 trace/user/device/ip）
func GRPCMetadataInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		} else {
			md = md.Copy()
		}

		if traceID := ctxmeta.TraceID(ctx); traceID != "" {
			md.Set(ctxmeta.MetadataTraceID, traceID)
		}
		if userUUID := ctxmeta.UserUUID(ctx); userUUID != "" {
			md.Set(ctxmeta.MetadataUserUUID, userUUID)
		}
		if deviceID := ctxmeta.DeviceID(ctx); deviceID != "" {
			md.Set(ctxmeta.MetadataDeviceID, deviceID)
		}
		if clientIP := ctxmeta.ClientIP(ctx); clientIP != "" {
			md.Set(ctxmeta.MetadataXRealIP, clientIP)
			md.Set(ctxmeta.MetadataClientIP, clientIP)
		}

		ctx = metadata.NewOutgoingContext(ctx, md)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
