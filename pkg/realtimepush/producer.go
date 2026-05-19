package realtimepush

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
)

// Writer 定义实时提醒写入 Kafka 所需的最小发送能力。
type Writer interface {
	SendWithKey(ctx context.Context, key, data []byte) error
}

// Publisher 定义业务服务发送实时提醒事件的统一接口。
type Publisher interface {
	Publish(ctx context.Context, event Event) error
}

// Producer 封装 realtime.push 事件生产逻辑。
type Producer struct {
	writer Writer
}

// NewProducer 基于已有 Kafka writer 创建实时提醒生产者。
func NewProducer(writer Writer) *Producer {
	return &Producer{writer: writer}
}

// NewKafkaProducer 创建使用默认 realtime.push topic 的实时提醒生产者。
func NewKafkaProducer(brokers []string) *Producer {
	return NewKafkaTopicProducer(brokers, DefaultTopic)
}

// NewKafkaTopicProducer 创建指定 topic 的实时提醒生产者。
func NewKafkaTopicProducer(brokers []string, topic string) *Producer {
	if topic == "" {
		topic = DefaultTopic
	}
	return NewProducer(kafka.NewProducer(brokers, topic))
}

// Publish 校验、补齐并发送一条实时提醒事件。
func (p *Producer) Publish(ctx context.Context, event Event) error {
	if p == nil || p.writer == nil {
		return errors.New("realtimepush: producer 未初始化")
	}

	fillEventDefaults(ctx, &event)
	data, err := event.Marshal()
	if err != nil {
		return fmt.Errorf("realtimepush: 编码事件失败: %w", err)
	}

	key := event.PartitionKey()
	if err := p.writer.SendWithKey(ctx, []byte(key), data); err != nil {
		return fmt.Errorf("realtimepush: 发送事件失败: %w", err)
	}
	return nil
}

// Close 关闭底层 writer；若 writer 不支持关闭则直接返回 nil。
func (p *Producer) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	closer, ok := p.writer.(interface{ Close() error })
	if !ok {
		return nil
	}
	return closer.Close()
}

// fillEventDefaults 补齐 trace_id 与服务端时间，减少各业务服务重复样板代码。
func fillEventDefaults(ctx context.Context, event *Event) {
	if event == nil {
		return
	}
	if event.TraceID == "" {
		event.TraceID = ctxmeta.TraceID(ctx)
	}
	if event.ServerTs <= 0 {
		event.ServerTs = time.Now().UnixMilli()
	}
}
