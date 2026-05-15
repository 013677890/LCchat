package model

import (
	"gorm.io/gorm"
	"time"
)

// GroupJoinRequest 维护 group-service 独占的入群申请记录。
//
// 该表只描述“申请进入某个群”的业务事实，不和好友申请共表，原因是：
//  1. 群申请有 owner/admin 审批、多次流转和缓存投影等独立规则；
//  2. 避免 relation-service 与 group-service 共同拥有同一张申请表；
//  3. 后续扩展审批备注、操作人和专属索引时不会污染其他领域。
type GroupJoinRequest struct {
	Id            int64          `gorm:"column:id;primaryKey;autoIncrement;comment:自增id"`
	GroupUuid     string         `gorm:"column:group_uuid;type:char(20);not null;index:idx_group_join_pending;comment:群uuid"`
	ApplicantUuid string         `gorm:"column:applicant_uuid;type:char(20);not null;index:idx_group_join_applicant;comment:申请人uuid"`
	Status        int8           `gorm:"column:status;not null;default:0;comment:0待处理 1通过 2拒绝 3撤销"`
	Reason        string         `gorm:"column:reason;type:varchar(255);comment:申请附言"`
	ReviewerUuid  string         `gorm:"column:reviewer_uuid;type:char(20);comment:处理人uuid"`
	ReviewRemark  string         `gorm:"column:review_remark;type:varchar(255);comment:处理备注"`
	ReviewedAt    *time.Time     `gorm:"column:reviewed_at;comment:处理时间"`
	CreatedAt     time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt     gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

// TableName 返回入群申请表名。
func (GroupJoinRequest) TableName() string { return "group_join_requests" }
