package consumer

import (
	"context"
	"fmt"
	"time"

	"github.com/013677890/LCchat-Backend/apps/user/internal/service"
	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/pkg/accountevent"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	"gorm.io/gorm"
)

const userCreatedIdempotentEventType = accountevent.EventTypeUserCreated + ":user-service"

// UserCreatedConsumer 负责消费注册完成事件并在 user 域确认资料初始化闭环。
type UserCreatedConsumer struct {
	consumer           *kafka.Consumer
	internalProfileSvc service.InternalProfileService
	db                 *gorm.DB
}

// NewUserCreatedConsumer 创建 user-service 的 user_created 事件消费者。
func NewUserCreatedConsumer(
	brokers []string,
	topic, groupID string,
	internalProfileSvc service.InternalProfileService,
	db *gorm.DB,
) *UserCreatedConsumer {
	return &UserCreatedConsumer{
		consumer: kafka.NewManualCommitConsumer(brokers, topic, groupID, kafka.ManualConsumerConfig{
			MinBytes:     1,
			MaxBytes:     10 << 20,
			MaxWait:      500 * time.Millisecond,
			ErrorBackoff: time.Second,
		}),
		internalProfileSvc: internalProfileSvc,
		db:                 db,
	}
}

// Start 启动 user_created 事件消费循环。
func (c *UserCreatedConsumer) Start(ctx context.Context) error {
	logger.Info(ctx, "User user_created 消费者启动中")
	return c.consumer.Start(ctx, c.handle)
}

// Close 关闭消费者。
func (c *UserCreatedConsumer) Close() error {
	if c.consumer == nil {
		return nil
	}
	return c.consumer.Close()
}

func (c *UserCreatedConsumer) handle(ctx context.Context, message []byte) error {
	payload, err := accountevent.DecodeUserCreated(message)
	if err != nil {
		logger.Warn(ctx, "解析 user_created 事件失败，按不可重试消息跳过", logger.ErrorField("error", err))
		return nil
	}

	processed, err := outbox.CheckIdempotent(c.db, userCreatedIdempotentEventType, payload.EventID)
	if err != nil {
		return fmt.Errorf("检查 user_created 幂等记录失败: %w", err)
	}
	if processed {
		return nil
	}

	if _, err := c.internalProfileSvc.CreateProfile(ctx, &userpb.CreateProfileRequest{
		UserUuid: payload.UserUUID,
		Nickname: payload.Nickname,
		Avatar:   payload.Avatar,
	}); err != nil {
		return fmt.Errorf("执行 CreateProfile 失败: %w", err)
	}
	if err := outbox.MarkIdempotent(c.db, userCreatedIdempotentEventType, payload.EventID); err != nil {
		return fmt.Errorf("写入 user_created 幂等记录失败: %w", err)
	}
	return nil
}
