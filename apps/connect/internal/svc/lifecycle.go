package svc

import (
	"context"
	"os"
	"strings"
	"time"

	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/logger"
)

const (
	// deviceStatusRPCTimeout UpdateDeviceStatus RPC 调用超时时间。
	deviceStatusRPCTimeout = 3 * time.Second

	// statusWorkerCount 设备状态 RPC 工作协程数量。
	statusWorkerCount = 64
	// statusQueueSize 设备状态 RPC 任务队列容量。
	// 队列满时新任务会被丢弃（仅 log Warn），不会阻塞调用方。
	// 注意：断连状态更新是离线设备 last_seen 的唯一来源，丢弃仅导致
	// last_seen 退化为登录时刻，属于可接受的降级。
	statusQueueSize = 8192
)

// deviceStatusTask 表示一条设备状态更新 RPC 任务。
type deviceStatusTask struct {
	logCtx   context.Context // 仅用于日志上下文，不用于 RPC 超时
	userUUID string
	deviceID string
	status   int8
}

func connectRouteAddr() string {
	return strings.TrimSpace(os.Getenv("CONNECT_SELF_GRPC_ADDR"))
}

// OnConnect 在连接建立后触发。
// 行为：
// 1. 无条件写入用户设备路由（presence 契约中"连接建立"的事实投影）；
// 2. 异步调用 auth-service RPC 将 DeviceSession.status 置为在线。
func (s *ConnectService) OnConnect(ctx context.Context, session *Session) {
	if addr := connectRouteAddr(); addr != "" {
		s.UpsertUserRoute(ctx, session.UserUUID, session.DeviceID, addr, time.Now(), s.effectiveRouteTTL())
	}
	s.updateDeviceStatusAsync(ctx, session, model.DeviceStatusOnline)
}

// OnHeartbeat 在收到客户端心跳后触发。
// 路由刷新必须无条件执行：presence 的新鲜度就是心跳新鲜度，任何本地节流都会在
// 路由 Key 被外部删除（Redis 重启/淘汰/清理竞态）后制造最长一个节流窗口的路由黑洞，
// 期间在线设备对 message-push 与在线状态查询完全不可见。
func (s *ConnectService) OnHeartbeat(ctx context.Context, session *Session) {
	if addr := connectRouteAddr(); addr != "" {
		s.RefreshUserRouteActive(ctx, session.UserUUID, session.DeviceID, addr, time.Now(), s.effectiveRouteTTL())
	}
}

// OnDisconnect 在连接断开后触发。
// 行为：
// 1. CAS 删除本设备路由，避免误删设备重连到其它节点后写入的新路由；
// 2. 异步调用 auth-service RPC 将 DeviceSession.status 置为离线
//    （该状态迁移时刻是离线设备 last_seen 的数据来源）。
func (s *ConnectService) OnDisconnect(ctx context.Context, session *Session) {
	s.RemoveUserRoute(ctx, session.UserUUID, session.DeviceID, connectRouteAddr())
	s.updateDeviceStatusAsync(ctx, session, model.DeviceStatusOffline)
}

// updateDeviceStatusAsync 将设备状态更新任务投递到工作队列。
// 降级策略：
// - statusQueue 为 nil 时静默跳过（authDeviceClient 不可用）。
// - 队列满时丢弃任务，仅 log Warn，不阻塞调用方。
func (s *ConnectService) updateDeviceStatusAsync(ctx context.Context, session *Session, status int8) {
	if s.statusQueue == nil {
		return
	}

	task := deviceStatusTask{
		logCtx:   ctx,
		userUUID: session.UserUUID,
		deviceID: session.DeviceID,
		status:   status,
	}

	select {
	case s.statusQueue <- task:
		// 成功投递
	default:
		// 队列满，丢弃任务
		logger.Warn(ctx, "设备状态更新队列已满，丢弃任务",
			logger.String("user_uuid", task.userUUID),
			logger.String("device_id", task.deviceID),
			logger.Int("status", int(task.status)),
		)
	}
}

// statusWorker 从队列消费任务，执行设备状态 RPC 调用。
// 每个任务独立创建 3s 超时上下文，失败仅 log Warn。
func (s *ConnectService) statusWorker() {
	defer s.statusWg.Done()

	for task := range s.statusQueue {
		rpcCtx, cancel := context.WithTimeout(context.Background(), deviceStatusRPCTimeout)

		_, err := s.authDeviceClient.UpdateDeviceStatus(rpcCtx, &authpb.UpdateDeviceStatusRequest{
			UserUuid: task.userUUID,
			DeviceId: task.deviceID,
			Status:   int32(task.status),
		})
		if err != nil {
			logger.Warn(task.logCtx, "UpdateDeviceStatus RPC 调用失败（不影响连接）",
				logger.String("user_uuid", task.userUUID),
				logger.String("device_id", task.deviceID),
				logger.Int("status", int(task.status)),
				logger.ErrorField("error", err),
			)
		}

		cancel()
	}
}
