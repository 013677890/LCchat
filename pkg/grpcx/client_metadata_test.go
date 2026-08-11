package grpcx

import (
	"context"
	"testing"

	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestMetadataUnaryClientInterceptorCreatesChildSpan(t *testing.T) {
	parent := context.Background()
	parent = ctxmeta.WithTraceID(parent, "trace-1")
	parent = ctxmeta.WithSpanID(parent, "span-parent")
	parent = ctxmeta.WithUserUUID(parent, "user-1")
	parent = ctxmeta.WithDeviceID(parent, "device-1")
	parent = ctxmeta.WithClientIP(parent, "127.0.0.1")

	interceptor := MetadataUnaryClientInterceptor()
	err := interceptor(parent, "/test.Service/Call", nil, nil, nil, func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		childSpanID := ctxmeta.SpanID(ctx)
		if childSpanID == "" || childSpanID == "span-parent" {
			t.Fatalf("child span_id = %q", childSpanID)
		}
		if got := ctxmeta.ParentSpanID(ctx); got != "span-parent" {
			t.Fatalf("parent_span_id = %q", got)
		}

		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			t.Fatal("missing outgoing metadata")
		}
		assertMetadataValue(t, md, ctxmeta.MetadataTraceID, "trace-1")
		assertMetadataValue(t, md, ctxmeta.MetadataSpanID, childSpanID)
		assertMetadataValue(t, md, ctxmeta.MetadataParentSpanID, "span-parent")
		assertMetadataValue(t, md, ctxmeta.MetadataUserUUID, "user-1")
		assertMetadataValue(t, md, ctxmeta.MetadataDeviceID, "device-1")
		assertMetadataValue(t, md, ctxmeta.MetadataClientIP, "127.0.0.1")
		assertMetadataValue(t, md, ctxmeta.MetadataXRealIP, "127.0.0.1")
		return nil
	})
	if err != nil {
		t.Fatalf("interceptor error: %v", err)
	}
}

func TestMetadataUnaryClientInterceptorDoesNotCreateSpanWithoutParentSpan(t *testing.T) {
	parent := ctxmeta.WithTraceID(context.Background(), "trace-1")

	interceptor := MetadataUnaryClientInterceptor()
	err := interceptor(parent, "/test.Service/Call", nil, nil, nil, func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		if got := ctxmeta.SpanID(ctx); got != "" {
			t.Fatalf("span_id = %q, want empty", got)
		}
		if got := ctxmeta.ParentSpanID(ctx); got != "" {
			t.Fatalf("parent_span_id = %q, want empty", got)
		}

		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			t.Fatal("missing outgoing metadata")
		}
		assertMetadataValue(t, md, ctxmeta.MetadataTraceID, "trace-1")
		if values := md.Get(ctxmeta.MetadataSpanID); len(values) != 0 {
			t.Fatalf("metadata span_id = %v, want empty", values)
		}
		if values := md.Get(ctxmeta.MetadataParentSpanID); len(values) != 0 {
			t.Fatalf("metadata parent_span_id = %v, want empty", values)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("interceptor error: %v", err)
	}
}

func assertMetadataValue(t *testing.T, md metadata.MD, key, want string) {
	t.Helper()
	values := md.Get(key)
	if len(values) == 0 {
		t.Fatalf("metadata %s missing", key)
	}
	if values[0] != want {
		t.Fatalf("metadata %s = %q, want %q", key, values[0], want)
	}
}
