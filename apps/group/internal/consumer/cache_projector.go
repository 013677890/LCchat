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
// 这里沿用 user / relation / auth 的可靠消费模式：
//  1. Kafka 手动提交；
//  2. 幂等表去重；
//  3. Redis 可重试错误返回上层，由同一消息重试；
//  4. payload 非法时记录告警并跳过，避免坏消息永久阻塞分区。
type CacheProjector struct {
	consumer      *kafka.Consumer
	projectorRepo repository.IGroupCacheProjectorRepository
	db            *gorm.DB
}

// NewCacheProjector 创建 group.cache 投影消费者。
//
// 构造阶段只完成依赖组装，不主动探测 Kafka / MySQL / Redis 连通性，
// 这样可以保持 provider 和其他服务消费者的启动风格一致。
func NewCacheProjector(
	brokers []string,
	topic, groupID string,
	projectorRepo repository.IGroupCacheProjectorRepository,
	db *gorm.DB,
) *CacheProjector {
	return &CacheProjector{
		consumer: kafka.NewManualCommitConsumer(brokers, topic, groupID, kafka.ManualConsumerConfig{
			MinBytes:     1,
			MaxBytes:     10 << 20,
			MaxWait:      500 * time.Millisecond,
			ErrorBackoff:   time.Second,
			DeadLetterSink: outbox.NewDeadLetterSink(db, "group-service:group.cache"),
		}),
		projectorRepo: projectorRepo,
		db:            db,
	}
}

// Start 启动 group.cache 消费循环。
func (c *CacheProjector) Start(ctx context.Context) error {
	logger.Info(ctx, "Group cache projector 启动中")
	if c == nil || c.consumer == nil {
		return nil
	}
	return c.consumer.Start(ctx, c.handle)
}

// Close 关闭消费者。
func (c *CacheProjector) Close() error {
	if c == nil || c.consumer == nil {
		return nil
	}
	return c.consumer.Close()
}

// handle 负责消费单条 group.cache 事件。
//
// 处理顺序保持稳定：
//  1. decode payload；
//  2. 检查幂等；
//  3. 投影 Redis；
//  4. 标记幂等完成。
func (c *CacheProjector) handle(ctx context.Context, message []byte) error {
	payload, err := groupevent.DecodeGroupCache(message)
	if err != nil {
		logger.Warn(ctx, "解析 group.cache 事件失败，按不可重试消息跳过", logger.ErrorField("error", err))
		return nil
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
			logger.Warn(ctx, "group.cache 事件内容非法，按不可重试消息跳过", logger.ErrorField("error", err))
			return nil
		}
		return fmt.Errorf("投影 group.cache 事件失败: %w", err)
	}

	if err := outbox.MarkIdempotent(c.db, groupCacheProjectorIdempotentEventType, payload.EventID); err != nil {
		return fmt.Errorf("写入 group.cache 幂等记录失败: %w", err)
	}
	return nil
}
