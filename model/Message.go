package model

import (
	"time"

	"gorm.io/gorm"
)

// Message 记录聊天消息（包含系统控制类消息）。
// 设计要点：
// - FromUuid 必填，系统/官方号请使用保留账号，不用空值。
// - MsgType 区分普通气泡消息与系统控制消息（见 const.go）。
// - Content 为 JSON / 文本串，前端按 MsgType 解析。
// - ClientMsgId 用于幂等（同一发送端的去重）。
// - ConvId 关联会话，Seq 为会话内递增序号（便于排序与去重）。
type Message struct {
	Id          int64          `gorm:"column:id;primaryKey;autoIncrement;comment:自增id"`
	ConvId      string         `gorm:"column:conv_id;type:char(40);not null;index:idx_conv_seq;index:idx_conv_time;comment:会话ID,关联 conversation.conv_id"`
	Seq         int64          `gorm:"column:seq;not null;index:idx_conv_seq;comment:会话内序号"`
	MsgId       string         `gorm:"column:msg_id;type:char(64);uniqueIndex;not null;comment:全局消息ID(雪花/UUID)"`
	ClientMsgId string         `gorm:"column:client_msg_id;type:char(64);not null;uniqueIndex:uidx_sender_client;comment:客户端幂等ID"`
	FromUuid    string         `gorm:"column:from_uuid;type:char(20);not null;comment:发送者uuid(系统消息也需填写保留账号)"`
	MsgType     int16          `gorm:"column:msg_type;not null;comment:消息类型(参考 const.go)"`
	Content     string         `gorm:"column:content;type:json;not null;comment:消息内容(JSON,根据msg_type解析)"`
	Status      int8           `gorm:"column:status;not null;default:0;comment:0正常 1撤回 2删除"`
	SendTime    time.Time      `gorm:"column:send_time;index:idx_conv_time;comment:发送时间(服务器时间)"`
	CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (Message) TableName() string { return "message" }

