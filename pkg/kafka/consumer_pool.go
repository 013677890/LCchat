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
//
// Workers 表示「本进程加入 Consumer Group 的 Reader 个数」，不是 partition 绑定列表。
// 建议默认等于 topic partition 数（本仓库业务 topic 创建时固定为 3）；
// 大于 partition 数，或因多副本 rebalance 导致某些 Reader 分到 0 个 partition 时，
// 多余 Reader 会空闲阻塞在 Fetch 上等待，这是正常 idle，不是错误。
const (
	DefaultPoolWorkers = 3
	MinPoolWorkers     = 1
	MaxPoolWorkers     = 64
)

// ManualConsumerPoolConfig 描述一组共享同一 consumer group 的手动提交 Reader。
//
// 并行模型：
//   - 每个 worker 持有独立 kafka.Reader 与独立 Fetch/Handle/Commit 循环；
//   - 同一 GroupID 下由 Kafka rebalance 分配 partition（应用不配置分区号）；
//   - 不同 partition 可并行；同 partition 在单个 Reader 内严格串行。
//
// 严禁：
//   - 多个 goroutine 共享同一个 Reader 并发 Fetch；
//   - 单 Reader Fetch 后把消息丢进无序 worker 池（会破坏会话/群 key 有序性）。
type ManualConsumerPoolConfig struct {
	// Name 用于日志与隔离指标标签（例如 group-cache-projector / message-push-msg.push）。
	Name string

	Brokers []string
	Topic   string
	// GroupID 必须与「同一业务消费者」一致；不同服务/职责禁止共用 group 抢消息。
	GroupID string

	// Workers 独立 Reader 数量，必须在 [MinPoolWorkers, MaxPoolWorkers]。
	// 它只表达本进程愿意提供的并行消费能力；实际分到几个 partition 由 Kafka 决定。
	Workers int

	// Config 应用到每一个 worker 的 Reader/重试/死信参数（各 Reader 独立副本语义）。
	Config ManualConsumerConfig
}

// ManualConsumerPool 同一 topic + 同一 consumer group 下的多 Reader 消费池。
// 它是服务侧 Kafka 消费的统一编排入口：构造 N 个 Consumer，Start 时开 N 个协程。
type ManualConsumerPool struct {
	name    string
	topic   string
	groupID string
	workers []*Consumer

	closeOnce sync.Once
	closeErr  error
}

// ParsePoolWorkers 解析 worker 并发配置字符串。
//
// 规则：
//   - 未配置（空串或纯空白）→ DefaultPoolWorkers（3）
//   - 显式配置必须是 [1, 64] 的正整数
//   - 零、负数、非法文本直接返回错误，禁止静默回退到 1 或 3
//
// 调用方应在服务初始化阶段调用；解析失败必须中止启动，避免带着错误容量上线。
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
// 构造阶段只校验参数并分配 Reader，不探测 Kafka 连通性；连通与 rebalance 在 Start 后发生。
// workers 非法、brokers/topic/group 为空时直接返回 error，禁止半初始化对象。
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
		workers = append(workers, newManualCommitConsumer(cfg.Brokers, cfg.Topic, cfg.GroupID, cfg.Config))
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

// WorkerCount 返回配置的独立 Reader/Consumer 数量。
// 注意：这是配置值，不等于当前 rebalance 实际分到的 partition 数。
func (p *ManualConsumerPool) WorkerCount() int {
	if p == nil {
		return 0
	}
	return len(p.workers)
}

// Workers 返回底层 Consumer 切片的浅拷贝，仅供同包测试或诊断使用。
// 调用方不得把同一 Reader 再交给其它 goroutine 并发 Fetch。
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
// 生命周期语义：
//  1. 为本次运行创建可取消的 runCtx；任一 worker 非 context.Canceled 致命退出时 cancel 兄弟；
//  2. 等待全部 worker 结束（包括被 cancel 的），再返回首个致命错误；
//  3. 不允许某个 worker 悄悄退出后其它 worker 以降级并发继续跑（半残 Pool）。
//
// 错误返回给上层后的进程策略不在本方法内决定：
//   - message-push 主业是消费：上层应使进程非零退出；
//   - auth/user 等 API 服务：上层应使用 RunIsolatedPool 隔离故障，不拖死 gRPC。
//
// handler 在多个 worker 间共享调用，必须可并发安全（或仅依赖线程安全依赖）。
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
				logger.String("pool_name", p.name),
				logger.Int("worker_index", index),
				logger.String("topic", p.topic),
				logger.String("group_id", p.groupID),
				logger.Int("worker_count", len(p.workers)),
			)

			err := c.Start(runCtx, handler)
			if err == nil && runCtx.Err() == nil {
				// Consumer 正常实现只会在 ctx 取消或致命错误时返回；无原因结束同样
				// 表示该 worker 已停止消费，必须按致命错误收敛整个 Pool。
				err = errors.New("kafka consumer worker 意外结束")
			}

			reason := "context_canceled"
			if err == nil {
				reason = "completed"
			} else if !errors.Is(err, context.Canceled) {
				reason = "fatal_error"

				// 致命错误：立刻取消兄弟 worker，避免以降级并发继续跑。
				cancel()
			}

			logExit := logger.Info
			if reason == "fatal_error" {
				logExit = logger.Error
			}
			logExit(runCtx, "kafka consumer pool worker 退出",
				logger.String("pool_name", p.name),
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
				"kafka consumer pool worker %d 致命退出 (pool=%s topic=%s group=%s): %w",
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
// 允许在 Start 未调用、Start 已返回或关停竞态下安全调用。
// 注意：Close 后同一 Pool 实例上再次 Start，可能因 Reader 已关闭而持续致命失败；
// 需要长期自愈时应重建 Pool，或确保只在 ctx 取消路径上停止监督循环。
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
					"关闭 kafka consumer pool worker %d 失败 (pool=%s): %w",
					i, p.name, err,
				))
			}
		}
		p.closeErr = errors.Join(errs...)
	})
	return p.closeErr
}
