package redisretry

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/redis/go-redis/v9"
)

// RedisRetryConsumer 消费 Kafka 中的缓存失效补偿任务。
type RedisRetryConsumer struct {
	consumer    *kafka.Consumer
	redisClient *redis.Client
	logger      kafka.Logger
}

// NewRedisRetryConsumer 创建只执行 DEL 的手动提交消费者。
func NewRedisRetryConsumer(
	brokers []string,
	topic string,
	groupID string,
	redisClient *redis.Client,
	deadLetterSink kafka.DeadLetterSink,
	logger kafka.Logger,
) *RedisRetryConsumer {
	consumer := kafka.NewManualCommitConsumer(brokers, topic, groupID, kafka.ManualConsumerConfig{
		MinBytes:       1,
		MaxBytes:       10 << 20,
		MaxWait:        500 * time.Millisecond,
		ErrorBackoff:   time.Second,
		DeadLetterSink: deadLetterSink,
	})
	return &RedisRetryConsumer{
		consumer:    consumer,
		redisClient: redisClient,
		logger:      logger,
	}
}

// Start 启动消费者并持续处理缓存失效任务。
func (c *RedisRetryConsumer) Start(ctx context.Context) error {
	c.logger.Info(ctx, "Redis 缓存失效消费者启动", nil)
	return c.consumer.Start(ctx, c.processMessage)
}

// Close 关闭消费者。
func (c *RedisRetryConsumer) Close() error {
	return c.consumer.Close()
}

func (c *RedisRetryConsumer) processMessage(ctx context.Context, message []byte) error {
	var task RedisTask
	if err := json.Unmarshal(message, &task); err != nil {
		return kafka.Permanent(fmt.Errorf("解析 Redis DEL 任务失败: %w", err))
	}
	if err := task.Validate(); err != nil {
		return kafka.Permanent(err)
	}
	ctx = contextFromTask(ctx, task)

	if err := c.redisClient.Del(ctx, task.Keys...).Err(); err != nil {
		return fmt.Errorf("执行 Redis DEL 失败: %w", err)
	}

	c.logger.Info(ctx, "Redis 缓存失效任务执行成功", map[string]interface{}{
		"keys":   task.Keys,
		"source": task.Source,
	})
	return nil
}

func contextFromTask(ctx context.Context, task RedisTask) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if task.TraceID != "" {
		ctx = ctxmeta.WithTraceID(ctx, task.TraceID)
	}
	if task.UserUUID != "" {
		ctx = ctxmeta.WithUserUUID(ctx, task.UserUUID)
	}
	if task.DeviceID != "" {
		ctx = ctxmeta.WithDeviceID(ctx, task.DeviceID)
	}
	if task.TraceID == "" || task.SpanID == "" {
		return ctx
	}
	ctx = ctxmeta.WithParentSpanID(ctx, task.SpanID)
	return ctxmeta.WithSpanID(ctx, ctxmeta.NewSpanID())
}
