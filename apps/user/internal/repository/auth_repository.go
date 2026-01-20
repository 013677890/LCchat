package repository

import (
	"ChatServer/model"
	"context"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// authRepositoryImpl 认证相关数据访问层实现
type authRepositoryImpl struct {
	db *gorm.DB
}

// NewAuthRepository 创建认证仓储实例
func NewAuthRepository(db *gorm.DB) IAuthRepository {
	return &authRepositoryImpl{db: db}
}

// GetByPhone 根据手机号查询用户信息
func (r *authRepositoryImpl) GetByPhone(ctx context.Context, telephone string) (*model.UserInfo, error) {
	var user model.UserInfo
	err := r.db.WithContext(ctx).
		Where("telephone = ? AND status = 0", telephone).
		First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByEmail 根据邮箱查询用户信息
func (r *authRepositoryImpl) GetByEmail(ctx context.Context, email string) (*model.UserInfo, error) {
	var user model.UserInfo
	err := r.db.WithContext(ctx).
		Where("email = ? AND status = 0", email).
		First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// ExistsByPhone 检查手机号是否已存在
func (r *authRepositoryImpl) ExistsByPhone(ctx context.Context, telephone string) (bool, error) {
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
func (r *authRepositoryImpl) ExistsByEmail(ctx context.Context, email string) (bool, error) {
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

// Create 创建新用户
func (r *authRepositoryImpl) Create(ctx context.Context, user *model.UserInfo) (*model.UserInfo, error) {
	// 密码加密
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user.Password = string(hashedPassword)

	err = r.db.WithContext(ctx).Create(user).Error
	if err != nil {
		return nil, err
	}
	return user, nil
}

// UpdateLastLogin 更新最后登录时间
func (r *authRepositoryImpl) UpdateLastLogin(ctx context.Context, userUUID string) error {
	return r.db.WithContext(ctx).
		Model(&model.UserInfo{}).
		Where("uuid = ?", userUUID).
		UpdateColumn("updated_at", gorm.Expr("NOW()")).Error
}

// UpdatePassword 更新密码
func (r *authRepositoryImpl) UpdatePassword(ctx context.Context, userUUID, password string) error {
	// 密码加密
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return r.db.WithContext(ctx).
		Model(&model.UserInfo{}).
		Where("uuid = ?", userUUID).
		Update("password", string(hashedPassword)).Error
}
