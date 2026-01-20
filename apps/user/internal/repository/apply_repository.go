package repository

import (
	"ChatServer/model"
	"context"

	"gorm.io/gorm"
)

// applyRepositoryImpl 好友申请数据访问层实现
type applyRepositoryImpl struct {
	db *gorm.DB
}

// NewApplyRepository 创建好友申请仓储实例
func NewApplyRepository(db *gorm.DB) IApplyRepository {
	return &applyRepositoryImpl{db: db}
}

// Create 创建好友申请
func (r *applyRepositoryImpl) Create(ctx context.Context, apply *model.ApplyRequest) (*model.ApplyRequest, error) {
	err := r.db.WithContext(ctx).Create(apply).Error
	if err != nil {
		return nil, err
	}
	return apply, nil
}

// GetByID 根据ID获取好友申请
func (r *applyRepositoryImpl) GetByID(ctx context.Context, id int64) (*model.ApplyRequest, error) {
	var apply model.ApplyRequest
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&apply).Error
	if err != nil {
		return nil, err
	}
	return &apply, nil
}

// GetPendingList 获取待处理的好友申请列表
func (r *applyRepositoryImpl) GetPendingList(ctx context.Context, targetUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error) {
	var applies []*model.ApplyRequest
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("target_uuid = ? AND apply_type = 0", targetUUID)

	if status > 0 {
		query = query.Where("status = ?", status)
	}

	// 统计总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	err := query.Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&applies).Error

	if err != nil {
		return nil, 0, err
	}

	return applies, total, nil
}

// GetSentList 获取发出的好友申请列表
func (r *applyRepositoryImpl) GetSentList(ctx context.Context, applicantUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error) {
	var applies []*model.ApplyRequest
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("applicant_uuid = ? AND apply_type = 0", applicantUUID)

	if status > 0 {
		query = query.Where("status = ?", status)
	}

	// 统计总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	err := query.Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&applies).Error

	if err != nil {
		return nil, 0, err
	}

	return applies, total, nil
}

// UpdateStatus 更新申请状态
func (r *applyRepositoryImpl) UpdateStatus(ctx context.Context, id int64, status int, remark string) error {
	updates := map[string]interface{}{
		"status": status,
		"is_read": true,
	}

	if remark != "" {
		updates["handle_remark"] = remark
	}

	return r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// MarkAsRead 标记申请已读
func (r *applyRepositoryImpl) MarkAsRead(ctx context.Context, ids []int64) error {
	return r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("id IN ?", ids).
		Update("is_read", true).Error
}

// GetUnreadCount 获取未读申请数量
func (r *applyRepositoryImpl) GetUnreadCount(ctx context.Context, targetUUID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("target_uuid = ? AND apply_type = 0 AND is_read = false", targetUUID).
		Count(&count).Error

	if err != nil {
		return 0, err
	}

	return count, nil
}

// ExistsPendingRequest 检查是否存在待处理的申请
func (r *applyRepositoryImpl) ExistsPendingRequest(ctx context.Context, applicantUUID, targetUUID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("applicant_uuid = ? AND target_uuid = ? AND apply_type = 0 AND status = 0", applicantUUID, targetUUID).
		Count(&count).Error

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetByIDWithInfo 根据ID获取好友申请（包含申请人信息）
func (r *applyRepositoryImpl) GetByIDWithInfo(ctx context.Context, id int64) (*model.ApplyRequest, *model.UserInfo, error) {
	var apply model.ApplyRequest
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&apply).Error
	if err != nil {
		return nil, nil, err
	}

	var user model.UserInfo
	err = r.db.WithContext(ctx).Where("uuid = ?", apply.ApplicantUuid).First(&user).Error
	if err != nil {
		return &apply, nil, err
	}

	return &apply, &user, nil
}
