package mysql

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestZapGormLoggerTrace(t *testing.T) {
	tests := []struct {
		name        string
		level       string
		elapsed     time.Duration
		err         error
		wantLevel   zapcore.Level
		wantMessage string
		wantCount   int
	}{
		{
			name:        "执行失败映射为错误日志",
			level:       "warn",
			elapsed:     10 * time.Millisecond,
			err:         errors.New("数据库不可用"),
			wantLevel:   zap.ErrorLevel,
			wantMessage: "MySQL 执行失败",
			wantCount:   1,
		},
		{
			name:        "慢查询映射为警告日志",
			level:       "warn",
			elapsed:     300 * time.Millisecond,
			wantLevel:   zap.WarnLevel,
			wantMessage: "MySQL 慢查询",
			wantCount:   1,
		},
		{
			name:        "info记录普通查询",
			level:       "info",
			elapsed:     10 * time.Millisecond,
			wantLevel:   zap.InfoLevel,
			wantMessage: "MySQL 执行完成",
			wantCount:   1,
		},
		{
			name:      "warn忽略普通查询",
			level:     "warn",
			elapsed:   10 * time.Millisecond,
			wantCount: 0,
		},
		{
			name:      "warn忽略记录不存在",
			level:     "warn",
			elapsed:   10 * time.Millisecond,
			err:       gorm.ErrRecordNotFound,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, observed := observer.New(zap.DebugLevel)
			log := newGormLogger(zap.New(core), tt.level)
			ctx := ctxmeta.WithTraceID(context.Background(), "trace-1")

			log.Trace(ctx, time.Now().Add(-tt.elapsed), func() (string, int64) {
				return "SELECT * FROM user_account WHERE uuid = ?", 2
			}, tt.err)

			entries := observed.All()
			require.Len(t, entries, tt.wantCount)
			if tt.wantCount == 0 {
				return
			}

			entry := entries[0]
			require.Equal(t, tt.wantLevel, entry.Level)
			require.Equal(t, tt.wantMessage, entry.Message)
			fields := entry.ContextMap()
			require.Equal(t, "trace-1", fields[ctxmeta.KeyTraceID])
			require.Equal(t, int64(2), fields["rows"])
			require.Equal(t, "SELECT * FROM user_account WHERE uuid = ?", fields["sql"])
			require.GreaterOrEqual(t, fields["elapsed_ms"].(float64), float64(tt.elapsed.Milliseconds()))
		})
	}
}

func TestZapGormLoggerParamsFilter(t *testing.T) {
	log := newGormLogger(zap.NewNop(), "warn")
	filter, ok := log.(gorm.ParamsFilter)
	require.True(t, ok)

	sql, params := filter.ParamsFilter(context.Background(), "SELECT * FROM users WHERE password = ?", "secret")
	require.Equal(t, "SELECT * FROM users WHERE password = ?", sql)
	require.Nil(t, params)
}

func TestZapGormLoggerLogModeReturnsIndependentLogger(t *testing.T) {
	core, observed := observer.New(zap.DebugLevel)
	base := newGormLogger(zap.New(core), "warn")
	info := base.LogMode(gormlogger.Info)

	query := func() (string, int64) { return "SELECT 1", 1 }
	base.Trace(context.Background(), time.Now(), query, nil)
	info.Trace(context.Background(), time.Now(), query, nil)

	require.Len(t, observed.All(), 1)
	require.Equal(t, "MySQL 执行完成", observed.All()[0].Message)
}

func TestZapGormLoggerDoesNotBuildUnusedSQL(t *testing.T) {
	log := newGormLogger(zap.NewNop(), "error")
	called := false

	log.Trace(context.Background(), time.Now(), func() (string, int64) {
		called = true
		return "SELECT 1", 1
	}, nil)

	require.False(t, called)
}
