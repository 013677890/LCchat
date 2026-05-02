package repository

import (
	"context"
	"time"

	"github.com/013677890/LCchat-Backend/apps/auth/mq"
	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// authRepositoryImpl 实现认证与账号安全相关的数据访问逻辑。
type authRepositoryImpl struct {
	db          *gorm.DB
	redisClient *redis.Client
}

// NewAuthRepository 创建认证仓储实例。
func NewAuthRepository(db *gorm.DB, redisClient *redis.Client) IAuthRepository {
	return &authRepositoryImpl{db: db, redisClient: redisClient}
}

// GetByEmail 根据邮箱查询可登录账号。
func (r *authRepositoryImpl) GetByEmail(ctx context.Context, email string) (*model.UserInfo, error) {
	var user model.UserInfo
	err := r.db.WithContext(ctx).Where("email = ? AND deleted_at IS NULL", email).First(&user).Error
	if err != nil {
		return nil, WrapDBError(err)
	}
	return &user, nil
}

// GetByUserUUID 根据用户 UUID 查询可登录账号。
func (r *authRepositoryImpl) GetByUserUUID(ctx context.Context, userUUID string) (*model.UserInfo, error) {
	var user model.UserInfo
	err := r.db.WithContext(ctx).Where("uuid = ? AND deleted_at IS NULL", userUUID).First(&user).Error
	if err != nil {
		return nil, WrapDBError(err)
	}
	return &user, nil
}

// ExistsByEmail 检查邮箱是否已被占用。
func (r *authRepositoryImpl) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.UserInfo{}).Where("email = ? AND deleted_at IS NULL", email).Count(&count).Error
	if err != nil {
		return false, WrapDBError(err)
	}
	return count > 0, nil
}

// Create 创建账号记录。
func (r *authRepositoryImpl) Create(ctx context.Context, user *model.UserInfo) (*model.UserInfo, error) {
	if err := r.createUser(r.db.WithContext(ctx), user); err != nil {
		return nil, err
	}
	return user, nil
}

// CreateWithOutboxEvent 在创建账号的同一事务中追加 Outbox 事件。
func (r *authRepositoryImpl) CreateWithOutboxEvent(ctx context.Context, user *model.UserInfo, eventType, payload string) (*model.UserInfo, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.createUser(tx, user); err != nil {
			return err
		}
		if err := outbox.InsertEvent(tx, eventType, user.Uuid, payload); err != nil {
			return WrapDBError(err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return user, nil
}

// UpdatePassword 更新密码哈希。
func (r *authRepositoryImpl) UpdatePassword(ctx context.Context, userUUID, password string) error {
	err := r.db.WithContext(ctx).Model(&model.UserInfo{}).
		Where("uuid = ? AND deleted_at IS NULL", userUUID).
		Update("password", password).Error
	if err != nil {
		return WrapDBError(err)
	}
	r.invalidateUserCache(ctx, userUUID)
	return nil
}

// UpdateEmail 更新邮箱。
func (r *authRepositoryImpl) UpdateEmail(ctx context.Context, userUUID, email string) error {
	err := r.db.WithContext(ctx).Model(&model.UserInfo{}).
		Where("uuid = ? AND deleted_at IS NULL", userUUID).
		Update("email", email).Error
	if err != nil {
		return WrapDBError(err)
	}
	r.invalidateUserCache(ctx, userUUID)
	return nil
}

// UpdateLoginDisplay 更新登录态依赖的昵称和头像字段。
func (r *authRepositoryImpl) UpdateLoginDisplay(ctx context.Context, userUUID, nickname, avatar string) error {
	updates := map[string]interface{}{"updated_at": time.Now()}
	if nickname != "" {
		updates["nickname"] = nickname
	}
	if avatar != "" {
		updates["avatar"] = avatar
	}
	err := r.db.WithContext(ctx).Model(&model.UserInfo{}).
		Where("uuid = ? AND deleted_at IS NULL", userUUID).
		Updates(updates).Error
	if err != nil {
		return WrapDBError(err)
	}
	r.invalidateUserCache(ctx, userUUID)
	return nil
}

// Delete 软删除账号。
func (r *authRepositoryImpl) Delete(ctx context.Context, userUUID string) error {
	err := r.db.WithContext(ctx).Where("uuid = ? AND deleted_at IS NULL", userUUID).Delete(&model.UserInfo{}).Error
	if err != nil {
		return WrapDBError(err)
	}
	r.invalidateUserCache(ctx, userUUID)
	return nil
}

// DeleteWithOutboxEvent 在软删除账号的同一事务中追加 Outbox 事件。
func (r *authRepositoryImpl) DeleteWithOutboxEvent(ctx context.Context, userUUID, eventType, payload string) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("uuid = ? AND deleted_at IS NULL", userUUID).Delete(&model.UserInfo{}).Error; err != nil {
			return WrapDBError(err)
		}
		if err := outbox.InsertEvent(tx, eventType, userUUID, payload); err != nil {
			return WrapDBError(err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	r.invalidateUserCache(ctx, userUUID)
	return nil
}

// VerifyVerifyCode 校验验证码是否匹配。
func (r *authRepositoryImpl) VerifyVerifyCode(ctx context.Context, email, verifyCode string, codeType int32) (bool, error) {
	if r.redisClient == nil {
		return false, ErrRedisNil
	}
	value, err := r.redisClient.Get(ctx, rediskey.VerifyCodeKey(email, codeType)).Result()
	if err != nil {
		return false, WrapRedisError(err)
	}
	return value == verifyCode, nil
}

// StoreVerifyCode 存储验证码。
func (r *authRepositoryImpl) StoreVerifyCode(ctx context.Context, email, verifyCode string, codeType int32, expireDuration time.Duration) error {
	if r.redisClient == nil {
		return ErrRedis
	}
	key := rediskey.VerifyCodeKey(email, codeType)
	err := r.redisClient.Set(ctx, key, verifyCode, expireDuration).Err()
	if err != nil {
		// 验证码写入属于高可靠 Redis 写，失败时保留同步报错并追加异步补偿。
		task := mq.BuildSetTask(key, verifyCode, expireDuration).
			WithSource("AuthRepository.StoreVerifyCode").
			WithMaxRetries(5)
		LogAndRetryRedisError(ctx, task, err)
		return WrapRedisError(err)
	}
	return nil
}

// DeleteVerifyCode 删除验证码。
func (r *authRepositoryImpl) DeleteVerifyCode(ctx context.Context, email string, codeType int32) error {
	if r.redisClient == nil {
		return nil
	}
	key := rediskey.VerifyCodeKey(email, codeType)
	err := r.redisClient.Del(ctx, key).Err()
	if err != nil {
		// 验证码删除允许短暂最终一致，失败后通过补偿任务兜底清理。
		task := mq.BuildDelTask(key).
			WithSource("AuthRepository.DeleteVerifyCode").
			WithMaxRetries(5)
		LogAndRetryRedisError(ctx, task, err)
		return WrapRedisError(err)
	}
	return nil
}

// VerifyVerifyCodeRateLimit 校验验证码发送限流。
func (r *authRepositoryImpl) VerifyVerifyCodeRateLimit(ctx context.Context, email, ip string) (bool, error) {
	if r.redisClient == nil {
		return false, nil
	}

	minuteCount, err := r.redisClient.Get(ctx, rediskey.VerifyCodeMinuteKey(email)).Int()
	if err != nil && err != redis.Nil {
		return false, WrapRedisError(err)
	}
	if minuteCount >= 1 {
		return true, nil
	}

	hour24Count, err := r.redisClient.Get(ctx, rediskey.VerifyCode24HKey(email)).Int()
	if err != nil && err != redis.Nil {
		return false, WrapRedisError(err)
	}
	if hour24Count >= 10 {
		return true, nil
	}

	hour1Count, err := r.redisClient.Get(ctx, rediskey.VerifyCodeIPKey(ip)).Int()
	if err != nil && err != redis.Nil {
		return false, WrapRedisError(err)
	}
	if hour1Count >= 100 {
		return true, nil
	}

	return false, nil
}

// IncrementVerifyCodeCount 增加验证码发送计数。
func (r *authRepositoryImpl) IncrementVerifyCodeCount(ctx context.Context, email, ip string) error {
	if r.redisClient == nil {
		return nil
	}
	// 验证码限流计数具有强时序语义，失败后直接返回，避免异步重放放大计数。
	pipe := r.redisClient.Pipeline()
	if _, err := pipe.Eval(ctx, luaIncrementWithExpire, []string{rediskey.VerifyCodeMinuteKey(email)}, int(rediskey.VerifyCodeMinuteTTL.Seconds())).Result(); err != nil {
		return WrapRedisError(err)
	}
	if _, err := pipe.Eval(ctx, luaIncrementWithExpire, []string{rediskey.VerifyCode24HKey(email)}, int(rediskey.VerifyCode24HTTL.Seconds())).Result(); err != nil {
		return WrapRedisError(err)
	}
	if _, err := pipe.Eval(ctx, luaIncrementWithExpire, []string{rediskey.VerifyCodeIPKey(ip)}, int(rediskey.VerifyCodeIPTTL.Seconds())).Result(); err != nil {
		return WrapRedisError(err)
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return WrapRedisError(err)
	}
	return nil
}

// BatchGetAccountStatus 批量查询账号存在性与状态。
func (r *authRepositoryImpl) BatchGetAccountStatus(ctx context.Context, userUUIDs []string) ([]*AccountStatusItem, error) {
	if len(userUUIDs) == 0 {
		return []*AccountStatusItem{}, nil
	}

	type rawAccountStatus struct {
		Uuid      string         `gorm:"column:uuid"`
		DeletedAt gorm.DeletedAt `gorm:"column:deleted_at"`
	}

	rows := make([]*rawAccountStatus, 0, len(userUUIDs))
	err := r.db.WithContext(ctx).Unscoped().Model(&model.UserInfo{}).
		Select("uuid, deleted_at").
		Where("uuid IN ?", userUUIDs).
		Find(&rows).Error
	if err != nil {
		return nil, WrapDBError(err)
	}

	statusMap := make(map[string]*AccountStatusItem, len(rows))
	for _, row := range rows {
		if row == nil || row.Uuid == "" {
			continue
		}
		item := &AccountStatusItem{UserUUID: row.Uuid, Exists: true, Status: 0}
		if row.DeletedAt.Valid {
			item.Status = 1
		}
		statusMap[row.Uuid] = item
	}

	items := make([]*AccountStatusItem, 0, len(userUUIDs))
	seen := make(map[string]struct{}, len(userUUIDs))
	for _, userUUID := range userUUIDs {
		if userUUID == "" {
			continue
		}
		if _, ok := seen[userUUID]; ok {
			continue
		}
		seen[userUUID] = struct{}{}
		if item, ok := statusMap[userUUID]; ok {
			items = append(items, item)
			continue
		}
		items = append(items, &AccountStatusItem{UserUUID: userUUID, Exists: false, Status: 1})
	}
	return items, nil
}

// invalidateUserCache 删除登录态依赖的用户缓存，并在失败时投递补偿任务。
func (r *authRepositoryImpl) invalidateUserCache(ctx context.Context, userUUID string) {
	if r.redisClient == nil || userUUID == "" {
		return
	}
	key := rediskey.UserInfoKey(userUUID)
	if err := r.redisClient.Del(ctx, key).Err(); err != nil {
		// 用户信息缓存失效允许最终一致，失败后异步补偿即可。
		task := mq.BuildDelTask(key).
			WithSource("AuthRepository.invalidateUserCache")
		LogAndRetryRedisError(ctx, task, err)
	}
}

func (r *authRepositoryImpl) createUser(db *gorm.DB, user *model.UserInfo) error {
	if user.Telephone == "" {
		if err := db.Omit("Telephone").Create(user).Error; err != nil {
			return WrapDBError(err)
		}
		return nil
	}
	if err := db.Create(user).Error; err != nil {
		return WrapDBError(err)
	}
	return nil
}
