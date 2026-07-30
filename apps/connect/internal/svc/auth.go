package svc

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/util"
	"strings"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"

	"github.com/redis/go-redis/v9"
)

var (
	// ErrTokenRequired 表示握手参数中缺少 token。
	ErrTokenRequired = errors.New("token is required")
	// ErrDeviceIDRequired 表示握手参数中缺少 device_id。
	ErrDeviceIDRequired = errors.New("device_id is required")
	// ErrTokenInvalid 表示 token 非法、已过期，或与设备不匹配。
	ErrTokenInvalid = errors.New("token is invalid")
)

// Authenticate 校验 WebSocket 握手参数与登录态。
// 校验流程：
// 1. 校验 token/device_id 是否为空；
// 2. 解析 JWT，校验 claims 基本字段；
// 3. 强校验 claims.DeviceID 与 query.device_id 一致；
// 4. 若 Redis 可用，校验 auth:at:{user_uuid}:{device_id} 中存储的 token md5。
//
// 降级策略（Fail-Close）：
// - Redis 异常时拒绝连接，避免被踢用户仍能接入的安全风险；
// - JWT 过期时间通常较长（如 7 天），仅 JWT 校验无法保证"踢线立即失效"。
func (s *ConnectService) Authenticate(ctx context.Context, token, deviceID, clientIP string) (*Session, error) {
	token = strings.TrimSpace(token)
	deviceID = strings.TrimSpace(deviceID)
	clientIP = strings.TrimSpace(clientIP)

	if token == "" {
		logger.Warn(ctx, "连接鉴权失败：缺少 token")
		return nil, ErrTokenRequired
	}
	if deviceID == "" {
		logger.Warn(ctx, "连接鉴权失败：缺少 device_id")
		return nil, ErrDeviceIDRequired
	}

	claims, err := util.ParseToken(token)
	if err != nil {
		logger.Warn(ctx, "连接鉴权失败：JWT 解析失败",
			logger.String("device_id", deviceID),
			logger.ErrorField("error", err),
		)
		return nil, ErrTokenInvalid
	}

	if claims.UserUUID == "" || claims.DeviceID == "" {
		logger.Warn(ctx, "连接鉴权失败：JWT claims 缺少必要字段",
			logger.String("user_uuid", claims.UserUUID),
			logger.String("claims_device_id", claims.DeviceID),
			logger.String("query_device_id", deviceID),
		)
		return nil, ErrTokenInvalid
	}

	if claims.DeviceID != deviceID {
		logger.Warn(ctx, "连接鉴权失败：device_id 不匹配",
			logger.String("user_uuid", claims.UserUUID),
			logger.String("claims_device_id", claims.DeviceID),
			logger.String("query_device_id", deviceID),
		)
		return nil, ErrTokenInvalid
	}

	// 与 user/auth 存储规则保持一致：
	// auth:at:{user_uuid}:{device_id} = md5(access_token)
	if s.redisClient != nil {
		key := rediskey.AccessTokenKey(claims.UserUUID, claims.DeviceID)
		storedHash, getErr := s.redisClient.Get(ctx, key).Result()

		switch {
		case getErr == redis.Nil:
			logger.Warn(ctx, "连接鉴权失败：AccessToken 哈希不存在",
				logger.String("user_uuid", claims.UserUUID),
				logger.String("device_id", claims.DeviceID),
				logger.String("redis_key", key),
			)
			return nil, ErrTokenInvalid
		case getErr != nil:
			// Redis 异常时拒绝连接（fail-close），保证踢线等安全操作的严格性。
			logger.Warn(ctx, "连接鉴权读取 Redis 失败，拒绝连接",
				logger.String("user_uuid", claims.UserUUID),
				logger.String("device_id", claims.DeviceID),
				logger.ErrorField("error", getErr),
			)
			return nil, ErrTokenInvalid
		default:
			currentHash := md5Hex(token)
			if storedHash != currentHash {
				logger.Warn(ctx, "连接鉴权失败：AccessToken 哈希不匹配",
					logger.String("user_uuid", claims.UserUUID),
					logger.String("device_id", claims.DeviceID),
					logger.String("redis_key", key),
					logger.String("stored_hash", storedHash),
					logger.String("current_hash", currentHash),
				)
				return nil, ErrTokenInvalid
			}
		}
	}

	return &Session{
		UserUUID: claims.UserUUID,
		DeviceID: claims.DeviceID,
		ClientIP: clientIP,
	}, nil
}

// md5Hex 返回字符串的 MD5 十六进制摘要。
// 用于与 auth 服务中存储的 access_token 哈希值进行比较。
func md5Hex(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}
