package consumer

import (
	"time"

	"github.com/segmentio/kafka-go"
)

// newKafkaReader 创建 message-push 使用的 Kafka Reader。
// 当前采用 consumer group 模式消费 `msg.push` topic。
func newKafkaReader(brokers []string, topic, groupID string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
		// MinBytes = 单次拉取最小字节数。
		// 设为 1 表示尽快返回，降低消息端到端延迟。
		MinBytes: 1,
		// MaxBytes = 单次拉取最大字节数。
		// 10 MiB 足以覆盖当前单批消息体积，同时避免单次拉取过大。
		MaxBytes: 10 << 20,
		// MaxWait = broker 端等待满足 MinBytes 的最长时间。
		// 默认 10s 对 IM 下行延迟过大；这里收紧到 500ms 以降低低流量下的投递延迟。
		MaxWait: 500 * time.Millisecond,
		// CommitInterval = 0 表示同步 commit。
		// 阶段一策略要求 offset 推进语义明确（失败也要 commit），异步 commit 会放大丢失窗口。
		CommitInterval: 0,
	})
}
