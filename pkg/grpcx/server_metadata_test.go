package grpcx

import (
	"context"
	"testing"

	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestMetadataUnaryInterceptorReadsSpanMetadata(t *testing.T) {
	md := metadata.New(map[string]string{
		ctxmeta.MetadataTraceID:      "trace-1",
		ctxmeta.MetadataSpanID:       "span-child",
		ctxmeta.MetadataParentSpanID: "span-parent",
		ctxmeta.MetadataUserUUID:     "user-1",
		ctxmeta.MetadataDeviceID:     "device-1",
		ctxmeta.MetadataClientIP:     "127.0.0.1",
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	interceptor := MetadataUnaryInterceptor()
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/test.Service/Call"}, func(ctx context.Context, req interface{}) (interface{}, error) {
		if got := ctxmeta.TraceID(ctx); got != "trace-1" {
			t.Fatalf("TraceID = %q", got)
		}
		if got := ctxmeta.SpanID(ctx); got != "span-child" {
			t.Fatalf("SpanID = %q", got)
		}
		if got := ctxmeta.ParentSpanID(ctx); got != "span-parent" {
			t.Fatalf("ParentSpanID = %q", got)
		}
		if got := ctxmeta.UserUUID(ctx); got != "user-1" {
			t.Fatalf("UserUUID = %q", got)
		}
		if got := ctxmeta.DeviceID(ctx); got != "device-1" {
			t.Fatalf("DeviceID = %q", got)
		}
		if got := ctxmeta.ClientIP(ctx); got != "127.0.0.1" {
			t.Fatalf("ClientIP = %q", got)
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("interceptor error: %v", err)
	}
}

func TestMetadataUnaryInterceptorDoesNotCreateSpanFromTraceOnly(t *testing.T) {
	md := metadata.New(map[string]string{
		ctxmeta.MetadataTraceID: "trace-1",
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	interceptor := MetadataUnaryInterceptor()
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/test.Service/Call"}, func(ctx context.Context, req interface{}) (interface{}, error) {
		if got := ctxmeta.TraceID(ctx); got != "trace-1" {
			t.Fatalf("TraceID = %q", got)
		}
		if got := ctxmeta.SpanID(ctx); got != "" {
			t.Fatalf("SpanID = %q, want empty", got)
		}
		if got := ctxmeta.ParentSpanID(ctx); got != "" {
			t.Fatalf("ParentSpanID = %q, want empty", got)
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("interceptor error: %v", err)
	}
}
