package ctxmeta

// Canonical context keys used across gateway/user services.
const (
	KeyTraceID      = "trace_id"
	KeySpanID       = "span_id"
	KeyParentSpanID = "parent_span_id"
	KeyUserUUID     = "user_uuid"
	KeyDeviceID     = "device_id"
	KeyClientIP     = "client_ip"
)

// Canonical HTTP headers used for context-related metadata.
const (
	HeaderRequestID    = "X-Request-ID"
	HeaderSpanID       = "X-Span-ID"
	HeaderParentSpanID = "X-Parent-Span-ID"
	HeaderDeviceID     = "X-Device-ID"
)

// Canonical gRPC metadata keys used for context propagation.
const (
	MetadataTraceID       = "trace_id"
	MetadataSpanID        = "span_id"
	MetadataParentSpanID  = "parent_span_id"
	MetadataUserUUID      = "user_uuid"
	MetadataDeviceID      = "device_id"
	MetadataClientIP      = "client_ip"
	MetadataXRealIP       = "x-real-ip"
	MetadataXForwardedFor = "x-forwarded-for"
)
