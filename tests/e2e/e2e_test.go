//go:build e2e

package e2e

import (
	"context"
	"testing"
)

// TestE2E 按依赖顺序执行一轮完整黑盒场景。
//
// 这些子测试不能并行：好友关系、群成员、消息序号和在线路由之间存在真实
// 的业务依赖。使用一个共享 Fixture 也能把同一轮测试的事件因果链完整串起来。
func TestE2E(t *testing.T) {
	fixture := NewFixture(t)
	ctx := context.Background()

	t.Run("认证与账号边界", func(t *testing.T) {
		testAuthEdges(t, fixture, ctx)
	})
	t.Run("好友与黑名单边界", func(t *testing.T) {
		testFriendEdges(t, fixture, ctx)
	})
	t.Run("消息与会话边界", func(t *testing.T) {
		testMessageEdges(t, fixture, ctx)
	})
	t.Run("群组权限与申请边界", func(t *testing.T) {
		testGroupEdges(t, fixture, ctx)
	})
	t.Run("WebSocket 实时事件与 ACK", func(t *testing.T) {
		testWebSocketEvents(t, fixture, ctx)
	})
	t.Run("WebSocket 连接生命周期", func(t *testing.T) {
		testWebSocketLifecycle(t, fixture, ctx)
	})
	t.Run("Redis 不可用时 ACK", func(t *testing.T) {
		testRedisUnavailableACK(t, fixture, ctx)
	})
}
