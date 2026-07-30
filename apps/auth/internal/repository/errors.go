package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/013677890/LCchat-Backend/pkg/logger"
	gmysql "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var (
	// ErrRecordNotFound 表示记录不存在。
	ErrRecordNotFound = errors.New("record not found")
	// ErrDuplicateKey 表示唯一键冲突。
	ErrDuplicateKey = errors.New("duplicate key")
	// ErrDatabase 表示数据库访问失败。
	ErrDatabase = errors.New("database error")
	// ErrRedisNil 表示 Redis 中不存在指定 key。
	ErrRedisNil = errors.New("redis: key not found")
	// ErrRedis 表示 Redis 访问失败。
	ErrRedis = errors.New("redis error")
)

func wrapError(err error, rules map[error]error, defaultErr error) error {
	if err == nil {
		return nil
	}
	if isDuplicateKeyError(err) {
		return ErrDuplicateKey
	}
	for source, target := range rules {
		if errors.Is(err, source) {
			return target
		}
	}
	return fmt.Errorf("%w: %v", defaultErr, err)
}

func isDuplicateKeyError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var mysqlErr *gmysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate entry") ||
		strings.Contains(msg, "unique constraint failed")
}

var (
	dbErrorRules = map[error]error{
		gorm.ErrRecordNotFound: ErrRecordNotFound,
		gorm.ErrDuplicatedKey:  ErrDuplicateKey,
	}
	redisErrorRules = map[error]error{
		redis.Nil: ErrRedisNil,
	}
)

// WrapDBError 统一包装数据库错误。
func WrapDBError(err error) error {
	return wrapError(err, dbErrorRules, ErrDatabase)
}

// WrapRedisError 统一包装 Redis 错误。
func WrapRedisError(err error) error {
	return wrapError(err, redisErrorRules, ErrRedis)
}

// LogRedisError 记录 Redis 降级日志。
func LogRedisError(ctx context.Context, err error) {
	logger.Warn(ctx, "Redis 操作错误，已降级处理", logger.ErrorField("error", err))
}
