package kafka

import (
	"context"
	"time"

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

			// 处理消息
			handleErr := handler(ctx, msg.Value)

			// 兼容旧行为：默认无论成功失败都提交；需要可靠重试时使用手动提交模式。
			if c.commitMode == CommitAlways || handleErr == nil {
				_ = c.reader.CommitMessages(ctx, msg)
			}

			if handleErr != nil && c.commitMode == CommitOnSuccess && c.errorBackoff > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(c.errorBackoff):
				}
			}
		}
	}
}

// Close 关闭消费者
func (c *Consumer) Close() error {
	return c.reader.Close()
}
