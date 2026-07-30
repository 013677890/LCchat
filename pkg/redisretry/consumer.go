package redisretry

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/redis/go-redis/v9"
)

// RedisRetryConsumer 消费 Kafka 中的 Redis 补偿任务并执行重试。
type RedisRetryConsumer struct {
	consumer    *kafka.Consumer
	redisClient *redis.Client
	producer    *kafka.Producer
	logger      kafka.Logger
}

// NewRedisRetryConsumer 创建 Redis 重试消费者。
func NewRedisRetryConsumer(
	brokers []string,
	topic string,
	groupID string,
	redisClient *redis.Client,
	producer *kafka.Producer,
	logger kafka.Logger,
) *RedisRetryConsumer {
	consumer := kafka.NewConsumer(brokers, topic, groupID)
	return &RedisRetryConsumer{
		consumer:    consumer,
		redisClient: redisClient,
		producer:    producer,
		logger:      logger,
	}
}

// Start 启动消费者并持续处理 Redis 重试任务。
func (c *RedisRetryConsumer) Start(ctx context.Context) error {
	c.logger.Info(ctx, "Redis 重试队列消费者启动", nil)

	return c.consumer.Start(ctx, func(ctx context.Context, message []byte) error {
		return c.processMessage(ctx, message)
	})
}

// Close 关闭消费者。
func (c *RedisRetryConsumer) Close() error {
	return c.consumer.Close()
}

func (c *RedisRetryConsumer) processMessage(ctx context.Context, message []byte) error {
	var task RedisTask
	if err := json.Unmarshal(message, &task); err != nil {
		return fmt.Errorf("解析 Redis 任务失败: %w", err)
	}
	ctx = contextFromTask(ctx, task)

	c.logger.Info(ctx, "处理 Redis 重试任务", map[string]interface{}{
		"type":        task.Type,
		"retry_count": task.RetryCount,
	})

	err := c.executeRedisTask(ctx, task)
	if err != nil {
		if task.RetryCount < task.MaxRetries {
			task.RetryCount++
			taskJSON, _ := json.Marshal(task)
			if retryErr := c.producer.Send(ctx, taskJSON); retryErr != nil {
				c.logger.Error(ctx, "重新发送 Redis 任务到 Kafka 失败", map[string]interface{}{
					"error":       retryErr.Error(),
					"retry_count": task.RetryCount,
				})
			} else {
				c.logger.Info(ctx, "Redis 任务重新发送到队列", map[string]interface{}{
					"retry_count": task.RetryCount,
					"max_retries": task.MaxRetries,
				})
			}
		} else {
			c.logger.Error(ctx, "Redis 任务达到最大重试次数，放弃处理", map[string]interface{}{
				"error":       err.Error(),
				"retry_count": task.RetryCount,
				"max_retries": task.MaxRetries,
				"task":        task,
			})
		}

		return err
	}

	c.logger.Info(ctx, "Redis 重试任务执行成功", map[string]interface{}{
		"type":        task.Type,
		"retry_count": task.RetryCount,
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
	if task.TraceID == "" {
		return ctx
	}
	if task.SpanID == "" {
		return ctx
	}
	ctx = ctxmeta.WithParentSpanID(ctx, task.SpanID)
	return ctxmeta.WithSpanID(ctx, ctxmeta.NewSpanID())
}

func (c *RedisRetryConsumer) executeRedisTask(ctx context.Context, task RedisTask) error {
	switch task.Type {
	case CmdSimple:
		return c.executeSimpleCommand(ctx, task)
	case CmdPipeline:
		return c.executePipeline(ctx, task)
	case CmdLua:
		return c.executeLuaScript(ctx, task)
	default:
		return fmt.Errorf("未知的命令类型: %s", task.Type)
	}
}

func (c *RedisRetryConsumer) executeSimpleCommand(ctx context.Context, task RedisTask) error {
	args := make([]interface{}, 0, len(task.Args)+1)
	args = append(args, task.Command)
	args = append(args, task.Args...)
	cmd := c.redisClient.Do(ctx, args...)
	return cmd.Err()
}

func (c *RedisRetryConsumer) executePipeline(ctx context.Context, task RedisTask) error {
	pipe := c.redisClient.Pipeline()

	for _, cmd := range task.PipelineCmds {
		args := make([]interface{}, 0, len(cmd.Args)+1)
		args = append(args, cmd.Command)
		args = append(args, cmd.Args...)
		pipe.Do(ctx, args...)
	}

	_, err := pipe.Exec(ctx)
	return err
}

func (c *RedisRetryConsumer) executeLuaScript(ctx context.Context, task RedisTask) error {
	script := redis.NewScript(task.LuaScript)
	cmd := script.Run(ctx, c.redisClient, task.LuaKeys, task.LuaArgs...)
	return cmd.Err()
}
