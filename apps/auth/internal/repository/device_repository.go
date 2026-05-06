package repository

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"

	"github.com/013677890/LCchat-Backend/apps/auth/mq"
	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	pkgdeviceactive "github.com/013677890/LCchat-Backend/pkg/deviceactive"
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
// 该仓储同时维护三类设备数据：
//  1. MySQL 中的 device_sessions 权威记录；
//  2. Redis 中的设备信息 Hash，用于设备列表与会话快照读取；
//  3. Redis ZSet 中的活跃时间索引，用于在线状态判断。
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
	LoginAt    string `json:"loginAt"`
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

// deviceActiveKey 构造用户设备活跃时间 ZSet 的 Redis Key。
func (r *deviceRepositoryImpl) deviceActiveKey(userUUID string) string {
	return rediskey.DeviceActiveKey(userUUID)
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
// 这里属于 DB 成功后的补偿型缓存刷新：主流程以数据库写入为准，缓存失败时通过补偿任务
// 异步重试，避免因为缓存抖动阻塞登录链路。
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
		// 设备信息缓存写入属于 DB 成功后的补偿型缓存刷新，失败时异步重试即可。
		task := mq.BuildPipelineTask([]mq.RedisCmd{
			{Command: "hset", Args: []interface{}{key, session.DeviceId, string(value)}},
			{Command: "expire", Args: []interface{}{key, int(rediskey.DeviceInfoTTL.Seconds())}},
		}).WithSource("DeviceRepository.storeDeviceInfoCache")
		LogAndRetryRedisError(ctx, task, err)
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
		// TTL 续期属于缓存保活场景，允许通过异步补偿恢复最终一致。
		task := mq.BuildPipelineTask([]mq.RedisCmd{
			{Command: "expire", Args: []interface{}{key, int(rediskey.DeviceInfoTTL.Seconds())}},
		}).WithSource("DeviceRepository.TouchDeviceInfoTTL")
		LogAndRetryRedisError(ctx, task, err)
		return WrapRedisError(err)
	}
	return nil
}

// GetActiveTimestamps 获取设备活跃时间戳。
func (r *deviceRepositoryImpl) GetActiveTimestamps(ctx context.Context, userUUID string, deviceIDs []string) (map[string]int64, error) {
	result := make(map[string]int64, len(deviceIDs))
	// 没有设备可查或 Redis 不可用时，直接返回空结果，交给上层按离线处理。
	if len(deviceIDs) == 0 || r.redisClient == nil {
		return result, nil
	}

	// cutoff 之外的数据即使仍在 ZSet 中也视为离线，不再回给上层。
	cutoff := pkgdeviceactive.CutoffUnix(time.Now())
	key := r.deviceActiveKey(userUUID)
	pipe := r.redisClient.Pipeline()
	scoreCmds := make(map[string]*redis.FloatCmd, len(deviceIDs))
	for _, deviceID := range deviceIDs {
		// 每个 device_id 对应一个 ZScore 命令，最后统一 pipeline 执行减少 RTT。
		scoreCmds[deviceID] = pipe.ZScore(ctx, key, deviceID)
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, WrapRedisError(err)
	}

	for deviceID, cmd := range scoreCmds {
		score, err := cmd.Result()
		// 单个设备没有活跃记录属于正常 miss，直接跳过即可。
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return nil, WrapRedisError(err)
		}
		sec := int64(score)
		// 活跃时间早于在线窗口的记录不再算在线设备。
		if sec < cutoff {
			continue
		}
		result[deviceID] = sec
	}
	return result, nil
}

// BatchGetActiveTimestamps 批量获取多用户设备活跃时间戳。
//
// 该接口面向“多用户、多设备”的在线态聚合查询，整体策略与单用户版本保持一致：
//  1. 先过滤空 user_uuid / device_id，并在每个用户组内完成设备去重；
//  2. 对所有 ZScore 命令统一走 pipeline，减少跨用户批量查询时的 RTT；
//  3. 只返回仍位于在线窗口内的时间戳，窗口外数据一律视为离线。
func (r *deviceRepositoryImpl) BatchGetActiveTimestamps(ctx context.Context, userDeviceIDs map[string][]string) (map[string]map[string]int64, error) {
	result := make(map[string]map[string]int64, len(userDeviceIDs))
	// 没有输入或 Redis 不可用时，直接返回空结果，由上层按离线态降级。
	if len(userDeviceIDs) == 0 || r.redisClient == nil {
		return result, nil
	}

	// cutoff 之前的活跃记录统一视为离线，避免把陈旧心跳继续暴露给上层。
	cutoff := pkgdeviceactive.CutoffUnix(time.Now())
	pipe := r.redisClient.Pipeline()
	// scoreCmds 保存 user_uuid -> device_id -> ZScore 命令句柄，便于统一回填结果。
	scoreCmds := make(map[string]map[string]*redis.FloatCmd, len(userDeviceIDs))
	for userUUID, deviceIDs := range userDeviceIDs {
		// 空 user_uuid 或空设备列表都没有查询意义，直接跳过。
		if userUUID == "" || len(deviceIDs) == 0 {
			continue
		}
		key := r.deviceActiveKey(userUUID)
		// userCmds 负责当前用户组内的 device_id 去重。
		userCmds := make(map[string]*redis.FloatCmd, len(deviceIDs))
		for _, deviceID := range deviceIDs {
			// 空 device_id 无法命中任何 member，直接过滤。
			if deviceID == "" {
				continue
			}
			// 同一用户下重复设备只保留一条命令，避免无意义重复读。
			if _, ok := userCmds[deviceID]; ok {
				continue
			}
			// 每个 device_id 对应一条 ZScore，最终由 pipeline 一次性执行。
			userCmds[deviceID] = pipe.ZScore(ctx, key, deviceID)
		}
		// 当前用户至少保留一条有效设备命令时才纳入后续结果处理。
		if len(userCmds) > 0 {
			scoreCmds[userUUID] = userCmds
		}
	}
	// 全量过滤后已无任何有效查询，就无需触发 Redis 请求。
	if len(scoreCmds) == 0 {
		return result, nil
	}

	// 一次性执行所有 ZScore，减少多用户多设备场景下的网络往返。
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, WrapRedisError(err)
	}

	for userUUID, userCmds := range scoreCmds {
		// 预分配当前用户结果 map，减少命中设备较多时的扩容开销。
		userResult := make(map[string]int64, len(userCmds))
		for deviceID, cmd := range userCmds {
			score, err := cmd.Result()
			// 单设备没有活跃记录属于正常 miss，不视为错误。
			if err == redis.Nil {
				continue
			}
			// 任一真实 Redis 错误都中止返回，避免混入不完整在线态。
			if err != nil {
				return nil, WrapRedisError(err)
			}
			sec := int64(score)
			// 早于在线窗口的记录不再算在线设备。
			if sec < cutoff {
				continue
			}
			userResult[deviceID] = sec
		}
		// 仅当当前用户至少有一台在线设备时才写回结果集。
		if len(userResult) > 0 {
			result[userUUID] = userResult
		}
	}
	return result, nil
}

// BatchGetLastSeenTimestamps 批量获取用户最近活跃时间戳。
//
// 每个用户只取活跃 ZSet 的最高分记录，表示“最近一次设备活跃时间”。
func (r *deviceRepositoryImpl) BatchGetLastSeenTimestamps(ctx context.Context, userUUIDs []string) (map[string]int64, error) {
	result := make(map[string]int64, len(userUUIDs))
	// 没有用户或 Redis 不可用时，直接返回空结果，由上层按“未知最近活跃时间”处理。
	if len(userUUIDs) == 0 || r.redisClient == nil {
		return result, nil
	}

	pipe := r.redisClient.Pipeline()
	// lastSeenCmds 保存每个 user_uuid 对应的“取最新一条活跃记录”命令句柄。
	lastSeenCmds := make(map[string]*redis.ZSliceCmd, len(userUUIDs))
	for _, userUUID := range userUUIDs {
		// 空 user_uuid 无法定位 ZSet，直接跳过。
		if userUUID == "" {
			continue
		}
		// 同一用户只取一次最近活跃记录，避免重复发命令。
		if _, ok := lastSeenCmds[userUUID]; ok {
			continue
		}
		// 取 ZSet 最高分记录即可拿到该用户最近一次设备活跃时间。
		lastSeenCmds[userUUID] = pipe.ZRevRangeWithScores(ctx, r.deviceActiveKey(userUUID), 0, 0)
	}
	// 全量过滤后无有效用户时，直接返回空结果。
	if len(lastSeenCmds) == 0 {
		return result, nil
	}

	// 统一执行最近活跃时间查询，减少批量场景下的 RTT。
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, WrapRedisError(err)
	}

	for userUUID, cmd := range lastSeenCmds {
		entries, err := cmd.Result()
		// 没有任何活跃记录时，保持该用户缺省为空即可。
		if err == redis.Nil || len(entries) == 0 {
			continue
		}
		// Redis 真错误直接返回，避免把部分用户结果当成完整结果使用。
		if err != nil {
			return nil, WrapRedisError(err)
		}
		sec := int64(entries[0].Score)
		// 只回填正数时间戳，过滤异常分值或脏数据。
		if sec > 0 {
			result[userUUID] = sec
		}
	}
	return result, nil
}

// SetActiveTimestamp 设置单设备活跃时间。
func (r *deviceRepositoryImpl) SetActiveTimestamp(ctx context.Context, userUUID, deviceID string, ts int64) error {
	// 单设备写入只是批量接口的便捷包装，确保两条路径复用同一套去重和补偿逻辑。
	if r.redisClient == nil {
		return nil
	}
	return r.BatchSetActiveTimestamps(ctx, []DeviceActiveItem{{UserUUID: userUUID, DeviceID: deviceID}}, ts)
}

// BatchSetActiveTimestamps 批量设置设备活跃时间。
func (r *deviceRepositoryImpl) BatchSetActiveTimestamps(ctx context.Context, items []DeviceActiveItem, ts int64) error {
	// Redis 不可用或没有任何输入时，直接返回；在线态允许上层按离线或旧值降级处理。
	if r.redisClient == nil || len(items) == 0 {
		return nil
	}

	// 先按 user_uuid 分组，并在组内对 device_id 去重，避免同一批次重复写相同 member。
	grouped := make(map[string]map[string]struct{})
	for _, item := range items {
		// user_uuid 或 device_id 缺失的脏数据不参与任何 Redis 写入。
		if item.UserUUID == "" || item.DeviceID == "" {
			continue
		}
		deviceSet := grouped[item.UserUUID]
		// 第一次遇到某个 user_uuid 时先初始化其设备去重集合。
		if deviceSet == nil {
			deviceSet = make(map[string]struct{})
			grouped[item.UserUUID] = deviceSet
		}
		// map-set 结构天然完成去重，重复 device_id 只会保留一份。
		deviceSet[item.DeviceID] = struct{}{}
	}
	// 过滤后若已没有任何有效设备，就无需再发 Redis 请求。
	if len(grouped) == 0 {
		return nil
	}

	// cutoff 之前的 score 会被统一裁剪掉，保证 ZSet 里只保留在线窗口内的数据。
	cutoff := pkgdeviceactive.CutoffUnix(time.Now())
	pipe := r.redisClient.Pipeline()
	// retryCmds 和 pipeline 保持一一对应，便于失败时按原语义做补偿重放。
	retryCmds := make([]mq.RedisCmd, 0, len(grouped)*3)
	for userUUID, deviceSet := range grouped {
		// 每个用户独立维护一个活跃时间 ZSet。
		key := r.deviceActiveKey(userUUID)
		// zItems 给当前 pipeline 执行使用；zArgs 给补偿任务原样重放使用。
		zItems := make([]redis.Z, 0, len(deviceSet))
		zArgs := make([]interface{}, 0, 1+len(deviceSet)*2)
		zArgs = append(zArgs, key)
		for deviceID := range deviceSet {
			// score 统一使用传入 ts；member 使用 device_id，便于后续按设备读取最近活跃时间。
			zItems = append(zItems, redis.Z{Score: float64(ts), Member: deviceID})
			zArgs = append(zArgs, float64(ts), deviceID)
		}
		// 理论上 deviceSet 已非空；这里再做一次保护，防止异常输入污染后续命令。
		if len(zItems) == 0 {
			continue
		}
		// 先批量写入/覆盖当前批次设备活跃时间。
		pipe.ZAdd(ctx, key, zItems...)
		// 再裁掉在线窗口之外的旧分数，防止 ZSet 无上限增长。
		pipe.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(cutoff, 10))
		// 最后续期整个 ZSet，保证热点用户的在线态索引持续可读。
		pipe.Expire(ctx, key, rediskey.DeviceActiveTTL)
		retryCmds = append(retryCmds,
			mq.RedisCmd{Command: "zadd", Args: zArgs},
			mq.RedisCmd{Command: "zremrangebyscore", Args: []interface{}{key, "-inf", strconv.FormatInt(cutoff, 10)}},
			mq.RedisCmd{Command: "expire", Args: []interface{}{key, int(rediskey.DeviceActiveTTL.Seconds())}},
		)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		// 设备活跃时间允许最终一致，失败后通过 Kafka 补偿重放即可。
		task := mq.BuildPipelineTask(retryCmds).
			WithSource("DeviceRepository.BatchSetActiveTimestamps")
		LogAndRetryRedisError(ctx, task, err)
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
		// Token 写入是登录态主链路的高可靠 Redis 写，失败时同步报错并追加异步补偿。
		task := mq.BuildSetTask(key, value, expireDuration).
			WithSource("DeviceRepository.StoreAccessToken")
		LogAndRetryRedisError(ctx, task, err)
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
		// RefreshToken 写入同样需要高可靠兜底，失败时异步补偿重试。
		task := mq.BuildSetTask(key, refreshToken, expireDuration).
			WithSource("DeviceRepository.StoreRefreshToken")
		LogAndRetryRedisError(ctx, task, err)
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
		// Token 删除允许短暂最终一致，失败后异步补偿可继续清理残留登录态。
		task := mq.BuildPipelineTask([]mq.RedisCmd{
			{Command: "del", Args: []interface{}{accessKey}},
			{Command: "del", Args: []interface{}{refreshKey}},
		}).WithSource("DeviceRepository.DeleteTokens")
		LogAndRetryRedisError(ctx, task, err)
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
		// 最后删除设备信息 Hash 和活跃时间 ZSet，防止旧在线态继续被读到。
		deviceInfoKey := r.deviceInfoKey(userUUID)
		deviceActiveKey := r.deviceActiveKey(userUUID)
		pipe := r.redisClient.Pipeline()
		pipe.Del(ctx, deviceInfoKey)
		pipe.Del(ctx, deviceActiveKey)
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			// 注销后缓存清理允许降级为最终一致，失败后通过补偿任务继续删除。
			task := mq.BuildPipelineTask([]mq.RedisCmd{
				{Command: "del", Args: []interface{}{deviceInfoKey}},
				{Command: "del", Args: []interface{}{deviceActiveKey}},
			}).WithSource("DeviceRepository.DeleteByUserUUID")
			LogAndRetryRedisError(ctx, task, err)
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
	// 只覆盖 status 字段，其余设备展示信息沿用原缓存值。
	item.Status = status
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
		// 在线状态缓存刷新允许最终一致，失败后异步重试即可。
		task := mq.BuildPipelineTask([]mq.RedisCmd{
			{Command: "hset", Args: []interface{}{cacheKey, deviceID, string(value)}},
			{Command: "expire", Args: []interface{}{cacheKey, int(rediskey.DeviceInfoTTL.Seconds())}},
		}).WithSource("DeviceRepository.UpdateOnlineStatus")
		LogAndRetryRedisError(ctx, task, err)
	}
	return nil
}
