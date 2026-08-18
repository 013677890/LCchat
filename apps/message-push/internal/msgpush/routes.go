package msgpush

import (
	"context"
	"fmt"

	"github.com/013677890/LCchat-Backend/apps/message-push/internal/metrics"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/pusherr"
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	route "github.com/013677890/LCchat-Backend/pkg/presence"
)

// deliveryTarget 在设备路由之外携带本次投递的业务边界。
//
// AllowUserPush 为 true 表示：在过滤发生前，该用户在对应 connect 节点上的全部已知设备
// 都属于目标，因而扇出阶段可以安全调用 PushToUser。
// 它不是「connect 是否支持该 RPC」的能力标记；能力由 eventSender 接口在编译期约束。
type deliveryTarget struct {
	Route         route.DeviceRoute
	AllowUserPush bool
}

// targetCollector 统一追加带投递语义的路由，防止不同事件分支各自实现过滤规则。
type targetCollector struct {
	targets []deliveryTarget
}

// resolveEventTargets 按事件类型解析投递目标，并标记可安全使用 PushToUser 的完整用户目标。
//
// 返回值：
//  1. targets：去重前的投递列表（调用方负责 dedupe）；
//  2. permanent：本次处理应记为永久错误（与是否仍有可投递目标相互独立）；
//  3. error：瞬时失败，已包装 pusherr.ErrRetriable。
//
// 例如群客户端缺失时 permanent=true，但仍继续收集发送方其他设备同步目标。
func (h *Handler) resolveEventTargets(
	ctx context.Context,
	event *msgpb.MsgPushEvent,
) ([]deliveryTarget, bool, error) {
	collector := &targetCollector{}
	switch event.Type {
	case "MSG_PUSH", "MSG_RECALL":
		permanentResult, err := h.collectMessageTargets(ctx, event, collector)
		return collector.targets, permanentResult, err
	case "MSG_MARK_READ":
		// 已读同步只推给「本账号除当前设备外」的其它端：当前设备本地已更新，无需再推。
		// 只要 DeviceId 非空就必须排除，因而 AllowUserPush=false，只能 PushToDevice。
		err := h.collectUserTargets(ctx, event.Type, event.ReceiverUuid, event.DeviceId, "", collector)
		if err != nil {
			return nil, false, fmt.Errorf("%w: 读取已读同步路由失败: %v", pusherr.ErrRetriable, err)
		}
	case "MSG_READ_RECEIPT":
		// 已读回执通知对端全部设备，不存在设备级排除，因此可以按节点使用 PushToUser。
		err := h.collectUserTargets(
			ctx,
			event.Type,
			event.ReceiverUuid,
			"",
			"message-push 未找到已读回执接收方在线路由，按离线处理",
			collector,
		)
		if err != nil {
			return nil, false, fmt.Errorf("%w: 读取已读回执路由失败: %v", pusherr.ErrRetriable, err)
		}
	}

	return collector.targets, false, nil
}

// collectMessageTargets 收集消息或撤回事件的接收方及发送方其他设备。
// 接收方/群成员属于完整用户目标，可以批量；发送方必须排除当前设备，只能精确单推。
func (h *Handler) collectMessageTargets(
	ctx context.Context,
	event *msgpb.MsgPushEvent,
	collector *targetCollector,
) (bool, error) {
	permanentResult := false
	switch event.ConvType {
	case msgpb.ConvType_CONV_TYPE_P2P:
		// P2P 接收方的所有在线设备都应收到事件，因此保留完整用户语义。
		err := h.collectUserTargets(
			ctx,
			event.Type,
			event.ReceiverUuid,
			"",
			"message-push 未找到接收方在线路由，按离线处理",
			collector,
		)
		if err != nil {
			return false, fmt.Errorf("%w: 读取用户路由失败: %v", pusherr.ErrRetriable, err)
		}
	case msgpb.ConvType_CONV_TYPE_GROUP:
		// 群成员先批量查路由；发送方在群成员阶段整体排除，稍后单独同步其他设备。
		var err error
		permanentResult, err = h.collectGroupTargets(ctx, event, collector)
		if err != nil {
			return false, err
		}
	default:
		logUnsupportedConversation(ctx, event)
		return true, nil
	}

	// 发送方当前设备必须排除，因此这一组目标不能使用按用户广播。
	// DeviceId 为空时无法表达排除项：只能按完整用户目标投递发送方全部在线设备。
	err := h.collectUserTargets(ctx, event.Type, event.FromUuid, event.DeviceId, "", collector)
	if err != nil {
		return false, fmt.Errorf("%w: 读取发送方路由失败: %v", pusherr.ErrRetriable, err)
	}
	return permanentResult, nil
}

// collectUserTargets 读取单用户路由，并保留是否完整覆盖该用户设备的语义。
// “是否可 PushToUser”必须在过滤发生处确定；进入扇出阶段后仅凭剩余路由无法判断
// 某台设备是本来不在线，还是因为业务规则被明确排除。
func (h *Handler) collectUserTargets(
	ctx context.Context,
	eventType string,
	userUUID string,
	excludeDeviceID string,
	missLog string,
	collector *targetCollector,
) error {
	if userUUID == "" {
		return nil
	}
	if h.routes == nil {
		return fmt.Errorf("%w: 路由仓储未初始化", pusherr.ErrRetriable)
	}
	userRoutes, err := h.routes.ListUserRoutes(ctx, userUUID)
	if err != nil {
		metrics.RouteHitRate.WithLabelValues(eventType, "error").Inc()
		return err
	}

	// 只有未排除任何设备时，路由快照才代表该用户在这些节点上的完整目标集合。
	appended := collector.appendRoutes(userRoutes, excludeDeviceID, excludeDeviceID == "")
	observeCollectedRoutes(ctx, eventType, userUUID, excludeDeviceID, missLog, appended)
	return nil
}

// collectGroupTargets 展开群成员路由；缺失群客户端时保留发送方其他设备同步。
// 返回 permanent=true 而非直接结束 Handle，是为了不因群扩散依赖缺失而漏掉
// 发送方其他设备同步，同时仍让处理耗时指标准确反映配置错误。
func (h *Handler) collectGroupTargets(
	ctx context.Context,
	event *msgpb.MsgPushEvent,
	collector *targetCollector,
) (bool, error) {
	if h.routes == nil {
		return false, fmt.Errorf("%w: 路由仓储未初始化", pusherr.ErrRetriable)
	}
	if h.groups == nil {
		logger.Warn(ctx, "message-push 群组客户端未初始化，跳过群聊扩散",
			logger.String("group_uuid", event.ReceiverUuid),
		)
		return true, nil
	}

	memberUUIDs, err := h.groups.GetGroupMembers(ctx, event.ReceiverUuid)
	if err != nil {
		return false, fmt.Errorf("%w: 获取群成员失败: %v", pusherr.ErrRetriable, err)
	}
	memberUUIDs = filterGroupMembers(memberUUIDs, event.FromUuid)
	if len(memberUUIDs) == 0 {
		logger.Warn(ctx, "message-push 未找到群成员，跳过群聊扩散",
			logger.String("group_uuid", event.ReceiverUuid),
		)
		return false, nil
	}

	return false, h.collectGroupMemberRoutes(ctx, event.Type, memberUUIDs, collector)
}

// collectGroupMemberRoutes 批量读取群成员路由，并按成员记录命中指标。
// 每个非发送方群成员都是完整用户目标，因此其路由可在对应 connect 节点上批量广播。
func (h *Handler) collectGroupMemberRoutes(
	ctx context.Context,
	eventType string,
	memberUUIDs []string,
	collector *targetCollector,
) error {
	memberRoutes, err := h.routes.ListUsersRoutes(ctx, memberUUIDs)
	if err != nil {
		metrics.RouteHitRate.WithLabelValues(eventType, "error").Inc()
		return fmt.Errorf("%w: 批量读取群成员路由失败: %v", pusherr.ErrRetriable, err)
	}
	for _, userUUID := range memberUUIDs {
		userRoutes := memberRoutes[userUUID]
		if len(userRoutes) == 0 {
			metrics.RouteHitRate.WithLabelValues(eventType, "miss").Inc()
			continue
		}
		metrics.RouteHitRate.WithLabelValues(eventType, "hit").Inc()
		collector.appendRoutes(userRoutes, "", true)
	}
	return nil
}

// appendRoutes 追加过滤后的设备路由，并返回实际追加数量。
// allowUserPush 由调用方根据“过滤前是否为完整用户目标”给出，不能由追加结果反推。
func (c *targetCollector) appendRoutes(
	routes []route.DeviceRoute,
	excludeDeviceID string,
	allowUserPush bool,
) int {
	appended := 0
	for _, deviceRoute := range routes {
		if excludeDeviceID != "" && deviceRoute.DeviceID == excludeDeviceID {
			continue
		}
		c.targets = append(c.targets, deliveryTarget{
			Route:         deviceRoute,
			AllowUserPush: allowUserPush,
		})
		appended++
	}
	return appended
}

// observeCollectedRoutes 记录单用户路由命中情况，并按需输出离线告警。
// 命中数使用过滤后的 appended：若仅当前设备在线但被业务排除，仍记 miss，
// 表示「对本账号其它端无需再同步」，而不是 Redis 真的查不到路由。
func observeCollectedRoutes(
	ctx context.Context,
	eventType string,
	userUUID string,
	excludeDeviceID string,
	missLog string,
	appended int,
) {
	if appended > 0 {
		metrics.RouteHitRate.WithLabelValues(eventType, "hit").Inc()
		return
	}
	metrics.RouteHitRate.WithLabelValues(eventType, "miss").Inc()
	if missLog != "" {
		logger.Warn(ctx, missLog,
			logger.String("user_uuid", userUUID),
			logger.String("exclude_device_id", excludeDeviceID),
		)
	}
}

// filterGroupMembers 清理空成员并排除消息发送方。
// 此处有意不改变成员顺序；最终设备级重复由统一去重逻辑处理。
func filterGroupMembers(memberUUIDs []string, excludeUserUUID string) []string {
	filtered := make([]string, 0, len(memberUUIDs))
	for _, userUUID := range memberUUIDs {
		if userUUID == "" || (excludeUserUUID != "" && userUUID == excludeUserUUID) {
			continue
		}
		filtered = append(filtered, userUUID)
	}
	return filtered
}

// logUnsupportedConversation 记录无法处理的会话类型。
func logUnsupportedConversation(ctx context.Context, event *msgpb.MsgPushEvent) {
	metrics.EventTypeSkipped.WithLabelValues(event.Type, "unsupported_conv_type").Inc()
	logger.Warn(ctx, "message-push 暂未处理该会话类型，先跳过",
		logger.String("event_type", event.Type),
		logger.Int("conv_type", int(event.ConvType)),
	)
}

// dedupeDeliveryTargets 按 user_uuid 和 device_id 去重，并合并同节点的批量推送能力。
// 同设备若短暂出现新旧节点路由，仍沿用原逻辑保留首次路由；只有地址相同时才合并
// AllowUserPush，避免 node-B 的完整用户语义错误地授权 node-A 执行整用户广播。
func dedupeDeliveryTargets(targets []deliveryTarget) []deliveryTarget {
	deduped := make([]deliveryTarget, 0, len(targets))
	targetIndexes := make(map[string]int, len(targets))
	for _, target := range targets {
		key := target.Route.UserUUID + "\x00" + target.Route.DeviceID
		index, exists := targetIndexes[key]
		if !exists {
			targetIndexes[key] = len(deduped)
			deduped = append(deduped, target)
			continue
		}
		// 跨节点重复通常来自迁移窗口，不能把后一节点的批量资格传播给首次保留的节点。
		if deduped[index].Route.ConnectGRPCAddr == target.Route.ConnectGRPCAddr {
			deduped[index].AllowUserPush = deduped[index].AllowUserPush || target.AllowUserPush
		}
	}
	return deduped
}
