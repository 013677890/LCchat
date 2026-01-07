package model

import (
	"time"

	"gorm.io/gorm"
)

// GroupMember 维护群成员关系（单独一张表，不在群表存成员 JSON）。
// 外键建议：group_member.group_uuid -> group_info.uuid；group_member.user_uuid -> user_info.uuid（需保持长度一致）。
type GroupMember struct {
	Id        int64          `gorm:"column:id;primaryKey;autoIncrement;comment:自增id"`
	GroupUuid string         `gorm:"column:group_uuid;type:char(20);not null;index;uniqueIndex:uidx_group_user;comment:群uuid"`
	UserUuid  string         `gorm:"column:user_uuid;type:char(20);not null;index;uniqueIndex:uidx_group_user;comment:用户uuid"`
	Role      int8           `gorm:"column:role;not null;default:0;comment:0成员 1管理员 2群主"`
	Remark    string         `gorm:"column:remark;type:varchar(64);comment:群名片/备注"`
	Status    int8           `gorm:"column:status;not null;default:0;comment:0正常 1退出 2踢出 3待审核"`
	MuteUntil *time.Time     `gorm:"column:mute_until;comment:禁言到期时间;default:null"`
	Inviter   string         `gorm:"column:inviter_uuid;type:char(20);comment:邀请人uuid"`
	JoinedAt  time.Time      `gorm:"column:joined_at;autoCreateTime;comment:入群时间"`
	CreatedAt time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (GroupMember) TableName() string { return "group_member" }
