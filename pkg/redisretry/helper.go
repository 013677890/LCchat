package redisretry

import (
	"context"

	"github.com/013677890/LCchat-Backend/pkg/logger"
)

// LogAndRetryRedisError 记录缓存操作失败并投递 DEL 失效补偿任务。
func LogAndRetryRedisError(ctx context.Context, task RedisTask, err error) {
	if err == nil {
		return
	}

	logger.Warn(ctx, "Redis 缓存操作失败，发送 DEL 失效补偿任务",
		logger.ErrorField("error", err),
		logger.String("source", task.Source),
	)

	task = task.WithContext(ctx).WithError(err)
	if kafkaErr := sendRedisTask(ctx, task); kafkaErr != nil {
		logger.Error(ctx, "发送 Redis 缓存失效任务到 Kafka 失败，放弃处理",
			logger.ErrorField("kafka_error", kafkaErr),
			logger.ErrorField("original_error", err),
			logger.String("source", task.Source),
		)
	}
}
