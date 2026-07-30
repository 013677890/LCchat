package svc

import (
	"context"
	"fmt"
	"strings"
	"time"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/redis/go-redis/v9"
)

// removeUserRouteCAS 仅当路由 field 当前值仍指向本节点时才删除（compare-and-delete）。
// 解决跨实例重连竞态：设备从 connect-1 重连到 connect-2 后，connect-1 的旧连接断开
// 不能误删 connect-2 写入的新路由，否则该在线设备会在心跳节流窗口内对 message-push 不可见。
// 路由 value 形如 "addr|activeAtMs"，因此按 "addr|" 前缀（或纯 addr 兜底）校验归属。
// KEYS[1]=routing key  ARGV[1]=deviceID(field)  ARGV[2]=本节点地址  ARGV[3]=删除后续 TTL 秒
// 返回 1 表示已删除，0 表示 field 不存在或已不属于本节点（不删）。
var removeUserRouteCAS = redis.NewScript(`
local cur = redis.call('HGET', KEYS[1], ARGV[1])
if not cur then
	return 0
end
local addr = ARGV[2]
local prefix = addr .. '|'
if cur == addr or string.sub(cur, 1, string.len(prefix)) == prefix then
	redis.call('HDEL', KEYS[1], ARGV[1])
	redis.call('EXPIRE', KEYS[1], ARGV[3])
	return 1
end
return 0
`)

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
// selfAddr 为本节点地址（CONNECT_SELF_GRPC_ADDR）：非空时走 Lua CAS，仅当该 field
// 仍指向本节点才删除，避免误删设备重连到其它节点后写入的新路由（跨实例重连竞态）。
// selfAddr 为空（降级/未配置地址）时回退到无条件删除，保持旧行为。
func (s *ConnectService) RemoveUserRoute(ctx context.Context, userUUID, deviceID, selfAddr string) {
	if s == nil || s.redisClient == nil || userUUID == "" || deviceID == "" {
		return
	}
	key := rediskey.UserRoutingKey(userUUID)

	if selfAddr = strings.TrimSpace(selfAddr); selfAddr != "" {
		ttlSeconds := int(rediskey.DeviceActiveTTL / time.Second)
		if err := removeUserRouteCAS.Run(ctx, s.redisClient,
			[]string{key}, deviceID, selfAddr, ttlSeconds).Err(); err != nil {
			logger.Warn(ctx, "CAS 删除用户设备路由失败（不阻塞）",
				logger.String("user_uuid", userUUID),
				logger.String("device_id", deviceID),
				logger.String("connect_addr", selfAddr),
				logger.ErrorField("error", err),
			)
		}
		return
	}

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
// 使用 SCAN 替代 KEYS 避免阻塞生产 Redis。
func (s *ConnectService) RemoveRoutesByConnectAddr(ctx context.Context, connectGRPCAddr string) {
	if s == nil || s.redisClient == nil || strings.TrimSpace(connectGRPCAddr) == "" {
		return
	}

	var cursor uint64
	pattern := "user:routing:*"
	cleanedCount := 0

	for {
		keys, nextCursor, err := s.redisClient.Scan(ctx, cursor, pattern, 100).Result()

		if err != nil {
			logger.Warn(ctx, "SCAN 用户路由失败",
				logger.String("connect_addr", connectGRPCAddr),
				logger.Int64("cursor", int64(cursor)),
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
			} else {
				cleanedCount += len(fields)
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if cleanedCount > 0 {
		logger.Info(ctx, "按 connect 地址清理路由完成",
			logger.String("connect_addr", connectGRPCAddr),
			logger.Int("cleaned_count", cleanedCount),
		)
	}
}
