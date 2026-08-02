//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
)

func testWebSocketLifecycle(t *testing.T, f *Fixture, ctx context.Context) {
	t.Helper()
	a, b := f.Users["A"], f.Users["B"]
	b1Token := b.Devices["B1"]
	// 生命周期专项也支持单独执行；消息发送前显式建立好友关系，避免依赖
	// “好友与黑名单边界”子测试的执行顺序。
	ensureFriend(t, f, ctx, a, b)

	// 额外登录一次同一设备，确保 query device_id 与 JWT claim 一致，随后用两个
	// WebSocket 连接验证“新连接替换旧连接”的真实行为。
	sameDevice := f.login(ctx, a, "SAME-DEVICE")
	old, err := OpenWS(ctx, f.cfg, "same-device-old", sameDevice.AccessToken, sameDevice.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = old.Close() })
	newConnection, err := OpenWS(ctx, f.cfg, "same-device-new", sameDevice.AccessToken, sameDevice.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = newConnection.Close() })
	t.Run("同用户同设备新连接关闭旧连接", func(t *testing.T) {
		if err := old.WaitClosed(eventContext(context.Background(), 5*time.Second)); err != nil {
			t.Fatalf("旧同设备连接没有被关闭: %v", err)
		}
	})
	t.Run("新同设备连接仍可心跳", func(t *testing.T) {
		if err := newConnection.Heartbeat(eventContext(context.Background(), f.cfg.EventTimeout)); err != nil {
			t.Fatal(err)
		}
	})

	// 不复用实时事件专项的 A2：前一个专项的 WebSocket 清理是异步的，
	// 复用同一 device_id 可能让旧连接的清理请求误删本专项新建路由。
	a2Token := f.login(ctx, a, "A2-LIFECYCLE")
	a2, err := OpenWS(ctx, f.cfg, "A2-lifecycle", a2Token.AccessToken, a2Token.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a2.Close() })

	t.Run("同用户多设备同时在线", func(t *testing.T) {
		response, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/messages/send", b1Token.AccessToken, b1Token.ID, map[string]any{
			"clientMsgId": "multi-device-" + f.suffix, "convType": 1, "targetUuid": a.UUID, "msgType": 1, "content": `{"text":"multi-device"}`,
		})
		if err != nil {
			t.Fatal(err)
		}
		var message map[string]any
		requireJSONData(t, "多设备消息发送", response, &message)
		for _, client := range []*WSClient{newConnection, a2} {
			var item msgpb.MsgItem
			expectEventPayload(t, client, "MSG_PUSH", &item, func() bool { return item.MsgId == stringValue(message, "msgId") })
		}
	})

	routeKey := "user:routing:" + a.UUID
	t.Run("心跳刷新 Redis 路由 TTL", func(t *testing.T) {
		// 先把 TTL 压到远低于正常值的水位再发心跳；
		// presence 契约要求心跳无条件刷新路由，TTL 必须被抬回接近满值，
		// 该断言能真正证伪"心跳没有触发刷新"。
		if err := f.redis.Expire(ctx, routeKey, 30*time.Second).Err(); err != nil {
			t.Fatal(err)
		}
		if err := newConnection.Heartbeat(eventContext(context.Background(), f.cfg.EventTimeout)); err != nil {
			t.Fatal(err)
		}
		after, err := f.redis.TTL(ctx, routeKey).Result()
		if err != nil {
			t.Fatal(err)
		}
		if after <= 60*time.Second {
			t.Errorf("心跳没有刷新路由 TTL：after=%s", after)
		}
	})
	t.Run("路由过期后心跳恢复", func(t *testing.T) {
		if err := f.redis.Expire(ctx, routeKey, time.Second).Err(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(1500 * time.Millisecond)
		if exists, err := f.redis.Exists(ctx, routeKey).Result(); err != nil {
			t.Fatal(err)
		} else if exists != 0 {
			t.Errorf("路由 key 未按预期过期：exists=%d", exists)
		}
		if err := newConnection.Heartbeat(eventContext(context.Background(), f.cfg.EventTimeout)); err != nil {
			t.Fatal(err)
		}
		if exists, err := f.redis.Exists(ctx, routeKey).Result(); err != nil {
			t.Fatal(err)
		} else if exists != 1 {
			t.Errorf("心跳没有恢复路由 key：exists=%d", exists)
		}
	})
	// 上一个子测试故意让路由过期，用来验证 Connect 的恢复能力。
	// 即使该能力当前失败，也不能让后面的“断线清理”和“重启恢复”失去
	// 前置路由，避免一个缺陷级联污染多个生命周期断言。
	if exists, err := f.redis.Exists(ctx, routeKey).Result(); err != nil {
		t.Errorf("检查生命周期路由恢复状态失败: %v", err)
	} else if exists == 0 {
		if err := newConnection.Close(); err != nil {
			t.Logf("重建生命周期主连接前关闭旧连接返回：%v", err)
		}
		replacement, openErr := OpenWS(ctx, f.cfg, "same-device-recovered", sameDevice.AccessToken, sameDevice.ID)
		if openErr != nil {
			t.Errorf("路由过期场景后重建主连接失败: %v", openErr)
		} else {
			newConnection = replacement
			t.Cleanup(func() { _ = replacement.Close() })
			if heartbeatErr := replacement.Heartbeat(eventContext(context.Background(), f.cfg.EventTimeout)); heartbeatErr != nil {
				t.Errorf("重建生命周期主连接心跳失败: %v", heartbeatErr)
			}
		}
	}
	t.Run("断线后路由清理且不影响另一设备", func(t *testing.T) {
		if err := a2.Close(); err != nil {
			t.Logf("关闭 A2 连接返回：%v", err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			value, err := f.redis.HGet(ctx, routeKey, a2Token.ID).Result()
			if err != nil && err.Error() != "redis: nil" {
				t.Fatal(err)
			}
			if value == "" {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		value, err := f.redis.HGet(ctx, routeKey, a2Token.ID).Result()
		if err == nil && value != "" {
			t.Errorf("断开 A2 后路由仍存在：%s", value)
		}
		response, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/messages/send", b1Token.AccessToken, b1Token.ID, map[string]any{
			"clientMsgId": "route-clean-" + f.suffix, "convType": 1, "targetUuid": a.UUID, "msgType": 1, "content": `{"text":"route-clean"}`,
		})
		if err != nil {
			t.Fatal(err)
		}
		var message map[string]any
		requireJSONData(t, "断线后发送消息", response, &message)
		var item msgpb.MsgItem
		expectEventPayload(t, newConnection, "MSG_PUSH", &item, func() bool { return item.MsgId == stringValue(message, "msgId") })
	})
	t.Run("Connect 重启后路由和消息恢复", func(t *testing.T) {
		restartCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := runCompose(restartCtx, f.cfg, "restart", "connect"); err != nil {
			t.Fatal(err)
		}
		if err := waitForHTTPHealth(restartCtx, f.cfg); err != nil {
			t.Fatal(err)
		}
		restarted, err := OpenWS(restartCtx, f.cfg, "after-connect-restart", a2Token.AccessToken, a2Token.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer restarted.Close()
		if err := restarted.Heartbeat(eventContext(context.Background(), f.cfg.EventTimeout)); err != nil {
			t.Fatal(err)
		}
		// Connect 重启后需要先确认新连接已经把新的 gRPC 地址写回 Redis，
		// 再发送消息，否则消息消费者可能正好读取到旧节点地址并误报恢复失败。
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			value, routeErr := f.redis.HGet(ctx, routeKey, a2Token.ID).Result()
			if routeErr == nil && value != "" {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if value, routeErr := f.redis.HGet(ctx, routeKey, a2Token.ID).Result(); routeErr != nil || value == "" {
			t.Fatalf("Connect 重启后 A2 路由没有恢复：value=%q err=%v", value, routeErr)
		}
		// Connect 容器虽然已经通过 HTTP 健康检查，但 message-push 内部的
		// gRPC 长连接还需要等待一次重连退避。等待完整传播窗口后再发消息，
		// 验证的是“节点重启后最终恢复”，而不是把重启瞬间的连接抖动当成
		// 永久路由故障。
		time.Sleep(5 * time.Second)
		response, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/messages/send", b1Token.AccessToken, b1Token.ID, map[string]any{
			"clientMsgId": "after-restart-" + f.suffix, "convType": 1, "targetUuid": a.UUID, "msgType": 1, "content": `{"text":"after-restart"}`,
		})
		if err != nil {
			t.Fatal(err)
		}
		var message map[string]any
		requireJSONData(t, "Connect 重启后发送消息", response, &message)
		var item msgpb.MsgItem
		expectEventPayload(t, restarted, "MSG_PUSH", &item, func() bool { return item.MsgId == stringValue(message, "msgId") })
	})
}

func testRedisUnavailableACK(t *testing.T, f *Fixture, ctx context.Context) {
	t.Helper()
	a, b := f.Users["A"], f.Users["B"]
	a1, b1 := a.Devices["A1"], b.Devices["B1"]
	aWS, err := OpenWS(ctx, f.cfg, "redis-down-A1", a1.AccessToken, a1.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer aWS.Close()
	bWS, err := OpenWS(ctx, f.cfg, "redis-down-B1", b1.AccessToken, b1.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer bWS.Close()
	if err := aWS.Heartbeat(eventContext(context.Background(), f.cfg.EventTimeout)); err != nil {
		t.Fatal(err)
	}
	if err := bWS.Heartbeat(eventContext(context.Background(), f.cfg.EventTimeout)); err != nil {
		t.Fatal(err)
	}
	response, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/messages/send", a1.AccessToken, a1.ID, map[string]any{
		"clientMsgId": "redis-down-" + f.suffix, "convType": 1, "targetUuid": b.UUID, "msgType": 1, "content": `{"text":"redis-down"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	var message map[string]any
	requireJSONData(t, "Redis 故障前发送消息", response, &message)
	var pushed msgpb.MsgItem
	expectEventPayload(t, bWS, "MSG_PUSH", &pushed, func() bool { return pushed.MsgId == stringValue(message, "msgId") })

	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := runCompose(stopCtx, f.cfg, "stop", "redis"); err != nil {
		t.Fatal(err)
	}
	restarted := false
	t.Cleanup(func() {
		if restarted {
			return
		}
		startCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := runCompose(startCtx, f.cfg, "start", "redis"); err != nil {
			t.Errorf("Redis 故障测试清理时启动 Redis 失败: %v", err)
		}
	})
	frame, err := sendExpectError(eventContext(context.Background(), f.cfg.EventTimeout), bWS, "MSG_ACK", stringValue(message, "convId"), int64Value(message, "seq"), stringValue(message, "msgId"))
	if err != nil {
		t.Errorf("Redis 不可用时 ACK 没有返回 error：%v", err)
	} else if frame.GetCode() == 0 {
		t.Errorf("Redis 不可用时 ACK 返回了 code=0")
	}
	startCtx, startCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer startCancel()
	if err := runCompose(startCtx, f.cfg, "start", "redis"); err != nil {
		t.Fatal(err)
	}
	restarted = true
	if err := waitForHTTPHealth(startCtx, f.cfg); err != nil {
		t.Fatal(err)
	}
	if err := f.redis.Ping(startCtx).Err(); err != nil {
		t.Fatal(fmt.Errorf("Redis 恢复后 Ping 失败: %w", err))
	}
}
