package repository

import (
	"context"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/redisretry"
)

// runRedisCacheTask 保持缓存更新异步执行，并在协程池拒绝任务时投递整键 DEL 补偿。
// 任务已开始后的 Redis 最终写失败由 go-redis WriteFailureHook 统一处理。
func runRedisCacheTask(
	ctx context.Context,
	source string,
	keys []string,
	timeout time.Duration,
	task func(context.Context),
) {
	if err := async.TryRunSafe(ctx, task, timeout); err != nil {
		retryTask := redisretry.BuildDelTask(keys...).WithSource(source + ".async-submit")
		redisretry.LogAndRetryRedisError(ctx, retryTask, err)
	}
}
