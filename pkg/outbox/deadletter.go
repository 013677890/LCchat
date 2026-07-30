package outbox

import (
	"context"

	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"gorm.io/gorm"
)

// 死信状态常量。
const (
	DeadEventStatusPending   = "pending"   // 待处理（默认）
	DeadEventStatusReplayed  = "replayed"  // 已重放成功
	DeadEventStatusDiscarded = "discarded" // 人工丢弃
)

// deadLetterSink 把死信写入 dead_events 表，满足 kafka.DeadLetterSink。
type deadLetterSink struct {
	db     *gorm.DB
	source string
}

// NewDeadLetterSink 创建一个把死信落库到 dead_events 的 kafka.DeadLetterSink。
// source 用于区分来源消费者（如 "relation-service:account.deleted"），便于查询/重放。
func NewDeadLetterSink(db *gorm.DB, source string) kafka.DeadLetterSink {
	return &deadLetterSink{db: db, source: source}
}

// Park 持久化一条死信。返回错误时消费者不会提交 offset，从而保证不丢消息。
func (s *deadLetterSink) Park(ctx context.Context, rec kafka.DeadLetterRecord) error {
	event := &DeadEvent{
		Source:         s.source,
		Topic:          rec.Topic,
		KafkaPartition: rec.Partition,
		KafkaOffset:    rec.Offset,
		MsgKey:         string(rec.Key),
		EventType:      rec.EventType,
		Payload:        rec.Payload,
		ErrorMsg:       rec.Err,
		Attempts:       rec.Attempts,
		Status:         DeadEventStatusPending,
		FirstFailedAt:  rec.FirstFailedAt,
		LastFailedAt:   rec.LastFailedAt,
	}
	return s.db.WithContext(ctx).Create(event).Error
}
