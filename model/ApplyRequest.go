package model

import (
	"time"

	"gorm.io/gorm"
)

// ApplyRequest 合并“加好友/加群”的申请记录。
// apply_type: 0=好友申请，1=加群申请
// status: 0=待处理 1=通过 2=拒绝 3=已过期
type ApplyRequest struct {
	Id             int64          `gorm:"column:id;primaryKey;autoIncrement;comment:自增id"`
	ApplyType      int8           `gorm:"column:apply_type;not null;comment:0好友 1加群"`
	ApplicantUuid  string         `gorm:"column:applicant_uuid;type:char(20);not null;index:idx_applicant_target;comment:申请人uuid"`
	TargetUuid     string         `gorm:"column:target_uuid;type:char(20);not null;index:idx_applicant_target;comment:好友申请为目标用户uuid;加群为群uuid"`
	Status         int8           `gorm:"column:status;not null;default:0;comment:0待处理 1通过 2拒绝 3过期"`
	IsRead         bool           `gorm:"column:is_read;not null;default:false;comment:申请是否已读"`
	Reason         string         `gorm:"column:reason;type:varchar(255);comment:申请附言"`
	HandleUserUuid string         `gorm:"column:handle_user_uuid;type:char(20);comment:处理人uuid(好友为目标用户;群为管理员/群主)"`
	HandleRemark   string         `gorm:"column:handle_remark;type:varchar(255);comment:处理备注"`
	ExpiredAt      *time.Time     `gorm:"column:expired_at;comment:过期时间"`
	CreatedAt      time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt      gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (ApplyRequest) TableName() string { return "apply_request" }


//如果一个人多次申请
//应该 找到之前那条 Status=0 的旧记录，更新它的 UpdatedAt 时间，并把 IsRead 重置为 0。
//原因： 防止 B 的列表里出现 10 条全是 A 发来的申请，体验很差。