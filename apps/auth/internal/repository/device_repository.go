package repository

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"

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
func NewDeviceRepository(db *gorm.DB, redisClient *redis.Client) IDeviceRepository {
	return &deviceRepositoryImpl{db: db, redisClient: redisClient}
}

type deviceCacheItem struct {
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	Platform   string `json:"platform"`
	AppVersion string `json:"appVersion"`
	UserAgent  string `json:"userAgent,omitempty"`
	Status     int8   `json:"status"`
	LoginAt    string `json:"loginAt"`
}

func (r *deviceRepositoryImpl) accessTokenKey(userUUID, deviceID string) string {
	return rediskey.AccessTokenKey(userUUID, deviceID)
}

func (r *deviceRepositoryImpl) refreshTokenKey(userUUID, deviceID string) string {
	return rediskey.RefreshTokenKey(userUUID, deviceID)
}

func (r *deviceRepositoryImpl) deviceInfoKey(userUUID string) string {
	return rediskey.DeviceInfoKey(userUUID)
}

func (r *deviceRepositoryImpl) deviceActiveKey(userUUID string) string {
	return rediskey.DeviceActiveKey(userUUID)
}

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
	now := time.Now()
	onlineStatus := model.DeviceStatusOnline
	session.Status = onlineStatus

	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO device_session (
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

	r.storeDeviceInfoCache(ctx, session, now)
	return nil
}

func (r *deviceRepositoryImpl) storeDeviceInfoCache(ctx context.Context, session *model.DeviceSession, loginAt time.Time) {
	if r.redisClient == nil {
		return
	}
	item := deviceCacheItem{
		DeviceID:   session.DeviceId,
		DeviceName: session.DeviceName,
		Platform:   session.Platform,
		AppVersion: session.AppVersion,
		UserAgent:  session.UserAgent,
		Status:     session.Status,
		LoginAt:    loginAt.UTC().Format(time.RFC3339),
	}
	value, err := json.Marshal(item)
	if err != nil {
		LogRedisError(ctx, err)
		return
	}

	pipe := r.redisClient.Pipeline()
	pipe.HSet(ctx, r.deviceInfoKey(session.UserUuid), session.DeviceId, value)
	pipe.Expire(ctx, r.deviceInfoKey(session.UserUuid), rediskey.DeviceInfoTTL)
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		LogRedisError(ctx, err)
	}
}

// TouchDeviceInfoTTL 续期设备信息缓存 TTL。
func (r *deviceRepositoryImpl) TouchDeviceInfoTTL(ctx context.Context, userUUID string) error {
	if r.redisClient == nil {
		return nil
	}
	err := r.redisClient.Expire(ctx, r.deviceInfoKey(userUUID), rediskey.DeviceInfoTTL).Err()
	if err != nil {
		return WrapRedisError(err)
	}
	return nil
}

// GetActiveTimestamps 获取设备活跃时间戳。
func (r *deviceRepositoryImpl) GetActiveTimestamps(ctx context.Context, userUUID string, deviceIDs []string) (map[string]int64, error) {
	result := make(map[string]int64, len(deviceIDs))
	if len(deviceIDs) == 0 || r.redisClient == nil {
		return result, nil
	}

	cutoff := pkgdeviceactive.CutoffUnix(time.Now())
	key := r.deviceActiveKey(userUUID)
	pipe := r.redisClient.Pipeline()
	scoreCmds := make(map[string]*redis.FloatCmd, len(deviceIDs))
	for _, deviceID := range deviceIDs {
		scoreCmds[deviceID] = pipe.ZScore(ctx, key, deviceID)
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, WrapRedisError(err)
	}

	for deviceID, cmd := range scoreCmds {
		score, err := cmd.Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return nil, WrapRedisError(err)
		}
		sec := int64(score)
		if sec < cutoff {
			continue
		}
		result[deviceID] = sec
	}
	return result, nil
}

// BatchGetActiveTimestamps 批量获取多用户设备活跃时间戳。
func (r *deviceRepositoryImpl) BatchGetActiveTimestamps(ctx context.Context, userDeviceIDs map[string][]string) (map[string]map[string]int64, error) {
	result := make(map[string]map[string]int64, len(userDeviceIDs))
	if len(userDeviceIDs) == 0 || r.redisClient == nil {
		return result, nil
	}

	cutoff := pkgdeviceactive.CutoffUnix(time.Now())
	pipe := r.redisClient.Pipeline()
	scoreCmds := make(map[string]map[string]*redis.FloatCmd, len(userDeviceIDs))
	for userUUID, deviceIDs := range userDeviceIDs {
		if userUUID == "" || len(deviceIDs) == 0 {
			continue
		}
		key := r.deviceActiveKey(userUUID)
		userCmds := make(map[string]*redis.FloatCmd, len(deviceIDs))
		for _, deviceID := range deviceIDs {
			if deviceID == "" {
				continue
			}
			if _, ok := userCmds[deviceID]; ok {
				continue
			}
			userCmds[deviceID] = pipe.ZScore(ctx, key, deviceID)
		}
		if len(userCmds) > 0 {
			scoreCmds[userUUID] = userCmds
		}
	}
	if len(scoreCmds) == 0 {
		return result, nil
	}

	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, WrapRedisError(err)
	}

	for userUUID, userCmds := range scoreCmds {
		userResult := make(map[string]int64, len(userCmds))
		for deviceID, cmd := range userCmds {
			score, err := cmd.Result()
			if err == redis.Nil {
				continue
			}
			if err != nil {
				return nil, WrapRedisError(err)
			}
			sec := int64(score)
			if sec < cutoff {
				continue
			}
			userResult[deviceID] = sec
		}
		if len(userResult) > 0 {
			result[userUUID] = userResult
		}
	}
	return result, nil
}

// BatchGetLastSeenTimestamps 批量获取用户最近活跃时间戳。
func (r *deviceRepositoryImpl) BatchGetLastSeenTimestamps(ctx context.Context, userUUIDs []string) (map[string]int64, error) {
	result := make(map[string]int64, len(userUUIDs))
	if len(userUUIDs) == 0 || r.redisClient == nil {
		return result, nil
	}

	pipe := r.redisClient.Pipeline()
	lastSeenCmds := make(map[string]*redis.ZSliceCmd, len(userUUIDs))
	for _, userUUID := range userUUIDs {
		if userUUID == "" {
			continue
		}
		if _, ok := lastSeenCmds[userUUID]; ok {
			continue
		}
		lastSeenCmds[userUUID] = pipe.ZRevRangeWithScores(ctx, r.deviceActiveKey(userUUID), 0, 0)
	}
	if len(lastSeenCmds) == 0 {
		return result, nil
	}

	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, WrapRedisError(err)
	}

	for userUUID, cmd := range lastSeenCmds {
		entries, err := cmd.Result()
		if err == redis.Nil || len(entries) == 0 {
			continue
		}
		if err != nil {
			return nil, WrapRedisError(err)
		}
		sec := int64(entries[0].Score)
		if sec > 0 {
			result[userUUID] = sec
		}
	}
	return result, nil
}

// SetActiveTimestamp 设置单设备活跃时间。
func (r *deviceRepositoryImpl) SetActiveTimestamp(ctx context.Context, userUUID, deviceID string, ts int64) error {
	if r.redisClient == nil {
		return nil
	}
	return r.BatchSetActiveTimestamps(ctx, []DeviceActiveItem{{UserUUID: userUUID, DeviceID: deviceID}}, ts)
}

// BatchSetActiveTimestamps 批量设置设备活跃时间。
func (r *deviceRepositoryImpl) BatchSetActiveTimestamps(ctx context.Context, items []DeviceActiveItem, ts int64) error {
	if r.redisClient == nil || len(items) == 0 {
		return nil
	}

	grouped := make(map[string]map[string]struct{})
	for _, item := range items {
		if item.UserUUID == "" || item.DeviceID == "" {
			continue
		}
		deviceSet := grouped[item.UserUUID]
		if deviceSet == nil {
			deviceSet = make(map[string]struct{})
			grouped[item.UserUUID] = deviceSet
		}
		deviceSet[item.DeviceID] = struct{}{}
	}
	if len(grouped) == 0 {
		return nil
	}

	cutoff := pkgdeviceactive.CutoffUnix(time.Now())
	pipe := r.redisClient.Pipeline()
	for userUUID, deviceSet := range grouped {
		key := r.deviceActiveKey(userUUID)
		zItems := make([]redis.Z, 0, len(deviceSet))
		for deviceID := range deviceSet {
			zItems = append(zItems, redis.Z{Score: float64(ts), Member: deviceID})
		}
		if len(zItems) == 0 {
			continue
		}
		pipe.ZAdd(ctx, key, zItems...)
		pipe.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(cutoff, 10))
		pipe.Expire(ctx, key, rediskey.DeviceActiveTTL)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return WrapRedisError(err)
	}
	return nil
}

// BatchGetOnlineStatus 批量获取用户设备会话。
func (r *deviceRepositoryImpl) BatchGetOnlineStatus(ctx context.Context, userUUIDs []string) (map[string][]*model.DeviceSession, error) {
	result := make(map[string][]*model.DeviceSession, len(userUUIDs))
	if len(userUUIDs) == 0 {
		return result, nil
	}

	uniqueUsers := make([]string, 0, len(userUUIDs))
	seenUsers := make(map[string]struct{}, len(userUUIDs))
	for _, userUUID := range userUUIDs {
		if userUUID == "" {
			continue
		}
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
		pipe := r.redisClient.Pipeline()
		cacheCmds := make(map[string]*redis.MapStringStringCmd, len(uniqueUsers))
		for _, userUUID := range uniqueUsers {
			cacheCmds[userUUID] = pipe.HGetAll(ctx, r.deviceInfoKey(userUUID))
		}
		_, err := pipe.Exec(ctx)
		if err != nil && err != redis.Nil {
			LogRedisError(ctx, err)
			missedUsers = append(missedUsers, uniqueUsers...)
		} else {
			for _, userUUID := range uniqueUsers {
				entries := cacheCmds[userUUID].Val()
				if len(entries) == 0 {
					missedUsers = append(missedUsers, userUUID)
					continue
				}

				sessions := make([]*model.DeviceSession, 0, len(entries))
				parseErrCount := 0
				for _, raw := range entries {
					var item deviceCacheItem
					if err := json.Unmarshal([]byte(raw), &item); err != nil {
						parseErrCount++
						continue
					}
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
				if parseErrCount > 0 {
					missedUsers = append(missedUsers, userUUID)
					continue
				}
				result[userUUID] = sessions
			}
		}
	} else {
		missedUsers = append(missedUsers, uniqueUsers...)
	}

	if len(missedUsers) > 0 {
		var dbSessions []*model.DeviceSession
		err := r.db.WithContext(ctx).
			Where("user_uuid IN ?", missedUsers).
			Order("updated_at DESC, id DESC").
			Find(&dbSessions).Error
		if err != nil {
			return nil, WrapDBError(err)
		}

		dbGrouped := make(map[string][]*model.DeviceSession, len(missedUsers))
		for _, session := range dbSessions {
			if session == nil || session.UserUuid == "" {
				continue
			}
			dbGrouped[session.UserUuid] = append(dbGrouped[session.UserUuid], session)
		}
		for _, userUUID := range missedUsers {
			result[userUUID] = dbGrouped[userUUID]
		}

		if r.redisClient != nil && len(dbSessions) > 0 {
			pipe := r.redisClient.Pipeline()
			touchedUsers := make(map[string]struct{}, len(dbSessions))
			for _, session := range dbSessions {
				if session == nil || session.UserUuid == "" || session.DeviceId == "" {
					continue
				}
				item := deviceCacheItem{
					DeviceID:   session.DeviceId,
					DeviceName: session.DeviceName,
					Platform:   session.Platform,
					AppVersion: session.AppVersion,
					UserAgent:  session.UserAgent,
					Status:     session.Status,
					LoginAt:    session.UpdatedAt.UTC().Format(time.RFC3339),
				}
				value, mErr := json.Marshal(item)
				if mErr != nil {
					continue
				}
				key := r.deviceInfoKey(session.UserUuid)
				pipe.HSet(ctx, key, session.DeviceId, value)
				touchedUsers[session.UserUuid] = struct{}{}
			}
			for userUUID := range touchedUsers {
				pipe.Expire(ctx, r.deviceInfoKey(userUUID), rediskey.DeviceInfoTTL)
			}
			if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
				LogRedisError(ctx, err)
			}
		}
	}

	for _, userUUID := range uniqueUsers {
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
	return WrapRedisError(r.redisClient.Set(ctx, r.accessTokenKey(userUUID, deviceID), md5Hash(accessToken), expireDuration).Err())
}

// StoreRefreshToken 存储 RefreshToken。
func (r *deviceRepositoryImpl) StoreRefreshToken(ctx context.Context, userUUID, deviceID, refreshToken string, expireDuration time.Duration) error {
	if r.redisClient == nil {
		return ErrRedis
	}
	return WrapRedisError(r.redisClient.Set(ctx, r.refreshTokenKey(userUUID, deviceID), refreshToken, expireDuration).Err())
}

// GetRefreshToken 获取 RefreshToken。
func (r *deviceRepositoryImpl) GetRefreshToken(ctx context.Context, userUUID, deviceID string) (string, error) {
	if r.redisClient == nil {
		return "", ErrRedisNil
	}
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
	pipe := r.redisClient.Pipeline()
	pipe.Del(ctx, r.accessTokenKey(userUUID, deviceID))
	pipe.Del(ctx, r.refreshTokenKey(userUUID, deviceID))
	if _, err := pipe.Exec(ctx); err != nil {
		return WrapRedisError(err)
	}
	return nil
}

// DeleteByUserUUID 删除用户的全部设备登录态。
func (r *deviceRepositoryImpl) DeleteByUserUUID(ctx context.Context, userUUID string) error {
	sessions, err := r.GetByUserUUID(ctx, userUUID)
	if err != nil {
		return err
	}

	for _, session := range sessions {
		if session == nil || session.DeviceId == "" {
			continue
		}
		if err := r.DeleteTokens(ctx, userUUID, session.DeviceId); err != nil {
			return err
		}
	}

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
		pipe := r.redisClient.Pipeline()
		pipe.Del(ctx, r.deviceInfoKey(userUUID))
		pipe.Del(ctx, r.deviceActiveKey(userUUID))
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			LogRedisError(ctx, err)
		}
	}
	return nil
}

// UpdateOnlineStatus 更新设备在线状态。
func (r *deviceRepositoryImpl) UpdateOnlineStatus(ctx context.Context, userUUID, deviceID string, status int8) error {
	result := r.db.WithContext(ctx).Model(&model.DeviceSession{}).
		Where("user_uuid = ? AND device_id = ? AND deleted_at IS NULL", userUUID, deviceID).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return WrapDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}

	if r.redisClient == nil {
		return nil
	}

	cacheKey := r.deviceInfoKey(userUUID)
	raw, err := r.redisClient.HGet(ctx, cacheKey, deviceID).Result()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		LogRedisError(ctx, err)
		return nil
	}

	var item deviceCacheItem
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		LogRedisError(ctx, err)
		return nil
	}
	item.Status = status
	value, err := json.Marshal(item)
	if err != nil {
		LogRedisError(ctx, err)
		return nil
	}

	pipe := r.redisClient.Pipeline()
	pipe.HSet(ctx, cacheKey, deviceID, value)
	pipe.Expire(ctx, cacheKey, rediskey.DeviceInfoTTL)
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		LogRedisError(ctx, err)
	}
	return nil
}
