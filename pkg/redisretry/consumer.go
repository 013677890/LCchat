package redisretry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	task, err := decodeRedisTask(message)
	if err != nil {
		return kafka.Permanent(fmt.Errorf("解析 Redis DEL 任务失败: %w", err))
	}
	if err := task.Validate(); err != nil {
		return kafka.Permanent(err)
	}
	ctx = contextFromTask(ctx, task)
	ctx = withCompensationContext(ctx)

	if err := c.redisClient.Del(ctx, task.Keys...).Err(); err != nil {
		return fmt.Errorf("执行 Redis DEL 失败: %w", err)
	}

	c.logger.Info(ctx, "Redis 缓存失效任务执行成功", map[string]interface{}{
		"keys":   task.Keys,
		"source": task.Source,
	})
	return nil
}

// decodeRedisTask 严格按当前 RedisTask 契约解码；未知字段和尾随 JSON 一律拒绝。
func decodeRedisTask(message []byte) (RedisTask, error) {
	decoder := json.NewDecoder(bytes.NewReader(message))
	decoder.DisallowUnknownFields()

	var task RedisTask
	if err := decoder.Decode(&task); err != nil {
		return RedisTask{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return RedisTask{}, errors.New("Redis DEL 任务包含尾随 JSON")
		}
		return RedisTask{}, err
	}
	return task, nil
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
