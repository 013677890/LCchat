package repository

import (
	"context"
	"errors"
	"github.com/013677890/LCchat-Backend/model"
	"gorm.io/gorm"
)

// loadLatestJoinRequestByApplicant 读取当前用户在指定群的最新申请记录。
//
// 这里不只看 pending，而是保留最近一次申请的终态，
// 让“我的申请状态”接口可以直接展示待审批、已通过、已拒绝、已撤销四类结果。
func (r *groupRepositoryImpl) loadLatestJoinRequestByApplicant(ctx context.Context, groupUUID, applicantUUID string) (*model.GroupJoinRequest, error) {
	if r == nil || r.db == nil || groupUUID == "" || applicantUUID == "" {
		return nil, nil
	}
	var joinRequest model.GroupJoinRequest
	err := r.db.WithContext(ctx).
		Where("group_uuid = ? AND applicant_uuid = ? AND deleted_at IS NULL", groupUUID, applicantUUID).
		Order("id DESC").
		Take(&joinRequest).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, WrapDBError(err)
	}
	return &joinRequest, nil
}
