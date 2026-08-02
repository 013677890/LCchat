package repository

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"time"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/redisretry"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// deviceRepositoryImpl 实现设备会话与在线状态相关的数据访问逻辑。
type deviceRepositoryImpl struct {
	db          *gorm.DB
	redisClient *redis.Client
}

// NewDeviceRepository 创建设备会话仓储实例。
//
// 该仓储维护两类设备数据：
//  1. MySQL 中的 device_sessions 权威记录；
//  2. Redis 中的设备信息 Hash，用于设备列表与会话快照读取。
//
// 在线状态的事实源是 connect 维护的 presence 路由投影，不再由本仓储承载。
func NewDeviceRepository(db *gorm.DB, redisClient *redis.Client) IDeviceRepository {
	return &deviceRepositoryImpl{db: db, redisClient: redisClient}
}

// deviceCacheItem 是写入 Redis 设备信息 Hash 的缓存结构。
//
// 该结构只保留设备列表和在线态计算所需字段，避免直接把数据库模型原样序列化进缓存。
type deviceCacheItem struct {
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	Platform   string `json:"platform"`
	AppVersion string `json:"appVersion"`
	UserAgent  string `json:"userAgent,omitempty"`
	Status     int8   `json:"status"`
	// LoginAt 最后一次状态迁移时刻（登录/上线/下线），RFC3339。
	// 与 DB 的 updated_at 同语义，是离线设备 last_seen 的缓存来源。
	LoginAt string `json:"loginAt"`
}

// parseCacheTransitionAt 解析缓存中的最后状态迁移时刻；格式异常时返回零值，
// 调用方按"无可用时间"处理。
func parseCacheTransitionAt(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

// accessTokenKey 构造某个用户单设备 AccessToken 的 Redis Key。
func (r *deviceRepositoryImpl) accessTokenKey(userUUID, deviceID string) string {
	return rediskey.AccessTokenKey(userUUID, deviceID)
}

// refreshTokenKey 构造某个用户单设备 RefreshToken 的 Redis Key。
func (r *deviceRepositoryImpl) refreshTokenKey(userUUID, deviceID string) string {
	return rediskey.RefreshTokenKey(userUUID, deviceID)
}

// deviceInfoKey 构造用户设备信息 Hash 的 Redis Key。
func (r *deviceRepositoryImpl) deviceInfoKey(userUUID string) string {
	return rediskey.DeviceInfoKey(userUUID)
}

// md5Hash 计算 Token 摘要，避免把原始 AccessToken 明文直接写入 Redis。
func md5Hash(s string) string {
	h := md5.New()
	_, _ = h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// GetByUserUUID 获取用户的全部设备会话。
func (r *deviceRepositoryImpl) GetByUserUUID(ctx context.Context, userUUID string) ([]*model.DeviceSession, error) {
	var sessions []*model.DeviceSession
	err := r.db.WithContext(ctx).
		Where("user_uuid = ?", userUUID).
		Order("updated_at DESC, id DESC").
		Find(&sessions).Error
	if err != nil {
		return nil, WrapDBError(err)
	}
	return sessions, nil
}

// GetByDeviceID 根据设备 ID 获取设备会话。
func (r *deviceRepositoryImpl) GetByDeviceID(ctx context.Context, userUUID, deviceID string) (*model.DeviceSession, error) {
	var session model.DeviceSession
	// 这里按 (user_uuid, device_id) 组合键精确定位单设备会话，避免跨用户误读同名设备。
	err := r.db.WithContext(ctx).
		Where("user_uuid = ? AND device_id = ?", userUUID, deviceID).
		First(&session).Error
	if err != nil {
		return nil, WrapDBError(err)
	}
	return &session, nil
}

// UpsertSession 创建或更新设备会话。
func (r *deviceRepositoryImpl) UpsertSession(ctx context.Context, session *model.DeviceSession) error {
	// created_at / updated_at 统一取当前时间，保证插入和更新路径共享同一时间基准。
	now := time.Now()
	// 登录成功后会话状态强制视为 online，避免沿用旧状态写回数据库。
	onlineStatus := model.DeviceStatusOnline
	session.Status = onlineStatus

	// 使用 INSERT ... ON DUPLICATE KEY UPDATE 统一处理“首次登录”和“同设备重复登录”两条路径：
	//  1. 首次登录时插入完整设备会话；
	//  2. 重复登录时覆盖设备展示信息、IP、UserAgent 和状态；
	//  3. updated_at 始终刷新为当前登录时间，供设备列表排序使用。
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO device_sessions (
			user_uuid, device_id, device_name, platform,
			app_version, ip, user_agent, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			device_name = VALUES(device_name),
			platform = VALUES(platform),
			app_version = VALUES(app_version),
			ip = VALUES(ip),
			user_agent = VALUES(user_agent),
			status = ?,
			updated_at = VALUES(updated_at)
	`, session.UserUuid, session.DeviceId, session.DeviceName, session.Platform,
		session.AppVersion, session.IP, session.UserAgent, onlineStatus, now, now, onlineStatus).Error

	if err != nil {
		return WrapDBError(err)
	}

	// 只有数据库权威记录成功后，才刷新 Redis 里的设备快照缓存。
	r.storeDeviceInfoCache(ctx, session, now)
	return nil
}

// storeDeviceInfoCache 将单设备会话快照写入 Redis Hash。
//
// 主流程以数据库写入为准；缓存刷新失败时只补偿删除整个用户设备缓存，
// 避免延迟重放旧快照覆盖更新后的数据库状态。
func (r *deviceRepositoryImpl) storeDeviceInfoCache(ctx context.Context, session *model.DeviceSession, loginAt time.Time) {
	if r.redisClient == nil {
		return
	}
	// 先裁剪出设备列表真正需要的字段，避免把数据库模型完整塞进缓存。
	item := deviceCacheItem{
		DeviceID:   session.DeviceId,
		DeviceName: session.DeviceName,
		Platform:   session.Platform,
		AppVersion: session.AppVersion,
		UserAgent:  session.UserAgent,
		Status:     session.Status,
		LoginAt:    loginAt.UTC().Format(time.RFC3339),
	}
	// JSON 序列化失败说明缓存载荷本身不可用，此时只记录降级日志，不影响主流程。
	value, err := json.Marshal(item)
	if err != nil {
		LogRedisError(ctx, err)
		return
	}

	key := r.deviceInfoKey(session.UserUuid)
	pipe := r.redisClient.Pipeline()
	// field 使用 device_id，便于设备列表按用户一次性 HGetAll 拉全量快照。
	pipe.HSet(ctx, key, session.DeviceId, value)
	// 每次写设备快照时顺手续期整个 Hash，保持热点用户缓存常驻。
	pipe.Expire(ctx, key, rediskey.DeviceInfoTTL)
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		task := redisretry.BuildDelTask(key).
			WithSource("DeviceRepository.storeDeviceInfoCache")
		redisretry.LogAndRetryRedisError(ctx, task, err)
	}
}

// TouchDeviceInfoTTL 续期设备信息缓存 TTL。
func (r *deviceRepositoryImpl) TouchDeviceInfoTTL(ctx context.Context, userUUID string) error {
	if r.redisClient == nil {
		return nil
	}
	// 这里只续期整个用户设备 Hash，不单独续期某个 field，保持缓存结构简单。
	key := r.deviceInfoKey(userUUID)
	err := r.redisClient.Expire(ctx, key, rediskey.DeviceInfoTTL).Err()
	if err != nil {
		// 延迟续期可能延长已经失效的旧快照，因此只把错误交给调用方同步处理。
		return WrapRedisError(err)
	}
	return nil
}

// BatchGetOnlineStatus 批量获取用户设备会话。
//
// 查询顺序保持“Redis 设备 Hash -> MySQL 回源 -> 尽力回填缓存”的分层策略：
//  1. 先对 user_uuid 去重，避免重复读取同一用户缓存；
//  2. 优先从 Redis Hash 还原设备快照，命中则直接返回；
//  3. 反序列化失败或缓存缺失时回源 MySQL；
//  4. 将数据库结果按用户分组后回写缓存，供后续在线态查询复用。
func (r *deviceRepositoryImpl) BatchGetOnlineStatus(ctx context.Context, userUUIDs []string) (map[string][]*model.DeviceSession, error) {
	result := make(map[string][]*model.DeviceSession, len(userUUIDs))
	if len(userUUIDs) == 0 {
		return result, nil
	}

	// 先按 user_uuid 去重，避免同一请求里重复读取同一个用户的设备缓存。
	uniqueUsers := make([]string, 0, len(userUUIDs))
	seenUsers := make(map[string]struct{}, len(userUUIDs))
	for _, userUUID := range userUUIDs {
		// 空 user_uuid 没有查询意义，直接跳过。
		if userUUID == "" {
			continue
		}

		// 同一用户只保留第一次出现，保持后续 Redis / MySQL 查询量最小。
		if _, ok := seenUsers[userUUID]; ok {
			continue
		}
		seenUsers[userUUID] = struct{}{}
		uniqueUsers = append(uniqueUsers, userUUID)
	}

	if len(uniqueUsers) == 0 {
		return result, nil
	}

	missedUsers := make([]string, 0, len(uniqueUsers))
	if r.redisClient != nil {
		// 先尝试批量走 Redis Hash，命中时可直接还原设备快照。
		pipe := r.redisClient.Pipeline()
		cacheCmds := make(map[string]*redis.MapStringStringCmd, len(uniqueUsers))
		for _, userUUID := range uniqueUsers {
			cacheCmds[userUUID] = pipe.HGetAll(ctx, r.deviceInfoKey(userUUID))
		}
		_, err := pipe.Exec(ctx)
		if err != nil && err != redis.Nil {
			// 整体缓存读取失败时统一降级到 MySQL 回源。
			LogRedisError(ctx, err)
			missedUsers = append(missedUsers, uniqueUsers...)
		} else {
			for _, userUUID := range uniqueUsers {
				entries := cacheCmds[userUUID].Val()

				// Hash 不存在或为空时，记为缓存 miss，稍后回源数据库。
				if len(entries) == 0 {
					missedUsers = append(missedUsers, userUUID)
					continue
				}

				sessions := make([]*model.DeviceSession, 0, len(entries))
				parseErrCount := 0
				for _, raw := range entries {
					var item deviceCacheItem
					// 任意一条缓存反序列化失败都视为该用户缓存不可信，后续整用户回源 MySQL。
					if err := json.Unmarshal([]byte(raw), &item); err != nil {
						parseErrCount++
						continue
					}

					// 只还原设备列表和在线态判断需要的最小字段集合。
					sessions = append(sessions, &model.DeviceSession{
						UserUuid:   userUUID,
						DeviceId:   item.DeviceID,
						DeviceName: item.DeviceName,
						Platform:   item.Platform,
						AppVersion: item.AppVersion,
						UserAgent:  item.UserAgent,
						Status:     item.Status,
						UpdatedAt:  parseCacheTransitionAt(item.LoginAt),
					})
				}

				// 只要出现解析失败，就以数据库结果为准，避免返回半新半旧缓存视图。
				if parseErrCount > 0 {
					missedUsers = append(missedUsers, userUUID)
					continue
				}

				// 缓存全部可用时，直接把该用户结果写入返回集。
				result[userUUID] = sessions
			}
		}
	} else {
		// 没有 Redis 时所有用户都走数据库回源。
		missedUsers = append(missedUsers, uniqueUsers...)
	}

	if len(missedUsers) > 0 {
		// 对所有缓存 miss / 缓存损坏的用户做一次批量数据库回源。
		var dbSessions []*model.DeviceSession
		err := r.db.WithContext(ctx).
			Where("user_uuid IN ?", missedUsers).
			Order("updated_at DESC, id DESC").
			Find(&dbSessions).Error

		if err != nil {
			return nil, WrapDBError(err)
		}

		// 先按 user_uuid 聚合数据库记录，便于后续按用户回填 result。
		dbGrouped := make(map[string][]*model.DeviceSession, len(missedUsers))
		for _, session := range dbSessions {
			// 防御空指针和空 user_uuid 脏数据。
			if session == nil || session.UserUuid == "" {
				continue
			}
			dbGrouped[session.UserUuid] = append(dbGrouped[session.UserUuid], session)
		}

		for _, userUUID := range missedUsers {
			// 即使某个用户没有任何设备会话，也显式写入 nil，保持结果键完整。
			result[userUUID] = dbGrouped[userUUID]
		}

		if r.redisClient != nil && len(dbSessions) > 0 {
			// 数据库回源成功后，尽力把结果重新写回缓存，提升后续命中率。
			pipe := r.redisClient.Pipeline()
			touchedUsers := make(map[string]struct{}, len(dbSessions))
			for _, session := range dbSessions {
				// 缓存回填前再次过滤无效会话，避免写入空 field 或空 key。
				if session == nil || session.UserUuid == "" || session.DeviceId == "" {
					continue
				}

				// 回填时仍只写 deviceCacheItem 允许暴露的最小字段集合。
				item := deviceCacheItem{
					DeviceID:   session.DeviceId,
					DeviceName: session.DeviceName,
					Platform:   session.Platform,
					AppVersion: session.AppVersion,
					UserAgent:  session.UserAgent,
					Status:     session.Status,
					LoginAt:    session.UpdatedAt.UTC().Format(time.RFC3339),
				}

				// 单条序列化失败只跳过该设备，不影响其他用户缓存回填。
				value, mErr := json.Marshal(item)
				if mErr != nil {
					continue
				}
				key := r.deviceInfoKey(session.UserUuid)

				// 按 device_id 覆盖写入单设备快照。
				pipe.HSet(ctx, key, session.DeviceId, value)
				touchedUsers[session.UserUuid] = struct{}{}
			}

			for userUUID := range touchedUsers {
				// 只给本次确实触碰过的用户续期，避免无意义扩大写入范围。
				pipe.Expire(ctx, r.deviceInfoKey(userUUID), rediskey.DeviceInfoTTL)
			}
			if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
				// 回填缓存失败只记降级日志，不影响主查询结果返回。
				LogRedisError(ctx, err)
			}
		}
	}

	for _, userUUID := range uniqueUsers {
		// 对完全没有命中结果的用户显式补 nil，保证调用方可按 key 直接读取。
		if _, ok := result[userUUID]; !ok {
			result[userUUID] = nil
		}
	}

	return result, nil
}

// StoreAccessToken 存储 AccessToken。
func (r *deviceRepositoryImpl) StoreAccessToken(ctx context.Context, userUUID, deviceID, accessToken string, expireDuration time.Duration) error {
	if r.redisClient == nil {
		return ErrRedis
	}
	key := r.accessTokenKey(userUUID, deviceID)
	// AccessToken 落 Redis 前先做 md5 摘要，避免明文 token 直接驻留缓存。
	value := md5Hash(accessToken)
	err := r.redisClient.Set(ctx, key, value, expireDuration).Err()
	if err != nil {
		// 登录态写入失败必须让登录链路失败，禁止稍后写入已经失效的 token。
		return WrapRedisError(err)
	}
	return nil
}

// StoreRefreshToken 存储 RefreshToken。
func (r *deviceRepositoryImpl) StoreRefreshToken(ctx context.Context, userUUID, deviceID, refreshToken string, expireDuration time.Duration) error {
	if r.redisClient == nil {
		return ErrRedis
	}
	key := r.refreshTokenKey(userUUID, deviceID)
	// RefreshToken 当前仍按明文保存，便于后续刷新链路做字面量比对。
	err := r.redisClient.Set(ctx, key, refreshToken, expireDuration).Err()
	if err != nil {
		// RefreshToken 同样可能在重试期间被轮换，禁止异步写回旧值。
		return WrapRedisError(err)
	}
	return nil
}

// GetRefreshToken 获取 RefreshToken。
func (r *deviceRepositoryImpl) GetRefreshToken(ctx context.Context, userUUID, deviceID string) (string, error) {
	if r.redisClient == nil {
		return "", ErrRedisNil
	}
	// 这里直接按 (user_uuid, device_id) 精确读取单设备 RefreshToken。
	value, err := r.redisClient.Get(ctx, r.refreshTokenKey(userUUID, deviceID)).Result()
	if err != nil {
		return "", WrapRedisError(err)
	}
	return value, nil
}

// DeleteTokens 删除指定设备的全部 Token。
func (r *deviceRepositoryImpl) DeleteTokens(ctx context.Context, userUUID, deviceID string) error {
	if r.redisClient == nil {
		return nil
	}
	// 同一设备的访问令牌和刷新令牌必须一起删除，避免残留半失效登录态。
	accessKey := r.accessTokenKey(userUUID, deviceID)
	refreshKey := r.refreshTokenKey(userUUID, deviceID)
	pipe := r.redisClient.Pipeline()
	// 两个 DEL 合并进一次 pipeline，减少退出登录时的网络往返。
	pipe.Del(ctx, accessKey)
	pipe.Del(ctx, refreshKey)
	if _, err := pipe.Exec(ctx); err != nil {
		// 设备重新登录会复用这些 key，延迟 DEL 可能误删新 token。
		return WrapRedisError(err)
	}
	return nil
}

// DeleteByUserUUID 删除用户的全部设备登录态。
//
// 该方法用于注销账号、主动退出全端登录等场景：
//  1. 先枚举该用户历史设备并删除 Redis Token；
//  2. 再把数据库中的会话状态统一标记为 logged_out；
//  3. 最后清理设备信息与活跃时间缓存，避免旧在线态继续被读到。
func (r *deviceRepositoryImpl) DeleteByUserUUID(ctx context.Context, userUUID string) error {
	// 先拉出该用户历史设备列表，后续要逐设备清除 Redis Token。
	sessions, err := r.GetByUserUUID(ctx, userUUID)
	if err != nil {
		return err
	}

	for _, session := range sessions {
		// 防御无效设备会话，避免删除空 device_id 对应的键。
		if session == nil || session.DeviceId == "" {
			continue
		}

		// 逐设备删除 Token；任一失败都直接上抛，保证注销主语义可靠。
		if err := r.DeleteTokens(ctx, userUUID, session.DeviceId); err != nil {
			return err
		}
	}

	// Token 清完后，再统一把数据库里的设备会话状态改成 logged_out。
	err = r.db.WithContext(ctx).Model(&model.DeviceSession{}).
		Where("user_uuid = ? AND deleted_at IS NULL", userUUID).
		Updates(map[string]interface{}{
			"status":     model.DeviceStatusLoggedOut,
			"updated_at": time.Now(),
		}).Error

	if err != nil {
		return WrapDBError(err)
	}

	if r.redisClient != nil {
		// 最后删除设备信息 Hash，防止旧会话快照继续被读到。
		deviceInfoKey := r.deviceInfoKey(userUUID)
		if err := r.redisClient.Del(ctx, deviceInfoKey).Err(); err != nil && err != redis.Nil {
			task := redisretry.BuildDelTask(deviceInfoKey).
				WithSource("DeviceRepository.DeleteByUserUUID")
			redisretry.LogAndRetryRedisError(ctx, task, err)
		}
	}

	return nil
}

// UpdateOnlineStatus 更新设备在线状态。
//
// 状态写入先落 MySQL，再尝试同步刷新 Redis 设备信息 Hash；如果缓存缺失或刷新失败，
// 保持最终一致即可，由后续读路径重新回源和回填。
func (r *deviceRepositoryImpl) UpdateOnlineStatus(ctx context.Context, userUUID, deviceID string, status int8) error {
	// 先更新数据库权威状态；缓存刷新始终建立在 DB 成功的前提下。
	result := r.db.WithContext(ctx).Model(&model.DeviceSession{}).
		Where("user_uuid = ? AND device_id = ? AND deleted_at IS NULL", userUUID, deviceID).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return WrapDBError(result.Error)
	}

	// 没有命中任何会话记录时，明确返回 not found，供上层做幂等判断。
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}

	// 没有 Redis 时到此为止，数据库状态已经是最终权威结果。
	if r.redisClient == nil {
		return nil
	}

	cacheKey := r.deviceInfoKey(userUUID)

	// 只尝试刷新该用户该设备的单条缓存 field，避免整用户缓存重建。
	raw, err := r.redisClient.HGet(ctx, cacheKey, deviceID).Result()

	if err == redis.Nil {
		// 缓存缺失属于正常情况，交给后续读路径按需重建即可。
		return nil
	}
	if err != nil {
		// 缓存读取失败只做降级日志，不回滚已成功的数据库写入。
		LogRedisError(ctx, err)
		return nil
	}

	var item deviceCacheItem
	// 单条缓存坏掉时不阻塞主流程，后续读路径会重新回源覆盖。
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		LogRedisError(ctx, err)
		return nil
	}

	// 覆盖 status 并刷新最后状态迁移时刻，其余设备展示信息沿用原缓存值；
	// 该时刻是离线设备 last_seen 的缓存来源，需与 DB 的 updated_at 保持同步。
	item.Status = status
	item.LoginAt = time.Now().UTC().Format(time.RFC3339)
	value, err := json.Marshal(item)
	if err != nil {
		// 重新序列化失败说明缓存载荷异常，同样按降级处理即可。
		LogRedisError(ctx, err)
		return nil
	}

	pipe := r.redisClient.Pipeline()

	// 写回单设备状态后顺手续期整个设备信息 Hash。
	pipe.HSet(ctx, cacheKey, deviceID, value)
	pipe.Expire(ctx, cacheKey, rediskey.DeviceInfoTTL)
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		task := redisretry.BuildDelTask(cacheKey).
			WithSource("DeviceRepository.UpdateOnlineStatus")
		redisretry.LogAndRetryRedisError(ctx, task, err)
	}

	return nil
}
