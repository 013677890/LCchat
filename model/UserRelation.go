package model

import (
	"time"

	"gorm.io/gorm"
)

// UserRelation 维护用户之间的单向关系（好友/拉黑/待确认）。
// 约束：uniqueIndex:uidx_user_peer 确保同一对用户不重复；长度与 user_info.uuid 保持一致（char(20)）。
type UserRelation struct {
	Id       int64  `gorm:"column:id;primaryKey;autoIncrement;comment:自增id"`
	UserUuid string `gorm:"column:user_uuid;type:char(20);not null;uniqueIndex:uidx_user_peer;comment:用户uuid"`
	PeerUuid string `gorm:"column:peer_uuid;type:char(20);not null;index;uniqueIndex:uidx_user_peer;comment:对端用户uuid"`
	Status   int8   `gorm:"column:status;not null;default:0;comment:关系状态 0.正常 1.拉黑 2.删除"`
	Remark   string `gorm:"column:remark;type:varchar(64);comment:好友备注"`
	Source   string `gorm:"column:source;type:varchar(64);comment:添加来源，如手机号/群/二维码"`
	//LastContactAt *time.Time     `gorm:"column:last_contact_at;comment:最近联系时间"`  性能问题，不存储最近联系时间
	GroupTag  string         `gorm:"column:group_tag;type:varchar(32);comment:标签"`
	CreatedAt time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (UserRelation) TableName() string { return "user_relation" }
