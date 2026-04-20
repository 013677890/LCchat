package consumer

import (
	"context"
	"fmt"
	"time"

	connectpb "github.com/013677890/LCchat-Backend/apps/connect/pb"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/connectcli"
	"github.com/013677890/LCchat-Backend/apps/message-push/internal/route"
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"google.golang.org/protobuf/proto"
)

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
func (h *EventHandler) Handle(ctx context.Context, value []byte) error {
	var event msgpb.MsgPushEvent
	if err := proto.Unmarshal(value, &event); err != nil {
		return fmt.Errorf("反序列化 MsgPushEvent 失败: %w", err)
	}

	if event.Type != "MSG_PUSH" {
		logger.Warn(ctx, "message-push 暂未处理该事件类型，先跳过",
			logger.String("event_type", event.Type),
		)
		return nil
	}
	if event.ConvType != msgpb.ConvType_CONV_TYPE_P2P {
		logger.Warn(ctx, "message-push 第一阶段仅支持 P2P，先跳过",
			logger.String("event_type", event.Type),
			logger.Int("conv_type", int(event.ConvType)),
		)
		return nil
	}
	if event.ReceiverUuid == "" {
		return fmt.Errorf("receiver_uuid 不能为空")
	}

	routes, err := h.routes.ListUserRoutes(ctx, event.ReceiverUuid)
	if err != nil {
		return err
	}
	if len(routes) == 0 {
		logger.Warn(ctx, "message-push 未找到接收方在线路由，按离线处理",
			logger.String("receiver_uuid", event.ReceiverUuid),
		)
		return nil
	}

	seq := event.Seq
	if seq == 0 {
		var item msgpb.MsgItem
		if err := proto.Unmarshal(event.Data, &item); err == nil {
			seq = item.Seq
		}
	}

	envelope := &connectpb.MessageEnvelope{
		Type:        event.Type,
		Data:        event.Data,
		Seq:         seq,
		ServerTs:    event.ServerTs,
		TraceId:     event.TraceId,
		AckRequired: false,
	}

	delivered := int32(0)
	for _, item := range routes {
		count, pushErr := h.sender.PushToUser(ctx, item.ConnectGRPCAddr, event.ReceiverUuid, envelope)
		if pushErr != nil {
			logger.Warn(ctx, "message-push 调用 connect 失败",
				logger.String("receiver_uuid", event.ReceiverUuid),
				logger.String("connect_addr", item.ConnectGRPCAddr),
				logger.ErrorField("error", pushErr),
			)
			continue
		}
		delivered += count
	}

	logger.Info(ctx, "message-push 处理完成",
		logger.String("receiver_uuid", event.ReceiverUuid),
		logger.Int64("seq", seq),
		logger.Int64("server_ts", event.ServerTs),
		logger.Int("route_count", len(routes)),
		logger.Int32("delivered_count", delivered),
	)
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

		msg, err := reader.FetchMessage(ctx)
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

		if err := c.handler.Handle(ctx, msg.Value); err != nil {
			logger.Warn(ctx, "message-push 处理 Kafka 消息失败，按阶段一策略提交 offset",
				logger.ErrorField("error", err),
			)
		}
		if err := reader.CommitMessages(ctx, msg); err != nil {
			logger.Warn(ctx, "message-push 提交 Kafka offset 失败",
				logger.ErrorField("error", err),
			)
		}
	}
}
