package outbox

import (
	"time"

	"gorm.io/gorm"
)

// InsertEvent 在业务事务内追加一条 CDC Outbox 事件。
//
// 该记录提交后会由 Debezium 从 MySQL Binlog 捕获并转发到 Kafka，
// 因此这里仅负责可靠落盘，不再维护数据库侧状态机。
func InsertEvent(tx *gorm.DB, eventType, entityID, payload string) error {
	event := &Event{
		EventType: eventType,
		EntityID:  entityID,
		Payload:   payload,
		CreatedAt: time.Now(),
	}
	return tx.Create(event).Error
}

// CheckIdempotent 检查事件是否已处理过（幂等）。
// 返回 true 表示已处理，调用方应跳过。
func CheckIdempotent(db *gorm.DB, eventType, eventID string) (bool, error) {
	var count int64
	err := db.Model(&IdempotentRecord{}).
		Where("event_type = ? AND event_id = ?", eventType, eventID).
		Count(&count).Error
	return count > 0, err
}

// MarkIdempotent 标记事件已处理。
func MarkIdempotent(db *gorm.DB, eventType, eventID string) error {
	record := &IdempotentRecord{
		EventType:   eventType,
		EventID:     eventID,
		ProcessedAt: time.Now(),
	}
	return db.Create(record).Error
}
