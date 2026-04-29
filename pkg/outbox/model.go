package outbox

import (
	"time"
)

// EventStatus 事件处理状态
type EventStatus int8

const (
	EventStatusPending    EventStatus = 0 // 待处理
	EventStatusProcessing EventStatus = 1 // 处理中
	EventStatusDone       EventStatus = 2 // 已完成
	EventStatusFailed     EventStatus = 3 // 失败（进入 DLQ）
)

// Event 是 Outbox 事件表的模型。
// 各服务共用此结构，由 Outbox worker 轮询并投递。
//
// 表名：outbox_events
// 建议索引：(status, created_at) 供 worker 轮询；(event_type, entity_id) 供去重。
type Event struct {
	ID          int64       `gorm:"column:id;primaryKey;autoIncrement"`
	EventType   string      `gorm:"column:event_type;type:varchar(64);not null;index:idx_type_entity;comment:事件类型，如 user_created / account_deleted / profile_display_changed"`
	EntityID    string      `gorm:"column:entity_id;type:varchar(64);not null;index:idx_type_entity;comment:关联实体 ID（通常是 user_uuid）"`
	Payload     string      `gorm:"column:payload;type:text;comment:事件负载 JSON"`
	Status      EventStatus `gorm:"column:status;type:tinyint;not null;default:0;index:idx_status_created;comment:0=pending 1=processing 2=done 3=failed"`
	RetryCount  int         `gorm:"column:retry_count;type:int;not null;default:0;comment:已重试次数"`
	MaxRetries  int         `gorm:"column:max_retries;type:int;not null;default:10;comment:最大重试次数"`
	LastError   string      `gorm:"column:last_error;type:text;comment:最近一次失败的错误信息"`
	CreatedAt   time.Time   `gorm:"column:created_at;not null;index:idx_status_created;comment:创建时间"`
	ProcessedAt *time.Time  `gorm:"column:processed_at;comment:处理完成时间"`
	NextRetryAt *time.Time  `gorm:"column:next_retry_at;comment:下次重试时间（指数退避）"`
}

func (Event) TableName() string {
	return "outbox_events"
}

// IdempotentRecord 幂等消费记录。
// 各消费端在处理事件前先检查此表是否已处理过，防止重复消费。
//
// 表名：idempotent_events
// 唯一索引：(event_type, entity_id)
type IdempotentRecord struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement"`
	EventType   string    `gorm:"column:event_type;type:varchar(64);not null;uniqueIndex:uk_type_entity;comment:事件类型"`
	EntityID    string    `gorm:"column:entity_id;type:varchar(64);not null;uniqueIndex:uk_type_entity;comment:实体 ID"`
	ProcessedAt time.Time `gorm:"column:processed_at;not null;comment:处理时间"`
}

func (IdempotentRecord) TableName() string {
	return "idempotent_events"
}
