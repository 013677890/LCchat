package kafka

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/logger"
	segmentkafka "github.com/segmentio/kafka-go"
)

// ManualConsumerConfig 定义手动提交型消费者的 Reader 与可靠性参数。
// 服务层通过 ManualConsumerPoolConfig.Config 注入；非法 worker 数在 Pool 构造阶段拒绝，
// 本结构只负责单 Reader 循环内的超时、退避与死信策略，不参与 partition 分配。
type ManualConsumerConfig struct {
	// MinBytes / MaxBytes / MaxWait 透传给 segmentio/kafka-go Reader。
	// MinBytes=1 优先降低端到端延迟；MaxWait 限制 broker 端空等，避免 IM 下行长尾。
	MinBytes     int
	MaxBytes     int
	MaxWait      time.Duration
	// ErrorBackoff 是 Fetch 临时失败、Commit 临时失败、业务可重试失败后的统一退避。
	// 该退避发生在 Reader 循环内，目的是吸收 broker 抖动，而不是把临时错误升级成进程退出。
	ErrorBackoff time.Duration
	// HandleTimeout 为单条消息的每次处理尝试施加超时预算。
	// 防止某条消息的下游（DB/Redis/gRPC）挂死导致 handle 永不返回，进而冻结整个分区。
	// 零值回退到 defaultHandleTimeout；显式负值表示关闭超时（仅当上层已自管预算时使用，
	// 例如 message-push 业务侧已有 30s 扇出超时）。
	HandleTimeout time.Duration

	// RetryBudget 为单条消息「原地重试」的墙钟预算：超出后判定为毒/持久失败消息，
	// 旁路到 DeadLetterSink 并提交 offset，从而解除队头阻塞。<=0 时回退到 defaultRetryBudget。
	// 仅在同时配置了 DeadLetterSink 时才生效；未配置死信时对可重试错误做无界原地重试
	// （分区会阻塞到恢复或 ctx 取消），永久错误则返回致命错误给 Pool。
	RetryBudget time.Duration

	// DeadLetterSink 为死信落地实现。
	// nil 时：瞬时错误原地重试；永久错误使 consumer 明确失败，禁止无落点地提交并静默跳过坏消息。
	// 非 nil 时：永久错误首轮即可 Park；可重试错误在预算耗尽后 Park；Park 成功才允许 commit。
	DeadLetterSink DeadLetterSink

	// ObserveFetch 观测每次 FetchMessage 的耗时；实现必须支持多个 worker 并发调用。
	// 该钩子只负责指标，不得改变 Fetch 错误的退避重试语义。
	ObserveFetch func(time.Duration)
	// ObserveCommitError 观测 offset 提交失败；实现必须支持多个 worker 并发调用。
	// Consumer 仍会停留在当前消息重试提交，钩子不得自行推进 offset。
	ObserveCommitError func()
}

// defaultHandleTimeout 是手动提交消费者单次处理尝试的默认超时。
// IM 消费者通常只做几次点查/点写，10s 给足余量又能把下游挂死收敛到秒级。
const defaultHandleTimeout = 10 * time.Second

// defaultRetryBudget 是单条消息原地重试的默认墙钟预算。
// 瞬时抖动通常秒级恢复，远不到该预算；毒/持久失败消息在此预算后被旁路到死信，
// 因此 worst-case 队头阻塞被钉死在该时长内。
const defaultRetryBudget = 2 * time.Minute

// defaults 补齐 Reader、处理超时与失败退避的默认参数。
// 调用方显式传入非法 worker 数由 Pool 构造阶段拒绝；这里不做静默并发回退。
func (c *ManualConsumerConfig) defaults() {
	if c.MinBytes <= 0 {
		c.MinBytes = 1
	}
	if c.MaxBytes <= 0 {
		c.MaxBytes = 10 << 20
	}
	if c.MaxWait <= 0 {
		c.MaxWait = 500 * time.Millisecond
	}
	if c.ErrorBackoff <= 0 {
		c.ErrorBackoff = 500 * time.Millisecond
	}
	if c.HandleTimeout == 0 {
		c.HandleTimeout = defaultHandleTimeout
	}
	if c.RetryBudget <= 0 {
		c.RetryBudget = defaultRetryBudget
	}
}

// Consumer 是单个 kafka.Reader 的串行消费循环实现。
// 它不负责多分区并行；并行由 ManualConsumerPool 创建 N 个独立 Consumer 完成。
// 同一时刻一个 Consumer 只处理一条消息：当前消息提交前不会 Fetch 下一条。
type Consumer struct {
	reader             messageReader
	errorBackoff       time.Duration
	handleTimeout      time.Duration
	retryBudget        time.Duration
	deadLetterSink     DeadLetterSink
	observeFetch       func(time.Duration)
	observeCommitError func()
}

// messageReader 抽出 Consumer 真正依赖的 kafka-go 最小接口。
//
// 生产环境仍使用 *kafka.Reader；接口的目的不是提供第二套实现，而是让“先死信、
// 后提交、提交失败不重复执行 handler”这些可靠性语义能够用确定性单测覆盖。
type messageReader interface {
	FetchMessage(ctx context.Context) (segmentkafka.Message, error)
	CommitMessages(ctx context.Context, msgs ...segmentkafka.Message) error
	Close() error
}

// newManualCommitConsumer 创建 Pool 内部使用的单 Reader 手动提交消费者。
// 故意不导出：服务层只能通过 NewManualConsumerPool 获得消费编排，禁止再出现
// 「单 Reader 公开入口 + Pool 入口」双路径。
//
// CommitInterval=0 表示同步 commit：当前消息处理/旁路完成并提交成功后，才允许
// 进入下一次 Fetch，保证同 partition 上的 offset 推进与处理顺序一致。
func newManualCommitConsumer(brokers []string, topic, groupID string, cfg ManualConsumerConfig) *Consumer {
	cfg.defaults()

	return &Consumer{
		reader: segmentkafka.NewReader(segmentkafka.ReaderConfig{
			Brokers:        brokers,
			Topic:          topic,
			GroupID:        groupID,
			MinBytes:       cfg.MinBytes,
			MaxBytes:       cfg.MaxBytes,
			MaxWait:        cfg.MaxWait,
			// 同步提交：异步 commit 会放大“已处理但 offset 未推进/推进过快”的窗口。
			CommitInterval: 0,
		}),
		errorBackoff:       cfg.ErrorBackoff,
		handleTimeout:      cfg.HandleTimeout,
		retryBudget:        cfg.RetryBudget,
		deadLetterSink:     cfg.DeadLetterSink,
		observeFetch:       cfg.ObserveFetch,
		observeCommitError: cfg.ObserveCommitError,
	}
}

// MessageHandler 处理单条 Kafka value。
// 返回约定：
//   - nil：本条可提交 offset；
//   - Permanent(err)：契约/解码等确定性失败，由死信或组件失败路径处理；
//   - 其它 error：可重试（DB/Redis/下游抖动），同一条消息原地重试，不得跳过。
// 禁止用 nil 表示“坏消息但想跳过”；跳过必须经死信或上层明确的 best-effort 适配层。
type MessageHandler func(ctx context.Context, message []byte) error

// Start 阻塞运行单个 Reader 的 Fetch→Handle→Commit 串行循环。
//
// 错误分层（调用方 ManualConsumerPool / RunIsolatedPool 依赖此约定）：
//  1. Kafka/broker 临时 Fetch 失败：ErrorBackoff 后继续，不返回、不退出进程；
//  2. 本 Reader 暂未分到 partition：kafka-go 在 Fetch 内阻塞等待 rebalance，属正常 idle；
//  3. ctx 取消：尽快返回 context 错误，供关停与 Pool cancel 使用；
//  4. Reader 已关闭（如 Close 竞态）：返回致命错误，由 Pool 取消兄弟 worker；
//  5. 业务处理不可恢复且无法旁路：返回致命错误。
//
// 本方法返回后，该 Reader 不再消费；Pool 会把非 Canceled 错误视为组件级致命失败。
func (c *Consumer) Start(ctx context.Context, handler MessageHandler) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		fetchStartedAt := time.Now()
		msg, err := c.reader.FetchMessage(ctx)
		if c.observeFetch != nil {
			c.observeFetch(time.Since(fetchStartedAt))
		}
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			// Reader 已关闭通常来自进程关停时 Close 与循环的竞态，或上层错误地重复 Close。
			// 继续退避无意义，必须作为致命错误让 Pool 收敛。
			if errors.Is(err, io.ErrClosedPipe) {
				return fmt.Errorf("kafka reader 已关闭，无法继续消费: %w", err)
			}

			// broker 抖动、断网和 rebalance 期间都可能暂时 Fetch 失败。
			// 统一在 Reader 循环内退避，避免 API 进程因 Kafka 短暂不可用而雪崩重启。
			// 注意：未分到 partition 时 kafka-go 会阻塞在 Fetch 内部等待分配，
			// 不会进入本分支，因此“分到 0 个 partition”不是错误。
			logger.Warn(ctx, "kafka 拉取消息失败，退避后继续等待",
				logger.ErrorField("error", err),
			)
			if err := c.waitBackoff(ctx); err != nil {
				return err
			}
			continue
		}

		// 成功 Fetch 后必须先处理完并提交，才进入下一次 Fetch，保证同 partition 串行有序。
		if err := c.consumeOnSuccess(ctx, handler, msg); err != nil {
			return err
		}
	}
}

// waitBackoff 等待统一错误退避，并在 ctx 取消时立即返回。
// 非正退避仅用于确定性测试，生产构造会通过 defaults 补为安全值。
func (c *Consumer) waitBackoff(ctx context.Context) error {
	if c.errorBackoff <= 0 {
		return nil
	}
	timer := time.NewTimer(c.errorBackoff)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// consumeOnSuccess 按“成功或可靠旁路后才提交”模式处理单条消息：
//   - handler 返回 nil：提交 offset，处理下一条；
//   - handler 返回非 nil（可重试错误）：退避后就地重试**同一条**，绝不跳到下一条。
//     原因：segmentio/kafka-go 的 FetchMessage 不会重投未提交消息，而 Kafka 的 offset 提交是累积的，
//     若失败后直接拉下一条并在其成功时提交，会把这条失败的 offset 一并提交，造成静默丢失。
//   - handler 返回 PermanentError 或 panic：第一次失败就写死信，落地成功后才提交；
//
// 队头阻塞旁路：当配置了 DeadLetterSink 且原地重试的墙钟时长超过 retryBudget 时，
// 判定该消息为毒/持久失败消息，写入死信后提交 offset 让分区前进。死信写入失败则绝不提交、
// 继续阻塞重试，保证「不丢消息」优先于「分区前进」。
// 未配置死信时：可重试错误做无界原地重试（分区阻塞到恢复或 ctx 取消）；
// 永久错误直接返回致命错误给 Pool，禁止无落点地提交跳过。
//
// 约定：handler 对永久错误（解码/payload 非法）返回 Permanent(err)，
// 对可重试错误（DB/Redis 抖动等）返回普通 error。禁止用 nil 静默跳过坏消息。
// ctx 取消时不提交 offset 直接返回，留待重启后从未提交位点重新消费。
func (c *Consumer) consumeOnSuccess(ctx context.Context, handler MessageHandler, msg segmentkafka.Message) error {
	var firstFailedAt time.Time
	attempts := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		handleErr, panicked := c.invokeHandler(ctx, handler, msg.Value)
		if panicked {
			logger.Error(ctx, "kafka 消费 handler panic，按永久错误立即落死信",
				logger.ErrorField("error", handleErr),
			)
			handleErr = Permanent(handleErr)
		}

		if handleErr == nil {
			return c.commitMessage(ctx, msg)
		}

		// 记录失败次数与首次失败时间，用于墙钟重试预算判定。
		attempts++
		now := time.Now()
		if firstFailedAt.IsZero() {
			firstFailedAt = now
		}

		// 解码/契约错误是确定性的，再等待重试预算只会无意义阻塞分区。
		// 未配置死信时直接返回致命错误让组件失败并报警，禁止“返回 nil 后提交”静默跳过；
		// 配置死信时只有 Park 成功才允许提交 offset。
		if IsPermanent(handleErr) {
			if c.deadLetterSink == nil {
				return fmt.Errorf("kafka 永久消息错误但未配置 DeadLetterSink: %w", handleErr)
			}
			if parkErr := c.parkDeadLetter(ctx, msg, handleErr, attempts, firstFailedAt, now); parkErr != nil {
				logger.Error(ctx, "kafka 永久错误死信落地失败，继续阻塞重试（不提交 offset）",
					logger.ErrorField("park_error", parkErr),
					logger.ErrorField("handle_error", handleErr),
					logger.String("topic", msg.Topic),
					logger.Int("partition", msg.Partition),
					logger.Int64("offset", msg.Offset),
				)
			} else {
				logger.Warn(ctx, "kafka 永久消息已首轮旁路到死信并提交 offset",
					logger.String("topic", msg.Topic),
					logger.Int("partition", msg.Partition),
					logger.Int64("offset", msg.Offset),
					logger.ErrorField("error", handleErr),
				)
				return c.commitMessage(ctx, msg)
			}
		}

		// 重试预算耗尽且配置了死信：旁路该毒消息，提交 offset 让分区前进。
		if !IsPermanent(handleErr) &&
			c.deadLetterSink != nil &&
			c.retryBudget > 0 &&
			now.Sub(firstFailedAt) >= c.retryBudget {
			if parkErr := c.parkDeadLetter(ctx, msg, handleErr, attempts, firstFailedAt, now); parkErr != nil {
				// 死信落地失败：绝不提交 offset，继续阻塞重试，保证不丢。
				logger.Error(ctx, "kafka 死信落地失败，继续阻塞重试（不提交 offset）",
					logger.ErrorField("park_error", parkErr),
					logger.ErrorField("handle_error", handleErr),
					logger.String("topic", msg.Topic),
					logger.Int("partition", msg.Partition),
					logger.Int64("offset", msg.Offset),
				)
			} else {
				logger.Warn(ctx, "kafka 消息重试预算耗尽，已旁路到死信并提交 offset 前进",
					logger.String("topic", msg.Topic),
					logger.Int("partition", msg.Partition),
					logger.Int64("offset", msg.Offset),
					logger.Int("attempts", attempts),
					logger.ErrorField("last_error", handleErr),
				)
				return c.commitMessage(ctx, msg)
			}
		}

		// 失败：退避后重试同一条消息。
		if err := c.waitBackoff(ctx); err != nil {
			return err
		}
	}
}

// commitMessage 在 handler/死信已经成功后只重试 offset 提交，不再次执行副作用。
//
// Kafka offset 是累积提交；如果这里忽略一次提交失败并继续 Fetch，后续 offset 的
// 成功提交会跨过当前消息。正常消息虽然有幂等保护、永久消息也已落死信，但显式卡在
// 当前 commit 仍能保证处理顺序和观测语义最清晰。
func (c *Consumer) commitMessage(ctx context.Context, msg segmentkafka.Message) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.reader.CommitMessages(ctx, msg); err == nil {
			return nil
		} else {
			if c.observeCommitError != nil {
				c.observeCommitError()
			}
			if errors.Is(err, io.ErrClosedPipe) {
				return fmt.Errorf("kafka reader 已关闭，无法提交 offset: %w", err)
			}
			logger.Error(ctx, "kafka offset 提交失败，保持当前消息并重试提交",
				logger.ErrorField("error", err),
				logger.String("topic", msg.Topic),
				logger.Int("partition", msg.Partition),
				logger.Int64("offset", msg.Offset),
			)
		}

		if err := c.waitBackoff(ctx); err != nil {
			return err
		}
	}
}

// parkDeadLetter 统一构造死信记录，确保“立即死信”和“预算耗尽死信”
// 携带完全一致的 Kafka 位点、原始 payload 与失败时间信息。
func (c *Consumer) parkDeadLetter(
	ctx context.Context,
	msg segmentkafka.Message,
	handleErr error,
	attempts int,
	firstFailedAt, lastFailedAt time.Time,
) error {
	return c.deadLetterSink.Park(ctx, DeadLetterRecord{
		Topic:         msg.Topic,
		Partition:     msg.Partition,
		Offset:        msg.Offset,
		Key:           msg.Key,
		Payload:       msg.Value,
		Err:           handleErr.Error(),
		Attempts:      attempts,
		FirstFailedAt: firstFailedAt,
		LastFailedAt:  lastFailedAt,
	})
}

// invokeHandler 调用 handler，并在配置了 handleTimeout 时为单次处理尝试施加超时。
// 超时只约束这一次尝试：到点后派生 ctx 取消，handler 内的 DB/Redis/gRPC 调用应及时返回错误，
// 再由 consumeOnSuccess 按可重试错误就地退避重试，从而避免某条消息因下游挂死而永久占住分区。
// handleTimeout <= 0 时不包裹，供明确接受无超时风险的测试或特殊调用方使用。
func (c *Consumer) invokeHandler(ctx context.Context, handler MessageHandler, value []byte) (error, bool) {
	if c.handleTimeout <= 0 {
		return safeHandle(ctx, handler, value)
	}
	hctx, cancel := context.WithTimeout(ctx, c.handleTimeout)
	defer cancel()
	return safeHandle(hctx, handler, value)
}

// safeHandle 调用 handler 并捕获 panic，返回 (err, panicked)。
// 捕获 panic 避免单条毒消息（如空指针、解码 panic）拖垮整个消费进程并在重启后陷入崩溃循环。
func safeHandle(ctx context.Context, handler MessageHandler, value []byte) (err error, panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panic: %v", r)
			panicked = true
		}
	}()
	return handler(ctx, value), false
}

// Close 关闭单个 Reader；服务层应调用 ManualConsumerPool.Close 聚合全部 worker 的关闭结果。
func (c *Consumer) Close() error {
	return c.reader.Close()
}
