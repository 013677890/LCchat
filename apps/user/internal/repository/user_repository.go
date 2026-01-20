package repository

import (
	"ChatServer/model"
	"context"

	"gorm.io/gorm"
)

// userRepositoryImpl 用户信息数据访问层实现
type userRepositoryImpl struct {
	db *gorm.DB
}

// NewUserRepository 创建用户信息仓储实例
func NewUserRepository(db *gorm.DB) IUserRepository {
	return &userRepositoryImpl{db: db}
}

// GetByUUID 根据UUID查询用户信息
func (r *userRepositoryImpl) GetByUUID(ctx context.Context, uuid string) (*model.UserInfo, error) {
	var user model.UserInfo
	err := r.db.WithContext(ctx).
		Where("uuid = ?", uuid).
		First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByPhone 根据手机号查询用户信息
func (r *userRepositoryImpl) GetByPhone(ctx context.Context, telephone string) (*model.UserInfo, error) {
	var user model.UserInfo
	err := r.db.WithContext(ctx).
		Where("telephone = ?", telephone).
		First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// BatchGetByUUIDs 批量查询用户信息
func (r *userRepositoryImpl) BatchGetByUUIDs(ctx context.Context, uuids []string) ([]*model.UserInfo, error) {
	if len(uuids) == 0 {
		return []*model.UserInfo{}, nil
	}

	var users []*model.UserInfo
	err := r.db.WithContext(ctx).
		Where("uuid IN ?", uuids).
		Find(&users).Error
	if err != nil {
		return nil, err
	}
	return users, nil
}

// Update 更新用户信息
func (r *userRepositoryImpl) Update(ctx context.Context, user *model.UserInfo) (*model.UserInfo, error) {
	err := r.db.WithContext(ctx).
		Model(&model.UserInfo{}).
		Where("uuid = ?", user.Uuid).
		Updates(user).Error
	if err != nil {
		return nil, err
	}
	return user, nil
}

// UpdateAvatar 更新用户头像
func (r *userRepositoryImpl) UpdateAvatar(ctx context.Context, userUUID, avatar string) error {
	return r.db.WithContext(ctx).
		Model(&model.UserInfo{}).
		Where("uuid = ?", userUUID).
		Update("avatar", avatar).Error
}

// UpdateBasicInfo 更新基本信息
func (r *userRepositoryImpl) UpdateBasicInfo(ctx context.Context, userUUID string, nickname, signature, birthday string, gender int8) error {
	updates := map[string]interface{}{}
	if nickname != "" {
		updates["nickname"] = nickname
	}
	if signature != "" {
		updates["signature"] = signature
	}
	if birthday != "" {
		updates["birthday"] = birthday
	}
	if gender >= 0 {
		updates["gender"] = gender
	}

	return r.db.WithContext(ctx).
		Model(&model.UserInfo{}).
		Where("uuid = ?", userUUID).
		Updates(updates).Error
}

// UpdateEmail 更新邮箱
func (r *userRepositoryImpl) UpdateEmail(ctx context.Context, userUUID, email string) error {
	return r.db.WithContext(ctx).
		Model(&model.UserInfo{}).
		Where("uuid = ?", userUUID).
		Update("email", email).Error
}

// UpdateTelephone 更新手机号
func (r *userRepositoryImpl) UpdateTelephone(ctx context.Context, userUUID, telephone string) error {
	return r.db.WithContext(ctx).
		Model(&model.UserInfo{}).
		Where("uuid = ?", userUUID).
		Update("telephone", telephone).Error
}

// Delete 软删除用户
func (r *userRepositoryImpl) Delete(ctx context.Context, userUUID string) error {
	return r.db.WithContext(ctx).
		Delete(&model.UserInfo{}, "uuid = ?", userUUID).Error
}

// ExistsByPhone 检查手机号是否已存在
func (r *userRepositoryImpl) ExistsByPhone(ctx context.Context, telephone string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.UserInfo{}).
		Where("telephone = ?", telephone).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ExistsByEmail 检查邮箱是否已存在
func (r *userRepositoryImpl) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.UserInfo{}).
		Where("email = ?", email).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
