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

// DeadEvent 是消费死信记录。
//
// 手动提交消费者在「有界重试」预算耗尽后，把毒/持久失败的消息旁路到此表并提交 offset，
// 从而解除队头阻塞：分区得以前进，消息不丢、可查询、可重放（重放安全由 idempotent_events 保证）。
//
// 表名：dead_events
type DeadEvent struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement"`
	Source         string    `gorm:"column:source;type:varchar(128);not null;index:idx_dead_source_status,priority:1;comment:来源消费者标识(service:topic)"`
	Topic          string    `gorm:"column:topic;type:varchar(191);not null;comment:Kafka topic"`
	KafkaPartition int       `gorm:"column:kafka_partition;not null;default:0;comment:Kafka 分区"`
	KafkaOffset    int64     `gorm:"column:kafka_offset;not null;default:0;comment:Kafka offset"`
	MsgKey         string    `gorm:"column:msg_key;type:varchar(191);not null;default:'';comment:Kafka 消息 key"`
	EventType      string    `gorm:"column:event_type;type:varchar(128);not null;default:'';comment:事件类型(尽力解析)"`
	Payload        []byte    `gorm:"column:payload;type:longblob;not null;comment:原始消息字节"`
	ErrorMsg       string    `gorm:"column:error_msg;type:text;comment:最后一次失败错误"`
	Attempts       int       `gorm:"column:attempts;not null;default:0;comment:原地重试次数"`
	Status         string    `gorm:"column:status;type:varchar(16);not null;default:'pending';index:idx_dead_source_status,priority:2;comment:pending|replayed|discarded"`
	FirstFailedAt  time.Time `gorm:"column:first_failed_at;not null;comment:首次失败时间"`
	LastFailedAt   time.Time `gorm:"column:last_failed_at;not null;comment:最后失败时间"`
	CreatedAt      time.Time `gorm:"column:created_at;not null;comment:创建时间"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null;comment:更新时间"`
}

func (DeadEvent) TableName() string {
	return "dead_events"
}
