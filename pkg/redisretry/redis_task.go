package redisretry

import (
	"context"
	"errors"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
)

// RedisTask 表示投递到 Kafka 的缓存失效补偿任务。
type RedisTask struct {
	Keys []string `json:"keys"`

	TraceID      string    `json:"trace_id,omitempty"`
	SpanID       string    `json:"span_id,omitempty"`
	ParentSpanID string    `json:"parent_span_id,omitempty"`
	UserUUID     string    `json:"user_uuid,omitempty"`
	DeviceID     string    `json:"device_id,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	OriginalErr  string    `json:"original_err"`
	Source       string    `json:"source,omitempty"`
}

// BuildDelTask 构造 DEL 重试任务。
func BuildDelTask(keys ...string) RedisTask {
	return RedisTask{
		Keys:      append([]string(nil), keys...),
		Timestamp: time.Now(),
	}
}

// Validate 拒绝空任务，避免消费者把无效消息当成成功处理。
func (t RedisTask) Validate() error {
	if len(t.Keys) == 0 {
		return errors.New("Redis DEL 任务缺少 key")
	}
	for _, key := range t.Keys {
		if key == "" {
			return errors.New("Redis DEL 任务包含空 key")
		}
	}
	return nil
}

// WithContext 为任务补充 trace/user/device 等上下文信息。
func (t RedisTask) WithContext(ctx context.Context) RedisTask {
	if traceID := ctxmeta.TraceID(ctx); traceID != "" {
		t.TraceID = traceID
	}
	if spanID := ctxmeta.SpanID(ctx); spanID != "" {
		t.SpanID = spanID
	}
	if parentSpanID := ctxmeta.ParentSpanID(ctx); parentSpanID != "" {
		t.ParentSpanID = parentSpanID
	}
	if userUUID := ctxmeta.UserUUID(ctx); userUUID != "" {
		t.UserUUID = userUUID
	}
	if deviceID := ctxmeta.DeviceID(ctx); deviceID != "" {
		t.DeviceID = deviceID
	}
	return t
}

// WithError 为任务记录原始错误。
func (t RedisTask) WithError(err error) RedisTask {
	t.OriginalErr = err.Error()
	return t
}

// WithSource 为任务记录调用来源。
func (t RedisTask) WithSource(source string) RedisTask {
	t.Source = source
	return t
}
