package kafka

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/013677890/LCchat-Backend/pkg/logger"
)

// 分区级并行消费默认与边界。
// worker 数建议等于 topic partition 数；大于 partition 时多余 worker 只会空闲。
const (
	DefaultPoolWorkers = 3
	MinPoolWorkers     = 1
	MaxPoolWorkers     = 64
)

// ManualConsumerPoolConfig 描述一组共享同一 consumer group 的手动提交 Reader。
//
// 每个 worker 持有独立 kafka.Reader 与独立 Fetch/Handle/Commit 循环；
// 同一 group ID 下由 Kafka 分配不同 partition，从而实现「不同 partition 并行、同 partition 串行」。
// 严禁多个 goroutine 共享同一个 Reader，也严禁 Fetch 后丢进无序 worker pool。
type ManualConsumerPoolConfig struct {
	// Name 用于日志（例如 group-cache-projector / msg-group-membership-projector）。
	Name string

	Brokers []string
	Topic   string
	GroupID string

	// Workers 独立 Reader 数量。必须在 [MinPoolWorkers, MaxPoolWorkers]。
	Workers int

	// Config 应用到每一个 worker 的 Reader/重试/死信参数。
	Config ManualConsumerConfig
}

// ManualConsumerPool 同一 topic + 同一 consumer group 下的多 Reader 消费池。
type ManualConsumerPool struct {
	name    string
	topic   string
	groupID string
	workers []*Consumer

	closeOnce sync.Once
	closeErr  error
}

// ParsePoolWorkers 解析 worker 并发配置。
//
// 规则：
//   - 未配置（空串）→ DefaultPoolWorkers（3）
//   - 显式配置必须是 [1, 64] 的正整数
//   - 零、负数、非法文本直接返回错误，禁止静默回退到 1 或 3
func ParsePoolWorkers(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultPoolWorkers, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("kafka consumer pool workers 必须是正整数: %q", raw)
	}
	if n < MinPoolWorkers || n > MaxPoolWorkers {
		return 0, fmt.Errorf(
			"kafka consumer pool workers 必须在 %d～%d: got %d",
			MinPoolWorkers, MaxPoolWorkers, n,
		)
	}
	return n, nil
}

// NewManualConsumerPool 创建 N 个独立手动提交 Consumer（每个一个 Reader）。
func NewManualConsumerPool(cfg ManualConsumerPoolConfig) (*ManualConsumerPool, error) {
	if strings.TrimSpace(cfg.Name) == "" {
		return nil, errors.New("kafka consumer pool name 不能为空")
	}
	if len(cfg.Brokers) == 0 {
		return nil, errors.New("kafka consumer pool brokers 不能为空")
	}
	if strings.TrimSpace(cfg.Topic) == "" {
		return nil, errors.New("kafka consumer pool topic 不能为空")
	}
	if strings.TrimSpace(cfg.GroupID) == "" {
		return nil, errors.New("kafka consumer pool group id 不能为空")
	}
	if cfg.Workers < MinPoolWorkers || cfg.Workers > MaxPoolWorkers {
		return nil, fmt.Errorf(
			"kafka consumer pool workers 必须在 %d～%d: got %d",
			MinPoolWorkers, MaxPoolWorkers, cfg.Workers,
		)
	}

	workers := make([]*Consumer, 0, cfg.Workers)
	for i := 0; i < cfg.Workers; i++ {
		workers = append(workers, NewManualCommitConsumer(cfg.Brokers, cfg.Topic, cfg.GroupID, cfg.Config))
	}
	return &ManualConsumerPool{
		name:    cfg.Name,
		topic:   cfg.Topic,
		groupID: cfg.GroupID,
		workers: workers,
	}, nil
}

// newManualConsumerPoolForTest 用预构造的 Consumer 组装池，仅供同包测试注入假 Reader。
func newManualConsumerPoolForTest(name, topic, groupID string, workers []*Consumer) (*ManualConsumerPool, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("kafka consumer pool name 不能为空")
	}
	if len(workers) == 0 {
		return nil, errors.New("kafka consumer pool workers 不能为空")
	}
	for i, w := range workers {
		if w == nil {
			return nil, fmt.Errorf("kafka consumer pool worker %d 不能为 nil", i)
		}
	}
	return &ManualConsumerPool{
		name:    name,
		topic:   topic,
		groupID: groupID,
		workers: workers,
	}, nil
}

// WorkerCount 返回独立 Reader/Consumer 数量。
func (p *ManualConsumerPool) WorkerCount() int {
	if p == nil {
		return 0
	}
	return len(p.workers)
}

// Workers 返回底层 Consumer 切片（只读用途；调用方不得共享同一 Reader 再开并发 Fetch）。
func (p *ManualConsumerPool) Workers() []*Consumer {
	if p == nil {
		return nil
	}
	out := make([]*Consumer, len(p.workers))
	copy(out, p.workers)
	return out
}

// Start 同时启动全部 worker：每个 worker 独立串行 Fetch→Handle→Commit。
//
// 任一 worker 返回非 context.Canceled 的致命错误时：
//  1. 取消其余 worker；
//  2. 等待全部收敛；
//  3. 返回首个致命错误。
// 不允许某个 worker 悄悄退出后继续以降级并发运行。
func (p *ManualConsumerPool) Start(ctx context.Context, handler MessageHandler) error {
	if p == nil || len(p.workers) == 0 {
		return errors.New("kafka consumer pool 未初始化")
	}
	if handler == nil {
		return errors.New("kafka consumer pool handler 不能为空")
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type workerResult struct {
		index int
		err   error
	}
	resultCh := make(chan workerResult, len(p.workers))
	var wg sync.WaitGroup

	for i, worker := range p.workers {
		wg.Add(1)
		go func(index int, c *Consumer) {
			defer wg.Done()
			logger.Info(runCtx, "kafka consumer pool worker 启动",
				logger.String("projector", p.name),
				logger.Int("worker_index", index),
				logger.String("topic", p.topic),
				logger.String("group_id", p.groupID),
				logger.Int("worker_count", len(p.workers)),
			)

			err := c.Start(runCtx, handler)

			reason := "context_canceled"
			if err == nil {
				reason = "completed"
			} else if !errors.Is(err, context.Canceled) {
				reason = "fatal_error"

				// 致命错误：立刻取消兄弟 worker，避免以降级并发继续跑。
				cancel()
			}

			logger.Info(runCtx, "kafka consumer pool worker 退出",
				logger.String("projector", p.name),
				logger.Int("worker_index", index),
				logger.String("topic", p.topic),
				logger.String("group_id", p.groupID),
				logger.String("reason", reason),
				logger.ErrorField("error", err),
			)
			resultCh <- workerResult{index: index, err: err}
		}(i, worker)
	}

	wg.Wait()
	close(resultCh)

	var firstFatal error
	canceled := false
	for res := range resultCh {
		if res.err == nil {
			continue
		}
		if errors.Is(res.err, context.Canceled) {
			canceled = true
			continue
		}
		if firstFatal == nil {
			firstFatal = fmt.Errorf(
				"kafka consumer pool worker %d 致命退出 (projector=%s topic=%s group=%s): %w",
				res.index, p.name, p.topic, p.groupID, res.err,
			)
		}
	}

	if firstFatal != nil {
		return firstFatal
	}

	// 父 context 取消或因致命错误触发的内部 cancel：若无致命错误则透传取消语义。
	if canceled {
		if err := ctx.Err(); err != nil {
			return err
		}
		return context.Canceled
	}

	return nil
}

// Close 关闭全部 Reader；每个 Reader 只关闭一次，并聚合关闭错误。
// 允许在部分初始化或 Start 未调用时安全调用。
func (p *ManualConsumerPool) Close() error {
	if p == nil {
		return nil
	}
	p.closeOnce.Do(func() {
		var errs []error
		for i, w := range p.workers {
			if w == nil {
				continue
			}
			if err := w.Close(); err != nil {
				errs = append(errs, fmt.Errorf(
					"关闭 kafka consumer pool worker %d 失败 (projector=%s): %w",
					i, p.name, err,
				))
			}
		}
		p.closeErr = errors.Join(errs...)
	})
	return p.closeErr
}
