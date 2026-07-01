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

	// DeadLetterSink 为死信落地实现。nil 表示不旁路（瞬时/永久失败一律无界原地重试，旧行为）。
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
	reader         *segmentkafka.Reader
	commitMode     CommitMode
	errorBackoff   time.Duration
	handleTimeout  time.Duration
	retryBudget    time.Duration
	deadLetterSink DeadLetterSink
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
//   - handler panic：视为永久错误，提交 offset 跳过并告警（panic 多为确定性错误，重试只会无限循环卡死分区）。
//
// 队头阻塞旁路：当配置了 DeadLetterSink 且原地重试的墙钟时长超过 retryBudget 时，
// 判定该消息为毒/持久失败消息，写入死信后提交 offset 让分区前进。死信写入失败则绝不提交、
// 继续阻塞重试，保证「不丢消息」优先于「分区前进」。未配置死信时退化为旧的无界原地重试。
//
// 约定：handler 对永久错误（解码/payload 非法）返回 nil 自行跳过，仅对可重试错误（DB/Redis 抖动等）返回非 nil。
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
			logger.Error(ctx, "kafka 消费 handler panic，提交 offset 跳过该消息",
				logger.ErrorField("error", handleErr),
			)
			_ = c.reader.CommitMessages(ctx, msg)
			return nil
		}
		if handleErr == nil {
			_ = c.reader.CommitMessages(ctx, msg)
			return nil
		}

		// 记录失败次数与首次失败时间，用于墙钟重试预算判定。
		attempts++
		now := time.Now()
		if firstFailedAt.IsZero() {
			firstFailedAt = now
		}

		// 重试预算耗尽且配置了死信：旁路该毒消息，提交 offset 让分区前进。
		if c.deadLetterSink != nil && c.retryBudget > 0 && now.Sub(firstFailedAt) >= c.retryBudget {
			rec := DeadLetterRecord{
				Topic:         msg.Topic,
				Partition:     msg.Partition,
				Offset:        msg.Offset,
				Key:           msg.Key,
				Payload:       msg.Value,
				Err:           handleErr.Error(),
				Attempts:      attempts,
				FirstFailedAt: firstFailedAt,
				LastFailedAt:  now,
			}
			if parkErr := c.deadLetterSink.Park(ctx, rec); parkErr != nil {
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
				_ = c.reader.CommitMessages(ctx, msg)
				return nil
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
