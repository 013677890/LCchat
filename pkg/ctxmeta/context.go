package ctxmeta

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

func normalize(v string) string {
	return strings.TrimSpace(v)
}

func with(ctx context.Context, key string, value string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	n := normalize(value)
	if n == "" {
		return ctx
	}
	return context.WithValue(ctx, key, n)
}

func get(ctx context.Context, key string) string {
	if ctx == nil {
		return ""
	}
	value, ok := ctx.Value(key).(string)
	if !ok {
		return ""
	}
	return normalize(value)
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return with(ctx, KeyTraceID, traceID)
}

func WithSpanID(ctx context.Context, spanID string) context.Context {
	return with(ctx, KeySpanID, spanID)
}

func WithParentSpanID(ctx context.Context, parentSpanID string) context.Context {
	return with(ctx, KeyParentSpanID, parentSpanID)
}

func WithUserUUID(ctx context.Context, userUUID string) context.Context {
	return with(ctx, KeyUserUUID, userUUID)
}

func WithDeviceID(ctx context.Context, deviceID string) context.Context {
	return with(ctx, KeyDeviceID, deviceID)
}

func WithClientIP(ctx context.Context, clientIP string) context.Context {
	return with(ctx, KeyClientIP, clientIP)
}

func TraceID(ctx context.Context) string {
	return get(ctx, KeyTraceID)
}

func SpanID(ctx context.Context) string {
	return get(ctx, KeySpanID)
}

func ParentSpanID(ctx context.Context) string {
	return get(ctx, KeyParentSpanID)
}

func UserUUID(ctx context.Context) string {
	return get(ctx, KeyUserUUID)
}

func DeviceID(ctx context.Context) string {
	return get(ctx, KeyDeviceID)
}

func ClientIP(ctx context.Context) string {
	return get(ctx, KeyClientIP)
}

// NewSpanID 生成轻量 span 标识。
//
// 当前项目暂未接入 OpenTelemetry，因此直接复用 UUID 作为日志关联 ID。
func NewSpanID() string {
	return uuid.NewString()
}

// CopyKnownFromParent copies canonical context metadata into a new background context.
func CopyKnownFromParent(parent context.Context) context.Context {
	ctx := context.Background()
	if parent == nil {
		return ctx
	}
	if traceID := TraceID(parent); traceID != "" {
		ctx = WithTraceID(ctx, traceID)
	}
	if spanID := SpanID(parent); spanID != "" {
		ctx = WithSpanID(ctx, spanID)
	}
	if parentSpanID := ParentSpanID(parent); parentSpanID != "" {
		ctx = WithParentSpanID(ctx, parentSpanID)
	}
	if userUUID := UserUUID(parent); userUUID != "" {
		ctx = WithUserUUID(ctx, userUUID)
	}
	if deviceID := DeviceID(parent); deviceID != "" {
		ctx = WithDeviceID(ctx, deviceID)
	}
	if clientIP := ClientIP(parent); clientIP != "" {
		ctx = WithClientIP(ctx, clientIP)
	}
	return ctx
}

// ChildContextFromParent copies canonical metadata and creates a child span.
//
// 适合异步任务、后台补偿任务等“脱离当前 goroutine 继续执行”的场景：
// trace_id 继承父链路，span_id 新生成，parent_span_id 指向父 context 的 span_id。
// 如果父 context 没有 span_id，则不补造 span，便于尽早暴露漏传链路。
func ChildContextFromParent(parent context.Context) context.Context {
	ctx := context.Background()
	if parent == nil {
		return ctx
	}
	traceID := TraceID(parent)
	if traceID != "" {
		ctx = WithTraceID(ctx, traceID)
	}
	if userUUID := UserUUID(parent); userUUID != "" {
		ctx = WithUserUUID(ctx, userUUID)
	}
	if deviceID := DeviceID(parent); deviceID != "" {
		ctx = WithDeviceID(ctx, deviceID)
	}
	if clientIP := ClientIP(parent); clientIP != "" {
		ctx = WithClientIP(ctx, clientIP)
	}
	parentSpanID := SpanID(parent)
	if traceID == "" || parentSpanID == "" {
		return ctx
	}
	ctx = WithSpanID(ctx, NewSpanID())
	ctx = WithParentSpanID(ctx, parentSpanID)
	return ctx
}
