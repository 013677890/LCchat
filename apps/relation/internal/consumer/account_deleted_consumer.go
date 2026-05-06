package consumer

import (
	"context"
	"encoding/json"
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

// AccountDeletedConsumer 负责消费 account.deleted 事件并执行 relation-service 侧清理。
type AccountDeletedConsumer struct {
	consumer   *kafka.Consumer
	friendRepo repository.IFriendRepository
	applyRepo  repository.IApplyRepository
	db         *gorm.DB
}

// NewAccountDeletedConsumer 创建 relation-service 的 account.deleted 消费者。
func NewAccountDeletedConsumer(
	brokers []string,
	topic, groupID string,
	friendRepo repository.IFriendRepository,
	applyRepo repository.IApplyRepository,
	db *gorm.DB,
) *AccountDeletedConsumer {
	return &AccountDeletedConsumer{
		consumer: kafka.NewManualCommitConsumer(brokers, topic, groupID, kafka.ManualConsumerConfig{
			MinBytes:     1,
			MaxBytes:     10 << 20,
			MaxWait:      500 * time.Millisecond,
			ErrorBackoff: time.Second,
		}),
		friendRepo: friendRepo,
		applyRepo:  applyRepo,
		db:         db,
	}
}

// Start 启动 relation-service 的 account.deleted 消费循环。
func (c *AccountDeletedConsumer) Start(ctx context.Context) error {
	logger.Info(ctx, "Relation account.deleted 消费者启动中")
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
		logger.Warn(ctx, "解析 relation account.deleted 事件失败，按不可重试消息跳过", logger.ErrorField("error", err))
		return nil
	}
	if payload.EventID == "" || payload.UserUUID == "" {
		logger.Warn(ctx, "relation account.deleted 事件缺少 event_id 或 user_uuid，按不可重试消息跳过")
		return nil
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
