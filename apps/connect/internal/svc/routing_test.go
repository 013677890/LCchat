package svc

import (
	"context"
	"fmt"
	"testing"
	"time"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newConnectServiceWithRedisForTest(t *testing.T) (*ConnectService, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	s := &ConnectService{redisClient: redis.NewClient(&redis.Options{Addr: mr.Addr()})}
	return s, mr
}

// TestRemoveUserRoute_CASKeepsForeignRoute 验证跨实例重连竞态：
// 设备重连到 connect-2 后，旧节点 connect-1 断开不应误删指向 connect-2 的新路由。
func TestRemoveUserRoute_CASKeepsForeignRoute(t *testing.T) {
	s, mr := newConnectServiceWithRedisForTest(t)
	ctx := context.Background()

	const (
		userUUID = "user-1"
		deviceID = "dev-1"
		node1    = "10.0.0.1:9091"
		node2    = "10.0.0.2:9091"
	)
	key := rediskey.UserRoutingKey(userUUID)

	// 现状：路由已指向 connect-2（设备已重连）。
	newRoute := fmt.Sprintf("%s|%d", node2, time.Now().UnixMilli())
	mr.HSet(key, deviceID, newRoute)

	// connect-1 旧连接断开，带本节点地址做 CAS 删除。
	s.RemoveUserRoute(ctx, userUUID, deviceID, node1)

	if got := mr.HGet(key, deviceID); got != newRoute {
		t.Fatalf("connect-2 的路由被 connect-1 误删: got %q want %q", got, newRoute)
	}
}

// TestRemoveUserRoute_CASDeletesOwnRoute 验证仍指向本节点时正常删除。
func TestRemoveUserRoute_CASDeletesOwnRoute(t *testing.T) {
	s, mr := newConnectServiceWithRedisForTest(t)
	ctx := context.Background()

	const (
		userUUID = "user-1"
		deviceID = "dev-1"
		node1    = "10.0.0.1:9091"
	)
	key := rediskey.UserRoutingKey(userUUID)

	mr.HSet(key, deviceID, fmt.Sprintf("%s|%d", node1, time.Now().UnixMilli()))

	s.RemoveUserRoute(ctx, userUUID, deviceID, node1)

	if mr.HGet(key, deviceID) != "" {
		t.Fatalf("本节点路由应被删除，但仍存在")
	}
}

// TestRemoveUserRoute_EmptyAddrUnconditionalDelete 验证未配置地址时回退无条件删除。
func TestRemoveUserRoute_EmptyAddrUnconditionalDelete(t *testing.T) {
	s, mr := newConnectServiceWithRedisForTest(t)
	ctx := context.Background()

	const (
		userUUID = "user-1"
		deviceID = "dev-1"
	)
	key := rediskey.UserRoutingKey(userUUID)
	mr.HSet(key, deviceID, "10.0.0.2:9091|123")

	s.RemoveUserRoute(ctx, userUUID, deviceID, "")

	if mr.HGet(key, deviceID) != "" {
		t.Fatalf("空地址应无条件删除，但路由仍存在")
	}
}
