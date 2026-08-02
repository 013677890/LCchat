package presence

import (
	"context"
	"fmt"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRepository(t *testing.T, window time.Duration) (*RedisRepository, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisRepository(client, window), server
}

func routeValue(addr string, activeAt time.Time) string {
	return fmt.Sprintf("%s|%d", addr, activeAt.UnixMilli())
}

func TestListUserRoutesWindowFilter(t *testing.T) {
	repo, server := newTestRepository(t, 2*time.Minute)
	now := time.Now()

	// 窗口内、窗口外、格式异常、仅地址（无 activeMs）四类 field 混合写入。
	server.HSet("user:routing:u1", "fresh", routeValue("connect-1:9091", now.Add(-30*time.Second)))
	server.HSet("user:routing:u1", "stale", routeValue("connect-1:9091", now.Add(-10*time.Minute)))
	server.HSet("user:routing:u1", "broken", "connect-1:9091|not-a-number")
	server.HSet("user:routing:u1", "addr-only", "connect-2:9091")

	routes, err := repo.ListUserRoutes(context.Background(), "u1")
	require.NoError(t, err)

	byDevice := make(map[string]DeviceRoute, len(routes))
	for _, route := range routes {
		byDevice[route.DeviceID] = route
	}

	// 窗口内路由保留，窗口外路由被读取阶段过滤，坏值被丢弃。
	assert.Contains(t, byDevice, "fresh")
	assert.NotContains(t, byDevice, "stale")
	assert.NotContains(t, byDevice, "broken")
	// 历史兼容：仅地址无 activeMs 时按 0 处理且不参与窗口过滤。
	assert.Contains(t, byDevice, "addr-only")
	assert.Equal(t, "connect-1:9091", byDevice["fresh"].ConnectGRPCAddr)
	assert.Equal(t, int64(0), byDevice["addr-only"].LastActiveMs)
}

func TestListUsersRoutesBatch(t *testing.T) {
	repo, server := newTestRepository(t, 2*time.Minute)
	now := time.Now()

	server.HSet("user:routing:u1", "d1", routeValue("connect-1:9091", now))
	server.HSet("user:routing:u2", "d2", routeValue("connect-2:9091", now.Add(-10*time.Minute)))

	result, err := repo.ListUsersRoutes(context.Background(), []string{"u1", "u2", "u3", "", "u1"})
	require.NoError(t, err)

	// u1 命中；u2 全部过期不返回条目；u3 无路由；空 uuid 被忽略；重复 uuid 只查一次。
	require.Len(t, result["u1"], 1)
	assert.Equal(t, "d1", result["u1"][0].DeviceID)
	assert.NotContains(t, result, "u2")
	assert.NotContains(t, result, "u3")
}

func TestListUserRoutesNilSafety(t *testing.T) {
	// nil 仓储与空参数都不应 panic，统一返回空结果。
	var nilRepo *RedisRepository
	routes, err := nilRepo.ListUserRoutes(context.Background(), "u1")
	require.NoError(t, err)
	assert.Empty(t, routes)

	repo, _ := newTestRepository(t, time.Minute)
	routes, err = repo.ListUserRoutes(context.Background(), "")
	require.NoError(t, err)
	assert.Empty(t, routes)
}

func TestParseRouteValue(t *testing.T) {
	addr, activeMs, ok := parseRouteValue("connect-1:9091|1700000000000")
	require.True(t, ok)
	assert.Equal(t, "connect-1:9091", addr)
	assert.Equal(t, int64(1700000000000), activeMs)

	_, _, ok = parseRouteValue("")
	assert.False(t, ok)

	_, _, ok = parseRouteValue("|123")
	assert.False(t, ok)
}
