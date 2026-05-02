package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/013677890/LCchat-Backend/apps/user/internal/repository"
	"github.com/013677890/LCchat-Backend/pkg/accountevent"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	"gorm.io/gorm"
)

const userAccountDeletedIdempotentEventType = accountevent.EventTypeAccountDeleted + ":user-service"

// AccountDeletedConsumer 负责消费 account.deleted 事件并执行 user-service 侧清理。
type AccountDeletedConsumer struct {
	consumer *kafka.Consumer
	userRepo repository.IUserRepository
	db       *gorm.DB
}

// NewAccountDeletedConsumer 创建 user-service 的 account.deleted 消费者。
func NewAccountDeletedConsumer(brokers []string, topic, groupID string, userRepo repository.IUserRepository, db *gorm.DB) *AccountDeletedConsumer {
	return &AccountDeletedConsumer{
		consumer: kafka.NewManualCommitConsumer(brokers, topic, groupID, kafka.ManualConsumerConfig{
			MinBytes:     1,
			MaxBytes:     10 << 20,
			MaxWait:      500 * time.Millisecond,
			ErrorBackoff: time.Second,
		}),
		userRepo: userRepo,
		db:       db,
	}
}

// Start 启动 user-service 的 account.deleted 消费循环。
func (c *AccountDeletedConsumer) Start(ctx context.Context) error {
	logger.Info(ctx, "User account.deleted 消费者启动中")
	return c.consumer.Start(ctx, c.handle)
}

// Close 关闭消费者。
func (c *AccountDeletedConsumer) Close() error {
	if c.consumer == nil {
		return nil
	}
	return c.consumer.Close()
}

func (c *AccountDeletedConsumer) handle(ctx context.Context, message []byte) error {
	var payload accountevent.AccountDeletedPayload
	if err := json.Unmarshal(message, &payload); err != nil {
		logger.Warn(ctx, "解析 account.deleted 事件失败，按不可重试消息跳过", logger.ErrorField("error", err))
		return nil
	}
	if payload.UserUUID == "" {
		logger.Warn(ctx, "account.deleted 事件缺少 user_uuid，按不可重试消息跳过")
		return nil
	}

	processed, err := outbox.CheckIdempotent(c.db, userAccountDeletedIdempotentEventType, payload.UserUUID)
	if err != nil {
		return fmt.Errorf("检查 account.deleted 幂等记录失败: %w", err)
	}
	if processed {
		return nil
	}

	// 迁移阶段 user_info 仍是共享表，这里继续做幂等删除与缓存清理，避免残留资料缓存。
	if err := c.userRepo.Delete(ctx, payload.UserUUID); err != nil {
		return fmt.Errorf("执行 user-service 资料清理失败: %w", err)
	}
	if err := outbox.MarkIdempotent(c.db, userAccountDeletedIdempotentEventType, payload.UserUUID); err != nil {
		return fmt.Errorf("写入 account.deleted 幂等记录失败: %w", err)
	}
	return nil
}
