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
}

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
}

// Consumer Kafka 消费者（通用）
type Consumer struct {
	reader       *segmentkafka.Reader
	commitMode   CommitMode
	errorBackoff time.Duration
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
		commitMode:   CommitOnSuccess,
		errorBackoff: cfg.ErrorBackoff,
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
				_, _ = safeHandle(ctx, handler, msg.Value)
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
// 约定：handler 对永久错误（解码/payload 非法）返回 nil 自行跳过，仅对可重试错误（DB/Redis 抖动等）返回非 nil，
// 从而保证这里的“就地重试”不会被毒消息卡死。
// ctx 取消时不提交 offset 直接返回，留待重启后从未提交位点重新消费。
func (c *Consumer) consumeOnSuccess(ctx context.Context, handler MessageHandler, msg segmentkafka.Message) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		handleErr, panicked := safeHandle(ctx, handler, msg.Value)
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
