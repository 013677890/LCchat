package model

import (
	"time"

	"gorm.io/gorm"
)

type GroupInfo struct {
	Id        int64          `gorm:"column:id;primaryKey;autoIncrement;comment:自增id"`
	Uuid      string         `gorm:"column:uuid;type:char(20);uniqueIndex;not null;comment:群组唯一id"`
	Name      string         `gorm:"column:name;type:varchar(64);not null;comment:群名称"`
	Notice    string         `gorm:"column:notice;type:varchar(500);comment:群公告"`
	MemberCnt int            `gorm:"column:member_cnt;not null;default:1;comment:群人数"` // 默认群主1人
	OwnerUuid string         `gorm:"column:owner_uuid;type:char(20);not null;index;comment:群主uuid"`
	AddMode   int8           `gorm:"column:add_mode;not null;default:0;comment:加群方式,0.直接 1.审核"`
	Avatar    string         `gorm:"column:avatar;type:varchar(255);not null;default:https://cube.elemecdn.com/0/88/03b0d39583f48206768a7534e55bcpng.png;comment:群头像URL"`
	Status    int8           `gorm:"column:status;not null;default:0;comment:状态,0.正常 1.禁用 2.解散"`
	CreatedAt time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (GroupInfo) TableName() string {
	return "group_info"
}
