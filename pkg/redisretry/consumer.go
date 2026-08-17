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
// 编排走 ManualConsumerPool；业务只允许安全 DEL，拒绝 SET/HSET 等可写回旧值的命令。
// 由 API 服务通过 kafka.RunIsolatedPool 启动，消费致命失败不拖死 gRPC。
type RedisRetryConsumer struct {
	pool        *kafka.ManualConsumerPool
	redisClient *redis.Client
	logger      kafka.Logger
}

// NewRedisRetryConsumer 创建只执行 DEL 的手动提交消费 Pool。
// workers 必须在 [1, 64]，非法值在 NewManualConsumerPool 阶段失败；
// 每个 Reader 串行处理任务，未知字段、尾随 JSON 或写命令标记永久错误进入死信。
func NewRedisRetryConsumer(
	brokers []string,
	topic string,
	groupID string,
	workers int,
	redisClient *redis.Client,
	deadLetterSink kafka.DeadLetterSink,
	logger kafka.Logger,
) (*RedisRetryConsumer, error) {
	pool, err := kafka.NewManualConsumerPool(kafka.ManualConsumerPoolConfig{
		Name:    "redis-retry-" + topic,
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
		Workers: workers,
		Config: kafka.ManualConsumerConfig{
			MinBytes:       1,
			MaxBytes:       10 << 20,
			MaxWait:        500 * time.Millisecond,
			ErrorBackoff:   time.Second,
			DeadLetterSink: deadLetterSink,
		},
	})
	if err != nil {
		return nil, err
	}
	return &RedisRetryConsumer{
		pool:        pool,
		redisClient: redisClient,
		logger:      logger,
	}, nil
}

// Start 启动全部 Reader 并持续处理缓存失效任务。
// 任一 worker 致命退出会取消同 Pool 兄弟并返回错误；上层 RunIsolatedPool 负责隔离与退避重启。
// Redis 临时 DEL 失败在消息内按公共 Consumer 策略原地重试，不在此处吞掉。
func (c *RedisRetryConsumer) Start(ctx context.Context) error {
	c.logger.Info(ctx, "Redis 缓存失效消费者启动", nil)
	return c.pool.Start(ctx, c.processMessage)
}

// Close 关闭 Pool 内全部 Reader，并聚合各 Reader 的关闭错误。
// 关停顺序应先 cancel 监督循环的 ctx，再 Close，避免 Reader 已关却仍被 isolate 重入 Start。
func (c *RedisRetryConsumer) Close() error {
	if c == nil || c.pool == nil {
		return nil
	}
	return c.pool.Close()
}

// processMessage 严格校验并执行 Redis DEL 补偿任务。
// 契约错误标记为永久错误进入死信；Redis 瞬时失败返回普通错误以保留原地重试语义。
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

// contextFromTask 从补偿任务恢复链路字段，便于异步 DEL 与原请求日志关联。
// 缺少 trace/span 时保持已有上下文，不伪造不存在的父子链路。
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
