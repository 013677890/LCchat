package model

import (
	"time"

	"gorm.io/gorm"
)

// Conversation 记录会话元数据，支持单聊/群聊。
// type: 1=单聊(P2P)，2=群聊(GROUP)，与 proto ConvType 枚举保持一致。
type Conversation struct {
	Id          int64          `gorm:"column:id;primaryKey;autoIncrement;comment:自增id"`
	ConvId      string         `gorm:"column:conv_id;type:char(40);not null;comment:会话ID(可用p2p-<sorted uuids>或群uuid)"`
	Type        int8           `gorm:"column:type;not null;comment:1单聊 2群聊(对齐proto ConvType)"`
	OwnerUuid   string         `gorm:"column:owner_uuid;type:char(20);not null;uniqueIndex:uidx_owner_conv;index:idx_owner_status_update,priority:1;comment:会话归属用户uuid(单聊每人一条，群聊每成员一条)"`
	TargetUuid  string         `gorm:"column:target_uuid;type:char(20);not null;uniqueIndex:uidx_owner_conv;comment:单聊为对端uuid,群聊为群uuid"`
	LastMsgId   string         `gorm:"column:last_msg_id;type:char(64);comment:最后消息ID"`
	LastMsgAt   *time.Time     `gorm:"column:last_msg_at;comment:最后消息时间"`
	LastMsgPrev string         `gorm:"column:last_msg_preview;type:varchar(255);comment:最后消息预览JSON(透传给前端解析)，结构: {sender_uuid, preview}"`
	MaxSeq      int64          `gorm:"column:max_seq;not null;default:0;comment:会话内当前最大seq,用于计算未读数和clear_seq"`
	ReadSeq     int64          `gorm:"column:read_seq;not null;default:0;comment:该用户已读到的最大seq,未读数=max(0,max_seq-read_seq)"`
	ClearSeq    int64          `gorm:"column:clear_seq;not null;default:0;comment:会话清空位点,删除会话时记录当前max_seq,拉取历史时过滤seq<=clear_seq"`
	UnreadCount int            `gorm:"column:unread_count;not null;default:0;comment:未读数(冗余字段,可由max_seq-read_seq动态计算)"`
	Mute        bool           `gorm:"column:mute;not null;default:false;comment:免打扰"`
	Pin         bool           `gorm:"column:pin;not null;default:false;comment:置顶"`
	Status      int8           `gorm:"column:status;not null;default:0;index:idx_owner_status_update,priority:2;comment:0正常 1关闭/删除"`
	CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;autoUpdateTime;index:idx_owner_status_update,priority:3"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (Conversation) TableName() string { return "conversation" }
