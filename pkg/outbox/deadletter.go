package outbox

import (
	"context"
	"time"

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

// ListPendingDeadEvents 读取待处理死信，供重放工具/admin 使用。
// source 为空时返回全部来源；limit 越界时收敛到 [1,500]，默认 100。
func ListPendingDeadEvents(db *gorm.DB, source string, limit int) ([]DeadEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := db.Where("status = ?", DeadEventStatusPending)
	if source != "" {
		query = query.Where("source = ?", source)
	}
	var events []DeadEvent
	err := query.Order("id ASC").Limit(limit).Find(&events).Error
	return events, err
}

// MarkDeadEventReplayed 标记死信已重放成功。
func MarkDeadEventReplayed(db *gorm.DB, id int64) error {
	return db.Model(&DeadEvent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{"status": DeadEventStatusReplayed, "updated_at": time.Now()}).Error
}

// MarkDeadEventDiscarded 标记死信被人工丢弃，不再重放。
func MarkDeadEventDiscarded(db *gorm.DB, id int64) error {
	return db.Model(&DeadEvent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{"status": DeadEventStatusDiscarded, "updated_at": time.Now()}).Error
}
