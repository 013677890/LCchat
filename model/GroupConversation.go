package model

import (
	"time"
)

// GroupConversation 群会话状态表（热数据）
//
// 这是整个群聊系统写入最频繁的地方：每发一条群消息就 UPDATE 一次 max_seq 和 last_msg_*。
//
// 设计亮点：
//   - GroupUuid 直接做主键（一对一关系），省去自增 ID 的 B+ 树开销
//   - 只有 6 个字段，MySQL 内存页能塞下极多记录，UPDATE 时磁盘 I/O 极低
//   - 与 Conversation 表（每人一条）解耦：此表记录群的全局状态，
//     Conversation 表记录每个成员的个人视图（read_seq / mute / pin 等）
//
// 变更频率：极高（每发一条群消息更新一次）
type GroupConversation struct {
	// GroupUuid 既是群 ID，也是这张表的业务主键
	GroupUuid   string     `gorm:"column:group_uuid;type:char(20);primaryKey;comment:群组唯一id"`
	MaxSeq      int64      `gorm:"column:max_seq;not null;default:0;comment:群消息当前最大seq"`
	LastMsgId   string     `gorm:"column:last_msg_id;type:char(64);comment:最后一条消息的ID(ULID)"`
	LastMsgPrev string     `gorm:"column:last_msg_preview;type:varchar(255);comment:最后消息预览JSON(透传给前端解析)，结构: {sender_uuid, preview}"`
	LastMsgAt   *time.Time `gorm:"column:last_msg_at;index:idx_last_msg_at;comment:最后消息时间(做活跃度排序)"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;autoUpdateTime;comment:状态最后变更时间"`
}

func (GroupConversation) TableName() string {
	return "group_conversation"
}
