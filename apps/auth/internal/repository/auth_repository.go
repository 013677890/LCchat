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
//
// 该仓储同时承接三类数据职责：
//  1. user_account 的权威读写；
//  2. 验证码与限流计数的 Redis 读写；
//  3. 注册/注销时的 outbox 事件落库。
func NewAuthRepository(db *gorm.DB, redisClient *redis.Client) IAuthRepository {
	return &authRepositoryImpl{db: db, redisClient: redisClient}
}

// GetByEmail 根据邮箱查询可登录账号。
func (r *authRepositoryImpl) GetByEmail(ctx context.Context, email string) (*model.UserAccount, error) {
	var user model.UserAccount
	// deleted_at IS NULL 明确只返回当前仍可登录的账号，避免把已注销账号重新暴露给登录链路。
	err := r.db.WithContext(ctx).Where("email = ? AND deleted_at IS NULL", email).First(&user).Error
	if err != nil {
		return nil, WrapDBError(err)
	}
	return &user, nil
}

// GetByUserUUID 根据用户 UUID 查询可登录账号。
func (r *authRepositoryImpl) GetByUserUUID(ctx context.Context, userUUID string) (*model.UserAccount, error) {
	var user model.UserAccount
	// 这里同样过滤掉已注销账号，保证 service 层看到的是当前有效账号视图。
	err := r.db.WithContext(ctx).Where("user_uuid = ? AND deleted_at IS NULL", userUUID).First(&user).Error
	if err != nil {
		return nil, WrapDBError(err)
	}
	return &user, nil
}

// ExistsByEmail 检查邮箱是否已被占用。
func (r *authRepositoryImpl) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	// 使用 count 而不是直接查整行，减少换绑邮箱等只关心“是否存在”的场景开销。
	err := r.db.WithContext(ctx).Model(&model.UserAccount{}).Where("email = ? AND deleted_at IS NULL", email).Count(&count).Error
	if err != nil {
		return false, WrapDBError(err)
	}
	return count > 0, nil
}

// Create 创建账号记录。
func (r *authRepositoryImpl) Create(ctx context.Context, user *model.UserAccount) (*model.UserAccount, error) {
	// 真实插入细节统一复用 createUser，避免普通创建和事务创建两条路径分叉。
	if err := r.createUser(r.db.WithContext(ctx), user); err != nil {
		return nil, err
	}
	return user, nil
}

// CreateWithOutboxEvent 在创建账号的同一事务中追加 Outbox 事件。
func (r *authRepositoryImpl) CreateWithOutboxEvent(ctx context.Context, user *model.UserAccount, eventType, payload string) (*model.UserAccount, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 第一步先写 user_account，保证 outbox 事件引用的聚合根已经存在。
		if err := r.createUser(tx, user); err != nil {
			return err
		}
		// 第二步把跨服务事件与账号创建放进同一事务，保证最终一致链路有事实来源。
		if err := outbox.InsertEvent(tx, eventType, user.UserUuid, payload); err != nil {
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
	// 这里只更新 password_hash 一个敏感字段，不顺带修改其他资料列。
	result := r.db.WithContext(ctx).Model(&model.UserAccount{}).
		Where("user_uuid = ? AND deleted_at IS NULL", userUUID).
		Update("password_hash", password)
	if result.Error != nil {
		return WrapDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}
	// 登录态展示缓存依赖 user_account 快照，密码修改后顺手失效同一个用户缓存。
	r.invalidateUserCache(ctx, userUUID)
	return nil
}

// UpdateEmail 更新邮箱。
func (r *authRepositoryImpl) UpdateEmail(ctx context.Context, userUUID, email string) error {
	// 邮箱是登录凭据之一，因此更新时仍只作用于未注销账号。
	result := r.db.WithContext(ctx).Model(&model.UserAccount{}).
		Where("user_uuid = ? AND deleted_at IS NULL", userUUID).
		Update("email", email)
	if result.Error != nil {
		return WrapDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}
	// 邮箱更新后旧缓存中的登录展示与账号快照都已过期，需要一起失效。
	r.invalidateUserCache(ctx, userUUID)
	return nil
}

// UpdateLoginDisplay 更新登录态依赖的昵称和头像字段。
func (r *authRepositoryImpl) UpdateLoginDisplay(ctx context.Context, userUUID, nickname, avatar string) error {
	// updated_at 始终刷新，保证账号快照能体现最近一次展示字段同步时间。
	updates := map[string]interface{}{"updated_at": time.Now()}

	// 只有传入了 nickname 时才覆盖 login_nickname，避免把空串误写进冗余列。
	if nickname != "" {
		updates["login_nickname"] = nickname
	}

	// avatar 同理按需更新，允许只同步昵称或只同步头像。
	if avatar != "" {
		updates["login_avatar"] = avatar
	}

	// 这里使用 Updates(map) 是为了让“只更新有值字段”的语义清晰可控。
	result := r.db.WithContext(ctx).Model(&model.UserAccount{}).
		Where("user_uuid = ? AND deleted_at IS NULL", userUUID).
		Updates(updates)
	if result.Error != nil {
		return WrapDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}
	r.invalidateUserCache(ctx, userUUID)
	return nil
}

// Delete 软删除账号。
func (r *authRepositoryImpl) Delete(ctx context.Context, userUUID string) error {
	now := time.Now()
	// 注销时同时把 status 置为禁用并写 deleted_at，避免后续仍按正常账号被查询出来。
	result := r.db.WithContext(ctx).Model(&model.UserAccount{}).
		Where("user_uuid = ? AND deleted_at IS NULL", userUUID).
		Updates(map[string]interface{}{"status": 1, "deleted_at": now, "updated_at": now})
	if result.Error != nil {
		return WrapDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}
	r.invalidateUserCache(ctx, userUUID)
	return nil
}

// DeleteWithOutboxEvent 在软删除账号的同一事务中追加 Outbox 事件。
func (r *authRepositoryImpl) DeleteWithOutboxEvent(ctx context.Context, userUUID, eventType, payload string) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()

		// 先在事务里软删除账号，确保后续 account_deleted 事件对应的数据库事实已经落盘。
		result := tx.Model(&model.UserAccount{}).
			Where("user_uuid = ? AND deleted_at IS NULL", userUUID).
			Updates(map[string]interface{}{"status": 1, "deleted_at": now, "updated_at": now})
		if result.Error != nil {
			return WrapDBError(result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrRecordNotFound
		}

		// 再追加 outbox 事件，让 user/relation 等下游服务后续做清理。
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
	// 验证码按 email + type 存储，先直接读取当前生效值，再在仓储层完成字面量比较。
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
	// key 由邮箱和验证码类型共同决定，支持注册/登录/重置密码等不同场景隔离存储。
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
	// 删除已消费验证码时仍按 email + type 精确清理，避免误删其他业务类型的验证码。
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

// CheckAndIncrementVerifyCodeRateLimit 原子校验验证码发送限流并占用一次计数。
func (r *authRepositoryImpl) CheckAndIncrementVerifyCodeRateLimit(ctx context.Context, email, ip string) (bool, error) {
	if r.redisClient == nil {
		return false, ErrRedis
	}

	// 检查与递增必须在 Redis 单脚本内完成，避免并发请求在检查通过后一起写入计数。
	result, err := r.redisClient.Eval(ctx, luaCheckAndIncrementVerifyCodeRateLimit, []string{
		rediskey.VerifyCodeMinuteKey(email),
		rediskey.VerifyCode24HKey(email),
		rediskey.VerifyCodeIPKey(ip),
	},
		1,
		10,
		100,
		int(rediskey.VerifyCodeMinuteTTL.Seconds()),
		int(rediskey.VerifyCode24HTTL.Seconds()),
		int(rediskey.VerifyCodeIPTTL.Seconds()),
	).Int()
	if err != nil {
		return false, WrapRedisError(err)
	}
	return result != 0, nil
}

// BatchGetAccountStatus 批量查询账号存在性与状态。
func (r *authRepositoryImpl) BatchGetAccountStatus(ctx context.Context, userUUIDs []string) ([]*AccountStatusItem, error) {
	if len(userUUIDs) == 0 {
		return []*AccountStatusItem{}, nil
	}

	type rawAccountStatus struct {
		UserUUID  string         `gorm:"column:user_uuid"`
		Status    int8           `gorm:"column:status"`
		DeletedAt gorm.DeletedAt `gorm:"column:deleted_at"`
	}

	rows := make([]*rawAccountStatus, 0, len(userUUIDs))

	// 使用 Unscoped 保留已软删除账号，这样上游才能把“存在但已注销”和“从未存在”区分开。
	err := r.db.WithContext(ctx).Unscoped().Model(&model.UserAccount{}).
		Select("user_uuid, status, deleted_at").
		Where("user_uuid IN ?", userUUIDs).
		Find(&rows).Error

	if err != nil {
		return nil, WrapDBError(err)
	}

	statusMap := make(map[string]*AccountStatusItem, len(rows))
	for _, row := range rows {
		if row == nil || row.UserUUID == "" {
			continue
		}
		item := &AccountStatusItem{UserUUID: row.UserUUID, Exists: true, Status: int32(row.Status)}
		if row.DeletedAt.Valid && item.Status == 0 {
			item.Status = 1
		}
		statusMap[row.UserUUID] = item
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
	key := rediskey.UserProfileKey(userUUID)
	if err := r.redisClient.Del(ctx, key).Err(); err != nil {
		// 用户信息缓存失效允许最终一致，失败后异步补偿即可。
		task := mq.BuildDelTask(key).
			WithSource("AuthRepository.invalidateUserCache")
		LogAndRetryRedisError(ctx, task, err)
	}
}

// createUser 根据手机号是否为空决定插入列集合。
//
// telephone 在当前迁移阶段允许为空；为空时显式 Omit，避免数据库层默认值或约束带来歧义。
func (r *authRepositoryImpl) createUser(db *gorm.DB, user *model.UserAccount) error {
	if user.Telephone == "" {
		// 不带手机号的注册路径只写入当前最小账号字段集合。
		if err := db.Omit("Telephone").Create(user).Error; err != nil {
			return WrapDBError(err)
		}
		return nil
	}
	// 已显式传入手机号时，按完整模型插入。
	if err := db.Create(user).Error; err != nil {
		return WrapDBError(err)
	}
	return nil
}
