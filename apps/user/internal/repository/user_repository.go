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

// userRepositoryImpl 用户资料数据访问层实现。
type userRepositoryImpl struct {
	db          *gorm.DB
	redisClient *redis.Client
}

// NewUserRepository 创建用户资料仓储实例。
//
// 该仓储统一承接 user_profile 的读写、二维码映射和资料展示事件出箱，
// 并通过 Redis 缓存降低高频资料查询对 MySQL 的压力。
func NewUserRepository(db *gorm.DB, redisClient *redis.Client) IUserRepository {
	return &userRepositoryImpl{db: db, redisClient: redisClient}
}

// GetByUUID 根据 UUID 查询用户资料。
//
// 查询链路采用“缓存优先 + 空值占位 + 异步回填”策略：
//  1. 先查 Redis，命中则直接返回；
//  2. 命中 "{}" 占位时视为用户不存在，避免缓存穿透；
//  3. 缓存 miss 时回源 MySQL，并把结果异步写回缓存。
func (r *userRepositoryImpl) GetByUUID(ctx context.Context, uuid string) (*model.UserProfile, error) {
	// ==================== 1. 先从 Redis 缓存中查询 ====================
	cacheKey := rediskey.UserProfileKey(uuid)
	if r.redisClient != nil {
		cachedData, err := r.redisClient.Get(ctx, cacheKey).Result()
		if err == nil {
			// 缓存命中，反序列化返回
			// 先判空
			if cachedData == "{}" {
				return nil, nil
			}
			var profile model.UserProfile
			if err := json.Unmarshal([]byte(cachedData), &profile); err == nil {
				return &profile, nil
			}
		}
		if err != nil && err != redis.Nil {
			LogRedisError(ctx, err) // 记录日志 降级处理
		}
	}

	// ==================== 2. 缓存未命中，查询 MySQL ====================
	var profile model.UserProfile
	err := r.db.WithContext(ctx).Where("user_uuid = ?", uuid).First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 存一份空到redis 5min过期
			randomDuration := getRandomExpireTime(rediskey.UserProfileEmptyTTL)
			if r.redisClient != nil {
				async.RunSafe(ctx, func(runCtx context.Context) {
					if err := r.redisClient.Set(runCtx, cacheKey, "{}", randomDuration).Err(); err != nil {
						LogRedisError(runCtx, err)
					}
				}, async.AsyncRedisTimeout)
			}
			return nil, nil
		} else {
			return nil, WrapDBError(err)
		}
	}

	// ==================== 3. 存入 Redis 缓存 ====================
	// 直接缓存资料域权威模型，保持缓存内容与资料域存储模型一致。
	profileJSON, err := json.Marshal(&profile)
	if err != nil {
		// 序列化失败，不影响主流程，只返回数据库数据
		return &profile, nil
	}

	// 存入缓存，设置过期时间为 1 小时（+-5min缓冲）
	// 随机时间防止缓存雪崩
	randomDuration := time.Duration(rand.Intn(10)) * time.Minute
	ttl := rediskey.UserProfileTTL - randomDuration
	if r.redisClient != nil {
		async.RunSafe(ctx, func(runCtx context.Context) {
			if err := r.redisClient.Set(runCtx, cacheKey, profileJSON, ttl).Err(); err != nil {
				LogRedisError(runCtx, err)
			}
		}, async.AsyncRedisTimeout)
	}

	return &profile, nil
}

// CreateProfile 创建或确认用户资料存在。
//
// 该方法服务于账号注册后的资料域初始化：若 profile 已存在则直接复用，
// 否则在事务内创建默认资料记录，保证重复消费注册事件时仍然幂等。
func (r *userRepositoryImpl) CreateProfile(ctx context.Context, userUUID, nickname, avatar string) (*model.UserProfile, error) {
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
	return &profile, nil
}

// BatchGetByUUIDs 批量查询用户资料。
//
// 返回结果按传入 uuids 的顺序排列，不存在的用户不会出现在结果中。实现上会：
//  1. 先批量读取 Redis；
//  2. 对 miss 部分回源 MySQL；
//  3. 异步回填命中和空占位，兼顾性能与防穿透。
func (r *userRepositoryImpl) BatchGetByUUIDs(ctx context.Context, uuids []string) ([]*model.UserProfile, error) {
	if len(uuids) == 0 {
		return []*model.UserProfile{}, nil
	}

	// 用于汇总所有查询结果 (uuid -> *UserProfile, nil 表示用户不存在)
	profileMap := make(map[string]*model.UserProfile, len(uuids))
	missUUIDs := make([]string, 0, len(uuids))

	// ==================== 1. 批量查询 Redis ====================
	if r.redisClient != nil {
		keys := make([]string, 0, len(uuids))
		for _, uuid := range uuids {
			keys = append(keys, rediskey.UserProfileKey(uuid))
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
					profileMap[uuid] = nil // 标记为已处理，用户不存在
					continue
				}

				var profile model.UserProfile
				if err := json.Unmarshal([]byte(raw), &profile); err != nil {
					// 反序列化失败，需要回源
					missUUIDs = append(missUUIDs, uuid)
					continue
				}
				profileMap[uuid] = &profile
			}
		} else {
			// Redis 完全不可用，全部回源
			missUUIDs = append(missUUIDs, uuids...)
		}
	} else {
		// Redis 完全不可用，全部回源
		missUUIDs = append(missUUIDs, uuids...)
	}

	// ==================== 2. 对未命中部分回源 MySQL ====================
	if len(missUUIDs) > 0 {
		var dbProfiles []*model.UserProfile
		err := r.db.WithContext(ctx).
			Where("user_uuid IN ?", missUUIDs).
			Find(&dbProfiles).
			Error
		if err != nil {
			return nil, WrapDBError(err)
		}

		// 将 DB 结果放入 Map
		foundUUIDs := make(map[string]struct{}, len(dbProfiles))
		for _, profile := range dbProfiles {
			if profile == nil || profile.UserUuid == "" {
				continue
			}
			profileMap[profile.UserUuid] = profile
			foundUUIDs[profile.UserUuid] = struct{}{}
		}

		// 标记不存在的用户
		for _, uuid := range missUUIDs {
			if _, ok := foundUUIDs[uuid]; !ok {
				profileMap[uuid] = nil // 用户不存在
			}
		}

		if r.redisClient != nil {
			// ==================== 3. 异步回填 Redis 缓存 ====================
			async.RunSafe(ctx, func(runCtx context.Context) {
				pipe := r.redisClient.Pipeline()

				for _, profile := range dbProfiles {
					if profile == nil || profile.UserUuid == "" {
						continue
					}
					profileJSON, err := json.Marshal(profile)
					if err != nil {
						continue
					}
					cacheKey := rediskey.UserProfileKey(profile.UserUuid)
					pipe.Set(runCtx, cacheKey, profileJSON, getRandomExpireTime(rediskey.UserProfileTTL))
				}

				// 对不存在的 UUID 写入空占位，避免缓存穿透
				for _, uuid := range missUUIDs {
					if _, ok := foundUUIDs[uuid]; ok {
						continue
					}
					cacheKey := rediskey.UserProfileKey(uuid)
					pipe.Set(runCtx, cacheKey, "{}", getRandomExpireTime(rediskey.UserProfileEmptyTTL))
				}

				if _, err := pipe.Exec(runCtx); err != nil {
					LogRedisError(runCtx, err)
				}
			}, async.AsyncRedisPipelineTimeout)
		}
	}

	// ==================== 4. 按原始 uuids 顺序构建结果 ====================
	result := make([]*model.UserProfile, 0, len(uuids))
	for _, uuid := range uuids {
		if profile, ok := profileMap[uuid]; ok && profile != nil {
			result = append(result, profile)
		}
		// profile == nil 表示用户不存在，跳过
	}

	return result, nil
}

// UpdateAvatarWithDisplayEvent 更新头像并写入展示字段变更事件。
func (r *userRepositoryImpl) UpdateAvatarWithDisplayEvent(ctx context.Context, userUUID, avatar string) (*model.UserProfile, error) {
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
	return &profile, nil
}

// UpdateBasicInfoWithDisplayEvent 更新基本信息并写入展示字段变更事件。
func (r *userRepositoryImpl) UpdateBasicInfoWithDisplayEvent(ctx context.Context, userUUID string, nickname, signature, birthday string, gender int8) (*model.UserProfile, error) {
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
	return &profile, nil
}

// Delete 删除用户资料。
//
// 资料域不保留 deleted_at 软删除标记；收到账号注销事件后直接删除 profile 记录，
// 同时失效资料缓存，避免上游继续读取到已注销用户的资料快照。
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

// applyBasicInfoUpdate 按“仅更新有值字段”的规则写入资料基础字段。
//
// 该帮助函数只负责构造更新集和执行 SQL，不承担事件写入；展示字段变更事件由外层事务
// 在拿到最新快照后统一写入 outbox。
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

// invalidateUserCache 删除单用户资料缓存，并在失败时投递补偿任务。
func (r *userRepositoryImpl) invalidateUserCache(ctx context.Context, userUUID, source string) {
	if r.redisClient == nil || userUUID == "" {
		return
	}

	cacheKey := rediskey.UserProfileKey(userUUID)
	if err := r.redisClient.Del(ctx, cacheKey).Err(); err != nil {
		task := mq.BuildDelTask(cacheKey).
			WithSource(source)
		LogAndRetryRedisError(ctx, task, err)
	}
}

// SaveQRCode 保存用户二维码映射。
//
// 这里同时维护 token -> userUUID 与 userUUID -> token 两个方向的 Redis Key，
// 便于“扫码解析”和“复用已有二维码”两个场景共用一份 48 小时有效期数据。
func (r *userRepositoryImpl) SaveQRCode(ctx context.Context, userUUID, token string) error {
	if r.redisClient == nil {
		return ErrRedis
	}

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

// GetUUIDByQRCodeToken 根据二维码 token 反查用户 UUID。
//
// 当 token 过期或不存在时返回 ErrRedisNil，供 service 层映射为二维码失效类业务反馈。
func (r *userRepositoryImpl) GetUUIDByQRCodeToken(ctx context.Context, token string) (string, error) {
	if r.redisClient == nil {
		return "", ErrRedis
	}

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

// GetQRCodeTokenByUserUUID 根据用户 UUID 获取当前二维码 token 与剩余有效期。
//
// 通过一次 pipeline 同时读取 token 和 TTL，避免 service 层再发第二次 Redis 请求拼接过期时间。
func (r *userRepositoryImpl) GetQRCodeTokenByUserUUID(ctx context.Context, userUUID string) (string, time.Time, error) {
	if r.redisClient == nil {
		return "", time.Time{}, ErrRedis
	}

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

// SearchUser 搜索用户资料。
//
// 当前资料域只支持按 nickname 前缀或 user_uuid 前缀搜索，不跨到 auth 域检索邮箱，
// 以保持四拆后的边界清晰。
func (r *userRepositoryImpl) SearchUser(ctx context.Context, keyword string, page, pageSize int) ([]*model.UserProfile, int64, error) {
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

	return profiles, total, nil
}
