// Package realtime 实现 Kafka topic `realtime.push` 的业务处理。
//
// 与 msgpush 的差异：
//   - 输入是 RealtimePushEvent（protobuf bytes），目标由 target.kind 显式给出，
//     而不是从 receiver_uuid + conv_type 推导；
//   - 投递目前一律 PushToDevice，不做节点有界并发 / PushToUser（规模与归因需求不同）；
//   - ACK 是否需要由事件字段 AckRequired 携带，本包不按事件类型硬编码。
//
// 本包不感知 Kafka offset / 重试预算；瞬时失败包装为 pusherr.ErrRetriable。
// 不 import msgpush，私有接口在本包内各自声明，避免跨业务耦合。
package realtime

import (
	"context"
	"fmt"
	"time"

	connectpb "github.com/013677890/LCchat-Backend/apps/connect/pb"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/connectcli"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/groupcli"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/metrics"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/pusherr"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	route "github.com/013677890/LCchat-Backend/pkg/presence"
	"github.com/013677890/LCchat-Backend/pkg/realtimepush"
)

// routeRepository 只暴露实时提醒路由解析需要的读能力，便于测试用内存 mock 替换。
type routeRepository interface {
	ListUserRoutes(ctx context.Context, userUUID string) ([]route.DeviceRoute, error)
	ListUsersRoutes(ctx context.Context, userUUIDs []string) (map[string][]route.DeviceRoute, error)
}

// deviceSender 是 realtime.push 精确投递到单台设备所需的能力。
// 当前链路不要求 PushToUser；若日后引入用户批量投递，应显式扩展接口而非静默降级。
type deviceSender interface {
	PushToDevice(ctx context.Context, connectAddr, userUUID, deviceID string, envelope *connectpb.MessageEnvelope) error
}

// groupTargetFetcher 隔离群成员/管理员查询依赖，使目标解析测试不需要真实 group-service。
type groupTargetFetcher interface {
	GetGroupMembers(ctx context.Context, groupUUID string) ([]string, error)
	GetGroupAdmins(ctx context.Context, groupUUID string) ([]string, error)
}

// Handler 处理 Kafka 中的 realtime.push 实时提醒事件（好友申请、群审批等非消息下行）。
type Handler struct {
	routes routeRepository
	sender deviceSender
	groups groupTargetFetcher
}

// NewHandler 创建 realtime.push 实时提醒处理器。
// 依赖允许为 nil 时在 Handle 路径上以 ErrRetriable 失败，便于测试构造部分依赖的 Handler；
// 生产装配应由 Wire 保证 routes/sender/groups 均非空。
func NewHandler(routes *route.RedisRepository, sender *connectcli.Sender, groups *groupcli.Client) *Handler {
	return &Handler{routes: routes, sender: sender, groups: groups}
}

// Handle 处理单条 realtime.push 事件，并按目标类型扩散到在线设备。
//
// 返回约定与 msgpush.Handler 对齐：
//   - nil：成功，或永久错误已跳过（调用方应 commit）；
//   - errors.Is(..., pusherr.ErrRetriable)：瞬时失败，可本地重试。
//
// 重试判定同样偏保守：任一设备成功即整体成功，避免对已送达设备重复推送。
func (h *Handler) Handle(ctx context.Context, value []byte) error {
	start := time.Now()
	result := "success"
	defer func() {
		metrics.ObserveHandleDuration(start, result)
	}()

	// 解码失败是永久错误：protobuf 形状非法时重试无意义。
	event, err := realtimepush.Decode(value)
	if err != nil {
		result = "permanent_error"
		metrics.EventTypeSkipped.WithLabelValues("unknown", "decode_error").Inc()
		logger.Warn(ctx, "message-push 反序列化 realtime.push 事件失败，跳过该消息",
			logger.ErrorField("error", err),
			logger.Int("payload_bytes", len(value)),
		)
		return nil
	}

	routes, err := h.resolveRealtimeRoutes(ctx, event)
	if err != nil {
		result = "retriable_error"
		return err
	}
	if len(routes) == 0 {
		// 目标离线或聚合结果为空：不算失败，提醒类事件允许丢失后由业务侧自愈/重发。
		return nil
	}
	if h.sender == nil {
		result = "retriable_error"
		return fmt.Errorf("%w: realtime.push connect 发送器未初始化", pusherr.ErrRetriable)
	}

	// AckRequired 由生产者写入事件；本包不按 type 推断，避免与业务侧约定漂移。
	envelope := &connectpb.MessageEnvelope{
		Type:        event.Type,
		Data:        event.Data,
		Seq:         event.Seq,
		ServerTs:    event.ServerTs,
		TraceId:     event.TraceID,
		AckRequired: event.AckRequired,
	}

	successCount, failedCount := h.pushRealtimeRoutes(ctx, event.Type, routes, envelope)

	logger.Info(ctx, "message-push realtime.push 处理完成",
		logger.String("event_type", event.Type),
		logger.String("target_kind", string(event.Target.Kind)),
		logger.String("trace_id", event.TraceID),
		logger.Int64("seq", event.Seq),
		logger.Int64("server_ts", event.ServerTs),
		logger.Int("route_count", len(routes)),
		logger.Int("succeeded_count", successCount),
		logger.Int("failed_count", failedCount),
	)

	if successCount == 0 && failedCount > 0 {
		result = "retriable_error"
		return fmt.Errorf("%w: realtime.push 所有设备推送均失败 (%d)", pusherr.ErrRetriable, failedCount)
	}
	return nil
}
