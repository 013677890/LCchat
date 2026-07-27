package consumer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/013677890/LCchat-Backend/apps/message-push/internal/metrics"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/msgevent"
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
	// 该预算用于给「消息内有界并发扇出」的总时长封顶，避免一条大群消息长时间占住分区。
	// 取值偏宽：正常群扩散远达不到，仅作为病态扇出的安全上限。
	handleAttemptTimeout = 30 * time.Second
)

var handleBackoffs = []time.Duration{
	100 * time.Millisecond,
	300 * time.Millisecond,
	800 * time.Millisecond,
}

// Handler 定义 Kafka 消息处理器的最小能力。
type Handler interface {
	Handle(ctx context.Context, value []byte) error
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
