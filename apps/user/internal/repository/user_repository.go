package repository

import (
	"ChatServer/model"
	"ChatServer/pkg/logger"
	"context"
	"encoding/json"
	"math/rand"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// userRepositoryImpl 用户数据访问层实现
// 职责：负责用户数据的CRUD操作，集成 Redis 缓存和 MySQL 持久化
// 设计原则：
//   - 返回数据库原始错误（如gorm.ErrRecordNotFound）
//   - 不进行业务判断（如密码校验）
//   - 不进行错误转换（错误转换在Service层完成）
//   - Redis 失败时自动降级到 MySQL（Soft Fail）
type userRepositoryImpl struct {
	db    *gorm.DB
	redis *goredis.Client
}

// NewUserRepository 创建用户仓储实例
// 如果 redis 为 nil，则只使用 MySQL（降级模式）
func NewUserRepository(db *gorm.DB, redis *goredis.Client) UserRepository {
	return &userRepositoryImpl{
		db:    db,
		redis: redis,
	}
}

// ==================== 缓存相关常量和工具函数 ====================

const (
	// 缓存 Key 前缀
	cacheKeyUserInfo = "user:info:uid:"  // 用户信息缓存 user:info:uid:{uuid}
	cacheKeyPhoneIdx = "user:idx:phone:" // 手机号索引 user:idx:phone:{phone} -> uuid

	// TTL 配置
	userInfoTTLBase   = 4 * time.Hour    // 用户信息基础 TTL：4小时
	userInfoTTLJitter = 30 * time.Minute // 随机抖动：0~30分钟
	phoneIdxTTL       = 0                // 手机号索引永不过期（关系稳定）
)

// getCacheTTL 获取带随机抖动的 TTL，防止缓存雪崩
func getCacheTTL() time.Duration {
	jitter := time.Duration(rand.Int63n(int64(userInfoTTLJitter)))
	return userInfoTTLBase + jitter
}

// buildUserInfoKey 构建用户信息缓存 Key
func buildUserInfoKey(uuid string) string {
	return cacheKeyUserInfo + uuid
}

// buildPhoneIndexKey 构建手机号索引 Key
func buildPhoneIndexKey(phone string) string {
	return cacheKeyPhoneIdx + phone
}

// getUserFromCache 从 Redis 获取用户信息
// 返回 nil 表示未命中或 Redis 故障（降级）
func (r *userRepositoryImpl) getUserFromCache(ctx context.Context, uuid string) *model.UserInfo {
	if r.redis == nil {
		return nil // Redis 未初始化，直接降级
	}

	key := buildUserInfoKey(uuid)
	data, err := r.redis.Get(ctx, key).Result()

	if err == goredis.Nil {
		// 缓存未命中（正常情况）
		return nil
	}

	if err != nil {
		// Redis 故障（网络超时/连接失败等）
		logger.Warn(ctx, "Redis GET 失败，降级到 MySQL",
			logger.String("key", key),
			logger.ErrorField("error", err),
		)
		return nil
	}

	// 反序列化
	var user model.UserInfo
	if err := json.Unmarshal([]byte(data), &user); err != nil {
		logger.Error(ctx, "Redis 数据反序列化失败",
			logger.String("key", key),
			logger.ErrorField("error", err),
		)
		// 删除脏数据
		r.redis.Del(ctx, key)
		return nil
	}

	return &user
}

// setUserToCache 将用户信息写入 Redis
func (r *userRepositoryImpl) setUserToCache(ctx context.Context, user *model.UserInfo) {
	if r.redis == nil || user == nil {
		return
	}

	key := buildUserInfoKey(user.Uuid)
	data, err := json.Marshal(user)
	if err != nil {
		logger.Error(ctx, "用户信息序列化失败",
			logger.String("uuid", user.Uuid),
			logger.ErrorField("error", err),
		)
		return
	}

	ttl := getCacheTTL()
	if err := r.redis.Set(ctx, key, data, ttl).Err(); err != nil {
		// Redis 写入失败不影响业务（Soft Fail）
		logger.Warn(ctx, "Redis SET 失败",
			logger.String("key", key),
			logger.Duration("ttl", ttl),
			logger.ErrorField("error", err),
		)
	}
}

// deleteUserCache 删除用户缓存（更新/删除用户时调用）
func (r *userRepositoryImpl) deleteUserCache(ctx context.Context, uuid string) {
	if r.redis == nil {
		return
	}

	key := buildUserInfoKey(uuid)
	if err := r.redis.Del(ctx, key).Err(); err != nil {
		logger.Warn(ctx, "Redis DEL 失败",
			logger.String("key", key),
			logger.ErrorField("error", err),
		)
	}
}

// getPhoneIndex 从 Redis 获取手机号对应的 UUID
// 返回空字符串表示未命中
func (r *userRepositoryImpl) getPhoneIndex(ctx context.Context, phone string) string {
	if r.redis == nil {
		return ""
	}

	key := buildPhoneIndexKey(phone)
	uuid, err := r.redis.Get(ctx, key).Result()

	if err == goredis.Nil {
		return "" // 未命中
	}

	if err != nil {
		logger.Warn(ctx, "Redis GET 手机号索引失败",
			logger.String("key", key),
			logger.ErrorField("error", err),
		)
		return ""
	}

	return uuid
}

// setPhoneIndex 设置手机号索引（phone -> uuid 映射）
// 索引永不过期，因为手机号和 UUID 的关系几乎不变
func (r *userRepositoryImpl) setPhoneIndex(ctx context.Context, phone, uuid string) {
	if r.redis == nil {
		return
	}

	key := buildPhoneIndexKey(phone)
	if err := r.redis.Set(ctx, key, uuid, phoneIdxTTL).Err(); err != nil {
		logger.Warn(ctx, "Redis SET 手机号索引失败",
			logger.String("key", key),
			logger.String("uuid", uuid),
			logger.ErrorField("error", err),
		)
	}
}

// ==================== Repository 接口实现 ====================

// GetByPhone 根据手机号查询用户信息
// 查询策略：
//  1. 查询手机号索引 (user:idx:phone:{phone}) 获取 UUID
//  2. 如果索引命中，转到 GetByUUID 流程
//  3. 如果索引未命中，查询 MySQL
//  4. 查询成功后，同时写入索引和用户信息缓存
func (r *userRepositoryImpl) GetByPhone(ctx context.Context, telephone string) (*model.UserInfo, error) {
	// 步骤1：尝试从 Redis 索引获取 UUID
	if uuid := r.getPhoneIndex(ctx, telephone); uuid != "" {
		logger.Debug(ctx, "手机号索引命中，转到 GetByUUID",
			logger.String("telephone", telephone),
			logger.String("uuid", uuid),
		)
		// 索引命中，复用 GetByUUID 的缓存逻辑
		return r.GetByUUID(ctx, uuid)
	}

	// 步骤2：索引未命中，查询 MySQL
	logger.Debug(ctx, "手机号索引未命中，查询 MySQL",
		logger.String("telephone", telephone),
	)

	var user model.UserInfo
	err := r.db.WithContext(ctx).Where("telephone = ?", telephone).First(&user).Error
	if err != nil {
		return nil, err
	}

	// 步骤3：同步写入缓存（保证下次查询能命中）
	// 注意：这里必须同时写入两个缓存
	// 写入手机号索引
	r.setPhoneIndex(ctx, telephone, user.Uuid)

	// 写入用户信息缓存
	r.setUserToCache(ctx, &user)

	logger.Debug(ctx, "已写入手机号索引和用户信息缓存",
		logger.String("telephone", telephone),
		logger.String("uuid", user.Uuid),
	)

	return &user, nil
}

// GetByUUID 根据UUID查询用户信息
// 查询策略：
//  1. 先查 Redis 缓存 (user:info:uid:{uuid})
//  2. 缓存未命中，查询 MySQL
//  3. 查询成功后，写入缓存
func (r *userRepositoryImpl) GetByUUID(ctx context.Context, uuid string) (*model.UserInfo, error) {
	// 步骤1：尝试从缓存获取
	if user := r.getUserFromCache(ctx, uuid); user != nil {
		logger.Debug(ctx, "用户信息缓存命中",
			logger.String("uuid", uuid),
		)
		return user, nil
	}

	// 步骤2：缓存未命中，查询 MySQL
	logger.Debug(ctx, "用户信息缓存未命中，查询 MySQL",
		logger.String("uuid", uuid),
	)

	var user model.UserInfo
	err := r.db.WithContext(ctx).Where("uuid = ?", uuid).First(&user).Error
	if err != nil {
		return nil, err
	}

	// 步骤3：同步写入缓存（保证下次查询能命中）
	r.setUserToCache(ctx, &user)

	logger.Debug(ctx, "已写入用户信息缓存",
		logger.String("uuid", uuid),
	)

	return &user, nil
}

// Create 创建新用户
func (r *userRepositoryImpl) Create(ctx context.Context, user *model.UserInfo) (*model.UserInfo, error) {
	err := r.db.WithContext(ctx).Create(user).Error
	if err != nil {
		return nil, err
	}

	// 创建成功后，同步写入缓存和索引
	r.setUserToCache(ctx, user)
	r.setPhoneIndex(ctx, user.Telephone, user.Uuid)

	logger.Debug(ctx, "新用户创建成功，已写入缓存",
		logger.String("uuid", user.Uuid),
		logger.String("telephone", user.Telephone),
	)

	return user, nil
}

// Update 更新用户信息
// 采用 Cache Aside 策略：先写库，后删缓存
func (r *userRepositoryImpl) Update(ctx context.Context, user *model.UserInfo) (*model.UserInfo, error) {
	err := r.db.WithContext(ctx).Save(user).Error
	if err != nil {
		return nil, err
	}

	// 更新成功后，同步删除缓存（让下次读取时重新加载）
	// 注意：不删除手机号索引，因为 UUID 不会变
	r.deleteUserCache(ctx, user.Uuid)

	logger.Debug(ctx, "用户信息已更新，缓存已删除",
		logger.String("uuid", user.Uuid),
	)

	return user, nil
}

// ExistsByPhone 检查手机号是否已存在
func (r *userRepositoryImpl) ExistsByPhone(ctx context.Context, telephone string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.UserInfo{}).Where("telephone = ?", telephone).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// UpdateLastLogin 更新最后登录时间
// 注意：这是高频写入操作，不删除用户信息缓存，避免缓存频繁失效
// last_login_time 字段可以考虑从 user:info 缓存中排除，或单独维护
func (r *userRepositoryImpl) UpdateLastLogin(ctx context.Context, userUUID string) error {
	return r.db.WithContext(ctx).Model(&model.UserInfo{}).
		Where("uuid = ?", userUUID).
		Update("updated_at", gorm.Expr("NOW()")).Error
}

// BatchGetByUUIDs 批量查询用户信息
// 查询策略（优化批量查询性能）：
//  1. 使用 Redis MGET 批量查询缓存
//  2. 分离"命中列表"和"未命中 UUID 列表"
//  3. 对未命中的 UUID 批量查询 MySQL (WHERE uuid IN ...)
//  4. 使用 Pipeline 批量写回 Redis
//
// 注意：这个实现比较复杂，涉及批量操作和并发控制
// 目前先保持简单实现，后续优化
func (r *userRepositoryImpl) BatchGetByUUIDs(ctx context.Context, uuids []string) ([]*model.UserInfo, error) {
	if len(uuids) == 0 {
		return []*model.UserInfo{}, nil
	}

	// TODO: 后续优化点
	// 1. 使用 MGET 批量查询 Redis
	// 2. 使用 Pipeline 批量写入 Redis
	// 3. 使用 sync.Map 或 errgroup 控制并发
	//
	// 当前实现：直接查 MySQL（简单可靠）
	logger.Debug(ctx, "批量查询用户信息（当前未启用缓存优化）",
		logger.Int("count", len(uuids)),
	)

	var users []*model.UserInfo
	err := r.db.WithContext(ctx).Where("uuid IN ?", uuids).Find(&users).Error
	if err != nil {
		return nil, err
	}

	// 异步批量写入缓存（批量操作可以异步，避免阻塞过久）
	go func() {
		bgCtx := context.Background()
		for _, user := range users {
			r.setUserToCache(bgCtx, user)
		}
		logger.Debug(bgCtx, "批量查询结果已写入缓存",
			logger.Int("count", len(users)),
		)
	}()

	return users, nil
}
