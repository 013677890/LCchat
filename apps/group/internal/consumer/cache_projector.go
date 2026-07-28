package consumer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/pkg/groupevent"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	"gorm.io/gorm"
)

const groupCacheProjectorIdempotentEventType = groupevent.EventTypeGroupCache + ":group-cache-projector"

// CacheProjector 负责消费 group.cache 事件并把最终事实投影到 Redis。
//
// 消费模型：
//   - 同一 consumer group 下启动 N 个独立 kafka.Reader（N 默认等于 partition 数）；
//   - 每个 Reader 内严格串行 Fetch → handle → Commit；
//   - 不同 partition 由 Kafka 分配给不同 Reader，从而并行投影不同群；
//   - 同群事件因 entity_id=group_uuid 作为 Kafka key，始终落在同一 partition，保持严格有序。
//
// 可靠性约定与单 Reader 时代一致：
//  1. Kafka 手动提交；
//  2. 幂等表去重；
//  3. Redis 可重试错误返回上层，由同一消息重试；
//  4. payload/schema 非法时返回 kafka.Permanent，首轮写入 dead_events 后再提交；
//  5. 禁止用 nil 静默吞掉旧格式消息。
type CacheProjector struct {
	pool          *kafka.ManualConsumerPool
	projectorRepo repository.IGroupCacheProjectorRepository
	db            *gorm.DB
}

// NewCacheProjector 创建 group.cache 分区级并行投影消费者。
//
// workers 为独立 Reader 数量，必须由调用方用 kafka.ParsePoolWorkers 严格解析后传入。
// 构造阶段只完成依赖组装，不主动探测 Kafka / MySQL / Redis 连通性。
func NewCacheProjector(
	brokers []string,
	topic, groupID string,
	workers int,
	projectorRepo repository.IGroupCacheProjectorRepository,
	db *gorm.DB,
) (*CacheProjector, error) {
	pool, err := kafka.NewManualConsumerPool(kafka.ManualConsumerPoolConfig{
		Name:    "group-cache-projector",
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
		Workers: workers,
		Config: kafka.ManualConsumerConfig{
			MinBytes:       1,
			MaxBytes:       10 << 20,
			MaxWait:        500 * time.Millisecond,
			ErrorBackoff:   time.Second,
			DeadLetterSink: outbox.NewDeadLetterSink(db, "group-service:group.cache"),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("创建 group cache projector consumer pool 失败: %w", err)
	}
	return &CacheProjector{
		pool:          pool,
		projectorRepo: projectorRepo,
		db:            db,
	}, nil
}

// Start 启动全部 partition worker；任一 worker 致命退出会取消并等待其余 worker。
func (c *CacheProjector) Start(ctx context.Context) error {
	if c == nil || c.pool == nil {
		return nil
	}
	logger.Info(ctx, "Group cache projector 启动中",
		logger.Int("worker_count", c.pool.WorkerCount()),
	)
	return c.pool.Start(ctx, c.handle)
}

// Close 关闭全部 Reader。
func (c *CacheProjector) Close() error {
	if c == nil || c.pool == nil {
		return nil
	}
	return c.pool.Close()
}

// WorkerCount 返回并行 Reader 数量，供测试与观测使用。
func (c *CacheProjector) WorkerCount() int {
	if c == nil || c.pool == nil {
		return 0
	}
	return c.pool.WorkerCount()
}

// handle 负责消费单条 group.cache 事件。
//
// 处理顺序保持稳定：
//  1. decode payload；
//  2. 检查幂等；
//  3. 投影 Redis；
//  4. 标记幂等完成。
//
// handle 可被多个 worker 并发调用，但同一 group_uuid 的事件不会并发进入
// （Kafka key + partition 内串行保证）。GORM/Redis 客户端线程安全，可共享。
func (c *CacheProjector) handle(ctx context.Context, message []byte) error {
	payload, err := groupevent.DecodeGroupCache(message)
	if err != nil {
		return kafka.Permanent(fmt.Errorf("解析 group.cache 严格事件失败: %w", err))
	}

	processed, err := outbox.CheckIdempotent(c.db, groupCacheProjectorIdempotentEventType, payload.EventID)
	if err != nil {
		return fmt.Errorf("检查 group.cache 幂等记录失败: %w", err)
	}
	if processed {
		return nil
	}

	if err := c.projectorRepo.ApplyGroupCacheEvent(ctx, payload); err != nil {
		if errors.Is(err, repository.ErrInvalidProjectorPayload) {
			return kafka.Permanent(fmt.Errorf("group.cache 事件内容非法: %w", err))
		}
		return fmt.Errorf("投影 group.cache 事件失败: %w", err)
	}

	if err := outbox.MarkIdempotent(c.db, groupCacheProjectorIdempotentEventType, payload.EventID); err != nil {
		return fmt.Errorf("写入 group.cache 幂等记录失败: %w", err)
	}
	return nil
}
