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
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/msgevent"
	"google.golang.org/protobuf/proto"
)

// routeRepository 只暴露事件路由解析需要的读能力，便于使用内存 mock 覆盖扇出逻辑。
type routeRepository interface {
	ListUserRoutes(ctx context.Context, userUUID string) ([]route.DeviceRoute, error)
	ListUsersRoutes(ctx context.Context, userUUIDs []string) (map[string][]route.DeviceRoute, error)
}

// deviceSender 是精确投递到单台设备所需的能力。
// RealtimeHandler 只需要这一能力；EventHandler 则必须使用下方更完整的 eventSender。
type deviceSender interface {
	PushToDevice(ctx context.Context, connectAddr, userUUID, deviceID string, envelope *connectpb.MessageEnvelope) error
}

// eventSender 是消息事件下行的完整发送契约。
// PushToUser 不是可选优化：完整用户目标必须批量投递；需要排除设备时才使用 PushToDevice。
// 将两种 RPC 放进同一接口可在编译期拒绝只实现旧单设备能力的 sender，禁止静默降级。
type eventSender interface {
	deviceSender
	PushToUser(ctx context.Context, connectAddr, userUUID string, envelope *connectpb.MessageEnvelope) (int32, error)
}

// groupMemberFetcher 隔离群成员查询依赖，使路由策略测试不需要真实 group-service。
type groupMemberFetcher interface {
	GetGroupMembers(ctx context.Context, groupUUID string) ([]string, error)
}

// EventHandler 处理 Kafka 中的 MsgPushEvent。
// maxFanoutConcurrency 只影响单条消息内部的节点级并发，不改变 Kafka 消费并发或提交语义。
type EventHandler struct {
	routes               routeRepository
	sender               eventSender
	groups               groupMemberFetcher
	maxFanoutConcurrency int
}

// NewEventHandler 创建事件处理器。
// maxFanoutConcurrency 是必填契约，必须大于零；依赖或配置非法时直接返回错误，
// 由服务初始化流程终止启动，不使用默认值或旧发送路径掩盖装配错误。
func NewEventHandler(
	routes *route.RedisRepository,
	sender *connectcli.Sender,
	groups *groupcli.Client,
	maxFanoutConcurrency int,
) (*EventHandler, error) {
	switch {
	case routes == nil:
		return nil, fmt.Errorf("message-push 创建 EventHandler 失败: 路由仓储不能为空")
	case sender == nil:
		return nil, fmt.Errorf("message-push 创建 EventHandler 失败: connect 发送器不能为空")
	case groups == nil:
		return nil, fmt.Errorf("message-push 创建 EventHandler 失败: group 客户端不能为空")
	case maxFanoutConcurrency <= 0:
		return nil, fmt.Errorf("message-push 创建 EventHandler 失败: 扇出并发上限必须大于零")
	}
	return &EventHandler{
		routes:               routes,
		sender:               sender,
		groups:               groups,
		maxFanoutConcurrency: maxFanoutConcurrency,
	}, nil
}

// Handle 处理单条 Kafka 事件。
// 返回 errRetriable 包装的错误表示调用方应重试；其它错误或 nil 表示无需重试。
func (h *EventHandler) Handle(ctx context.Context, value []byte) error {
	start := time.Now()
	result := "success"
	defer func() { metrics.ObserveHandleDuration(start, result) }()

	// 解码或事件合约错误无法通过重试恢复，因此记录后直接放行 offset。
	event, ok := decodeMsgPushEvent(ctx, value)
	if !ok || !validateMsgPushEvent(ctx, event) {
		result = "permanent_error"
		return nil
	}
	// 路由阶段同时携带“能否按用户广播”的语义，避免在扇出阶段重新猜测事件规则。
	targets, permanentResult, err := h.resolveEventTargets(ctx, event)
	if permanentResult {
		result = "permanent_error"
	}
	if err != nil {
		result = "retriable_error"
		return err
	}
	// 先按原有 user_uuid+device_id 规则去重，确保后续分组不会重复投递同一设备。
	targets = dedupeDeliveryTargets(targets)
	if len(targets) == 0 {
		return nil
	}
	if h.sender == nil {
		result = "retriable_error"
		return fmt.Errorf("%w: connect 发送器未初始化", errRetriable)
	}

	// envelope 构造后只读，可安全地被多个节点 goroutine 共享，无需复制消息正文。
	envelope, seq := buildMessageEnvelope(event)
	summary := h.pushEventTargets(ctx, event.Type, targets, envelope)
	logEventFanout(ctx, event, seq, len(targets), summary)
	// 保持原判定：全部已尝试设备失败才重试，部分成功不重复推送已成功设备。
	if summary.SuccessCount == 0 && summary.FailedCount > 0 {
		result = "retriable_error"
		return fmt.Errorf("%w: 所有目标设备推送均失败 (%d)", errRetriable, summary.FailedCount)
	}
	return nil
}

// decodeMsgPushEvent 解码事件；合约错误属于永久错误，记录后直接放行。
func decodeMsgPushEvent(ctx context.Context, value []byte) (*msgpb.MsgPushEvent, bool) {
	event, err := msgevent.DecodeMsgPush(value)
	if err == nil {
		return event, true
	}
	metrics.EventTypeSkipped.WithLabelValues("unknown", "decode_error").Inc()
	logger.Warn(ctx, "message-push 解析 msg.push outbox payload 失败，跳过该消息",
		logger.ErrorField("error", err),
		logger.Int("payload_bytes", len(value)),
	)
	return nil, false
}

// validateMsgPushEvent 校验当前阶段支持的事件合约。
func validateMsgPushEvent(ctx context.Context, event *msgpb.MsgPushEvent) bool {
	switch event.Type {
	case "MSG_PUSH", "MSG_RECALL", "MSG_MARK_READ", "MSG_READ_RECEIPT":
	default:
		metrics.EventTypeSkipped.WithLabelValues(event.Type, "unsupported_type").Inc()
		logger.Warn(ctx, "message-push 暂未处理该事件类型，先跳过",
			logger.String("event_type", event.Type),
		)
		return false
	}
	if event.ReceiverUuid != "" {
		return true
	}
	metrics.EventTypeSkipped.WithLabelValues(event.Type, "validation_error").Inc()
	logger.Warn(ctx, "message-push receiver_uuid 为空，跳过该消息",
		logger.String("trace_id", event.TraceId),
	)
	return false
}

// buildMessageEnvelope 构造 connect 下行消息，并兼容从 MsgItem 补齐 seq。
// 部分历史事件未在外层写 seq，但 MSG_PUSH 的 data 内仍有 MsgItem.seq；
// 这里尽力补齐，使 ACK、客户端排序和旧事件兼容逻辑保持一致。
func buildMessageEnvelope(event *msgpb.MsgPushEvent) (*connectpb.MessageEnvelope, int64) {
	seq := event.Seq
	if seq == 0 {
		var item msgpb.MsgItem
		if err := proto.Unmarshal(event.Data, &item); err == nil {
			seq = item.Seq
		}
	}
	return &connectpb.MessageEnvelope{
		Type:        event.Type,
		Data:        event.Data,
		Seq:         seq,
		ServerTs:    event.ServerTs,
		TraceId:     event.TraceId,
		AckRequired: ackRequiredForEvent(event.Type, seq),
	}, seq
}

// logEventFanout 输出单条事件的最终扇出结果，并携带各 connect 节点统计。
// 节点结果集中放入最终日志，既满足慢节点定位需求，也避免每个成功节点各打一条日志。
func logEventFanout(
	ctx context.Context,
	event *msgpb.MsgPushEvent,
	seq int64,
	routeCount int,
	summary fanoutSummary,
) {
	logger.Info(ctx, "message-push 处理完成",
		logger.String("event_type", event.Type),
		logger.String("receiver_uuid", event.ReceiverUuid),
		logger.String("from_uuid", event.FromUuid),
		logger.String("exclude_device_id", event.DeviceId),
		logger.Int64("seq", seq),
		logger.Int64("server_ts", event.ServerTs),
		logger.Int("route_count", routeCount),
		logger.Int("succeeded_count", summary.SuccessCount),
		logger.Int("failed_count", summary.FailedCount),
		logger.Int("delivered_count", summary.SuccessCount),
		logger.Any("nodes", summary.Nodes),
	)
}
