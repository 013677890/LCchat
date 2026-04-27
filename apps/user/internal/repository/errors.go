package repository

import (
	"github.com/013677890/LCchat-Backend/apps/user/mq"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// ==================== Repository 层统一错误定义 ====================

var (
	// ErrRecordNotFound 记录不存在
	ErrRecordNotFound = errors.New("record not found")

	// ErrDuplicateKey 唯一键冲突
	ErrDuplicateKey = errors.New("duplicate key")

	// ErrDatabase 数据库操作错误
	ErrDatabase = errors.New("database error")

	// ErrRedisNil Redis Key 不存在
	ErrRedisNil = errors.New("redis: key not found")

	// ErrRedis Redis 操作错误
	ErrRedis = errors.New("redis error")

	// ErrApplyNotFound 申请不存在或已处理
	ErrApplyNotFound = errors.New("apply not found or already processed")
)

// ==================== 核心包装函数 ====================

// wrapError 通用错误包装函数
// err: 要包装的错误
// rules: 映射规则 map[源错误]目标错误
// defaultErr: 默认错误
func wrapError(err error, rules map[error]error, defaultErr error) error {
	if err == nil {
		return nil
	}

	// 检查映射规则
	for source, target := range rules {
		if errors.Is(err, source) {
			return target
		}
	}

	// 未匹配任何规则，包装默认错误（保留原始错误信息用于日志）
	return fmt.Errorf("%w: %v", defaultErr, err)
}

// ==================== 预定义规则 ====================

var (
	// dbErrorRules 数据库错误映射规则
	dbErrorRules = map[error]error{
		gorm.ErrRecordNotFound: ErrRecordNotFound,
		gorm.ErrDuplicatedKey:  ErrDuplicateKey,
	}

	// redisErrorRules Redis 错误映射规则
	redisErrorRules = map[error]error{
		redis.Nil: ErrRedisNil,
	}
)

// ==================== 便捷函数 ====================

// WrapDBError 包装数据库错误
func WrapDBError(err error) error {
	return wrapError(err, dbErrorRules, ErrDatabase)
}

// WrapRedisError 包装 Redis 错误
func WrapRedisError(err error) error {
	return wrapError(err, redisErrorRules, ErrRedis)
}

// 日志记录redis错误
func LogRedisError(ctx context.Context, err error) {
	logger.Warn(ctx, "Redis 操作错误，已降级处理", logger.ErrorField("error", err))
}

// LogAndRetryRedisError 日志记录redis错误并发送到kafka重试
// task: 要重试的 Redis 任务（由调用方构造）
func LogAndRetryRedisError(ctx context.Context, task mq.RedisTask, err error) {
	// 1. 记录 Redis 错误日志
	logger.Warn(ctx, "Redis 操作失败，发送到重试队列",
		logger.ErrorField("error", err),
		logger.String("task_type", string(task.Type)),
		logger.String("command", task.Command),
	)

	// 2. 为任务添加上下文信息和错误信息
	task = task.WithContext(ctx).WithError(err)

	// 3. 发送到 Kafka 重试队列
	if kafkaErr := mq.SendRedisTask(ctx, task); kafkaErr != nil {
		// Kafka 发送失败，记录错误日志用于监控报警，然后放弃
		logger.Error(ctx, "发送 Redis 重试任务到 Kafka 失败，放弃处理",
			logger.ErrorField("kafka_error", kafkaErr),
			logger.ErrorField("original_error", err),
			logger.String("task_type", string(task.Type)),
		)
	}
}
