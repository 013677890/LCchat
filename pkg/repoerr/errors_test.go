package repoerr

import (
	"context"
	"errors"
	"testing"

	gmysql "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestWrapDBError 验证数据库错误归一化规则。
func TestWrapDBError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		target error
	}{
		{name: "nil", err: nil},
		{name: "record_not_found", err: gorm.ErrRecordNotFound, target: ErrRecordNotFound},
		{name: "gorm_duplicate", err: gorm.ErrDuplicatedKey, target: ErrDuplicateKey},
		{name: "mysql_1062", err: &gmysql.MySQLError{Number: 1062, Message: "Duplicate entry"}, target: ErrDuplicateKey},
		{name: "duplicate_entry_text", err: errors.New("Duplicate entry 'alice' for key 'uk_name'"), target: ErrDuplicateKey},
		{name: "sqlite_unique_text", err: errors.New("UNIQUE constraint failed: users.name"), target: ErrDuplicateKey},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapDBError(tt.err)
			if tt.err == nil {
				require.NoError(t, got)
				return
			}
			require.ErrorIs(t, got, tt.target)
		})
	}
}

// TestWrapDBErrorUnknown 验证未知数据库错误保留仓储分类与原始信息。
func TestWrapDBErrorUnknown(t *testing.T) {
	err := WrapDBError(errors.New("connection reset"))
	require.ErrorIs(t, err, ErrDatabase)
	require.Contains(t, err.Error(), "connection reset")
}

// TestWrapRedisError 验证 Redis 错误归一化规则。
func TestWrapRedisError(t *testing.T) {
	require.NoError(t, WrapRedisError(nil))
	require.ErrorIs(t, WrapRedisError(redis.Nil), ErrRedisNil)

	err := WrapRedisError(errors.New("timeout"))
	require.ErrorIs(t, err, ErrRedis)
	require.Contains(t, err.Error(), "timeout")
}

// TestLogRedisErrorNil 验证 nil 错误不会触发日志路径或 panic。
func TestLogRedisErrorNil(t *testing.T) {
	require.NotPanics(t, func() {
		LogRedisError(context.Background(), nil)
	})
}
