package redisretry

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

type compensationContextKey struct{}

// WriteFailureHook 在 go-redis 完成内置短重试后，将最终写失败转换成整键 DEL 补偿任务。
// DEL 是幂等操作，且不会用可能过期的局部数据覆盖 MySQL 事实。
type WriteFailureHook struct {
	source   string
	reporter func(context.Context, RedisTask, error)
}

// NewWriteFailureHook 创建 Redis 写失败补偿 Hook。
func NewWriteFailureHook(source string) *WriteFailureHook {
	return newWriteFailureHook(source, LogAndRetryRedisError)
}

func newWriteFailureHook(
	source string,
	reporter func(context.Context, RedisTask, error),
) *WriteFailureHook {
	return &WriteFailureHook{source: source, reporter: reporter}
}

func (h *WriteFailureHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h *WriteFailureHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		err := next(ctx, cmd)
		if shouldReportWriteFailure(ctx, err) {
			h.report(ctx, cmd.Name(), writeKeys(cmd), err)
		}
		return err
	}
}

func (h *WriteFailureHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		err := next(ctx, cmds)
		if isCompensationContext(ctx) || err == nil || errors.Is(err, redis.Nil) {
			return err
		}

		keys := make([]string, 0)
		commandErrorSeen := false
		for _, cmd := range cmds {
			cmdErr := cmd.Err()
			if cmdErr == nil {
				continue
			}
			commandErrorSeen = true
			if !shouldReportWriteFailure(ctx, cmdErr) {
				continue
			}
			keys = append(keys, writeKeys(cmd)...)
		}
		// 连接级 Pipeline 错误理论上会写入每个命令；保守兜底，避免版本差异造成漏补偿。
		if len(keys) == 0 && !commandErrorSeen {
			for _, cmd := range cmds {
				keys = append(keys, writeKeys(cmd)...)
			}
		}
		h.report(ctx, "pipeline", keys, err)
		return err
	}
}

func (h *WriteFailureHook) report(ctx context.Context, command string, keys []string, err error) {
	keys = uniqueNonEmpty(keys)
	if len(keys) == 0 || h == nil || h.reporter == nil {
		return
	}
	source := strings.TrimSpace(h.source)
	if source == "" {
		source = "redis"
	}
	task := BuildDelTask(keys...).WithSource(source + "." + command)
	h.reporter(ctx, task, err)
}

func shouldReportWriteFailure(ctx context.Context, err error) bool {
	return err != nil &&
		!errors.Is(err, redis.Nil) &&
		!redis.HasErrorPrefix(err, "NOSCRIPT") &&
		!isCompensationContext(ctx)
}

func withCompensationContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, compensationContextKey{}, true)
}

func isCompensationContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	marked, _ := ctx.Value(compensationContextKey{}).(bool)
	return marked
}

func writeKeys(cmd redis.Cmder) []string {
	if cmd == nil {
		return nil
	}
	args := cmd.Args()
	if len(args) < 2 {
		return nil
	}

	switch strings.ToLower(cmd.Name()) {
	case "del":
		return stringArgs(args[1:])
	case "eval", "evalsha":
		if len(args) < 4 {
			return nil
		}
		keyCount, err := strconv.Atoi(fmt.Sprint(args[2]))
		if err != nil || keyCount <= 0 || 3+keyCount > len(args) {
			return nil
		}
		return stringArgs(args[3 : 3+keyCount])
	case "set", "incr", "expire", "hset", "zadd":
		return []string{fmt.Sprint(args[1])}
	default:
		return nil
	}
}

func stringArgs(args []interface{}) []string {
	keys := make([]string, 0, len(args))
	for _, arg := range args {
		keys = append(keys, fmt.Sprint(arg))
	}
	return keys
}

func uniqueNonEmpty(keys []string) []string {
	seen := make(map[string]struct{}, len(keys))
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	return result
}
