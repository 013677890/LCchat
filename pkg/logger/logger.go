package logger

import (
	"context"
	"os"
	"strings"
	"time"

	"ChatServer/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var global *zap.Logger

// L 返回全局 logger（未初始化时为 nil）。
// 使用场景：在包内无需显式传递 logger 时，直接 logger.L().Info(...)
func L() *zap.Logger {
	return global
}

// ReplaceGlobal 设置全局 logger，并同步 zap 的全局实例。
// 说明：zap.L()/zap.S() 会被替换，便于全局使用；需在进程启动时调用一次。
func ReplaceGlobal(l *zap.Logger) {
	global = l
	zap.ReplaceGlobals(l)
}

// Build 根据配置构建 zap Logger。
// - 默认输出 stdout/stderr（容器场景方便 docker logs）。
// - 可通过 OutputPaths/ErrorOutputPaths 写入文件（无滚动，滚动由外部系统负责）。
// - 自动根据 Level 解析日志级别，配置错误时回退到 info。
func Build(cfg config.LoggerConfig) (*zap.Logger, error) {
	level := zap.NewAtomicLevel()
	if err := level.UnmarshalText([]byte(strings.ToLower(cfg.Level))); err != nil {
		// 回退到 info，避免配置错误导致崩溃
		level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	encoderCfg := zapcore.EncoderConfig{
		TimeKey:        "ts",                                          // 时间戳
		LevelKey:       "level",                                       // 日志级别
		NameKey:        "logger",                                      // 日志名称
		CallerKey:      "caller",                                      // 调用者
		MessageKey:     "msg",                                         // 消息
		StacktraceKey:  "stack",                                       // 堆栈
		LineEnding:     zapcore.DefaultLineEnding,                     // 行结束符
		EncodeTime:     zapcore.TimeEncoderOfLayout(time.RFC3339Nano), // 统一时间格式
		EncodeDuration: zapcore.MillisDurationEncoder,                 // 耗时以毫秒输出
		EncodeCaller:   zapcore.ShortCallerEncoder,                    // 文件:行 短路径
	}
	// 根据 Encoding 配置选择编码器
	var encoder zapcore.Encoder
	if strings.ToLower(cfg.Encoding) == "console" {
		if cfg.EnableColor {
			encoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder // 彩色等级
		} else {
			encoderCfg.EncodeLevel = zapcore.CapitalLevelEncoder // 普通等级
		}
		encoder = zapcore.NewConsoleEncoder(encoderCfg) // 控制台编码器
	} else {
		encoderCfg.EncodeLevel = zapcore.LowercaseLevelEncoder // 小写等级
		encoder = zapcore.NewJSONEncoder(encoderCfg)           // JSON编码器
	}

	outSync := buildSyncer(cfg.OutputPaths, zapcore.AddSync(os.Stdout))      // 普通日志输出
	errSync := buildSyncer(cfg.ErrorOutputPaths, zapcore.AddSync(os.Stderr)) // 错误日志输出

	core := zapcore.NewCore(encoder, outSync, level)
	opts := []zap.Option{
		zap.ErrorOutput(errSync),
		zap.AddCaller(),
	}
	if cfg.Development {
		opts = append(opts, zap.Development(), zap.AddStacktrace(zapcore.ErrorLevel))
	}

	return zap.New(core, opts...), nil
}

// buildSyncer 根据配置构建 WriteSyncer：
// - 支持 stdout/stderr 关键字。
// - 支持直接写文件（无滚动），打开失败则回退到 fallback。
func buildSyncer(paths []string, fallback zapcore.WriteSyncer) zapcore.WriteSyncer {
	if len(paths) == 0 {
		return fallback
	}
	var syncers []zapcore.WriteSyncer
	for _, p := range paths {
		switch strings.ToLower(p) {
		case "stdout":
			syncers = append(syncers, zapcore.AddSync(os.Stdout))
		case "stderr":
			syncers = append(syncers, zapcore.AddSync(os.Stderr))
		default:
			// 写入指定文件（无轮转），创建失败时忽略该路径
			f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err == nil {
				syncers = append(syncers, zapcore.AddSync(f))
			}
		}
	}
	if len(syncers) == 0 {
		return fallback
	}
	return zapcore.NewMultiWriteSyncer(syncers...)
}

func Info(ctx context.Context, msg string, fields ...zap.Field) {
	if ctx == nil {
		global.Info(msg, fields...)
	} else {
		traceId := ctx.Value("trace_id") //这里获取trace_id, 从gin的上下文中获取
		if traceId != nil {
			fields = append(fields, zap.String("trace_id", traceId.(string)))
		}
		global.Info(msg, fields...)
	}
}

func Warn(ctx context.Context, msg string, fields ...zap.Field) {
	if ctx == nil {
		global.Warn(msg, fields...)
	} else {
		traceId := ctx.Value("trace_id")
		if traceId != nil {
			fields = append(fields, zap.String("trace_id", traceId.(string)))
		}
		global.Warn(msg, fields...)
	}
}

func Error(ctx context.Context, msg string, fields ...zap.Field) {
	if ctx == nil {
		global.Error(msg, fields...)
	} else {
		traceId := ctx.Value("trace_id")
		if traceId != nil {
			fields = append(fields, zap.String("trace_id", traceId.(string)))
		}
		global.Error(msg, fields...)
	}
}

func Fatal(ctx context.Context, msg string, fields ...zap.Field) {
	if ctx == nil {
		global.Fatal(msg, fields...)
	} else {
		traceId := ctx.Value("trace_id")
		if traceId != nil {
			fields = append(fields, zap.String("trace_id", traceId.(string)))
		}
		global.Fatal(msg, fields...)
	}
}

func Debug(ctx context.Context, msg string, fields ...zap.Field) {
	if ctx == nil {
		global.Debug(msg, fields...)
	} else {
		traceId := ctx.Value("trace_id")
		if traceId != nil {
			fields = append(fields, zap.String("trace_id", traceId.(string)))
		}
		global.Debug(msg, fields...)
	}
}

// ========== Field 辅助函数封装 ==========
// 以下函数用于创建日志字段，业务代码无需直接导入 zap 包

// String 创建字符串类型字段
func String(key, value string) zap.Field {
	return zap.String(key, value)
}

// Int 创建整数类型字段
func Int(key string, value int) zap.Field {
	return zap.Int(key, value)
}

// Int64 创建 int64 类型字段
func Int64(key string, value int64) zap.Field {
	return zap.Int64(key, value)
}

// Bool 创建布尔类型字段
func Bool(key string, value bool) zap.Field {
	return zap.Bool(key, value)
}

// ErrorField 创建错误类型字段
func ErrorField(key string, err error) zap.Field {
	return zap.Error(err)
}

// Any 创建任意类型字段
func Any(key string, value interface{}) zap.Field {
	return zap.Any(key, value)
}

// Duration 创建时间间隔类型字段
func Duration(key string, value time.Duration) zap.Field {
	return zap.Duration(key, value)
}

// Time 创建时间类型字段
func Time(key string, value time.Time) zap.Field {
	return zap.Time(key, value)
}
