package kafka

import (
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"context"

	"go.uber.org/zap"
)

// ==================== Logger 接口 ====================

// Logger 定义 Kafka 消费者需要的日志接口
type Logger interface {
	Info(ctx context.Context, msg string, fields map[string]interface{})
	Error(ctx context.Context, msg string, fields map[string]interface{})
}

// ==================== Zap Logger Adapter ====================

// ZapLoggerAdapter 将 zap.Logger 适配到 kafka.Logger 接口
type ZapLoggerAdapter struct {
	logger *zap.Logger
}

// NewZapLoggerAdapter 创建 Zap 日志适配器
func NewZapLoggerAdapter(logger *zap.Logger) Logger {
	return &ZapLoggerAdapter{logger: logger}
}

func (a *ZapLoggerAdapter) Info(ctx context.Context, msg string, fields map[string]interface{}) {
	zapFields := convertFieldsToZap(ctx, fields)
	a.logger.Info(msg, zapFields...)
}

func (a *ZapLoggerAdapter) Error(ctx context.Context, msg string, fields map[string]interface{}) {
	zapFields := convertFieldsToZap(ctx, fields)
	a.logger.Error(msg, zapFields...)
}

// convertFieldsToZap 将 map[string]interface{} 转换为 zap.Field 切片
func convertFieldsToZap(ctx context.Context, fields map[string]interface{}) []zap.Field {
	zapFields := make([]zap.Field, 0, len(fields)+3)

	// 从 context 中提取标准字段
	if traceID := ctxmeta.TraceID(ctx); traceID != "" {
		zapFields = append(zapFields, zap.String(ctxmeta.KeyTraceID, traceID))
	}
	if userUUID := ctxmeta.UserUUID(ctx); userUUID != "" {
		zapFields = append(zapFields, zap.String(ctxmeta.KeyUserUUID, userUUID))
	}
	if deviceID := ctxmeta.DeviceID(ctx); deviceID != "" {
		zapFields = append(zapFields, zap.String(ctxmeta.KeyDeviceID, deviceID))
	}

	// 添加自定义字段
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}

	return zapFields
}
