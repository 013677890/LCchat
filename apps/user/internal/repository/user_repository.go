package repository

import (
	"ChatServer/apps/user/mq"
	"ChatServer/model"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// userRepositoryImpl 用户信息数据访问层实现
type userRepositoryImpl struct {
	db          *gorm.DB
	redisClient *redis.Client
}

// NewUserRepository 创建用户信息仓储实例
func NewUserRepository(db *gorm.DB, redisClient *redis.Client) IUserRepository {
	return &userRepositoryImpl{db: db, redisClient: redisClient}
}

// GetByUUID 根据UUID查询用户信息
func (r *userRepositoryImpl) GetByUUID(ctx context.Context, uuid string) (*model.UserInfo, error) {
	// ==================== 1. 先从 Redis 缓存中查询 ====================
	cacheKey := fmt.Sprintf("user:info:%s", uuid)
	cachedData, err := r.redisClient.Get(ctx, cacheKey).Result()
	if err == nil {
		// 缓存命中，反序列化返回
		// 先判空
		if cachedData == "{}" {
			return nil, nil
		}
		var user model.UserInfo
		if err := json.Unmarshal([]byte(cachedData), &user); err == nil {
			return &user, nil
		}
	}
	if err != nil && err != redis.Nil {
		LogRedisError(ctx, err) // 记录日志 降级处理
	}

	// ==================== 2. 缓存未命中，查询 MySQL ====================
	var user model.UserInfo
	err = r.db.WithContext(ctx).Where("uuid = ? AND deleted_at IS NULL", uuid).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 存一份空到redis 5min过期
			randomDuration := getRandomExpireTime(5 * time.Minute)
			err = r.redisClient.Set(ctx, cacheKey, "{}", randomDuration).Err()
			if err != nil {
				// 发送到重试队列
				task := mq.BuildSetTask(cacheKey, "{}", randomDuration).
					WithSource("UserRepository.GetByUUID.EmptyCache")
				LogAndRetryRedisError(ctx, task, err)
			}
			return nil, nil
		} else {
			return nil, WrapDBError(err)
		}
	}

	// ==================== 3. 存入 Redis 缓存 ====================
	// 序列化用户信息
	userJSON, err := json.Marshal(user)
	if err != nil {
		// 序列化失败，不影响主流程，只返回数据库数据
		return &user, nil
	}

	// 存入缓存，设置过期时间为 1 小时（+-5min缓冲）
	// 随机时间防止缓存雪崩
	randomDuration := time.Duration(rand.Intn(10)) * time.Minute
	ttl := 1*time.Hour - randomDuration
	err = r.redisClient.Set(ctx, cacheKey, userJSON, ttl).Err()
	if err != nil {
		// 发送到重试队列，不影响主流程
		task := mq.BuildSetTask(cacheKey, string(userJSON), ttl).
			WithSource("UserRepository.GetByUUID.SetCache")
		LogAndRetryRedisError(ctx, task, err)
		return &user, nil
	}

	return &user, nil
}

// GetByPhone 根据手机号查询用户信息
func (r *userRepositoryImpl) GetByPhone(ctx context.Context, telephone string) (*model.UserInfo, error) {
	return nil, nil // TODO: 实现查询用户信息
}

// BatchGetByUUIDs 批量查询用户信息
func (r *userRepositoryImpl) BatchGetByUUIDs(ctx context.Context, uuids []string) ([]*model.UserInfo, error) {
	return nil, nil // TODO: 实现批量查询用户信息
}

// Update 更新用户信息
func (r *userRepositoryImpl) Update(ctx context.Context, user *model.UserInfo) (*model.UserInfo, error) {
	return nil, nil // TODO: 实现更新用户信息
}

// UpdateAvatar 更新用户头像
func (r *userRepositoryImpl) UpdateAvatar(ctx context.Context, userUUID, avatar string) error {
	return nil // TODO: 实现更新用户头像
}

// UpdateBasicInfo 更新基本信息
func (r *userRepositoryImpl) UpdateBasicInfo(ctx context.Context, userUUID string, nickname, signature, birthday string, gender int8) error {
	// 构造更新字段
	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}

	if nickname != "" {
		updates["nickname"] = nickname
	}
	if signature != "" {
		updates["signature"] = signature
	}
	if birthday != "" {
		updates["birthday"] = birthday
	}
	if gender > 0 {
		updates["gender"] = gender
	}

	// 执行更新
	err := r.db.WithContext(ctx).
		Model(&model.UserInfo{}).
		Where("uuid = ? AND deleted_at IS NULL", userUUID).
		Updates(updates).
		Error
	if err != nil {
		return WrapDBError(err)
	}

	// 更新成功后，删除Redis缓存
	cacheKey := fmt.Sprintf("user:info:%s", userUUID)
	err = r.redisClient.Del(ctx, cacheKey).Err()
	if err != nil {
		// 发送到重试队列
		task := mq.BuildDelTask(cacheKey).
			WithSource("UserRepository.UpdateNickname")
		LogAndRetryRedisError(ctx, task, err)
	}

	return nil
}

// UpdateEmail 更新邮箱
func (r *userRepositoryImpl) UpdateEmail(ctx context.Context, userUUID, email string) error {
	// 更新邮箱到数据库
	err := r.db.WithContext(ctx).
		Model(&model.UserInfo{}).
		Where("uuid = ? AND deleted_at IS NULL", userUUID).
		Update("email", email).
		Error
	if err != nil {
		return WrapDBError(err)
	}

	// 更新成功后，删除Redis缓存
	cacheKey := fmt.Sprintf("user:info:%s", userUUID)
	err = r.redisClient.Del(ctx, cacheKey).Err()
	if err != nil {
		// 发送到重试队列
		task := mq.BuildDelTask(cacheKey).
			WithSource("UserRepository.UpdateEmail")
		LogAndRetryRedisError(ctx, task, err)
	}

	return nil
}

// UpdateTelephone 更新手机号
func (r *userRepositoryImpl) UpdateTelephone(ctx context.Context, userUUID, telephone string) error {
	return nil // TODO: 实现更新手机号
}

// Delete 软删除用户
func (r *userRepositoryImpl) Delete(ctx context.Context, userUUID string) error {
	return nil // TODO: 实现软删除用户
}

// ExistsByPhone 检查手机号是否已存在
func (r *userRepositoryImpl) ExistsByPhone(ctx context.Context, telephone string) (bool, error) {
	return false, nil // TODO: 实现检查手机号是否已存在
}

// ExistsByEmail 检查邮箱是否已存在
func (r *userRepositoryImpl) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.UserInfo{}).
		Where("email = ? AND deleted_at IS NULL", email).
		Count(&count).
		Error
	if err != nil {
		return false, WrapDBError(err)
	}
	return count > 0, nil
}

// UpdatePassword 更新密码
func (r *userRepositoryImpl) UpdatePassword(ctx context.Context, userUUID, password string) error {
	// 更新密码到数据库
	err := r.db.WithContext(ctx).
		Model(&model.UserInfo{}).
		Where("uuid = ? AND deleted_at IS NULL", userUUID).
		Update("password", password).
		Error
	if err != nil {
		return WrapDBError(err)
	}

	// 更新成功后，删除Redis缓存
	cacheKey := fmt.Sprintf("user:info:%s", userUUID)
	err = r.redisClient.Del(ctx, cacheKey).Err()
	if err != nil {
		// 发送到重试队列
		task := mq.BuildDelTask(cacheKey).
			WithSource("UserRepository.UpdatePassword")
		LogAndRetryRedisError(ctx, task, err)
	}

	return nil
}
