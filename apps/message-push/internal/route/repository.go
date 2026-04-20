package route

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/redis/go-redis/v9"
)

// DeviceRoute 表示一个在线设备当前所在的 connect 节点。
type DeviceRoute struct {
	UserUUID        string
	DeviceID        string
	ConnectGRPCAddr string
	LastActiveMs    int64
}

// Repository 读取用户设备路由。
type Repository interface {
	ListUserRoutes(ctx context.Context, userUUID string) ([]DeviceRoute, error)
	ListUsersRoutes(ctx context.Context, userUUIDs []string) (map[string][]DeviceRoute, error)
}

// RedisRepository 基于 Redis hash 的设备路由仓储。
type RedisRepository struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisRepository 创建 Redis 路由仓储。
func NewRedisRepository(client *redis.Client, ttl time.Duration) *RedisRepository {
	return &RedisRepository{client: client, ttl: ttl}
}

// ListUserRoutes 读取单个用户的有效设备路由。
func (r *RedisRepository) ListUserRoutes(ctx context.Context, userUUID string) ([]DeviceRoute, error) {
	if userUUID == "" || r == nil || r.client == nil {
		return nil, nil
	}
	values, err := r.client.HGetAll(ctx, rediskey.UserRoutingKey(userUUID)).Result()
	if err != nil {
		return nil, fmt.Errorf("读取用户路由失败: %w", err)
	}
	return r.parseUserRoutes(userUUID, values, time.Now()), nil
}

// ListUsersRoutes 批量读取用户设备路由。
func (r *RedisRepository) ListUsersRoutes(ctx context.Context, userUUIDs []string) (map[string][]DeviceRoute, error) {
	result := make(map[string][]DeviceRoute, len(userUUIDs))
	if len(userUUIDs) == 0 || r == nil || r.client == nil {
		return result, nil
	}

	pipe := r.client.Pipeline()
	cmds := make(map[string]*redis.MapStringStringCmd, len(userUUIDs))
	for _, userUUID := range userUUIDs {
		if userUUID == "" {
			continue
		}
		if _, exists := cmds[userUUID]; exists {
			continue
		}
		cmds[userUUID] = pipe.HGetAll(ctx, rediskey.UserRoutingKey(userUUID))
	}
	if len(cmds) == 0 {
		return result, nil
	}

	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, fmt.Errorf("批量读取用户路由失败: %w", err)
	}

	now := time.Now()
	for userUUID, cmd := range cmds {
		values, err := cmd.Result()
		if err != nil && err != redis.Nil {
			return nil, fmt.Errorf("解析用户路由结果失败: %w", err)
		}
		routes := r.parseUserRoutes(userUUID, values, now)
		if len(routes) > 0 {
			result[userUUID] = routes
		}
	}
	return result, nil
}

func (r *RedisRepository) parseUserRoutes(userUUID string, values map[string]string, now time.Time) []DeviceRoute {
	if len(values) == 0 {
		return nil
	}
	cutoffMs := int64(0)
	if r.ttl > 0 {
		cutoffMs = now.Add(-r.ttl).UnixMilli()
	}

	routes := make([]DeviceRoute, 0, len(values))
	for deviceID, raw := range values {
		addr, activeMs, ok := parseRouteValue(raw)
		if !ok || addr == "" || deviceID == "" {
			continue
		}
		if cutoffMs > 0 && activeMs > 0 && activeMs < cutoffMs {
			continue
		}
		routes = append(routes, DeviceRoute{
			UserUUID:        userUUID,
			DeviceID:        deviceID,
			ConnectGRPCAddr: addr,
			LastActiveMs:    activeMs,
		})
	}
	return routes
}

func parseRouteValue(raw string) (string, int64, bool) {
	parts := strings.Split(raw, "|")
	if len(parts) == 0 {
		return "", 0, false
	}
	addr := strings.TrimSpace(parts[0])
	if addr == "" {
		return "", 0, false
	}
	if len(parts) == 1 {
		return addr, 0, true
	}
	activeMs, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return "", 0, false
	}
	return addr, activeMs, true
}
