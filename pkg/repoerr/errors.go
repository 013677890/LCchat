package repoerr

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
	// ErrRecordNotFound 表示仓储中不存在指定记录。
	ErrRecordNotFound = errors.New("record not found")
	// ErrDuplicateKey 表示数据库唯一键冲突。
	ErrDuplicateKey = errors.New("duplicate key")
	// ErrDatabase 表示未被细分的数据库访问错误。
	ErrDatabase = errors.New("database error")
	// ErrRedisNil 表示 Redis 中不存在指定 key。
	ErrRedisNil = errors.New("redis: key not found")
	// ErrRedis 表示未被细分的 Redis 访问错误。
	ErrRedis = errors.New("redis error")
)

// WrapDBError 将 DB、GORM 或驱动错误归一化为仓储哨兵错误。
func WrapDBError(err error) error {
	if err == nil {
		return nil
	}
	if isDuplicateKeyError(err) {
		return ErrDuplicateKey
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrRecordNotFound
	}
	return fmt.Errorf("%w: %v", ErrDatabase, err)
}

// WrapRedisError 将 Redis 错误归一化为仓储哨兵错误。
func WrapRedisError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, redis.Nil) {
		return ErrRedisNil
	}
	return fmt.Errorf("%w: %v", ErrRedis, err)
}

// LogRedisError 在 Redis 降级路径记录 Warn；err 为 nil 时不记录日志。
func LogRedisError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	logger.Warn(ctx, "Redis 操作错误，已降级处理", logger.ErrorField("error", err))
}

// isDuplicateKeyError 判断错误是否表示数据库唯一键冲突。
func isDuplicateKeyError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var mysqlErr *gmysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return true
	}
	errMessage := strings.ToLower(err.Error())
	return strings.Contains(errMessage, "duplicate entry") ||
		strings.Contains(errMessage, "unique constraint failed")
}
