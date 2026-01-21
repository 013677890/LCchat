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
	return nil, nil // TODO: 创建好友申请
}

// GetByID 根据ID获取好友申请
func (r *applyRepositoryImpl) GetByID(ctx context.Context, id int64) (*model.ApplyRequest, error) {
	return nil, nil // TODO: 根据ID获取好友申请
}

// GetPendingList 获取待处理的好友申请列表
func (r *applyRepositoryImpl) GetPendingList(ctx context.Context, targetUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error) {
	return nil, 0, nil // TODO: 获取待处理的好友申请列表
}

// GetSentList 获取发出的好友申请列表
func (r *applyRepositoryImpl) GetSentList(ctx context.Context, applicantUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error) {
	return nil, 0, nil // TODO: 获取发出的好友申请列表
}

// UpdateStatus 更新申请状态
func (r *applyRepositoryImpl) UpdateStatus(ctx context.Context, id int64, status int, remark string) error {
	return nil // TODO: 更新申请状态
}

// MarkAsRead 标记申请已读
func (r *applyRepositoryImpl) MarkAsRead(ctx context.Context, ids []int64) error {
	return nil // TODO: 标记申请已读
}

// GetUnreadCount 获取未读申请数量
func (r *applyRepositoryImpl) GetUnreadCount(ctx context.Context, targetUUID string) (int64, error) {
	return 0, nil // TODO: 获取未读申请数量
}

// ExistsPendingRequest 检查是否存在待处理的申请
func (r *applyRepositoryImpl) ExistsPendingRequest(ctx context.Context, applicantUUID, targetUUID string) (bool, error) {
	return false, nil // TODO: 检查是否存在待处理的申请
}

// GetByIDWithInfo 根据ID获取好友申请（包含申请人信息）
func (r *applyRepositoryImpl) GetByIDWithInfo(ctx context.Context, id int64) (*model.ApplyRequest, *model.UserInfo, error) {
	return nil, nil, nil // TODO: 根据ID获取好友申请（包含申请人信息）
}
