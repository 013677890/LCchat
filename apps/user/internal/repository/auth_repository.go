package repository

import (
	"ChatServer/model"
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// authRepositoryImpl 认证相关数据访问层实现
type authRepositoryImpl struct {
	db          *gorm.DB
	redisClient *redis.Client
}

// NewAuthRepository 创建认证仓储实例
func NewAuthRepository(db *gorm.DB, redisClient *redis.Client) IAuthRepository {
	return &authRepositoryImpl{db: db, redisClient: redisClient}
}

// GetByPhone 根据手机号查询用户信息
func (r *authRepositoryImpl) GetByPhone(ctx context.Context, telephone string) (*model.UserInfo, error) {
	return nil, nil // TODO: 根据手机号查询用户信息
}

// GetByEmail 根据邮箱查询用户信息
func (r *authRepositoryImpl) GetByEmail(ctx context.Context, email string) (*model.UserInfo, error) {
	var user model.UserInfo
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, WrapDBError(err)
	}
	return &user, nil
}

// VerifyVerifyCode 校验验证码
// type: 验证码类型 (1:注册 2:登录 3:重置密码 4:换绑邮箱)
func (r *authRepositoryImpl) VerifyVerifyCode(ctx context.Context, email, verifyCode string, codeType int32) (bool, error) {
	// 从Redis中获取验证码
	// 格式：user:verify_code:{email}:{type}
	verifyCodeKey := fmt.Sprintf("user:verify_code:%s:%d", email, codeType)
	verifyCodeValue, err := r.redisClient.Get(ctx, verifyCodeKey).Result()
	if err != nil {
		return false, WrapRedisError(err)
	}
	return verifyCodeValue == verifyCode, nil
}

// StoreVerifyCode 存储验证码到Redis（带过期时间）
// type: 验证码类型 (1:注册 2:登录 3:重置密码 4:换绑邮箱)
func (r *authRepositoryImpl) StoreVerifyCode(ctx context.Context, email, verifyCode string, codeType int32, expireDuration time.Duration) error {
	// 格式：user:verify_code:{email}:{type}
	verifyCodeKey := fmt.Sprintf("user:verify_code:%s:%d", email, codeType)

	// 使用 Set 方法设置值并指定过期时间
	err := r.redisClient.Set(ctx, verifyCodeKey, verifyCode, expireDuration).Err()
	if err != nil {
		return WrapRedisError(err)
	}
	return nil
}

// DeleteVerifyCode 删除验证码（消耗验证码）
// type: 验证码类型 (1:注册 2:登录 3:重置密码 4:换绑邮箱)
func (r *authRepositoryImpl) DeleteVerifyCode(ctx context.Context, email string, codeType int32) error {
	// 格式：user:verify_code:{email}:{type}
	verifyCodeKey := fmt.Sprintf("user:verify_code:%s:%d", email, codeType)
	err := r.redisClient.Del(ctx, verifyCodeKey).Err()
	if err != nil {
		return WrapRedisError(err)
	}
	return nil
}

// ExistsByPhone 检查手机号是否已存在
func (r *authRepositoryImpl) ExistsByPhone(ctx context.Context, telephone string) (bool, error) {
	return false, nil // TODO: 检查手机号是否已存在
}

// ExistsByEmail 检查邮箱是否已存在
func (r *authRepositoryImpl) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return false, nil // TODO: 检查邮箱是否已存在
}

// Create 创建新用户
func (r *authRepositoryImpl) Create(ctx context.Context, user *model.UserInfo) (*model.UserInfo, error) {
	err := r.db.WithContext(ctx).Create(user).Error
	if err != nil {
		return nil, WrapDBError(err)
	}
	return user, nil
}

// UpdateLastLogin 更新最后登录时间
func (r *authRepositoryImpl) UpdateLastLogin(ctx context.Context, userUUID string) error {
	return nil // TODO: 更新最后登录时间
}

// UpdatePassword 更新密码
func (r *authRepositoryImpl) UpdatePassword(ctx context.Context, userUUID, password string) error {
	err := r.db.WithContext(ctx).Model(&model.UserInfo{}).
		Where("uuid = ?", userUUID).
		Update("password", password).Error
	if err != nil {
		return WrapDBError(err)
	}
	return nil
}

// VerifyVerifyCodeRateLimit 验证码限流校验
// 返回值: true=触发限流(不允许发送), false=未触发限流(允许发送)
func (r *authRepositoryImpl) VerifyVerifyCodeRateLimit(ctx context.Context, email, ip string) (bool, error) {
	// ==================== 1分钟限流（基于邮箱）====================
	minuteKey := fmt.Sprintf("user:verify_code:1m:%s", email)
	minuteCount, err := r.redisClient.Get(ctx, minuteKey).Int()
	if err != nil && err != redis.Nil {
		return false, WrapRedisError(err)
	}

	if minuteCount >= 1 {
		return true, nil // 1分钟内已发送过，限流
	}

	// ==================== 24小时限流（基于邮箱）====================
	hour24Key := fmt.Sprintf("user:verify_code:24h:%s", email)
	hour24Count, err := r.redisClient.Get(ctx, hour24Key).Int()
	if err != nil && err != redis.Nil {
		return false, WrapRedisError(err)
	}

	if hour24Count >= 10 {
		return true, nil // 24小时内已发送超过10次，限流
	}

	// ==================== 1小时限流（基于IP）====================
	hour1Key := fmt.Sprintf("user:verify_code:1h:%s", ip)
	hour1Count, err := r.redisClient.Get(ctx, hour1Key).Int()
	if err != nil && err != redis.Nil {
		return false, WrapRedisError(err)
	}

	if hour1Count >= 100 {
		return true, nil // 1小时内该IP已发送超过100次，限流
	}

	return false, nil // 未触发限流，允许发送
}

// IncrementVerifyCodeCount 递增验证码发送计数（发送验证码时调用）
func (r *authRepositoryImpl) IncrementVerifyCodeCount(ctx context.Context, email, ip string) error {
	// 使用 Lua 脚本保证原子性：只在首次创建时设置过期时间
	// 使用pipe包装，保证原子性
	pipe := r.redisClient.Pipeline()

	// 1分钟计数器（过期时间60秒）
	minuteKey := fmt.Sprintf("user:verify_code:1m:%s", email)
	if _, err := pipe.Eval(ctx, luaIncrementWithExpire, []string{minuteKey}, 60).Result(); err != nil {
		return WrapRedisError(err)
	}

	// 24小时计数器（过期时间24小时 = 86400秒）
	hour24Key := fmt.Sprintf("user:verify_code:24h:%s", email)
	if _, err := pipe.Eval(ctx, luaIncrementWithExpire, []string{hour24Key}, 86400).Result(); err != nil {
		return WrapRedisError(err)
	}

	// 1小时IP计数器（过期时间1小时 = 3600秒）
	hour1Key := fmt.Sprintf("user:verify_code:1h:%s", ip)
	if _, err := pipe.Eval(ctx, luaIncrementWithExpire, []string{hour1Key}, 3600).Result(); err != nil {
		return WrapRedisError(err)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return WrapRedisError(err)
	}

	return nil
}
