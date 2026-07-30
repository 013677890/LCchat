package kafka

import (
	"context"
	"fmt"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/logger"
	segmentkafka "github.com/segmentio/kafka-go"
)

// ==================== Consumer 定义 ====================

// CommitMode 定义消费者提交 offset 的策略。
type CommitMode int8

const (
	// CommitAlways 表示无论 handler 是否报错都提交 offset。
	CommitAlways CommitMode = iota
	// CommitOnSuccess 表示只有 handler 成功后才提交 offset。
	CommitOnSuccess
)

// ManualConsumerConfig 定义手动提交型消费者的 Reader 参数。
type ManualConsumerConfig struct {
	MinBytes     int
	MaxBytes     int
	MaxWait      time.Duration
	ErrorBackoff time.Duration
	// HandleTimeout 为单条消息的每次处理尝试施加超时预算。
	// 防止某条消息的下游(DB/Redis/gRPC)挂死导致 handle 永不返回、进而冻结整个分区。
	// <=0 时回退到 defaultHandleTimeout；如需关闭可显式传负值由调用方自担风险。
	HandleTimeout time.Duration

	// RetryBudget 为单条消息「原地重试」的墙钟预算：超出后判定为毒/持久失败消息，
	// 旁路到 DeadLetterSink 并提交 offset，从而解除队头阻塞。<=0 时回退到 defaultRetryBudget。
	// 仅在同时配置了 DeadLetterSink 时才生效；否则保持「无界原地重试」旧行为。
	RetryBudget time.Duration

	// DeadLetterSink 为死信落地实现。nil 时瞬时错误保持原地重试；永久错误会让
	// consumer 明确失败，禁止在没有可追查落点时提交并静默跳过坏消息。
	DeadLetterSink DeadLetterSink
}

// defaultHandleTimeout 是手动提交消费者单次处理尝试的默认超时。
// IM 消费者通常只做几次点查/点写，10s 给足余量又能把下游挂死收敛到秒级。
const defaultHandleTimeout = 10 * time.Second

// defaultRetryBudget 是单条消息原地重试的默认墙钟预算。
// 瞬时抖动通常秒级恢复，远不到该预算；毒/持久失败消息在此预算后被旁路到死信，
// 因此 worst-case 队头阻塞被钉死在该时长内。
const defaultRetryBudget = 2 * time.Minute

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

// Consumer Kafka 消费者（通用）
type Consumer struct {
	reader         messageReader
	commitMode     CommitMode
	errorBackoff   time.Duration
	handleTimeout  time.Duration
	retryBudget    time.Duration
	deadLetterSink DeadLetterSink
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

// NewConsumer 创建 Kafka 消费者
func NewConsumer(brokers []string, topic, groupID string) *Consumer {
	return &Consumer{
		reader: segmentkafka.NewReader(segmentkafka.ReaderConfig{
			Brokers: brokers,
			Topic:   topic,
			GroupID: groupID,
		}),
		commitMode: CommitAlways,
	}
}

// NewManualCommitConsumer 创建“仅成功后提交”的 Kafka 消费者。
func NewManualCommitConsumer(brokers []string, topic, groupID string, cfg ManualConsumerConfig) *Consumer {
	cfg.defaults()

	return &Consumer{
		reader: segmentkafka.NewReader(segmentkafka.ReaderConfig{
			Brokers:        brokers,
			Topic:          topic,
			GroupID:        groupID,
			MinBytes:       cfg.MinBytes,
			MaxBytes:       cfg.MaxBytes,
			MaxWait:        cfg.MaxWait,
			CommitInterval: 0,
		}),
		commitMode:     CommitOnSuccess,
		errorBackoff:   cfg.ErrorBackoff,
		handleTimeout:  cfg.HandleTimeout,
		retryBudget:    cfg.RetryBudget,
		deadLetterSink: cfg.DeadLetterSink,
	}
}

// MessageHandler 消息处理函数类型
type MessageHandler func(ctx context.Context, message []byte) error

// Start 启动消费者（阻塞式运行）
func (c *Consumer) Start(ctx context.Context, handler MessageHandler) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// 读取消息
			msg, err := c.reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				continue
			}

			// CommitAlways：处理一次后无论成败都提交（兼容旧行为）。
			if c.commitMode == CommitAlways {
				_, _ = c.invokeHandler(ctx, handler, msg.Value)
				_ = c.reader.CommitMessages(ctx, msg)
				continue
			}

			// CommitOnSuccess：成功才提交，失败就地重试同一条。
			if err := c.consumeOnSuccess(ctx, handler, msg); err != nil {
				return err
			}
		}
	}
}

// consumeOnSuccess 在 CommitOnSuccess 模式下处理单条消息：
//   - handler 返回 nil：提交 offset，处理下一条；
//   - handler 返回非 nil（可重试错误）：退避后就地重试**同一条**，绝不跳到下一条。
//     原因：segmentio/kafka-go 的 FetchMessage 不会重投未提交消息，而 Kafka 的 offset 提交是累积的，
//     若失败后直接拉下一条并在其成功时提交，会把这条失败的 offset 一并提交，造成静默丢失。
//   - handler 返回 PermanentError 或 panic：第一次失败就写死信，落地成功后才提交；
//
// 队头阻塞旁路：当配置了 DeadLetterSink 且原地重试的墙钟时长超过 retryBudget 时，
// 判定该消息为毒/持久失败消息，写入死信后提交 offset 让分区前进。死信写入失败则绝不提交、
// 继续阻塞重试，保证「不丢消息」优先于「分区前进」。未配置死信时退化为旧的无界原地重试。
//
// 约定：handler 对永久错误（解码/payload 非法）返回 Permanent(err)，
// 对可重试错误（DB/Redis 抖动等）返回普通 error。禁止再用 nil 静默跳过坏消息。
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
		// 没有配置死信时直接返回错误让组件失败并报警，绝不恢复旧的“返回 nil 后提交”
		// 行为；配置死信时只有 Park 成功才允许提交 offset。
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
		if c.errorBackoff <= 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.errorBackoff):
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
			logger.Error(ctx, "kafka offset 提交失败，保持当前消息并重试提交",
				logger.ErrorField("error", err),
				logger.String("topic", msg.Topic),
				logger.Int("partition", msg.Partition),
				logger.Int64("offset", msg.Offset),
			)
		}

		if c.errorBackoff <= 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.errorBackoff):
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
// handleTimeout <= 0 时不包裹（保持旧行为），供 CommitAlways 等无需该约束的场景使用。
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

// Close 关闭消费者
func (c *Consumer) Close() error {
	return c.reader.Close()
}
