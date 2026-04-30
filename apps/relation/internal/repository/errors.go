package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var (
	ErrRecordNotFound = errors.New("record not found")
	ErrDuplicateKey   = errors.New("duplicate key")
	ErrDatabase       = errors.New("database error")
	ErrRedisNil       = errors.New("redis: key not found")
	ErrRedis          = errors.New("redis error")
	ErrApplyNotFound  = errors.New("apply not found or already processed")
)

// WrapDBError 将底层 GORM 错误归一化为 relation 仓储层错误。
//
// 这样 service 层只需要识别少量稳定错误类型，而不必感知具体 ORM 返回值。
func WrapDBError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrRecordNotFound
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrDuplicateKey
	}
	return fmt.Errorf("%w: %v", ErrDatabase, err)
}

// WrapRedisError 将底层 Redis 错误归一化为仓储层错误。
//
// 当前 relation-service 仍以 DB-first 为主，但保留该包装函数可降低后续恢复缓存逻辑时
// 的改造成本。
func WrapRedisError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, redis.Nil) {
		return ErrRedisNil
	}
	return fmt.Errorf("%w: %v", ErrRedis, err)
}

// LogRedisError 统一记录 Redis 降级日志。
//
// relation-service 对 Redis 的使用都遵循“尽力命中缓存、失败降级 DB”的原则，因此这里
// 统一输出 warn 日志，避免每个仓储方法重复拼装相同的日志字段。
func LogRedisError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	logger.Warn(ctx, "Redis 操作错误，已降级处理", logger.ErrorField("error", err))
}
