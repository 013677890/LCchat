package repository

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"time"

	"github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/accountevent"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/cachex"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	"github.com/013677890/LCchat-Backend/pkg/redisbloom"
	"github.com/013677890/LCchat-Backend/pkg/redisretry"
	"github.com/013677890/LCchat-Backend/pkg/util"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// userRepositoryImpl 用户资料数据访问层实现。
type userRepositoryImpl struct {
	db          *gorm.DB
	redisClient *redis.Client
}

// userUUIDBloomFilter 维护 user_profile.user_uuid 的存在性索引。
//
// 容量按千万级用户预留；后续规模超过容量时，需要新建更大过滤器并全量回填。
var userUUIDBloomFilter = redisbloom.Filter{
	Key:       rediskey.UserUUIDBloomKey,
	ErrorRate: 0.001,
	Capacity:  10_000_000,
}

// NewUserRepository 创建用户资料仓储实例。
//
// 该仓储统一承接 user_profile 的读写、二维码映射和资料展示事件出箱，
// 并通过 Redis 缓存与 user_uuid Bloom Filter 降低高频资料查询对 MySQL 的压力。
func NewUserRepository(db *gorm.DB, redisClient *redis.Client) IUserRepository {
	repo := &userRepositoryImpl{db: db, redisClient: redisClient}
	repo.initUserUUIDBloom(context.Background())
	return repo
}

// initUserUUIDBloom 在仓储构造阶段初始化 user_uuid Bloom Filter。
//
// 初始化失败只记录日志并降级：存量读路径仍可回源 DB；但新增写路径会再次执行 BF.ADD，
// 若当时 RedisBloom 仍不可用，则拒绝写 DB，避免产生“DB 有记录但 Bloom 没记录”的误判。
func (r *userRepositoryImpl) initUserUUIDBloom(ctx context.Context) {
	if r == nil || r.redisClient == nil {
		return
	}
	initCtx, cancel := context.WithTimeout(ctx, async.AsyncRedisTimeout)
	defer cancel()
	if err := userUUIDBloomFilter.Ensure(initCtx, r.redisClient); err != nil {
		LogRedisError(initCtx, err)
	}
}

// userUUIDMayExist 判断 user_uuid 是否可能存在。
//
// 返回 false 表示 Bloom 明确判定不存在，可以跳过 DB 防穿透；Bloom 命令失败时返回 true，
// 让读链路回源 DB，避免 RedisBloom 故障扩大成用户资料不可读。
func (r *userRepositoryImpl) userUUIDMayExist(ctx context.Context, userUUID string) bool {
	if r == nil || r.redisClient == nil || userUUID == "" {
		return true
	}
	exists, usable, err := userUUIDBloomFilter.Exists(ctx, r.redisClient, userUUID)
	if err != nil {
		LogRedisError(ctx, err)
	}
	if !usable {
		return true
	}
	return exists
}

// filterUserUUIDsByBloom 用 BF.MEXISTS 过滤批量查询中的确定不存在 UUID。
//
// 只要 RedisBloom 不能给出可靠结果，就保留原始列表继续查 DB，保证故障时优先正确性。
func (r *userRepositoryImpl) filterUserUUIDsByBloom(ctx context.Context, userUUIDs []string) []string {
	if r == nil || r.redisClient == nil || len(userUUIDs) == 0 {
		return userUUIDs
	}
	exists, usable, err := userUUIDBloomFilter.MExists(ctx, r.redisClient, userUUIDs)
	if err != nil {
		LogRedisError(ctx, err)
	}
	if !usable || len(exists) != len(userUUIDs) {
		return userUUIDs
	}
	filtered := make([]string, 0, len(userUUIDs))
	for i, userUUID := range userUUIDs {
		if exists[i] {
			filtered = append(filtered, userUUID)
		}
	}
	return filtered
}

// ensureUserUUIDInBloom 在写 DB 前把 user_uuid 放入 Bloom Filter。
//
// 新增记录时必须先写 Bloom 再写 DB：如果 DB 最终失败，只会留下 false positive，
// 后续最多多查一次 DB；反过来若 DB 成功但 Bloom 缺失，则会出现 false negative 并误判用户不存在。
func (r *userRepositoryImpl) ensureUserUUIDInBloom(ctx context.Context, userUUID string) error {
	if r == nil || r.redisClient == nil || userUUID == "" {
		return nil
	}
	return userUUIDBloomFilter.Add(ctx, r.redisClient, userUUID)
}

// addUserUUIDToBloomBestEffort 用于读回源命中后的自愈补写。
//
// 这里不是创建事实的关键路径，失败只记录日志，不能影响一次已经查到 DB 的正常读取。
func (r *userRepositoryImpl) addUserUUIDToBloomBestEffort(ctx context.Context, userUUID string) {
	if err := r.ensureUserUUIDInBloom(ctx, userUUID); err != nil {
		LogRedisError(ctx, err)
	}
}

// addUserUUIDsToBloomAsync 批量补写 DB 命中的 user_uuid。
//
// 该方法用于 RedisBloom 故障恢复后的渐进自愈，失败不改变本次查询结果。
func (r *userRepositoryImpl) addUserUUIDsToBloomAsync(ctx context.Context, userUUIDs []string) {
	if r == nil || r.redisClient == nil || len(userUUIDs) == 0 {
		return
	}
	items := make([]string, 0, len(userUUIDs))
	seen := make(map[string]struct{}, len(userUUIDs))
	for _, userUUID := range userUUIDs {
		if userUUID == "" {
			continue
		}
		if _, ok := seen[userUUID]; ok {
			continue
		}
		seen[userUUID] = struct{}{}
		items = append(items, userUUID)
	}

	if len(items) == 0 {
		return
	}
	async.RunSafe(ctx, func(runCtx context.Context) {
		if err := userUUIDBloomFilter.MAdd(runCtx, r.redisClient, items); err != nil {
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)
}

// GetByUUID 根据 UUID 查询用户资料。
//
// 查询链路采用“缓存优先 + 空值占位 + 异步回填”策略：
//  1. 先查 Redis，命中则直接返回；
//  2. 命中 "{}" 占位时视为用户不存在，避免缓存穿透；
//  3. 缓存 miss 时先用 Bloom 拦截确定不存在的 UUID；
//  4. Bloom 判定可能存在或不可用时回源 MySQL，并把结果异步写回缓存。
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

	// ==================== 2. 缓存未命中，先用 Bloom 拦截穿透 ====================
	if !r.userUUIDMayExist(ctx, uuid) {
		r.setUserProfileEmptyCacheAsync(ctx, uuid)
		return nil, nil
	}

	// ==================== 3. Bloom 判定可能存在，查询 MySQL ====================
	var profile model.UserProfile
	err := r.db.WithContext(ctx).Where("user_uuid = ?", uuid).First(&profile).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			r.setUserProfileEmptyCacheAsync(ctx, uuid)
			return nil, nil
		} else {
			return nil, WrapDBError(err)
		}
	}

	r.addUserUUIDToBloomBestEffort(ctx, profile.UserUuid)

	// ==================== 4. 存入 Redis 缓存 ====================
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
	// Bloom 是读路径的前置存在性索引，必须先于 DB 写入成功。
	// 若 Bloom 写入成功而 DB 失败，只会留下 false positive；反过来会造成真实用户被误判不存在。
	if err := r.ensureUserUUIDInBloom(ctx, userUUID); err != nil {
		return nil, WrapRedisError(err)
	}

	var profile model.UserProfile
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("user_uuid = ?", userUUID).First(&profile).Error

		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return WrapDBError(err)
		}

		now := time.Now()
		profile = model.UserProfile{
			UserUuid:  userUUID,
			Nickname:  nickname,
			Avatar:    avatar,
			Gender:    3,
			Signature: "",
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_uuid"}},
			DoNothing: true,
		}).Create(&profile).Error; err != nil {
			return WrapDBError(err)
		}

		// 冲突忽略后必须回查权威记录，避免返回被并发请求忽略的内存对象。
		if err := tx.Where("user_uuid = ?", userUUID).First(&profile).Error; err != nil {
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
//  2. 对 miss 部分先用 Bloom 过滤确定不存在的 UUID；
//  3. 对 Bloom 判定可能存在的 UUID 回源 MySQL；
//  4. 异步回填命中和空占位，兼顾性能与防穿透。
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
		queryUUIDs := r.filterUserUUIDsByBloom(ctx, missUUIDs)
		var dbProfiles []*model.UserProfile
		if len(queryUUIDs) > 0 {
			err := r.db.WithContext(ctx).
				Where("user_uuid IN ?", queryUUIDs).
				Find(&dbProfiles).
				Error
			if err != nil {
				return nil, WrapDBError(err)
			}
		}

		// 将 DB 结果放入 Map
		foundUUIDs := make(map[string]struct{}, len(dbProfiles))
		foundUUIDList := make([]string, 0, len(dbProfiles))
		for _, profile := range dbProfiles {
			if profile == nil || profile.UserUuid == "" {
				continue
			}
			profileMap[profile.UserUuid] = profile
			foundUUIDs[profile.UserUuid] = struct{}{}
			foundUUIDList = append(foundUUIDList, profile.UserUuid)
		}

		r.addUserUUIDsToBloomAsync(ctx, foundUUIDList)

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
					pipe.Set(runCtx, cacheKey, profileJSON, cachex.JitterTTL(rediskey.UserProfileTTL))
				}

				// 对不存在的 UUID 写入空占位，避免缓存穿透
				for _, uuid := range missUUIDs {
					if _, ok := foundUUIDs[uuid]; ok {
						continue
					}
					cacheKey := rediskey.UserProfileKey(uuid)
					pipe.Set(runCtx, cacheKey, "{}", cachex.JitterTTL(rediskey.UserProfileEmptyTTL))
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

// setUserProfileEmptyCacheAsync 写入用户资料空值缓存。
//
// Bloom 判定不存在和 DB 查无记录都会写短 TTL 空占位，避免同一个不存在 UUID 在 Bloom 不可用时反复打 DB。
func (r *userRepositoryImpl) setUserProfileEmptyCacheAsync(ctx context.Context, userUUID string) {
	if r == nil || r.redisClient == nil || userUUID == "" {
		return
	}
	cacheKey := rediskey.UserProfileKey(userUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		if err := r.redisClient.Set(runCtx, cacheKey, "{}", cachex.JitterTTL(rediskey.UserProfileEmptyTTL)).Err(); err != nil {
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)
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
		task := redisretry.BuildDelTask(cacheKey).
			WithSource(source)
		redisretry.LogAndRetryRedisError(ctx, task, err)
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
