package model

import (
	"time"

	"gorm.io/gorm"
)

// GroupInfo 描述群基础资料与群级管理开关。
//
// 群成员、群名片、单人禁言等成员维度事实不放在这里，避免群表承载过多职责；
// 这里只保留对整个群生效的资料字段和全员禁言等群级策略。
type GroupInfo struct {
	Id        int64  `gorm:"column:id;primaryKey;autoIncrement;comment:自增id"`
	Uuid      string `gorm:"column:uuid;type:char(20);uniqueIndex;not null;comment:群组唯一id"`
	Name      string `gorm:"column:name;type:varchar(64);not null;comment:群名称"`
	Notice    string `gorm:"column:notice;type:varchar(500);comment:群公告"`
	MemberCnt int    `gorm:"column:member_cnt;not null;default:1;comment:群人数"` // 默认群主1人
	OwnerUuid string `gorm:"column:owner_uuid;type:char(20);not null;index;comment:群主uuid"`
	AddMode   int8   `gorm:"column:add_mode;not null;default:0;comment:加群方式,0.直接 1.审核"`
	MuteAll   bool   `gorm:"column:mute_all;not null;default:false;comment:是否全员禁言"`
	Avatar    string `gorm:"column:avatar;type:varchar(255);not null;default:https://cube.elemecdn.com/0/88/03b0d39583f48206768a7534e55bcpng.png;comment:群头像URL"`
	Status    int8   `gorm:"column:status;not null;default:0;comment:状态,0.正常 1.禁用 2.解散"`
	// CacheVersion 是群聚合的严格递增缓存投影版本。
	//
	// 每写入一条 group.cache outbox 事件，都必须在同一个数据库事务里先将该值加一，
	// 再把新值写进事件。它不是展示字段，也不能由客户端传入；唯一职责是让 Redis
	// 在 Lua 内拒绝重复或乱序事件，并给读回填/定时对账提供同一套版本栅栏。
	CacheVersion int64          `gorm:"column:cache_version;not null;default:0;comment:群缓存投影严格递增版本"`
	CreatedAt    time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt    gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

// TableName 返回群基础资料表名。
func (GroupInfo) TableName() string {
	return "groups"
}
