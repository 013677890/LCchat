package svc

import (
	"context"
	"fmt"
	"strings"
	"time"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/pkg/logger"
)

// UpsertUserRoute 写入用户设备路由到 Redis。
func (s *ConnectService) UpsertUserRoute(ctx context.Context, userUUID, deviceID, connectGRPCAddr string, activeAt time.Time, ttl time.Duration) {
	if s == nil || s.redisClient == nil || userUUID == "" || deviceID == "" || connectGRPCAddr == "" {
		return
	}
	if activeAt.IsZero() {
		activeAt = time.Now()
	}
	key := rediskey.UserRoutingKey(userUUID)
	value := fmt.Sprintf("%s|%d", connectGRPCAddr, activeAt.UnixMilli())
	pipe := s.redisClient.Pipeline()
	pipe.HSet(ctx, key, deviceID, value)
	if ttl > 0 {
		pipe.Expire(ctx, key, ttl)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		logger.Warn(ctx, "写入用户设备路由失败（不阻塞）",
			logger.String("user_uuid", userUUID),
			logger.String("device_id", deviceID),
			logger.String("connect_addr", connectGRPCAddr),
			logger.ErrorField("error", err),
		)
	}
}

// RefreshUserRouteActive 刷新已有用户设备路由的活跃时间与 TTL。
func (s *ConnectService) RefreshUserRouteActive(ctx context.Context, userUUID, deviceID, connectGRPCAddr string, activeAt time.Time, ttl time.Duration) {
	s.UpsertUserRoute(ctx, userUUID, deviceID, connectGRPCAddr, activeAt, ttl)
}

// RemoveUserRoute 删除一个用户设备路由。
func (s *ConnectService) RemoveUserRoute(ctx context.Context, userUUID, deviceID string) {
	if s == nil || s.redisClient == nil || userUUID == "" || deviceID == "" {
		return
	}
	key := rediskey.UserRoutingKey(userUUID)
	pipe := s.redisClient.Pipeline()
	pipe.HDel(ctx, key, deviceID)
	pipe.Expire(ctx, key, rediskey.DeviceActiveTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		logger.Warn(ctx, "删除用户设备路由失败（不阻塞）",
			logger.String("user_uuid", userUUID),
			logger.String("device_id", deviceID),
			logger.ErrorField("error", err),
		)
	}
}

// RemoveRoutesByConnectAddr 清理当前节点留下的全部路由。
func (s *ConnectService) RemoveRoutesByConnectAddr(ctx context.Context, connectGRPCAddr string) {
	if s == nil || s.redisClient == nil || strings.TrimSpace(connectGRPCAddr) == "" {
		return
	}
	keys, err := s.redisClient.Keys(ctx, "user:routing:*").Result()
	if err != nil {
		logger.Warn(ctx, "扫描用户路由失败",
			logger.String("connect_addr", connectGRPCAddr),
			logger.ErrorField("error", err),
		)
		return
	}
	for _, key := range keys {
		values, err := s.redisClient.HGetAll(ctx, key).Result()
		if err != nil {
			logger.Warn(ctx, "读取用户路由失败，跳过清理",
				logger.String("key", key),
				logger.ErrorField("error", err),
			)
			continue
		}
		fields := make([]string, 0)
		for field, raw := range values {
			if strings.HasPrefix(raw, connectGRPCAddr+"|") || raw == connectGRPCAddr {
				fields = append(fields, field)
			}
		}
		if len(fields) == 0 {
			continue
		}
		args := make([]any, 0, len(fields)+1)
		args = append(args, key)
		for _, field := range fields {
			args = append(args, field)
		}
		if err := s.redisClient.Do(ctx, append([]any{"HDEL"}, args...)...).Err(); err != nil {
			logger.Warn(ctx, "按 connect 地址清理用户路由失败",
				logger.String("key", key),
				logger.String("connect_addr", connectGRPCAddr),
				logger.ErrorField("error", err),
			)
		}
	}
}
