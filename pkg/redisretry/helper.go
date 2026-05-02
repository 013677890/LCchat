package redisretry

import (
	"context"

	"github.com/013677890/LCchat-Backend/pkg/logger"
)

// LogAndRetryRedisError 统一记录 Redis 失败日志并投递 Kafka 重试任务。
func LogAndRetryRedisError(ctx context.Context, task RedisTask, err error) {
	if err == nil {
		return
	}

	logger.Warn(ctx, "Redis 操作失败，发送到重试队列",
		logger.ErrorField("error", err),
		logger.String("task_type", string(task.Type)),
		logger.String("command", task.Command),
	)

	task = task.WithContext(ctx).WithError(err)
	if kafkaErr := SendRedisTask(ctx, task); kafkaErr != nil {
		logger.Error(ctx, "发送 Redis 重试任务到 Kafka 失败，放弃处理",
			logger.ErrorField("kafka_error", kafkaErr),
			logger.ErrorField("original_error", err),
			logger.String("task_type", string(task.Type)),
		)
	}
}
