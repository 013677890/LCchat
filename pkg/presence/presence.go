// Package presence 定义在线路由（presence）投影的共享读取契约。
//
// 数据事实源是 Redis Hash `user:routing:{user_uuid}`：
//   - 唯一写方：connect（连接建立 / 心跳无条件刷新 / 断连 CAS 删除）；
//   - field = device_id，value = `connectGrpcAddr|lastActiveMs`，key 级 TTL；
//   - 消费方：message-push（推送寻址，窗口宽）、auth（在线状态聚合，窗口严）。
//
// 各消费方按自身语义传入不同的过滤窗口，但解析与过滤逻辑必须收敛在本包，
// 避免"窗口常量散落多处"导致读写两侧约束漂移。
package presence

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/redis/go-redis/v9"
)

const (
	// DefaultRouteTTL connect 写入路由 key 的默认 TTL。
	// 需覆盖数个心跳周期（客户端约定 ~30s 一次），容忍网络抖动；
	// 对应 env：CONNECT_ROUTE_TTL_SECONDS。
	DefaultRouteTTL = 360 * time.Second

	// DefaultOnlineWindow auth 在线判定的默认读取窗口。
	// 连续丢约 4 个心跳（~120s）即判离线；对应 env：PRESENCE_ONLINE_WINDOW_SECONDS。
	DefaultOnlineWindow = 120 * time.Second

	// DefaultPushWindow message-push 推送寻址的默认读取窗口。
	// 推送宁可多尝试（目标不在时 connect 会拒绝），因此比在线判定窗口更宽；
	// 对应 env：MESSAGE_PUSH_ROUTE_TTL_SECONDS。
	DefaultPushWindow = 360 * time.Second
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
// ttl 是调用方语义下的活跃过滤窗口：lastActiveMs 早于 now-ttl 的路由在读取阶段被丢弃。
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

// parseUserRoutes 把 Redis hash 字段解析为设备路由列表。
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

		// 路由值里带最近活跃时间时，读取阶段直接过滤过期设备。
		// 这样即使 Redis 清理存在短暂延迟，也不会把长时间无活跃的设备当作在线。
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

// parseRouteValue 解析单个路由值。
// 约定格式：`connectGrpcAddr|lastActiveMs`；兼容历史数据允许只存地址，此时 lastActiveMs 记为 0。
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
