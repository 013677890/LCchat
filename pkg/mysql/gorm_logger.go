package mysql

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	applogger "github.com/013677890/LCchat-Backend/pkg/logger"

	"go.uber.org/zap"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const defaultSlowQueryThreshold = 200 * time.Millisecond

type zapGormLogger struct {
	log                       *zap.Logger
	level                     gormlogger.LogLevel
	slowThreshold             time.Duration
	ignoreRecordNotFoundError bool
}

func newGormLogger(log *zap.Logger, level string) gormlogger.Interface {
	if log == nil {
		log = zap.NewNop()
	}
	return &zapGormLogger{
		log:                       log.Named("gorm"),
		level:                     parseLogLevel(level),
		slowThreshold:             defaultSlowQueryThreshold,
		ignoreRecordNotFoundError: true,
	}
}

func (l *zapGormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	clone := *l
	clone.level = level
	return &clone
}

func (l *zapGormLogger) Info(ctx context.Context, message string, data ...interface{}) {
	if l.level < gormlogger.Info {
		return
	}
	applogger.WithContext(ctx, l.log).Info("GORM 信息", zap.String("detail", fmt.Sprintf(message, data...)))
}

func (l *zapGormLogger) Warn(ctx context.Context, message string, data ...interface{}) {
	if l.level < gormlogger.Warn {
		return
	}
	applogger.WithContext(ctx, l.log).Warn("GORM 警告", zap.String("detail", fmt.Sprintf(message, data...)))
}

func (l *zapGormLogger) Error(ctx context.Context, message string, data ...interface{}) {
	if l.level < gormlogger.Error {
		return
	}
	applogger.WithContext(ctx, l.log).Error("GORM 错误", zap.String("detail", fmt.Sprintf(message, data...)))
}

func (l *zapGormLogger) Trace(ctx context.Context, begin time.Time, query func() (string, int64), err error) {
	if l.level == gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	log := applogger.WithContext(ctx, l.log)

	switch {
	case err != nil && l.level >= gormlogger.Error &&
		(!errors.Is(err, gorm.ErrRecordNotFound) || !l.ignoreRecordNotFoundError):
		log.Error("MySQL 执行失败", append(traceFields(query, elapsed), zap.Error(err))...)
	case elapsed > l.slowThreshold && l.level >= gormlogger.Warn:
		log.Warn("MySQL 慢查询", traceFields(query, elapsed)...)
	case l.level == gormlogger.Info:
		log.Info("MySQL 执行完成", traceFields(query, elapsed)...)
	}
}

func traceFields(query func() (string, int64), elapsed time.Duration) []zap.Field {
	sql, rows := query()
	return []zap.Field{
		zap.Float64("elapsed_ms", float64(elapsed.Nanoseconds())/float64(time.Millisecond)),
		zap.Int64("rows", rows),
		zap.String("sql", sql),
	}
}

// ParamsFilter 保留 SQL 占位符，避免把密码、令牌等参数写入日志。
func (l *zapGormLogger) ParamsFilter(_ context.Context, sql string, _ ...interface{}) (string, []interface{}) {
	return sql, nil
}

// parseLogLevel 将字符串解析为 GORM 日志级别，默认 warn。
func parseLogLevel(level string) gormlogger.LogLevel {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "silent":
		return gormlogger.Silent
	case "error":
		return gormlogger.Error
	case "info":
		return gormlogger.Info
	default:
		return gormlogger.Warn
	}
}
