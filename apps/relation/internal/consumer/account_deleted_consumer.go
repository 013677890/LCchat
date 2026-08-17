package consumer

import (
	"context"
	"fmt"
	"time"

	"github.com/013677890/LCchat-Backend/apps/relation/internal/repository"
	"github.com/013677890/LCchat-Backend/pkg/accountevent"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	"gorm.io/gorm"
)

const relationAccountDeletedIdempotentEventType = accountevent.EventTypeAccountDeleted + ":relation-service"

// AccountDeletedConsumer 消费 account.deleted 并执行 relation-service 侧关系清理。
// 使用 ManualConsumerPool；group 与其它服务隔离，由 RelationApp 经 RunIsolatedPool 旁路运行。
type AccountDeletedConsumer struct {
	pool       *kafka.ManualConsumerPool
	friendRepo repository.IFriendRepository
	applyRepo  repository.IApplyRepository
	db         *gorm.DB
}

// NewAccountDeletedConsumer 创建 relation-service 的 account.deleted 消费 Pool。
// workers 非法或 Pool 参数不完整时直接返回错误，禁止回退为单 Reader。
func NewAccountDeletedConsumer(
	brokers []string,
	topic, groupID string,
	workers int,
	friendRepo repository.IFriendRepository,
	applyRepo repository.IApplyRepository,
	db *gorm.DB,
) (*AccountDeletedConsumer, error) {
	pool, err := kafka.NewManualConsumerPool(kafka.ManualConsumerPoolConfig{
		Name:    "relation-account-deleted",
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
		Workers: workers,
		Config: kafka.ManualConsumerConfig{
			MinBytes:       1,
			MaxBytes:       10 << 20,
			MaxWait:        500 * time.Millisecond,
			ErrorBackoff:   time.Second,
			DeadLetterSink: outbox.NewDeadLetterSink(db, "relation-service:account.deleted"),
		},
	})
	if err != nil {
		return nil, err
	}
	return &AccountDeletedConsumer{
		pool:       pool,
		friendRepo: friendRepo,
		applyRepo:  applyRepo,
		db:         db,
	}, nil
}

// Start 启动 account.deleted 消费循环；致命错误由 RelationApp 的 RunIsolatedPool 隔离。
func (c *AccountDeletedConsumer) Start(ctx context.Context) error {
	logger.Info(ctx, "Relation account.deleted 消费者启动中")
	return c.pool.Start(ctx, c.handle)
}

// Close 关闭 Pool 内全部 Reader；应在取消 Start 的 ctx 之后调用。
func (c *AccountDeletedConsumer) Close() error {
	if c == nil || c.pool == nil {
		return nil
	}
	return c.pool.Close()
}

// handle 解码并幂等清理 relation 域账号关系。
// 契约错误进入 dead_events；数据库错误可重试；成功后才推进 offset。
func (c *AccountDeletedConsumer) handle(ctx context.Context, message []byte) error {
	payload, err := accountevent.DecodeAccountDeleted(message)
	if err != nil {
		return kafka.Permanent(fmt.Errorf("解析 relation account.deleted 事件失败: %w", err))
	}

	processed, err := outbox.CheckIdempotent(c.db, relationAccountDeletedIdempotentEventType, payload.EventID)
	if err != nil {
		return fmt.Errorf("检查 relation account.deleted 幂等记录失败: %w", err)
	}
	if processed {
		return nil
	}

	// relation 侧收到账号注销事件后，统一清理 user_relations / apply_requests 中的残留数据。
	if err := c.friendRepo.CleanupAccountRelations(ctx, payload.UserUUID); err != nil {
		return fmt.Errorf("清理 relation 用户关系失败: %w", err)
	}
	if err := c.applyRepo.CleanupAccountApplies(ctx, payload.UserUUID); err != nil {
		return fmt.Errorf("清理 relation 用户申请失败: %w", err)
	}
	if err := outbox.MarkIdempotent(c.db, relationAccountDeletedIdempotentEventType, payload.EventID); err != nil {
		return fmt.Errorf("写入 relation account.deleted 幂等记录失败: %w", err)
	}
	return nil
}
