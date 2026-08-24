// Package msgpush 实现 Kafka topic `msg.push` 的业务处理。
//
// 流水线：解码 MsgPushEvent → 按事件类型/会话类型解析投递目标（含 AllowUserPush 语义）
// → 按 connect 节点有界并发扇出 → 完整用户目标走 PushToUser，排除设备走 PushToDevice。
//
// 本包不感知 Kafka offset / 重试预算；瞬时失败包装为 pusherr.ErrRetriable，
// 由 consumer 适配层统一重试与 best-effort 提交。也不 import realtime 包。
package msgpush

import (
	"context"
	"fmt"
	"time"

	connectpb "github.com/013677890/LCchat-Backend/apps/connect/pb"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/connectcli"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/groupcli"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/metrics"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/pusherr"
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/pkg/event"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	route "github.com/013677890/LCchat-Backend/pkg/presence"
	"google.golang.org/protobuf/proto"
)

// routeRepository 只暴露事件路由解析需要的读能力，便于使用内存 mock 覆盖扇出逻辑。
type routeRepository interface {
	ListUserRoutes(ctx context.Context, userUUID string) ([]route.DeviceRoute, error)
	ListUsersRoutes(ctx context.Context, userUUIDs []string) (map[string][]route.DeviceRoute, error)
}

// deviceSender 是精确投递到单台设备所需的能力。
// 需要排除当前设备、或只能点对点同步时使用；完整用户目标必须走下方 eventSender.PushToUser。
type deviceSender interface {
	PushToDevice(ctx context.Context, connectAddr, userUUID, deviceID string, envelope *connectpb.MessageEnvelope) error
}

// eventSender 是消息事件下行的完整发送契约。
// PushToUser 不是可选优化：完整用户目标必须批量投递；需要排除设备时才使用 PushToDevice。
// 将两种 RPC 放进同一接口可在编译期拒绝只实现单设备能力的 sender，禁止静默降级。
type eventSender interface {
	deviceSender
	PushToUser(ctx context.Context, connectAddr, userUUID string, envelope *connectpb.MessageEnvelope) (int32, error)
}

// groupMemberFetcher 隔离群成员查询依赖，使路由策略测试不需要真实 group-service。
type groupMemberFetcher interface {
	GetGroupMembers(ctx context.Context, groupUUID string) ([]string, error)
}

// Handler 处理 Kafka 中的 MsgPushEvent（消息/撤回/已读同步/已读回执）。
//
// maxFanoutConcurrency 只影响单条消息内部的「按 connect 节点」并发上限，
// 与 Kafka Pool 的 workers、partition 分配无关；两层并发必须分开理解。
type Handler struct {
	routes               routeRepository
	sender               eventSender
	groups               groupMemberFetcher
	maxFanoutConcurrency int
}

// NewHandler 创建 msg.push 下行处理器。
//
// maxFanoutConcurrency 是必填契约，必须大于零；依赖或配置非法时直接返回错误，
// 由服务初始化流程终止启动。禁止用默认值掩盖装配错误，避免生产以错误扇出容量运行。
func NewHandler(
	routes *route.RedisRepository,
	sender *connectcli.Sender,
	groups *groupcli.Client,
	maxFanoutConcurrency int,
) (*Handler, error) {
	switch {
	case routes == nil:
		return nil, fmt.Errorf("message-push 创建 msg.push Handler 失败: 路由仓储不能为空")
	case sender == nil:
		return nil, fmt.Errorf("message-push 创建 msg.push Handler 失败: connect 发送器不能为空")
	case groups == nil:
		return nil, fmt.Errorf("message-push 创建 msg.push Handler 失败: group 客户端不能为空")
	case maxFanoutConcurrency <= 0:
		return nil, fmt.Errorf("message-push 创建 msg.push Handler 失败: 扇出并发上限必须大于零")
	}
	return &Handler{
		routes:               routes,
		sender:               sender,
		groups:               groups,
		maxFanoutConcurrency: maxFanoutConcurrency,
	}, nil
}

// Handle 处理单条 msg.push Kafka 消息。
//
// 返回约定（供 consumer 适配层识别）：
//   - nil：成功，或永久错误已跳过（调用方应 commit）；
//   - errors.Is(..., pusherr.ErrRetriable)：瞬时失败，可本地重试。
//
// 重试判定刻意偏保守：只要已尝试设备里有一台成功，就不整体重试，
// 避免把已入队成功的设备再推一遍造成客户端重复插入。
func (h *Handler) Handle(ctx context.Context, value []byte) error {
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
		// permanent 与可投递目标可并存：例如群客户端缺失时仍同步发送方其他设备，
		// 但指标必须反映配置/合约问题，不能记成纯 success。
		result = "permanent_error"
	}
	if err != nil {
		result = "retriable_error"
		return err
	}

	// 先按 user_uuid+device_id 去重，确保后续按节点分组不会对同一设备重复 RPC。
	targets = dedupeDeliveryTargets(targets)
	if len(targets) == 0 {
		// 全部离线或无可投递目标：不算失败，客户端补拉即可。
		return nil
	}
	if h.sender == nil {
		result = "retriable_error"
		return fmt.Errorf("%w: connect 发送器未初始化", pusherr.ErrRetriable)
	}

	// envelope 构造后只读，可安全地被多个节点 goroutine 共享，无需复制消息正文。
	envelope, seq := buildMessageEnvelope(event)
	summary := h.pushEventTargets(ctx, event.Type, targets, envelope)
	logEventFanout(ctx, event, seq, len(targets), summary)

	// 全部已尝试设备失败才重试；部分成功不重复推送已成功设备。
	if summary.SuccessCount == 0 && summary.FailedCount > 0 {
		result = "retriable_error"
		return fmt.Errorf("%w: 所有目标设备推送均失败 (%d)", pusherr.ErrRetriable, summary.FailedCount)
	}
	return nil
}

// decodeMsgPushEvent 解码 Outbox/CDC 顶层 JSON Object 为 MsgPushEvent。
// 解码失败属于永久错误：payload 形状非法时重试无意义，记指标后返回 ok=false。
func decodeMsgPushEvent(ctx context.Context, value []byte) (*msgpb.MsgPushEvent, bool) {
	decodedEvent, err := event.DecodeMsgPush(value)
	if err == nil {
		return decodedEvent, true
	}
	metrics.EventTypeSkipped.WithLabelValues("unknown", "decode_error").Inc()
	logger.Warn(ctx, "message-push 解析 msg.push outbox payload 失败，跳过该消息",
		logger.ErrorField("error", err),
		logger.Int("payload_bytes", len(value)),
	)
	return nil, false
}

// validateMsgPushEvent 校验当前进程支持的事件类型与必填字段。
// 不支持的类型或缺失 receiver_uuid 一律跳过：重试无法修复合约错误，且不应阻塞分区。
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

// buildMessageEnvelope 构造 connect 下行 Envelope。
//
// 外层 seq 优先；若为 0 且 data 可反序列化为 MsgItem，则回填 MsgItem.seq。
// 这样 ACK 判定与客户端排序仍能拿到有效位点，而不要求所有生产者都在外层重复写 seq。
// 反序列化失败时保持 seq=0，由 ackRequiredForEvent 决定不强制 ACK。
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
// 节点结果集中放入最终日志，既满足慢节点定位需求，也避免每个成功节点各打一条 Info。
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
