package svc

import (
	"context"
	"errors"
	"strings"

	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/util"
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
//
// AccessToken 是无状态 JWT，握手鉴权不读取 Redis。登出或踢设备会撤销 RefreshToken，
// 但已经签发的 AccessToken 在 exp 前仍可重新握手；这是与 HTTP 鉴权一致的有限失效窗口。
// 已建立连接也不会因 Redis 状态变化自动断开，设备踢线语义由连接管理链路单独负责。
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

	return &Session{
		UserUUID: claims.UserUUID,
		DeviceID: claims.DeviceID,
		ClientIP: clientIP,
	}, nil
}
