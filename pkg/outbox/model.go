package outbox

import (
	"time"
)

// Event 是 CDC 模式下的 Outbox 事件模型。
//
// 在 Debezium + Kafka Connect 架构下，outbox_events 只承担“本地事务内可靠落盘”的职责，
// 不再在数据库侧维护 pending / processing / failed 等状态机字段。
//
// 表名：outbox_events
type Event struct {
	ID        int64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	EventType string    `json:"event_type" gorm:"column:event_type;type:varchar(128);not null;index:idx_outbox_event_type_created;comment:领域事件类型"`
	EntityID  string    `json:"entity_id" gorm:"column:entity_id;type:varchar(64);not null;index:idx_outbox_entity_id;comment:事件分区键，对应业务实体 ID"`
	Payload   string    `json:"payload" gorm:"column:payload;type:longtext;not null;comment:事件负载 JSON"`
	CreatedAt time.Time `json:"created_at" gorm:"column:created_at;not null;index:idx_outbox_event_type_created;comment:创建时间"`
}

func (Event) TableName() string {
	return "outbox_events"
}

// IdempotentRecord 幂等消费记录。
// 各消费端在处理事件前先检查此表是否已处理过，防止重复消费。
//
// 表名：idempotent_events
// 唯一索引：(event_type, event_id)
type IdempotentRecord struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement"`
	EventType   string    `gorm:"column:event_type;type:varchar(64);not null;uniqueIndex:uk_type_event;comment:事件类型"`
	EventID     string    `gorm:"column:event_id;type:varchar(64);not null;uniqueIndex:uk_type_event;comment:事件唯一标识"`
	ProcessedAt time.Time `gorm:"column:processed_at;not null;comment:处理时间"`
}

func (IdempotentRecord) TableName() string {
	return "idempotent_events"
}
