package realtime

import (
	"context"
	"fmt"

	"github.com/013677890/LCchat-Backend/apps/message-push/internal/metrics"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/pusherr"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	route "github.com/013677890/LCchat-Backend/pkg/presence"
	"github.com/013677890/LCchat-Backend/pkg/realtimepush"
)

// resolveRealtimeRoutes 根据 RealtimePushEvent.target.kind 解析在线设备路由。
//
// 不支持的 kind 记 permanent 跳过（返回 nil, nil），而不是 ErrRetriable：
// 新 kind 未部署处理逻辑时重试只会空转并阻塞分区。
func (h *Handler) resolveRealtimeRoutes(ctx context.Context, event realtimepush.Event) ([]route.DeviceRoute, error) {
	switch event.Target.Kind {
	case realtimepush.TargetKindUser:
		// 用户全部在线设备。
		return h.listRealtimeUserRoutes(ctx, event.Type, event.Target.UserUUID, "")
	case realtimepush.TargetKindDevice:
		// 精确到单设备；deviceID 为空时过滤结果为空，视为无目标。
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

// listRealtimeUserRoutes 读取单用户路由；deviceID 非空时只保留指定设备。
// Redis/依赖瞬时失败包装为 ErrRetriable；路由未命中返回空切片，由 Handle 视为离线跳过。
func (h *Handler) listRealtimeUserRoutes(ctx context.Context, eventType, userUUID, deviceID string) ([]route.DeviceRoute, error) {
	if h.routes == nil {
		return nil, fmt.Errorf("%w: realtime.push 路由仓储未初始化", pusherr.ErrRetriable)
	}
	userRoutes, err := h.routes.ListUserRoutes(ctx, userUUID)
	if err != nil {
		metrics.RouteHitRate.WithLabelValues(eventType, "error").Inc()
		return nil, fmt.Errorf("%w: 读取实时提醒用户路由失败: %v", pusherr.ErrRetriable, err)
	}
	filteredRoutes := filterDeviceRoutes(userRoutes, deviceID)
	observeRouteLookup(eventType, len(filteredRoutes))
	return filteredRoutes, nil
}

// listRealtimeUsersRoutes 批量读取多用户路由。
// 读取前去重空/重复 UUID，避免对同一用户重复打 Redis；读后再按设备键去重一次，
// 防止同一设备因列表重复或路由重复被推两次。
func (h *Handler) listRealtimeUsersRoutes(ctx context.Context, eventType string, userUUIDs []string) ([]route.DeviceRoute, error) {
	if h.routes == nil {
		return nil, fmt.Errorf("%w: realtime.push 路由仓储未初始化", pusherr.ErrRetriable)
	}
	normalizedUserUUIDs := uniqueNonEmptyStrings(userUUIDs)
	if len(normalizedUserUUIDs) == 0 {
		return nil, nil
	}
	usersRoutes, err := h.routes.ListUsersRoutes(ctx, normalizedUserUUIDs)
	if err != nil {
		metrics.RouteHitRate.WithLabelValues(eventType, "error").Inc()
		return nil, fmt.Errorf("%w: 批量读取实时提醒用户路由失败: %v", pusherr.ErrRetriable, err)
	}

	routes := make([]route.DeviceRoute, 0, len(normalizedUserUUIDs))
	for _, userUUID := range normalizedUserUUIDs {
		userRoutes := usersRoutes[userUUID]
		observeRouteLookup(eventType, len(userRoutes))
		routes = append(routes, userRoutes...)
	}

	return dedupeDeviceRoutes(routes), nil
}

// listRealtimeGroupRoutes 先经 group gRPC 展开成员/管理员，再复用多用户路由批量读取。
// fetch 由调用方注入（成员或管理员），便于单测替换而不 mock 整个 group 客户端。
func (h *Handler) listRealtimeGroupRoutes(ctx context.Context, eventType, groupUUID string, fetch func(context.Context, string) ([]string, error)) ([]route.DeviceRoute, error) {
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

// fetchGroupMembers 获取群成员 UUID 列表；groups 未装配视为瞬时配置错误，可重试。
func (h *Handler) fetchGroupMembers(ctx context.Context, groupUUID string) ([]string, error) {
	if h.groups == nil {
		return nil, fmt.Errorf("%w: realtime.push 群组客户端未初始化", pusherr.ErrRetriable)
	}
	userUUIDs, err := h.groups.GetGroupMembers(ctx, groupUUID)
	if err != nil {
		return nil, fmt.Errorf("%w: 获取群成员失败: %v", pusherr.ErrRetriable, err)
	}
	return uniqueNonEmptyStrings(userUUIDs), nil
}

// fetchGroupAdmins 获取群主和管理员 UUID 列表；groups 未装配视为瞬时配置错误，可重试。
func (h *Handler) fetchGroupAdmins(ctx context.Context, groupUUID string) ([]string, error) {
	if h.groups == nil {
		return nil, fmt.Errorf("%w: realtime.push 群组客户端未初始化", pusherr.ErrRetriable)
	}
	userUUIDs, err := h.groups.GetGroupAdmins(ctx, groupUUID)
	if err != nil {
		return nil, fmt.Errorf("%w: 获取群管理员失败: %v", pusherr.ErrRetriable, err)
	}
	return uniqueNonEmptyStrings(userUUIDs), nil
}

// filterDeviceRoutes 按 deviceID 过滤路由。
// deviceID 为空时返回原路由副本（避免调用方误改底层切片）；非空时最多保留一台匹配设备。
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

// dedupeDeviceRoutes 按 user_uuid + device_id 去重，保留首次出现的路由。
// 跨 connect 节点的同设备冲突也只保留首次：迁移窗口内以后到的节点为准会造成重复推送。
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

// uniqueNonEmptyStrings 清理空字符串并按首次出现顺序去重，稳定批量 Redis/gRPC 入参。
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

// observeRouteLookup 按单用户维度记录路由命中/未命中，供离线率看板使用。
func observeRouteLookup(eventType string, routeCount int) {
	if routeCount == 0 {
		metrics.RouteHitRate.WithLabelValues(eventType, "miss").Inc()
		return
	}
	metrics.RouteHitRate.WithLabelValues(eventType, "hit").Inc()
}
