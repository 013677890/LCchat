package model

import (
	"time"

	"gorm.io/gorm"
)

// Conversation 记录会话元数据，支持单聊/群聊。
// type: 0=p2p，1=group
type Conversation struct {
	Id          int64          `gorm:"column:id;primaryKey;autoIncrement;comment:自增id"`
	ConvId      string         `gorm:"column:conv_id;type:char(40);not null;comment:会话ID(可用p2p-<sorted uuids>或群uuid)"`
	Type        int8           `gorm:"column:type;not null;comment:0单聊 1群聊"`
	OwnerUuid   string         `gorm:"column:owner_uuid;type:char(20);not null;uniqueIndex:uidx_owner_conv;index:idx_owner_status_update,priority:1;comment:会话归属用户uuid(单聊每人一条，群聊每成员一条)"`
	TargetUuid  string         `gorm:"column:target_uuid;type:char(20);not null;uniqueIndex:uidx_owner_conv;comment:单聊为对端uuid,群聊为群uuid"`
	LastMsgId   string         `gorm:"column:last_msg_id;type:char(64);comment:最后消息ID"`
	LastMsgAt   *time.Time     `gorm:"column:last_msg_at;comment:最后消息时间"`
	LastMsgPrev string         `gorm:"column:last_msg_preview;type:varchar(255);comment:最后消息预览（文本内容或占位[图片]/[语音]等）"`
	UnreadCount int            `gorm:"column:unread_count;not null;default:0;comment:未读数"`
	Mute        bool           `gorm:"column:mute;not null;default:false;comment:免打扰"`
	Pin         bool           `gorm:"column:pin;not null;default:false;comment:置顶"`
	Status      int8           `gorm:"column:status;not null;default:0;index:idx_owner_status_update,priority:2;comment:0正常 1关闭/删除"`
	CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;autoUpdateTime;index:idx_owner_status_update,priority:3"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (Conversation) TableName() string { return "conversation" }
