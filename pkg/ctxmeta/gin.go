package ctxmeta

import (
	"context"

	"github.com/gin-gonic/gin"
)

func setGinString(c *gin.Context, key string, value string) string {
	if c == nil {
		return ""
	}
	n := normalize(value)
	if n == "" {
		return ""
	}
	c.Set(key, n)
	return n
}

func getGinString(c *gin.Context, key string) string {
	if c == nil {
		return ""
	}
	value, exists := c.Get(key)
	if !exists {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return normalize(text)
}

func SetTraceID(c *gin.Context, traceID string) string {
	return setGinString(c, KeyTraceID, traceID)
}

func SetSpanID(c *gin.Context, spanID string) string {
	return setGinString(c, KeySpanID, spanID)
}

func SetParentSpanID(c *gin.Context, parentSpanID string) string {
	return setGinString(c, KeyParentSpanID, parentSpanID)
}

func SetUserUUID(c *gin.Context, userUUID string) string {
	return setGinString(c, KeyUserUUID, userUUID)
}

func SetDeviceID(c *gin.Context, deviceID string) string {
	return setGinString(c, KeyDeviceID, deviceID)
}

func SetClientIP(c *gin.Context, clientIP string) string {
	return setGinString(c, KeyClientIP, clientIP)
}

func TraceIDFromGin(c *gin.Context) string {
	return getGinString(c, KeyTraceID)
}

func SpanIDFromGin(c *gin.Context) string {
	return getGinString(c, KeySpanID)
}

func ParentSpanIDFromGin(c *gin.Context) string {
	return getGinString(c, KeyParentSpanID)
}

func UserUUIDFromGin(c *gin.Context) string {
	return getGinString(c, KeyUserUUID)
}

func DeviceIDFromGin(c *gin.Context) string {
	return getGinString(c, KeyDeviceID)
}

func ClientIPFromGin(c *gin.Context) string {
	return getGinString(c, KeyClientIP)
}

// BuildContextFromGin builds a context.Context by copying canonical values from gin.Context.
func BuildContextFromGin(c *gin.Context) context.Context {
	if c == nil || c.Request == nil {
		return context.Background()
	}
	ctx := c.Request.Context()
	if traceID := TraceIDFromGin(c); traceID != "" {
		ctx = WithTraceID(ctx, traceID)
	}
	if spanID := SpanIDFromGin(c); spanID != "" {
		ctx = WithSpanID(ctx, spanID)
	}
	if parentSpanID := ParentSpanIDFromGin(c); parentSpanID != "" {
		ctx = WithParentSpanID(ctx, parentSpanID)
	}
	if userUUID := UserUUIDFromGin(c); userUUID != "" {
		ctx = WithUserUUID(ctx, userUUID)
	}
	if deviceID := DeviceIDFromGin(c); deviceID != "" {
		ctx = WithDeviceID(ctx, deviceID)
	}
	if clientIP := ClientIPFromGin(c); clientIP != "" {
		ctx = WithClientIP(ctx, clientIP)
	}
	return ctx
}
