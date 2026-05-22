package consumer

import (
	"context"
	"fmt"
	"time"

	connectpb "github.com/013677890/LCchat-Backend/apps/connect/pb"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/connectcli"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/groupcli"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/metrics"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/route"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/realtimepush"
)

type groupTargetFetcher interface {
	GetGroupMembers(ctx context.Context, groupUUID string) ([]string, error)
	GetGroupAdmins(ctx context.Context, groupUUID string) ([]string, error)
}

// RealtimeHandler 处理 Kafka 中的 realtime.push 实时提醒事件。
type RealtimeHandler struct {
	routes routeRepository
	sender deviceSender
	groups groupTargetFetcher
}

// NewRealtimeHandler 创建非消息类实时提醒处理器。
func NewRealtimeHandler(routes *route.RedisRepository, sender *connectcli.Sender, groups *groupcli.Client) *RealtimeHandler {
	return &RealtimeHandler{routes: routes, sender: sender, groups: groups}
}

// Handle 处理单条 realtime.push 事件，并按目标类型扩散到在线设备。
func (h *RealtimeHandler) Handle(ctx context.Context, value []byte) error {
	start := time.Now()
	result := "success"
	defer func() {
		metrics.ObserveHandleDuration(start, result)
	}()

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
		return nil
	}
	if h.sender == nil {
		result = "retriable_error"
		return fmt.Errorf("%w: realtime.push connect 发送器未初始化", errRetriable)
	}

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
		return fmt.Errorf("%w: realtime.push 所有设备推送均失败 (%d)", errRetriable, failedCount)
	}
	return nil
}

// resolveRealtimeRoutes 根据实时提醒目标解析在线设备路由。
func (h *RealtimeHandler) resolveRealtimeRoutes(ctx context.Context, event realtimepush.Event) ([]route.DeviceRoute, error) {
	switch event.Target.Kind {
	case realtimepush.TargetKindUser:
		return h.listRealtimeUserRoutes(ctx, event.Type, event.Target.UserUUID, "")
	case realtimepush.TargetKindDevice:
		return h.listRealtimeUserRoutes(ctx, event.Type, event.Target.UserUUID, event.Target.DeviceID)
	case realtimepush.TargetKindUserList:
		return h.listRealtimeUsersRoutes(ctx, event.Type, event.Target.UserUUIDs)
	case realtimepush.TargetKindGroupMembers:
		return h.listRealtimeGroupRoutes(ctx, event.Type, event.Target.GroupUUID, h.fetchGroupMembers)
	case realtimepush.TargetKindGroupAdmins:
		return h.listRealtimeGroupRoutes(ctx, event.Type, event.Target.GroupUUID, h.fetchGroupAdmins)
	default:
		metrics.EventTypeSkipped.WithLabelValues(event.Type, "unsupported_target").Inc()
		logger.Warn(ctx, "message-push realtime.push 暂不支持该目标类型，跳过",
			logger.String("event_type", event.Type),
			logger.String("target_kind", string(event.Target.Kind)),
		)
		return nil, nil
	}
}

// listRealtimeUserRoutes 读取单用户路由，deviceID 非空时只保留指定设备。
func (h *RealtimeHandler) listRealtimeUserRoutes(ctx context.Context, eventType, userUUID, deviceID string) ([]route.DeviceRoute, error) {
	if h.routes == nil {
		return nil, fmt.Errorf("%w: realtime.push 路由仓储未初始化", errRetriable)
	}
	userRoutes, err := h.routes.ListUserRoutes(ctx, userUUID)
	if err != nil {
		metrics.RouteHitRate.WithLabelValues(eventType, "error").Inc()
		return nil, fmt.Errorf("%w: 读取实时提醒用户路由失败: %v", errRetriable, err)
	}
	filteredRoutes := filterDeviceRoutes(userRoutes, deviceID)
	observeRouteLookup(eventType, len(filteredRoutes))
	return filteredRoutes, nil
}

// listRealtimeUsersRoutes 批量读取多用户路由，并在读取前清理重复用户。
func (h *RealtimeHandler) listRealtimeUsersRoutes(ctx context.Context, eventType string, userUUIDs []string) ([]route.DeviceRoute, error) {
	if h.routes == nil {
		return nil, fmt.Errorf("%w: realtime.push 路由仓储未初始化", errRetriable)
	}
	normalizedUserUUIDs := uniqueNonEmptyStrings(userUUIDs)
	if len(normalizedUserUUIDs) == 0 {
		return nil, nil
	}
	usersRoutes, err := h.routes.ListUsersRoutes(ctx, normalizedUserUUIDs)
	if err != nil {
		metrics.RouteHitRate.WithLabelValues(eventType, "error").Inc()
		return nil, fmt.Errorf("%w: 批量读取实时提醒用户路由失败: %v", errRetriable, err)
	}

	routes := make([]route.DeviceRoute, 0, len(normalizedUserUUIDs))
	for _, userUUID := range normalizedUserUUIDs {
		userRoutes := usersRoutes[userUUID]
		observeRouteLookup(eventType, len(userRoutes))
		routes = append(routes, userRoutes...)
	}
	return dedupeDeviceRoutes(routes), nil
}

// listRealtimeGroupRoutes 先解析群聚合目标成员，再批量读取成员在线路由。
func (h *RealtimeHandler) listRealtimeGroupRoutes(ctx context.Context, eventType, groupUUID string, fetch func(context.Context, string) ([]string, error)) ([]route.DeviceRoute, error) {
	userUUIDs, err := fetch(ctx, groupUUID)
	if err != nil {
		return nil, err
	}
	if len(userUUIDs) == 0 {
		logger.Warn(ctx, "message-push realtime.push 群聚合目标为空，跳过扩散",
			logger.String("event_type", eventType),
			logger.String("group_uuid", groupUUID),
		)
		return nil, nil
	}
	return h.listRealtimeUsersRoutes(ctx, eventType, userUUIDs)
}

// fetchGroupMembers 获取群成员 UUID 列表。
func (h *RealtimeHandler) fetchGroupMembers(ctx context.Context, groupUUID string) ([]string, error) {
	if h.groups == nil {
		return nil, fmt.Errorf("%w: realtime.push 群组客户端未初始化", errRetriable)
	}
	userUUIDs, err := h.groups.GetGroupMembers(ctx, groupUUID)
	if err != nil {
		return nil, fmt.Errorf("%w: 获取群成员失败: %v", errRetriable, err)
	}
	return uniqueNonEmptyStrings(userUUIDs), nil
}

// fetchGroupAdmins 获取群主和管理员 UUID 列表。
func (h *RealtimeHandler) fetchGroupAdmins(ctx context.Context, groupUUID string) ([]string, error) {
	if h.groups == nil {
		return nil, fmt.Errorf("%w: realtime.push 群组客户端未初始化", errRetriable)
	}
	userUUIDs, err := h.groups.GetGroupAdmins(ctx, groupUUID)
	if err != nil {
		return nil, fmt.Errorf("%w: 获取群管理员失败: %v", errRetriable, err)
	}
	return uniqueNonEmptyStrings(userUUIDs), nil
}

// pushRealtimeRoutes 逐设备调用 connect PushToDevice，并返回成功/失败数量。
func (h *RealtimeHandler) pushRealtimeRoutes(ctx context.Context, eventType string, routes []route.DeviceRoute, envelope *connectpb.MessageEnvelope) (int, int) {
	routes = dedupeDeviceRoutes(routes)
	var successCount, failedCount int
	for _, deviceRoute := range routes {
		pushStart := time.Now()
		pushErr := h.sender.PushToDevice(ctx, deviceRoute.ConnectGRPCAddr, deviceRoute.UserUUID, deviceRoute.DeviceID, envelope)
		if pushErr != nil {
			failedCount++
			metrics.PushToDeviceTotal.WithLabelValues(eventType, "error").Inc()
			metrics.ObservePushToDeviceDuration(pushStart, "error")
			logger.Warn(ctx, "message-push realtime.push 调用 connect PushToDevice 失败",
				logger.String("target_user_uuid", deviceRoute.UserUUID),
				logger.String("device_id", deviceRoute.DeviceID),
				logger.String("connect_addr", deviceRoute.ConnectGRPCAddr),
				logger.ErrorField("error", pushErr),
			)
			continue
		}
		successCount++
		metrics.PushToDeviceTotal.WithLabelValues(eventType, "success").Inc()
		metrics.ObservePushToDeviceDuration(pushStart, "success")
	}
	metrics.DeliveredDevices.Observe(float64(successCount))
	return successCount, failedCount
}

// filterDeviceRoutes 按 deviceID 过滤路由；deviceID 为空时返回原路由副本。
func filterDeviceRoutes(routes []route.DeviceRoute, deviceID string) []route.DeviceRoute {
	if len(routes) == 0 {
		return nil
	}
	if deviceID == "" {
		return append([]route.DeviceRoute(nil), routes...)
	}
	filteredRoutes := make([]route.DeviceRoute, 0, 1)
	for _, deviceRoute := range routes {
		if deviceRoute.DeviceID == deviceID {
			filteredRoutes = append(filteredRoutes, deviceRoute)
		}
	}
	return filteredRoutes
}

// dedupeDeviceRoutes 按 user_uuid + device_id 去重设备路由。
func dedupeDeviceRoutes(routes []route.DeviceRoute) []route.DeviceRoute {
	if len(routes) == 0 {
		return nil
	}
	dedupedRoutes := make([]route.DeviceRoute, 0, len(routes))
	seenRoutes := make(map[string]struct{}, len(routes))
	for _, deviceRoute := range routes {
		key := deviceRoute.UserUUID + "\x00" + deviceRoute.DeviceID
		if _, exists := seenRoutes[key]; exists {
			continue
		}
		seenRoutes[key] = struct{}{}
		dedupedRoutes = append(dedupedRoutes, deviceRoute)
	}
	return dedupedRoutes
}

// uniqueNonEmptyStrings 清理空字符串并保持首次出现顺序去重。
func uniqueNonEmptyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	normalizedValues := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalizedValues = append(normalizedValues, value)
	}
	return normalizedValues
}

// observeRouteLookup 统一记录路由命中或未命中指标。
func observeRouteLookup(eventType string, routeCount int) {
	if routeCount == 0 {
		metrics.RouteHitRate.WithLabelValues(eventType, "miss").Inc()
		return
	}
	metrics.RouteHitRate.WithLabelValues(eventType, "hit").Inc()
}
