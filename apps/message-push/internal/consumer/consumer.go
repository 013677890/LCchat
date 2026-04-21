package consumer

import (
	"context"
	"errors"
	"fmt"
	"time"

	connectpb "github.com/013677890/LCchat-Backend/apps/connect/pb"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/connectcli"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/metrics"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/route"
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/pkg/logger"
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
)

var handleBackoffs = []time.Duration{
	100 * time.Millisecond,
	300 * time.Millisecond,
	800 * time.Millisecond,
}

// EventHandler 处理 Kafka 中的 MsgPushEvent。
type EventHandler struct {
	routes *route.RedisRepository
	sender *connectcli.Sender
}

// NewEventHandler 创建事件处理器。
func NewEventHandler(routes *route.RedisRepository, sender *connectcli.Sender) *EventHandler {
	return &EventHandler{routes: routes, sender: sender}
}

// Handle 处理单条 Kafka 事件。
// 返回 errRetriable 包装的错误表示调用方应重试；其它错误或 nil 表示无需重试。
func (h *EventHandler) Handle(ctx context.Context, value []byte) error {
	start := time.Now()
	result := "success"
	defer func() {
		metrics.ObserveHandleDuration(start, result)
	}()

	var event msgpb.MsgPushEvent
	if err := proto.Unmarshal(value, &event); err != nil {
		result = "permanent_error"
		metrics.EventTypeSkipped.WithLabelValues("unknown", "decode_error").Inc()
		// proto 结构异常属于永久错误，重试无意义。直接记录并放行。
		logger.Warn(ctx, "message-push 反序列化 MsgPushEvent 失败，跳过该消息",
			logger.ErrorField("error", err),
			logger.Int("payload_bytes", len(value)),
		)
		return nil
	}

	// 第一阶段只接收真实消息下行事件。
	// 召回、已读等其他事件类型暂时由上游写入，但在本服务中先跳过。
	if event.Type != "MSG_PUSH" {
		result = "permanent_error"
		metrics.EventTypeSkipped.WithLabelValues(event.Type, "unsupported_type").Inc()
		logger.Warn(ctx, "message-push 暂未处理该事件类型，先跳过",
			logger.String("event_type", event.Type),
		)
		return nil
	}
	// 当前实现仅支持单聊扩散。
	// 群聊需要先做成员展开，再对每个成员查路由并逐个推送。
	if event.ConvType != msgpb.ConvType_CONV_TYPE_P2P {
		result = "permanent_error"
		metrics.EventTypeSkipped.WithLabelValues(event.Type, "unsupported_conv_type").Inc()
		logger.Warn(ctx, "message-push 第一阶段仅支持 P2P，先跳过",
			logger.String("event_type", event.Type),
			logger.Int("conv_type", int(event.ConvType)),
		)
		return nil
	}
	if event.ReceiverUuid == "" {
		result = "permanent_error"
		metrics.EventTypeSkipped.WithLabelValues(event.Type, "validation_error").Inc()
		// 字段校验失败属于永久错误，避免回推重试。
		logger.Warn(ctx, "message-push receiver_uuid 为空，跳过该消息",
			logger.String("trace_id", event.TraceId),
		)
		return nil
	}

	// 先从 Redis 读取接收方在线设备路由。
	// 查不到路由时按离线处理，不视为消费失败。
	routes, err := h.routes.ListUserRoutes(ctx, event.ReceiverUuid)
	if err != nil {
		result = "retriable_error"
		metrics.RouteHitRate.WithLabelValues("error").Inc()
		// Redis 读失败通常是瞬时问题，交由上层短重试。
		return fmt.Errorf("%w: 读取用户路由失败: %v", errRetriable, err)
	}
	if len(routes) == 0 {
		metrics.RouteHitRate.WithLabelValues("miss").Inc()
		logger.Warn(ctx, "message-push 未找到接收方在线路由，按离线处理",
			logger.String("receiver_uuid", event.ReceiverUuid),
		)
		return nil
	}
	metrics.RouteHitRate.WithLabelValues("hit").Inc()

	// 优先使用事件顶层 seq。
	// 这是下游 MessageEnvelope.seq 的直接来源；若历史事件未带该字段，则退回到 MsgItem.seq。
	seq := event.Seq
	if seq == 0 {
		var item msgpb.MsgItem
		if unmarshalErr := proto.Unmarshal(event.Data, &item); unmarshalErr == nil {
			seq = item.Seq
		}
	}

	// 组装发往 connect 的统一信封。
	// connect 只关心投递协议，不关心上游 MsgPushEvent 的完整结构。
	envelope := &connectpb.MessageEnvelope{
		Type:        event.Type,
		Data:        event.Data,
		Seq:         seq,
		ServerTs:    event.ServerTs,
		TraceId:     event.TraceId,
		AckRequired: false,
	}

	// 按设备逐个推送，使用 PushToDevice 确保协议层幂等。
	// 单设备失败不影响其它设备，尽量提升整体投递成功率。
	var (
		delivered      int32
		succeededCount int
		failedCount    int
	)
	for _, deviceRoute := range routes {
		pushStart := time.Now()
		pushErr := h.sender.PushToDevice(ctx, deviceRoute.ConnectGRPCAddr, event.ReceiverUuid, deviceRoute.DeviceID, envelope)
		if pushErr != nil {
			failedCount++
			metrics.PushToDeviceTotal.WithLabelValues("error").Inc()
			metrics.ObservePushToDeviceDuration(pushStart, "error")
			logger.Warn(ctx, "message-push 调用 connect PushToDevice 失败",
				logger.String("receiver_uuid", event.ReceiverUuid),
				logger.String("device_id", deviceRoute.DeviceID),
				logger.String("connect_addr", deviceRoute.ConnectGRPCAddr),
				logger.ErrorField("error", pushErr),
			)
			continue
		}
		succeededCount++
		delivered++
		metrics.PushToDeviceTotal.WithLabelValues("success").Inc()
		metrics.ObservePushToDeviceDuration(pushStart, "success")
	}
	metrics.DeliveredDevices.Observe(float64(delivered))

	logger.Info(ctx, "message-push 处理完成",
		logger.String("receiver_uuid", event.ReceiverUuid),
		logger.Int64("seq", seq),
		logger.Int64("server_ts", event.ServerTs),
		logger.Int("route_count", len(routes)),
		logger.Int("succeeded_count", succeededCount),
		logger.Int("failed_count", failedCount),
		logger.Int32("delivered_count", delivered),
	)

	// 全部设备都失败才触发重试；部分成功视为整体成功，避免重试导致已送达设备收到重复消息。
	if succeededCount == 0 && failedCount > 0 {
		result = "retriable_error"
		return fmt.Errorf("%w: 所有设备推送均失败 (%d)", errRetriable, failedCount)
	}
	return nil
}

// Consumer 自研 msg.push 消费循环。
type Consumer struct {
	brokers []string
	topic   string
	groupID string
	handler *EventHandler
}

// NewConsumer 创建消费者。
func NewConsumer(brokers []string, topic, groupID string, handler *EventHandler) *Consumer {
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
			// 拉取失败大多是短暂网络抖动或 broker 瞬时不可用。
			// 这里做一个很短的退避，避免空转打满 CPU。
			time.Sleep(200 * time.Millisecond)
			continue
		}

		// 处理单条消息：
		// - nil / 永久错误：立即 commit，向前推进 offset。
		// - errRetriable：做有限次本地退避重试，仍失败则告警后 commit（避免单条消息卡死消费链路）。
		handleErr := c.runHandleWithRetry(ctx, msg.Value)
		if handleErr != nil {
			logger.Warn(ctx, "message-push 本地重试仍失败，按阶段一策略提交 offset",
				logger.ErrorField("error", handleErr),
			)
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
// 返回最后一次仍失败的错误，成功时返回 nil。
func (c *Consumer) runHandleWithRetry(ctx context.Context, payload []byte) error {
	var lastErr error
	for attempt := 0; attempt < handleMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			metrics.HandleRetries.WithLabelValues("failed").Observe(float64(attempt))
			return err
		}
		err := c.handler.Handle(ctx, payload)
		if err == nil {
			metrics.HandleRetries.WithLabelValues("success").Observe(float64(attempt))
			return nil
		}
		if !errors.Is(err, errRetriable) {
			// 永久错误：交由 Handle 内的 Warn 已记录，不再重试。
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
