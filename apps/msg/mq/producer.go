package mq

import (
	"context"
	"fmt"

	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"google.golang.org/protobuf/proto"
)

// Producer 封装 msg-service 向 Kafka msg.push topic 写入消息推送事件的逻辑。
//
// Kafka 分区策略：
//   - Partition Key = conv_id（保证同一会话的消息在同一 Partition 内有序消费）
//   - Value = MsgPushEvent 序列化后的 Protobuf bytes
type Producer struct {
	kafkaProducer *kafka.Producer
	topic         string
}

// NewProducer 创建 msg.push Kafka 生产者
func NewProducer(kafkaProducer *kafka.Producer, topic string) *Producer {
	return &Producer{
		kafkaProducer: kafkaProducer,
		topic:         topic,
	}
}

// Publish 发送一条推送事件到 Kafka msg.push topic。
// convId 作为 partition key，保证同一会话的消息在同一分区内顺序消费。
// event 为 proto.Message（通常是 *pb.MsgPushEvent），内部序列化为 bytes 后写入。
func (p *Producer) Publish(ctx context.Context, convId string, event proto.Message) error {
	data, err := proto.Marshal(event)
	if err != nil {
		return fmt.Errorf("mq.Producer: marshal event failed: %w", err)
	}
	if err := p.kafkaProducer.SendWithKey(ctx, []byte(convId), data); err != nil {
		return fmt.Errorf("mq.Producer: send to kafka failed: %w", err)
	}
	return nil
}

// Close 关闭底层 Kafka 生产者
func (p *Producer) Close() error {
	return p.kafkaProducer.Close()
}
