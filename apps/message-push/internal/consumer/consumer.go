package consumer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/013677890/LCchat-Backend/apps/message-push/internal/metrics"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/msgevent"
	"github.com/013677890/LCchat-Backend/pkg/realtimepush"
)

// errRetriable 表示当前消息处理失败属于可重试范畴（Redis 抖动、connect 瞬时不可达等）。
// 业务 Handler 应 errors.Is 判断；适配层会做有限次重试，耗尽后仍返回 nil 以放行 offset。
// 永久性错误（proto 反序列化失败、字段校验失败）不应包装为本错误，而应由 Handler 记日志后返回 nil。
var errRetriable = errors.New("message-push: retriable handle error")

// 单条消息的本地重试上限与退避梯度。
// message-push 下行是 best-effort：只做「就地短重试」，不写 dead_events；
// 超出后告警并提交 offset 让分区前进，依赖客户端按会话 seq 补拉自愈。
// 这与 group.cache 投影/领域事件的「死信优先」语义刻意不同。
const (
	handleMaxAttempts = 3

	// handleAttemptTimeout 为单次业务处理尝试的超时预算。
	// message-push 无 DB；Redis/gRPC 已各自有界，这里主要封顶「大群节点扇出」总时长，
	// 避免单条消息长时间占住分区。正常群扩散远达不到 30s，仅作病态安全上限。
	handleAttemptTimeout = 30 * time.Second
)

// handleBackoffs 与 handleMaxAttempts 对齐：第 1/2 次失败后的等待，最后一次失败不再等待。
var handleBackoffs = []time.Duration{
	100 * time.Millisecond,
	300 * time.Millisecond,
	800 * time.Millisecond,
}

// Handler 定义 message-push 业务处理器的最小能力（EventHandler / RealtimeHandler）。
// 错误约定见 errRetriable 与 runHandleWithRetry。
type Handler interface {
	Handle(ctx context.Context, value []byte) error
}

// Consumer 用统一 ManualConsumerPool 编排 message-push 的独立 Reader。
//
// 并行：workers 个 Reader 同 group，Kafka 自动分 partition；同会话 key 仍有序。
// 提交：公共 Consumer 仅在 handle 返回 nil 时 commit；本类型的 handle 适配层
// 在 best-effort 场景下会把「业务失败耗尽」也映射为 nil，从而放行 offset。
type Consumer struct {
	pool    *kafka.ManualConsumerPool
	handler Handler
}

// NewConsumer 创建 message-push 的消费 Pool。
// workers 必须在 [1, 64]，非法值直接返回错误，禁止静默改写容量。
// 业务有限重试、丢弃指标仍由本包 handle 适配层负责，不依赖公共死信。
func NewConsumer(brokers []string, topic, groupID string, workers int, handler Handler) (*Consumer, error) {
	if handler == nil {
		return nil, fmt.Errorf("message-push 事件处理器未初始化")
	}
	pool, err := kafka.NewManualConsumerPool(kafka.ManualConsumerPoolConfig{
		Name:    "message-push-" + topic,
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
		Workers: workers,
		Config: kafka.ManualConsumerConfig{
			MinBytes:     1,
			MaxBytes:     10 << 20,
			MaxWait:      500 * time.Millisecond,
			// Fetch 临时失败退避略短于默认 500ms，优先降低下行空窗；仍在循环内完成，不退进程。
			ErrorBackoff: 200 * time.Millisecond,
			// 业务每次尝试已有 handleAttemptTimeout；若再套 Pool 默认 10s，
			// 大群扇出会在业务重试策略完成前被取消，因此显式关闭 Pool 层 HandleTimeout。
			HandleTimeout: -1,
			// 不配置 DeadLetterSink：best-effort 丢弃由 handle 返回 nil 表达，不进 dead_events。
			ObserveFetch: func(duration time.Duration) {
				metrics.KafkaFetchDuration.Observe(duration.Seconds())
			},
			ObserveCommitError: func() {
				metrics.KafkaCommitErrors.Inc()
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return &Consumer{pool: pool, handler: handler}, nil
}

// Start 启动全部 Reader 并阻塞到 ctx 取消或 Pool 致命失败。
// 任一 worker 致命退出时 Pool 会取消兄弟并把错误返回给 MessagePushApp；
// Kafka 临时错误由公共 Consumer 退避重试，不因 broker 短暂抖动退出进程。
func (c *Consumer) Start(ctx context.Context) error {
	if c == nil || c.pool == nil || c.handler == nil {
		return fmt.Errorf("message-push consumer 未初始化")
	}
	return c.pool.Start(ctx, c.handle)
}

// Close 关闭 Pool 内全部 Reader，并聚合关闭错误。
// 可在 Start 未调用、Run 返回后或关停路径上安全调用；应在 cancel 消费 ctx 之后调用。
func (c *Consumer) Close() error {
	if c == nil || c.pool == nil {
		return nil
	}
	return c.pool.Close()
}

// handle 把 message-push 的 best-effort 语义适配成公共 Consumer 的「nil 才提交」。
//
// 映射规则：
//   - 业务成功 / 永久错误已由 Handler 内部跳过 → 返回 nil → commit；
//   - 可重试错误在本地预算内重试；耗尽后记丢弃指标并返回 nil → commit（放行分区）；
//   - ctx 取消 → 返回 cancel 错误 → 不 commit，关停后从上次位点继续。
//
// 丢弃不是静默吞掉：必须打 Warn + MessagesDroppedAfterRetry，依赖客户端补拉。
func (c *Consumer) handle(ctx context.Context, payload []byte) error {
	handleErr := c.runHandleWithRetry(ctx, payload)
	if handleErr == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}

	eventType := eventTypeForMetric(payload)
	metrics.MessagesDroppedAfterRetry.WithLabelValues(eventType).Inc()
	logger.Warn(ctx, "message-push 本地重试仍失败，提交 offset 丢弃该事件（依赖客户端 seq 拉取兜底）",
		logger.String("event_type", eventType),
		logger.ErrorField("error", handleErr),
	)
	// 返回 nil：允许公共层 commit。这是产品选择的 best-effort，不是解码成功。
	return nil
}

// runHandleWithRetry 对可重试错误做有限次退避重试。
// 成功或「业务判定的永久错误」（非 errRetriable 且非尝试超时）立即返回 nil 语义由上层解释；
// 可重试耗尽返回 lastErr，由 handle 决定是否丢弃提交。
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
		if ctxErr := ctx.Err(); ctxErr != nil {
			metrics.HandleRetries.WithLabelValues("failed").Observe(float64(attempt))
			return ctxErr
		}
		if err == nil {
			metrics.HandleRetries.WithLabelValues("success").Observe(float64(attempt))
			return nil
		}
		// 单次尝试超时属于瞬时失败，应计入有限重试；只有业务 Handler 明确
		// 判定的永久错误才提交跳过，避免一次病态扇出被误当成成功。
		if !errors.Is(err, errRetriable) && !errors.Is(err, context.DeadlineExceeded) {
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

// eventTypeForMetric 在丢弃指标/告警处依次识别 msg.push 与 realtime.push 的事件类型。
// 二次解码只发生在稀有的重试耗尽路径；两种当前契约都不匹配时回退 unknown，避免无界标签。
func eventTypeForMetric(value []byte) string {
	event, err := msgevent.DecodeMsgPush(value)
	if err == nil && event.Type != "" {
		return event.Type
	}
	realtimeEvent, err := realtimepush.Decode(value)
	if err == nil && realtimeEvent.Type != "" {
		return realtimeEvent.Type
	}
	return "unknown"
}
