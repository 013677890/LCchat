package consumer

import (
	"context"
	"errors"
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

// errRetriable 表示当前消息处理失败属于可重试范畴（Redis 抖动、connect 瞬时不可达等）。
// 永久性错误（proto 反序列化失败、字段校验失败）应返回 nil 并在日志里告警，避免阻塞消费进度。
var errRetriable = errors.New("message-push: retriable handle error")

// 单条消息的本地重试上限与退避梯度。
// 这里只做"就地短重试"，不投递 DLQ；超出后记录告警并按阶段一策略让出 offset，
// 避免一条长期失败的消息阻塞整条消费链路。
const (
	handleMaxAttempts = 3

	// handleAttemptTimeout 为单次处理尝试的超时预算。
	// message-push 无 DB，单个下游已各自有界（Redis ReadTimeout、connect/group gRPC 方法级超时），
	// 该预算用于给「群扩散逐设备串行调用」的总时长封顶，避免一条大群消息长时间占住分区。
	// 取值偏宽：正常群扩散远达不到，仅作为病态扇出的安全上限。
	handleAttemptTimeout = 30 * time.Second
)

var handleBackoffs = []time.Duration{
	100 * time.Millisecond,
	300 * time.Millisecond,
	800 * time.Millisecond,
}

type routeRepository interface {
	ListUserRoutes(ctx context.Context, userUUID string) ([]route.DeviceRoute, error)
	ListUsersRoutes(ctx context.Context, userUUIDs []string) (map[string][]route.DeviceRoute, error)
}

type deviceSender interface {
	PushToDevice(ctx context.Context, connectAddr, userUUID, deviceID string, envelope *connectpb.MessageEnvelope) error
}

type groupMemberFetcher interface {
	GetGroupMembers(ctx context.Context, groupUUID string) ([]string, error)
}

// Handler 定义 Kafka 消息处理器的最小能力。
type Handler interface {
	Handle(ctx context.Context, value []byte) error
}

// EventHandler 处理 Kafka 中的 MsgPushEvent。
type EventHandler struct {
	routes routeRepository
	sender deviceSender
	groups groupMemberFetcher
}

// NewEventHandler 创建事件处理器。
func NewEventHandler(routes *route.RedisRepository, sender *connectcli.Sender, groups *groupcli.Client) *EventHandler {
	return &EventHandler{routes: routes, sender: sender, groups: groups}
}

// Handle 处理单条 Kafka 事件。
// 返回 errRetriable 包装的错误表示调用方应重试；其它错误或 nil 表示无需重试。
func (h *EventHandler) Handle(ctx context.Context, value []byte) error {
	start := time.Now()
	result := "success"
	defer func() {
		metrics.ObserveHandleDuration(start, result)
	}()

	event, err := msgevent.DecodeMsgPush(value)
	if err != nil {
		result = "permanent_error"
		metrics.EventTypeSkipped.WithLabelValues("unknown", "decode_error").Inc()
		// payload 合约错误属于永久错误，重试无意义。直接记录并放行。
		logger.Warn(ctx, "message-push 解析 msg.push outbox payload 失败，跳过该消息",
			logger.ErrorField("error", err),
			logger.Int("payload_bytes", len(value)),
		)
		return nil
	}

	// 第一阶段支持四类下行事件，共享统一下发链路。
	switch event.Type {
	case "MSG_PUSH", "MSG_RECALL", "MSG_MARK_READ", "MSG_READ_RECEIPT":
		// 支持的事件类型继续处理。
	default:
		result = "permanent_error"
		metrics.EventTypeSkipped.WithLabelValues(event.Type, "unsupported_type").Inc()
		logger.Warn(ctx, "message-push 暂未处理该事件类型，先跳过",
			logger.String("event_type", event.Type),
		)
		return nil
	}

	if event.ReceiverUuid == "" {
		result = "permanent_error"
		metrics.EventTypeSkipped.WithLabelValues(event.Type, "validation_error").Inc()
		logger.Warn(ctx, "message-push receiver_uuid 为空，跳过该消息",
			logger.String("trace_id", event.TraceId),
		)
		return nil
	}

	var routes []route.DeviceRoute
	appendUserRoutes := func(userUUID, excludeDeviceID, logLabel string) error {
		if userUUID == "" {
			return nil
		}
		if h.routes == nil {
			return fmt.Errorf("%w: 路由仓储未初始化", errRetriable)
		}
		userRoutes, routeErr := h.routes.ListUserRoutes(ctx, userUUID)
		if routeErr != nil {
			metrics.RouteHitRate.WithLabelValues(event.Type, "error").Inc()
			return routeErr
		}

		appended := 0
		for _, deviceRoute := range userRoutes {
			if excludeDeviceID != "" && deviceRoute.DeviceID == excludeDeviceID {
				continue
			}
			routes = append(routes, deviceRoute)
			appended++
		}
		if appended == 0 {
			metrics.RouteHitRate.WithLabelValues(event.Type, "miss").Inc()
			if logLabel != "" {
				logger.Warn(ctx, logLabel,
					logger.String("user_uuid", userUUID),
					logger.String("exclude_device_id", excludeDeviceID),
				)
			}
		} else {
			metrics.RouteHitRate.WithLabelValues(event.Type, "hit").Inc()
		}
		return nil
	}

	appendGroupRoutes := func(groupUUID, excludeUserUUID string) error {
		if h.routes == nil {
			result = "retriable_error"
			return fmt.Errorf("%w: 路由仓储未初始化", errRetriable)
		}
		if h.groups == nil {
			result = "permanent_error"
			logger.Warn(ctx, "message-push 群组客户端未初始化，跳过群聊扩散",
				logger.String("group_uuid", groupUUID),
			)
			return nil
		}
		memberUUIDs, groupErr := h.groups.GetGroupMembers(ctx, groupUUID)
		if groupErr != nil {
			result = "retriable_error"
			return fmt.Errorf("%w: 获取群成员失败: %v", errRetriable, groupErr)
		}
		filteredMemberUUIDs := make([]string, 0, len(memberUUIDs))
		for _, userUUID := range memberUUIDs {
			if userUUID == "" || (excludeUserUUID != "" && userUUID == excludeUserUUID) {
				continue
			}
			filteredMemberUUIDs = append(filteredMemberUUIDs, userUUID)
		}
		if len(filteredMemberUUIDs) == 0 {
			logger.Warn(ctx, "message-push 未找到群成员，跳过群聊扩散",
				logger.String("group_uuid", groupUUID),
			)
			return nil
		}
		memberRoutes, routeErr := h.routes.ListUsersRoutes(ctx, filteredMemberUUIDs)
		if routeErr != nil {
			result = "retriable_error"
			metrics.RouteHitRate.WithLabelValues(event.Type, "error").Inc()
			return fmt.Errorf("%w: 批量读取群成员路由失败: %v", errRetriable, routeErr)
		}
		for _, userUUID := range filteredMemberUUIDs {
			userRoutes := memberRoutes[userUUID]
			if len(userRoutes) == 0 {
				metrics.RouteHitRate.WithLabelValues(event.Type, "miss").Inc()
				continue
			}
			metrics.RouteHitRate.WithLabelValues(event.Type, "hit").Inc()
			routes = append(routes, userRoutes...)
		}
		return nil
	}

	switch event.Type {
	case "MSG_PUSH", "MSG_RECALL":
		switch event.ConvType {
		case msgpb.ConvType_CONV_TYPE_P2P:
			if err := appendUserRoutes(event.ReceiverUuid, "", "message-push 未找到接收方在线路由，按离线处理"); err != nil {
				result = "retriable_error"
				return fmt.Errorf("%w: 读取用户路由失败: %v", errRetriable, err)
			}
		case msgpb.ConvType_CONV_TYPE_GROUP:
			if err := appendGroupRoutes(event.ReceiverUuid, event.FromUuid); err != nil {
				return err
			}
		default:
			result = "permanent_error"
			metrics.EventTypeSkipped.WithLabelValues(event.Type, "unsupported_conv_type").Inc()
			logger.Warn(ctx, "message-push 暂未处理该会话类型，先跳过",
				logger.String("event_type", event.Type),
				logger.Int("conv_type", int(event.ConvType)),
			)
			return nil
		}

		// 发消息/撤回需要同步发送方其他设备，但不能把这条规则泛化到已读类事件。
		if err := appendUserRoutes(event.FromUuid, event.DeviceId, ""); err != nil {
			result = "retriable_error"
			return fmt.Errorf("%w: 读取发送方路由失败: %v", errRetriable, err)
		}
	case "MSG_MARK_READ":
		// 已读同步只投递给发起用户的其他设备；忽略 conv_type，避免群聊已读被误当作群扩散。
		if err := appendUserRoutes(event.ReceiverUuid, event.DeviceId, ""); err != nil {
			result = "retriable_error"
			return fmt.Errorf("%w: 读取已读同步路由失败: %v", errRetriable, err)
		}
	case "MSG_READ_RECEIPT":
		// 已读回执只通知对端，不回灌给已读发起人的其他设备。
		if err := appendUserRoutes(event.ReceiverUuid, "", "message-push 未找到已读回执接收方在线路由，按离线处理"); err != nil {
			result = "retriable_error"
			return fmt.Errorf("%w: 读取已读回执路由失败: %v", errRetriable, err)
		}
	}

	if len(routes) == 0 {
		return nil
	}
	if h.sender == nil {
		result = "retriable_error"
		return fmt.Errorf("%w: connect 发送器未初始化", errRetriable)
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
	routes = dedupedRoutes

	seq := event.Seq
	if seq == 0 {
		var item msgpb.MsgItem
		if unmarshalErr := proto.Unmarshal(event.Data, &item); unmarshalErr == nil {
			seq = item.Seq
		}
	}

	envelope := &connectpb.MessageEnvelope{
		Type:        event.Type,
		Data:        event.Data,
		Seq:         seq,
		ServerTs:    event.ServerTs,
		TraceId:     event.TraceId,
		AckRequired: ackRequiredForEvent(event.Type, seq),
	}

	var (
		successCount int
		failedCount  int
	)
	for _, deviceRoute := range routes {
		if ctx.Err() != nil {
			// 处理预算到期/关停：停止继续逐设备扇出，避免对剩余设备做无谓的快失败调用；
			// 未投递设备由客户端按 seq 拉取兜底（与重试耗尽丢弃同一兜底路径）。
			break
		}
		pushStart := time.Now()
		pushErr := h.sender.PushToDevice(ctx, deviceRoute.ConnectGRPCAddr, deviceRoute.UserUUID, deviceRoute.DeviceID, envelope)
		if pushErr != nil {
			failedCount++
			metrics.PushToDeviceTotal.WithLabelValues(event.Type, "error").Inc()
			metrics.ObservePushToDeviceDuration(pushStart, "error")
			logger.Warn(ctx, "message-push 调用 connect PushToDevice 失败",
				logger.String("target_user_uuid", deviceRoute.UserUUID),
				logger.String("device_id", deviceRoute.DeviceID),
				logger.String("connect_addr", deviceRoute.ConnectGRPCAddr),
				logger.ErrorField("error", pushErr),
			)
			continue
		}
		successCount++
		metrics.PushToDeviceTotal.WithLabelValues(event.Type, "success").Inc()
		metrics.ObservePushToDeviceDuration(pushStart, "success")
	}
	metrics.DeliveredDevices.Observe(float64(successCount))

	logger.Info(ctx, "message-push 处理完成",
		logger.String("receiver_uuid", event.ReceiverUuid),
		logger.String("from_uuid", event.FromUuid),
		logger.String("exclude_device_id", event.DeviceId),
		logger.Int64("seq", seq),
		logger.Int64("server_ts", event.ServerTs),
		logger.Int("route_count", len(routes)),
		logger.Int("succeeded_count", successCount),
		logger.Int("failed_count", failedCount),
		logger.Int("delivered_count", successCount),
	)

	if successCount == 0 && failedCount > 0 {
		result = "retriable_error"
		return fmt.Errorf("%w: 所有设备推送均失败 (%d)", errRetriable, failedCount)
	}
	return nil
}

// Consumer 自研 Kafka 消费循环。
type Consumer struct {
	brokers []string
	topic   string
	groupID string
	handler Handler
}

// NewConsumer 创建消费者。
func NewConsumer(brokers []string, topic, groupID string, handler Handler) *Consumer {
	return &Consumer{brokers: brokers, topic: topic, groupID: groupID, handler: handler}
}

// Start 启动消费循环。
func (c *Consumer) Start(ctx context.Context) error {
	if c.handler == nil {
		return fmt.Errorf("event handler 未初始化")
	}
	reader := newKafkaReader(c.brokers, c.topic, c.groupID)
	defer reader.Close()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		fetchStart := time.Now()
		msg, err := reader.FetchMessage(ctx)
		metrics.KafkaFetchDuration.Observe(time.Since(fetchStart).Seconds())
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			logger.Warn(ctx, "message-push 拉取 Kafka 消息失败",
				logger.ErrorField("error", err),
			)
			time.Sleep(200 * time.Millisecond)
			continue
		}

		handleErr := c.runHandleWithRetry(ctx, msg.Value)
		if handleErr != nil {
			if ctx.Err() != nil {
				// 关闭流程中被取消，不计为故障丢弃，避免污染告警指标。
				logger.Warn(ctx, "message-push 处理在关闭中被取消，提交 offset",
					logger.ErrorField("error", handleErr),
				)
			} else {
				// 本地重试耗尽仍失败：提交 offset 放弃该事件（阶段一策略），依赖客户端按 seq 拉取兜底。
				// 记录按事件类型聚合的丢弃指标，便于在 Redis/connect 持续异常时及时告警。
				eventType := eventTypeForMetric(msg.Value)
				metrics.MessagesDroppedAfterRetry.WithLabelValues(eventType).Inc()
				logger.Warn(ctx, "message-push 本地重试仍失败，提交 offset 丢弃该事件（依赖客户端 seq 拉取兜底）",
					logger.String("event_type", eventType),
					logger.ErrorField("error", handleErr),
				)
			}
		}
		if err := reader.CommitMessages(ctx, msg); err != nil {
			metrics.KafkaCommitErrors.Inc()
			logger.Warn(ctx, "message-push 提交 Kafka offset 失败",
				logger.ErrorField("error", err),
			)
		}
	}
}

// runHandleWithRetry 对可重试错误做有限次退避重试；永久错误或成功立即返回。
func (c *Consumer) runHandleWithRetry(ctx context.Context, payload []byte) error {
	var lastErr error
	for attempt := 0; attempt < handleMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			metrics.HandleRetries.WithLabelValues("failed").Observe(float64(attempt))
			return err
		}
		// 每次尝试套一层超时预算，防止单条消息（尤其大群扇出）长时间占住分区。
		// 仅约束本次尝试；外层 ctx 仍用于关停判断与退避，二者互不干扰。
		attemptCtx, cancel := context.WithTimeout(ctx, handleAttemptTimeout)
		err := c.handler.Handle(attemptCtx, payload)
		cancel()
		if err == nil {
			metrics.HandleRetries.WithLabelValues("success").Observe(float64(attempt))
			return nil
		}
		if !errors.Is(err, errRetriable) {
			metrics.HandleRetries.WithLabelValues("success").Observe(float64(attempt))
			return nil
		}
		lastErr = err
		if attempt == handleMaxAttempts-1 {
			break
		}
		backoff := handleBackoffs[attempt]
		logger.Warn(ctx, "message-push 可重试错误，等待后重试",
			logger.Int("attempt", attempt+1),
			logger.Int("max_attempts", handleMaxAttempts),
			logger.Any("backoff", backoff.String()),
			logger.ErrorField("error", err),
		)
		select {
		case <-ctx.Done():
			metrics.HandleRetries.WithLabelValues("failed").Observe(float64(attempt + 1))
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
	metrics.HandleRetries.WithLabelValues("failed").Observe(float64(handleMaxAttempts))
	return lastErr
}

// eventTypeForMetric 在丢弃指标/告警处尽力解析事件类型作为标签；解析失败回退 "unknown"。
// 进入丢弃分支的消息必然已在 Handle 内成功解码过（解码失败属永久错误，返回 nil 不会重试/丢弃），
// 因此这里的二次解码几乎总能成功，且只在稀有的丢弃路径触发，成本可忽略。
func eventTypeForMetric(value []byte) string {
	event, err := msgevent.DecodeMsgPush(value)
	if err != nil || event.Type == "" {
		return "unknown"
	}
	return event.Type
}
