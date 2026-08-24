package consumer

import (
	"context"
	"fmt"
	"time"

	"github.com/013677890/LCchat-Backend/apps/user/internal/repository"
	"github.com/013677890/LCchat-Backend/pkg/event"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	"gorm.io/gorm"
)

const userAccountDeletedIdempotentEventType = event.EventTypeAccountDeleted + ":user-service"

// AccountDeletedConsumer 消费 account.deleted 并执行 user-service 侧清理。
// 使用 ManualConsumerPool；与 auth/relation 的同名 topic 使用不同 consumer group，互不抢消息。
type AccountDeletedConsumer struct {
	pool     *kafka.ManualConsumerPool
	userRepo repository.IUserRepository
	db       *gorm.DB
}

// NewAccountDeletedConsumer 创建 user-service 的 account.deleted 消费 Pool。
// workers 非法或 Pool 参数不完整时直接返回错误，禁止回退为单 Reader。
func NewAccountDeletedConsumer(brokers []string, topic, groupID string, workers int, userRepo repository.IUserRepository, db *gorm.DB) (*AccountDeletedConsumer, error) {
	pool, err := kafka.NewManualConsumerPool(kafka.ManualConsumerPoolConfig{
		Name:    "user-account-deleted",
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
		Workers: workers,
		Config: kafka.ManualConsumerConfig{
			MinBytes:       1,
			MaxBytes:       10 << 20,
			MaxWait:        500 * time.Millisecond,
			ErrorBackoff:   time.Second,
			DeadLetterSink: outbox.NewDeadLetterSink(db, "user-service:account.deleted"),
		},
	})
	if err != nil {
		return nil, err
	}
	return &AccountDeletedConsumer{
		pool:     pool,
		userRepo: userRepo,
		db:       db,
	}, nil
}

// Start 启动 account.deleted 消费循环；致命错误由 UserApp 的 RunIsolatedPool 隔离。
func (c *AccountDeletedConsumer) Start(ctx context.Context) error {
	logger.Info(ctx, "User account.deleted 消费者启动中")
	return c.pool.Start(ctx, c.handle)
}

// Close 关闭 Pool 内全部 Reader；应在取消 Start 的 ctx 之后调用。
func (c *AccountDeletedConsumer) Close() error {
	if c == nil || c.pool == nil {
		return nil
	}
	return c.pool.Close()
}

// handle 解码并幂等清理 user 域账号资料。
// 契约错误进入 dead_events；数据库错误可重试；成功后才推进 offset。
func (c *AccountDeletedConsumer) handle(ctx context.Context, message []byte) error {
	payload, err := event.DecodeAccountDeleted(message)
	if err != nil {
		return kafka.Permanent(fmt.Errorf("解析 account.deleted 事件失败: %w", err))
	}

	processed, err := outbox.CheckIdempotent(c.db, userAccountDeletedIdempotentEventType, payload.EventID)
	if err != nil {
		return fmt.Errorf("检查 account.deleted 幂等记录失败: %w", err)
	}
	if processed {
		return nil
	}

	// 资料域收到账号注销事件后，直接删除 user_profile 权威记录并清理缓存。
	if err := c.userRepo.Delete(ctx, payload.UserUUID); err != nil {
		return fmt.Errorf("执行 user-service 资料清理失败: %w", err)
	}
	if err := outbox.MarkIdempotent(c.db, userAccountDeletedIdempotentEventType, payload.EventID); err != nil {
		return fmt.Errorf("写入 account.deleted 幂等记录失败: %w", err)
	}
	return nil
}
