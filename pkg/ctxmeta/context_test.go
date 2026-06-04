package ctxmeta

import (
	"context"
	"testing"
)

func TestCopyKnownFromParentCopiesSpanMetadata(t *testing.T) {
	parent := context.Background()
	parent = WithTraceID(parent, "trace-1")
	parent = WithSpanID(parent, "span-1")
	parent = WithParentSpanID(parent, "root")
	parent = WithUserUUID(parent, "user-1")
	parent = WithDeviceID(parent, "device-1")
	parent = WithClientIP(parent, "127.0.0.1")

	ctx := CopyKnownFromParent(parent)

	if got := TraceID(ctx); got != "trace-1" {
		t.Fatalf("TraceID = %q", got)
	}
	if got := SpanID(ctx); got != "span-1" {
		t.Fatalf("SpanID = %q", got)
	}
	if got := ParentSpanID(ctx); got != "root" {
		t.Fatalf("ParentSpanID = %q", got)
	}
	if got := UserUUID(ctx); got != "user-1" {
		t.Fatalf("UserUUID = %q", got)
	}
	if got := DeviceID(ctx); got != "device-1" {
		t.Fatalf("DeviceID = %q", got)
	}
	if got := ClientIP(ctx); got != "127.0.0.1" {
		t.Fatalf("ClientIP = %q", got)
	}
}

func TestChildContextFromParentCreatesChildSpan(t *testing.T) {
	parent := context.Background()
	parent = WithTraceID(parent, "trace-1")
	parent = WithSpanID(parent, "span-parent")
	parent = WithParentSpanID(parent, "span-grandparent")
	parent = WithUserUUID(parent, "user-1")

	ctx := ChildContextFromParent(parent)

	if got := TraceID(ctx); got != "trace-1" {
		t.Fatalf("TraceID = %q", got)
	}
	if got := ParentSpanID(ctx); got != "span-parent" {
		t.Fatalf("ParentSpanID = %q", got)
	}
	if got := SpanID(ctx); got == "" || got == "span-parent" {
		t.Fatalf("SpanID = %q, want new child span", got)
	}
	if got := UserUUID(ctx); got != "user-1" {
		t.Fatalf("UserUUID = %q", got)
	}
}

func TestChildContextFromParentDoesNotCreateSpanWithoutParentSpan(t *testing.T) {
	parent := context.Background()
	parent = WithTraceID(parent, "trace-1")
	parent = WithUserUUID(parent, "user-1")

	ctx := ChildContextFromParent(parent)

	if got := TraceID(ctx); got != "trace-1" {
		t.Fatalf("TraceID = %q", got)
	}
	if got := SpanID(ctx); got != "" {
		t.Fatalf("SpanID = %q, want empty", got)
	}
	if got := ParentSpanID(ctx); got != "" {
		t.Fatalf("ParentSpanID = %q, want empty", got)
	}
}
