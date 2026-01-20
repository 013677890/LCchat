package repository

import (
	"ChatServer/model"
	"context"

	"gorm.io/gorm"
)

// friendRepositoryImpl 好友关系数据访问层实现
type friendRepositoryImpl struct {
	db *gorm.DB
}

// NewFriendRepository 创建好友关系仓储实例
func NewFriendRepository(db *gorm.DB) IFriendRepository {
	return &friendRepositoryImpl{db: db}
}

// SearchUser 搜索用户（按手机号或昵称）
func (r *friendRepositoryImpl) SearchUser(ctx context.Context, keyword string, page, pageSize int) ([]*model.UserInfo, int64, error) {
	var users []*model.UserInfo
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.UserInfo{}).
		Where("status = 0")

	// 支持手机号或昵称搜索
	query = query.Where("telephone LIKE ? OR nickname LIKE ?",
		"%"+keyword+"%", "%"+keyword+"%")

	// 统计总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	err := query.Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&users).Error

	if err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// GetFriendList 获取好友列表
func (r *friendRepositoryImpl) GetFriendList(ctx context.Context, userUUID, groupTag string, page, pageSize int) ([]*model.UserRelation, int64, error) {
	var relations []*model.UserRelation
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND status = 0", userUUID)

	if groupTag != "" {
		query = query.Where("group_tag = ?", groupTag)
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
		Find(&relations).Error

	if err != nil {
		return nil, 0, err
	}

	return relations, total, nil
}

// GetFriendRelation 获取好友关系
func (r *friendRepositoryImpl) GetFriendRelation(ctx context.Context, userUUID, friendUUID string) (*model.UserRelation, error) {
	var relation model.UserRelation
	err := r.db.WithContext(ctx).
		Where("user_uuid = ? AND peer_uuid = ? AND status = 0", userUUID, friendUUID).
		First(&relation).Error
	if err != nil {
		return nil, err
	}
	return &relation, nil
}

// CreateFriendRelation 创建好友关系（双向）
func (r *friendRepositoryImpl) CreateFriendRelation(ctx context.Context, userUUID, friendUUID string) error {
	relations := []*model.UserRelation{
		{
			UserUuid:  userUUID,
			PeerUuid:  friendUUID,
			Status:    0,
		},
		{
			UserUuid:  friendUUID,
			PeerUuid:  userUUID,
			Status:    0,
		},
	}

	return r.db.WithContext(ctx).Create(&relations).Error
}

// DeleteFriendRelation 删除好友关系（单向）
func (r *friendRepositoryImpl) DeleteFriendRelation(ctx context.Context, userUUID, friendUUID string) error {
	return r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ?", userUUID, friendUUID).
		Update("status", 2).Error
}

// SetFriendRemark 设置好友备注
func (r *friendRepositoryImpl) SetFriendRemark(ctx context.Context, userUUID, friendUUID, remark string) error {
	return r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ?", userUUID, friendUUID).
		Update("remark", remark).Error
}

// SetFriendTag 设置好友标签
func (r *friendRepositoryImpl) SetFriendTag(ctx context.Context, userUUID, friendUUID, groupTag string) error {
	return r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ?", userUUID, friendUUID).
		Update("group_tag", groupTag).Error
}

// GetTagList 获取标签列表
func (r *friendRepositoryImpl) GetTagList(ctx context.Context, userUUID string) ([]string, error) {
	var tags []string
	err := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND group_tag != ''", userUUID).
		Distinct("group_tag").
		Pluck("group_tag", &tags).Error
	if err != nil {
		return nil, err
	}
	return tags, nil
}

// IsFriend 检查是否是好友
func (r *friendRepositoryImpl) IsFriend(ctx context.Context, userUUID, friendUUID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ? AND status = 0", userUUID, friendUUID).
		Count(&count).Error

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetRelationStatus 获取关系状态
func (r *friendRepositoryImpl) GetRelationStatus(ctx context.Context, userUUID, peerUUID string) (*model.UserRelation, error) {
	var relation model.UserRelation
	err := r.db.WithContext(ctx).
		Where("user_uuid = ? AND peer_uuid = ?", userUUID, peerUUID).
		First(&relation).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return &model.UserRelation{
				UserUuid: userUUID,
				PeerUuid: peerUUID,
				Status:   -1, // 表示无关系
			}, nil
		}
		return nil, err
	}
	return &relation, nil
}

// SyncFriendList 增量同步好友列表
func (r *friendRepositoryImpl) SyncFriendList(ctx context.Context, userUUID string, version int64, limit int) ([]*model.UserRelation, int64, error) {
	var relations []*model.UserRelation
	var latestVersion int64

	query := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND status = 0", userUUID)

	// 如果提供了版本号，获取该版本之后的变更
	if version > 0 {
		query = query.Where("updated_at > ?", version)
	}

	// 查询变更记录
	err := query.
		Order("updated_at ASC").
		Limit(limit).
		Find(&relations).Error

	if err != nil {
		return nil, 0, err
	}

	// 获取最新版本号
	err = r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Select("COALESCE(MAX(updated_at), 0)").
		Where("user_uuid = ? AND status = 0", userUUID).
		Scan(&latestVersion).Error

	if err != nil {
		return nil, 0, err
	}

	return relations, latestVersion, nil
}
