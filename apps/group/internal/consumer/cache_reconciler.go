package consumer

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/pkg/logger"
)

// maxCacheReconcileErrorSamples 限制单轮保留的逐群错误样本。
//
// Redis 全局故障时可能每个群都失败；若把全表规模的 error 链全部驻留到扫描结束，
// 大群量环境会产生没有上限的内存占用。继续扫描能避免坏群阻塞游标，但诊断只需要
// 有界样本和遗漏计数。
const (
	maxCacheReconcileErrorSamples = 20

	// cacheReconcileJitterDivisor 把基准周期的五分之一作为双向抖动窗口，即 ±20%。
	//
	// 对账会扫描全部群并读取 MySQL、重建 Redis。如果所有 group-service 实例都按
	// 完全相同的固定周期启动，重启或发布后很容易长期同频，周期性放大数据库与缓存
	// 压力。20% 在 6 小时默认基准下会把下一轮均匀分散到 4 小时 48 分至
	// 7 小时 12 分之间，同时仍能把漂移修复时延稳定约束在小时级。
	cacheReconcileJitterDivisor = 5
)

// CacheReconcilerConfig 控制 group 缓存周期对账的扫描节奏。
type CacheReconcilerConfig struct {
	Interval  time.Duration
	BatchSize int
}

// CacheReconciler 周期扫描 groups 表，并按群从 MySQL 权威快照修复 Redis。
//
// Kafka projector 负责低延迟增量投影；reconciler 负责发现并修复以下长期漂移：
//   - 某条事件被人工错误处置或历史上未投影；
//   - Redis key 被驱逐、误删或结构污染；
//   - 用户群反向索引缺少 add/remove tombstone。
//
// 两条路径共享同一个 cache_version + Lua 栅栏，因此对账不是第二个无序写者：
// 旧 DB 快照只会被拒绝，永远不能覆盖已经投影的更高版本。
type CacheReconciler struct {
	repo      repository.IGroupCacheProjectorRepository
	interval  time.Duration
	batchSize int
}

func NewCacheReconciler(
	repo repository.IGroupCacheProjectorRepository,
	cfg CacheReconcilerConfig,
) (*CacheReconciler, error) {
	if repo == nil {
		return nil, errors.New("group cache reconciler repository 未初始化")
	}
	if cfg.Interval <= 0 {
		return nil, fmt.Errorf("group cache reconcile interval 必须大于 0")
	}
	if cfg.BatchSize <= 0 {
		return nil, fmt.Errorf("group cache reconcile batch size 必须大于 0")
	}
	return &CacheReconciler{repo: repo, interval: cfg.Interval, batchSize: cfg.BatchSize}, nil
}

// Start 立即执行首轮对账，随后按“基准间隔 ±20% 抖动”运行，直到进程 context 取消。
//
// 单轮失败只记录并等待下一轮，不终止 group-service；缓存修复是后台增强能力，
// 业务读路径仍可回源 MySQL。context 取消则立即结束，不启动新批次。
//
// 下一次 timer 在本轮完成后才创建，而不是使用固定 ticker。这样即使一次全表扫描
// 超过配置周期，也不会积压 tick 或让多个对账轮次重叠；配置的 Interval 表示两轮
// 对账之间的基准等待时间，而不是墙上时钟的固定触发频率。
func (r *CacheReconciler) Start(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.runAndReport(ctx)
	for {
		timer := time.NewTimer(nextCacheReconcileDelay(r.interval))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			r.runAndReport(ctx)
		}
	}
}

// nextCacheReconcileDelay 生成下一轮对账等待时间。
//
// rand/v2 的包级生成器可安全并发使用；每轮重新取样，避免进程启动后形成固定相位。
// 随机偏移包含上下界，并委托给 cacheReconcileDelay 完成夹紧和映射，便于用确定输入
// 精确测试下界、基准值与上界。
func nextCacheReconcileDelay(base time.Duration) time.Duration {
	if base <= 0 {
		return base
	}
	jitterWindow := base / cacheReconcileJitterDivisor
	randomOffset := time.Duration(rand.Int64N(int64(2*jitterWindow) + 1))
	return cacheReconcileDelay(base, randomOffset)
}

// cacheReconcileDelay 把随机偏移映射到 [base*80%, base*120%]。
//
// 构造器已禁止非正 base；这里仍对非正值原样返回，使该纯函数在独立测试或未来复用
// 时不会制造负抖动窗口。offset 被夹紧到合法范围，防止未来替换随机源时越界。
func cacheReconcileDelay(base, offset time.Duration) time.Duration {
	if base <= 0 {
		return base
	}
	jitterWindow := base / cacheReconcileJitterDivisor
	if offset < 0 {
		offset = 0
	} else if offset > 2*jitterWindow {
		offset = 2 * jitterWindow
	}
	return base - jitterWindow + offset
}

func (r *CacheReconciler) runAndReport(ctx context.Context) {
	startedAt := time.Now()
	if err := r.RunOnce(ctx); err != nil {
		if ctx.Err() == nil {
			logger.Warn(ctx, "Group cache reconcile 单轮存在失败",
				logger.ErrorField("error", err),
				logger.Int64("elapsed_ms", time.Since(startedAt).Milliseconds()),
			)
		}
		return
	}
	logger.Info(ctx, "Group cache reconcile 单轮完成",
		logger.Int64("elapsed_ms", time.Since(startedAt).Milliseconds()),
	)
}

// RunOnce 用 ID keyset 游标完成一轮全表扫描。
//
// 某个群修复失败不会阻断后续群，函数在扫描结束后用 errors.Join 汇总有界错误样本；
// 这既避免一个脏群形成永久队头阻塞，也防止全局 Redis 故障时按群数无限积累 error。
func (r *CacheReconciler) RunOnce(ctx context.Context) error {
	if r == nil || r.repo == nil {
		return nil
	}
	var (
		afterID      int64
		errs         []error
		failureCount int
	)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		targets, err := r.repo.ListGroupCacheReconcileTargets(ctx, afterID, r.batchSize)
		if err != nil {
			return fmt.Errorf("扫描 group cache reconcile targets 失败: %w", err)
		}
		if len(targets) == 0 {
			break
		}
		for _, target := range targets {
			if target.ID <= afterID || target.GroupUUID == "" {
				return fmt.Errorf(
					"group cache reconcile 游标未前进: after_id=%d target_id=%d group_uuid=%q",
					afterID,
					target.ID,
					target.GroupUUID,
				)
			}
			afterID = target.ID
			if err := r.repo.ReconcileGroupCache(ctx, target.GroupUUID); err != nil {
				failureCount++
				if len(errs) < maxCacheReconcileErrorSamples {
					errs = append(errs, fmt.Errorf("reconcile group %s: %w", target.GroupUUID, err))
				}
			}
		}
		if len(targets) < r.batchSize {
			break
		}
	}
	if omitted := failureCount - len(errs); omitted > 0 {
		errs = append(errs, fmt.Errorf(
			"%d additional group cache reconcile errors omitted after %d samples",
			omitted,
			maxCacheReconcileErrorSamples,
		))
	}
	return errors.Join(errs...)
}
