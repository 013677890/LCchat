package kafka

import (
	"context"
	"time"

	"github.com/segmentio/kafka-go"
)

// ==================== Producer 定义 ====================

// Producer Kafka 生产者（通用）
type Producer struct {
	writer *kafka.Writer
}

// keyAwareBalancer 让带 key 的消息按 key 固定分区，无 key 的消息仍按负载均衡分区。
type keyAwareBalancer struct {
	hash       *kafka.Hash
	leastBytes *kafka.LeastBytes
}

// newKeyAwareBalancer 创建兼顾有序性与分区均衡的 Kafka 分区选择器。
func newKeyAwareBalancer() *keyAwareBalancer {
	return &keyAwareBalancer{
		hash:       &kafka.Hash{},
		leastBytes: &kafka.LeastBytes{},
	}
}

// Balance 根据消息 key 选择分区策略，避免空 key 被 Hash 固定到同一分区。
func (b *keyAwareBalancer) Balance(msg kafka.Message, partitions ...int) int {
	if len(msg.Key) > 0 {
		return b.hash.Balance(msg, partitions...)
	}
	return b.leastBytes.Balance(msg, partitions...)
}

// NewProducer 创建 Kafka 生产者
func NewProducer(brokers []string, topic string) *Producer {
	return &Producer{
		writer: &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Topic:        topic,
			RequiredAcks: kafka.RequireAll,
			Balancer:     newKeyAwareBalancer(),
		},
	}
}

// Send 发送消息到 Kafka（无 Partition Key，由 Balancer 自动分配分区）
func (p *Producer) Send(ctx context.Context, data []byte) error {
	return p.writer.WriteMessages(ctx, kafka.Message{
		Value: data,
		Time:  time.Now(),
	})
}

// SendWithKey 发送带 Partition Key 的消息到 Kafka
// 相同 key 的消息会被路由到同一 Partition，保证有序消费。
// 典型用法：msg-service 以 conv_id 作为 key，确保同会话消息有序。
func (p *Producer) SendWithKey(ctx context.Context, key, data []byte) error {
	return p.writer.WriteMessages(ctx, kafka.Message{
		Key:   key,
		Value: data,
		Time:  time.Now(),
	})
}

// Close 关闭生产者
func (p *Producer) Close() error {
	return p.writer.Close()
}
