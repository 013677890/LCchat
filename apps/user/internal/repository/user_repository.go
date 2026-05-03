package repository

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/013677890/LCchat-Backend/apps/user/mq"
	"github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/accountevent"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	"github.com/013677890/LCchat-Backend/pkg/util"
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

func profileToUserInfo(profile *model.UserProfile) *model.UserInfo {
	if profile == nil {
		return nil
	}
	return &model.UserInfo{
		Uuid:      profile.UserUuid,
		Nickname:  profile.Nickname,
		Avatar:    profile.Avatar,
		Gender:    profile.Gender,
		Signature: profile.Signature,
		Birthday:  profile.Birthday,
		CreatedAt: profile.CreatedAt,
		UpdatedAt: profile.UpdatedAt,
	}
}

// GetByUUID 根据UUID查询用户信息
func (r *userRepositoryImpl) GetByUUID(ctx context.Context, uuid string) (*model.UserInfo, error) {
	// ==================== 1. 先从 Redis 缓存中查询 ====================
	cacheKey := rediskey.UserInfoKey(uuid)
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
	var profile model.UserProfile
	err = r.db.WithContext(ctx).Where("user_uuid = ?", uuid).First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 存一份空到redis 5min过期
			randomDuration := getRandomExpireTime(rediskey.UserInfoEmptyTTL)
			async.RunSafe(ctx, func(runCtx context.Context) {
				if err := r.redisClient.Set(runCtx, cacheKey, "{}", randomDuration).Err(); err != nil {
					LogRedisError(runCtx, err)
				}
			}, async.AsyncRedisTimeout)
			return nil, nil
		} else {
			return nil, WrapDBError(err)
		}
	}

	// ==================== 3. 存入 Redis 缓存 ====================
	// 序列化用户信息
	userInfo := profileToUserInfo(&profile)
	userJSON, err := json.Marshal(userInfo)
	if err != nil {
		// 序列化失败，不影响主流程，只返回数据库数据
		return userInfo, nil
	}

	// 存入缓存，设置过期时间为 1 小时（+-5min缓冲）
	// 随机时间防止缓存雪崩
	randomDuration := time.Duration(rand.Intn(10)) * time.Minute
	ttl := rediskey.UserInfoTTL - randomDuration
	async.RunSafe(ctx, func(runCtx context.Context) {
		if err := r.redisClient.Set(runCtx, cacheKey, userJSON, ttl).Err(); err != nil {
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)

	return userInfo, nil
}

// CreateProfile 创建或确认用户资料存在。
func (r *userRepositoryImpl) CreateProfile(ctx context.Context, userUUID, nickname, avatar string) (*model.UserInfo, error) {
	var profile model.UserProfile
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("user_uuid = ?", userUUID).First(&profile).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return WrapDBError(err)
		}

		profile = model.UserProfile{
			UserUuid:  userUUID,
			Nickname:  nickname,
			Avatar:    avatar,
			Gender:    3,
			Signature: "",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := tx.Create(&profile).Error; err != nil {
			return WrapDBError(err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	r.invalidateUserCache(ctx, userUUID, "UserRepository.CreateProfile")
	return profileToUserInfo(&profile), nil
}

// GetByPhone 根据手机号查询用户信息
func (r *userRepositoryImpl) GetByPhone(ctx context.Context, telephone string) (*model.UserInfo, error) {
	return nil, nil // TODO: 实现查询用户信息
}

// BatchGetByUUIDs 批量查询用户信息
// 返回结果按传入的 uuids 顺序排列，不存在的用户不包含在结果中
func (r *userRepositoryImpl) BatchGetByUUIDs(ctx context.Context, uuids []string) ([]*model.UserInfo, error) {
	if len(uuids) == 0 {
		return []*model.UserInfo{}, nil
	}

	// 用于汇总所有查询结果 (uuid -> *UserInfo, nil 表示用户不存在)
	userMap := make(map[string]*model.UserInfo, len(uuids))
	missUUIDs := make([]string, 0, len(uuids))

	// ==================== 1. 批量查询 Redis ====================
	keys := make([]string, 0, len(uuids))
	for _, uuid := range uuids {
		keys = append(keys, rediskey.UserInfoKey(uuid))
	}

	cachedValues, err := r.redisClient.MGet(ctx, keys...).Result()
	if err != nil && err != redis.Nil {
		LogRedisError(ctx, err)
		// Redis 异常时降级走 DB 全量查询
		cachedValues = nil
	}

	if cachedValues != nil {
		for i, value := range cachedValues {
			uuid := uuids[i]

			if value == nil {
				// key 不存在，需要回源
				missUUIDs = append(missUUIDs, uuid)
				continue
			}

			var raw string
			switch v := value.(type) {
			case string:
				raw = v
			case []byte:
				raw = string(v)
			default:
				missUUIDs = append(missUUIDs, uuid)
				continue
			}

			// 空占位符 `{}` 表示用户不存在，标记为已处理（nil），不回源
			if raw == "" || raw == "{}" {
				userMap[uuid] = nil // 标记为已处理，用户不存在
				continue
			}

			var user model.UserInfo
			if err := json.Unmarshal([]byte(raw), &user); err != nil {
				// 反序列化失败，需要回源
				missUUIDs = append(missUUIDs, uuid)
				continue
			}
			userMap[uuid] = &user
		}
	} else {
		// Redis 完全不可用，全部回源
		missUUIDs = append(missUUIDs, uuids...)
	}

	// ==================== 2. 对未命中部分回源 MySQL ====================
	if len(missUUIDs) > 0 {
		var dbProfiles []*model.UserProfile
		err = r.db.WithContext(ctx).
			Where("user_uuid IN ?", missUUIDs).
			Find(&dbProfiles).
			Error
		if err != nil {
			return nil, WrapDBError(err)
		}

		// 将 DB 结果放入 Map
		foundUUIDs := make(map[string]struct{}, len(dbProfiles))
		for _, profile := range dbProfiles {
			user := profileToUserInfo(profile)
			if user != nil && user.Uuid != "" {
				userMap[user.Uuid] = user
				foundUUIDs[user.Uuid] = struct{}{}
			}
		}

		// 标记不存在的用户
		for _, uuid := range missUUIDs {
			if _, ok := foundUUIDs[uuid]; !ok {
				userMap[uuid] = nil // 用户不存在
			}
		}

		// ==================== 3. 异步回填 Redis 缓存 ====================
		async.RunSafe(ctx, func(runCtx context.Context) {
			pipe := r.redisClient.Pipeline()

			for _, profile := range dbProfiles {
				user := profileToUserInfo(profile)
				if user == nil || user.Uuid == "" {
					continue
				}
				userJSON, err := json.Marshal(user)
				if err != nil {
					continue
				}
				cacheKey := rediskey.UserInfoKey(user.Uuid)
				pipe.Set(runCtx, cacheKey, userJSON, getRandomExpireTime(rediskey.UserInfoTTL))
			}

			// 对不存在的 UUID 写入空占位，避免缓存穿透
			for _, uuid := range missUUIDs {
				if _, ok := foundUUIDs[uuid]; ok {
					continue
				}
				cacheKey := rediskey.UserInfoKey(uuid)
				pipe.Set(runCtx, cacheKey, "{}", getRandomExpireTime(rediskey.UserInfoEmptyTTL))
			}

			if _, err := pipe.Exec(runCtx); err != nil {
				LogRedisError(runCtx, err)
			}
		}, async.AsyncRedisPipelineTimeout)
	}

	// ==================== 4. 按原始 uuids 顺序构建结果 ====================
	result := make([]*model.UserInfo, 0, len(uuids))
	for _, uuid := range uuids {
		if user, ok := userMap[uuid]; ok && user != nil {
			result = append(result, user)
		}
		// user == nil 表示用户不存在，跳过
	}

	return result, nil
}

// Update 更新用户信息
func (r *userRepositoryImpl) Update(ctx context.Context, user *model.UserInfo) (*model.UserInfo, error) {
	return nil, nil // TODO: 实现更新用户信息
}

// UpdateAvatar 更新用户头像
func (r *userRepositoryImpl) UpdateAvatar(ctx context.Context, userUUID, avatar string) error {
	// 更新头像到数据库
	err := r.db.WithContext(ctx).
		Model(&model.UserProfile{}).
		Where("user_uuid = ?", userUUID).
		Update("avatar", avatar).
		Error
	if err != nil {
		return WrapDBError(err)
	}

	// 更新成功后，删除 Redis 缓存。
	r.invalidateUserCache(ctx, userUUID, "UserRepository.UpdateAvatar")

	return nil
}

// UpdateAvatarWithDisplayEvent 更新头像并写入展示字段变更事件。
func (r *userRepositoryImpl) UpdateAvatarWithDisplayEvent(ctx context.Context, userUUID, avatar string) (*model.UserInfo, error) {
	var profile model.UserProfile
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.UserProfile{}).
			Where("user_uuid = ?", userUUID).
			Update("avatar", avatar).Error; err != nil {
			return WrapDBError(err)
		}

		if err := tx.Where("user_uuid = ?", userUUID).First(&profile).Error; err != nil {
			return WrapDBError(err)
		}

		eventID := util.GenIDString()
		payload, err := accountevent.Encode(accountevent.ProfileDisplayChangedPayload{
			EventID:  eventID,
			UserUUID: profile.UserUuid,
			Nickname: profile.Nickname,
			Avatar:   profile.Avatar,
		})
		if err != nil {
			return err
		}

		if err := outbox.InsertEvent(tx, accountevent.EventTypeProfileDisplayChanged, profile.UserUuid, payload); err != nil {
			return WrapDBError(err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	r.invalidateUserCache(ctx, userUUID, "UserRepository.UpdateAvatarWithDisplayEvent")
	return profileToUserInfo(&profile), nil
}

// UpdateBasicInfo 更新基本信息
func (r *userRepositoryImpl) UpdateBasicInfo(ctx context.Context, userUUID string, nickname, signature, birthday string, gender int8) error {
	err := r.applyBasicInfoUpdate(r.db.WithContext(ctx), userUUID, nickname, signature, birthday, gender)
	if err != nil {
		return err
	}

	r.invalidateUserCache(ctx, userUUID, "UserRepository.UpdateBasicInfo")
	return nil
}

// UpdateBasicInfoWithDisplayEvent 更新基本信息并写入展示字段变更事件。
func (r *userRepositoryImpl) UpdateBasicInfoWithDisplayEvent(ctx context.Context, userUUID string, nickname, signature, birthday string, gender int8) (*model.UserInfo, error) {
	var profile model.UserProfile
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.applyBasicInfoUpdate(tx, userUUID, nickname, signature, birthday, gender); err != nil {
			return err
		}

		if err := tx.Where("user_uuid = ?", userUUID).First(&profile).Error; err != nil {
			return WrapDBError(err)
		}

		eventID := util.GenIDString()
		payload, err := accountevent.Encode(accountevent.ProfileDisplayChangedPayload{
			EventID:  eventID,
			UserUUID: profile.UserUuid,
			Nickname: profile.Nickname,
			Avatar:   profile.Avatar,
		})
		if err != nil {
			return err
		}

		if err := outbox.InsertEvent(tx, accountevent.EventTypeProfileDisplayChanged, profile.UserUuid, payload); err != nil {
			return WrapDBError(err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	r.invalidateUserCache(ctx, userUUID, "UserRepository.UpdateBasicInfoWithDisplayEvent")
	return profileToUserInfo(&profile), nil
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

	r.invalidateUserCache(ctx, userUUID, "UserRepository.UpdateEmail")

	return nil
}

// UpdateTelephone 更新手机号
func (r *userRepositoryImpl) UpdateTelephone(ctx context.Context, userUUID, telephone string) error {
	return nil // TODO: 实现更新手机号
}

// Delete 软删除用户（注销账号）
// 设置 deleted_at 字段，删除 Redis 缓存
func (r *userRepositoryImpl) Delete(ctx context.Context, userUUID string) error {
	// 资料表不保留 deleted_at，收到账号注销事件后直接删除 profile。
	err := r.db.WithContext(ctx).
		Where("user_uuid = ?", userUUID).
		Delete(&model.UserProfile{}).
		Error
	if err != nil {
		return WrapDBError(err)
	}

	r.invalidateUserCache(ctx, userUUID, "UserRepository.Delete")

	return nil
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

	r.invalidateUserCache(ctx, userUUID, "UserRepository.UpdatePassword")

	return nil
}

func (r *userRepositoryImpl) applyBasicInfoUpdate(db *gorm.DB, userUUID string, nickname, signature, birthday string, gender int8) error {
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

	if err := db.Model(&model.UserProfile{}).
		Where("user_uuid = ?", userUUID).
		Updates(updates).
		Error; err != nil {
		return WrapDBError(err)
	}
	return nil
}

func (r *userRepositoryImpl) invalidateUserCache(ctx context.Context, userUUID, source string) {
	if r.redisClient == nil || userUUID == "" {
		return
	}

	cacheKey := rediskey.UserInfoKey(userUUID)
	if err := r.redisClient.Del(ctx, cacheKey).Err(); err != nil {
		task := mq.BuildDelTask(cacheKey).
			WithSource(source)
		LogAndRetryRedisError(ctx, task, err)
	}
}

// SaveQRCode 保存用户二维码
// 将二维码 token 与用户 UUID 的映射关系存储到 Redis
// 同时保存反向映射: user:qrcode:user:{userUUID} -> token
// 过期时间: 48小时
func (r *userRepositoryImpl) SaveQRCode(ctx context.Context, userUUID, token string) error {
	// 1. 保存 token -> userUUID 映射
	tokenKey := rediskey.QRCodeTokenKey(token)
	err := r.redisClient.Set(ctx, tokenKey, userUUID, rediskey.QRCodeTTL).Err()
	if err != nil {
		return WrapRedisError(err)
	}

	// 2. 保存 userUUID -> token 反向映射
	userKey := rediskey.QRCodeUserKey(userUUID)
	err = r.redisClient.Set(ctx, userKey, token, rediskey.QRCodeTTL).Err()
	if err != nil {
		return WrapRedisError(err)
	}

	return nil
}

// GetUUIDByQRCodeToken 根据 token 获取用户 UUID
func (r *userRepositoryImpl) GetUUIDByQRCodeToken(ctx context.Context, token string) (string, error) {
	tokenKey := rediskey.QRCodeTokenKey(token)
	userUUID, err := r.redisClient.Get(ctx, tokenKey).Result()
	if err != nil {
		if err == redis.Nil {
			return "", ErrRedisNil
		}
		return "", WrapRedisError(err)
	}
	return userUUID, nil
}

// GetQRCodeTokenByUserUUID 根据用户 UUID 获取二维码 token和剩余时间
func (r *userRepositoryImpl) GetQRCodeTokenByUserUUID(ctx context.Context, userUUID string) (string, time.Time, error) {
	userKey := rediskey.QRCodeUserKey(userUUID)
	pipe := r.redisClient.Pipeline()
	pipe.Get(ctx, userKey)
	pipe.TTL(ctx, userKey)
	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return "", time.Time{}, WrapRedisError(err)
	}
	token := cmds[0].(*redis.StringCmd).Val()
	expireTime := time.Now().Add(cmds[1].(*redis.DurationCmd).Val().Round(time.Second))
	return token, expireTime, nil
}

// SearchUser 搜索用户（按邮箱、昵称、UUID）
func (r *userRepositoryImpl) SearchUser(ctx context.Context, keyword string, page, pageSize int) ([]*model.UserInfo, int64, error) {
	// 计算偏移量
	offset := (page - 1) * pageSize

	// 构建查询条件
	query := r.db.WithContext(ctx).
		Model(&model.UserProfile{}).
		Where("nickname LIKE ? OR user_uuid LIKE ?",
			keyword+"%",
			keyword+"%")

	// 先查询总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}

	// 查询用户列表
	var profiles []*model.UserProfile
	if err := query.
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&profiles).
		Error; err != nil {
		return nil, 0, WrapDBError(err)
	}

	users := make([]*model.UserInfo, 0, len(profiles))
	for _, profile := range profiles {
		if user := profileToUserInfo(profile); user != nil {
			users = append(users, user)
		}
	}

	return users, total, nil
}
