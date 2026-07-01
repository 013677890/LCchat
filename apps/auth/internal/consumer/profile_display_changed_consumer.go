package consumer

import (
	"context"
	"fmt"
	"time"

	"github.com/013677890/LCchat-Backend/apps/auth/internal/service"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/pkg/accountevent"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	"gorm.io/gorm"
)

const authProfileDisplayChangedIdempotentEventType = accountevent.EventTypeProfileDisplayChanged + ":auth-service"

// ProfileDisplayChangedConsumer 负责消费资料展示字段变更事件并回写 auth 域登录展示字段。
type ProfileDisplayChangedConsumer struct {
	consumer        *kafka.Consumer
	internalAuthSvc service.InternalAuthService
	db              *gorm.DB
}

// NewProfileDisplayChangedConsumer 创建 auth-service 的资料展示事件消费者。
func NewProfileDisplayChangedConsumer(
	brokers []string,
	topic, groupID string,
	internalAuthSvc service.InternalAuthService,
	db *gorm.DB,
) *ProfileDisplayChangedConsumer {
	return &ProfileDisplayChangedConsumer{
		consumer: kafka.NewManualCommitConsumer(brokers, topic, groupID, kafka.ManualConsumerConfig{
			MinBytes:     1,
			MaxBytes:     10 << 20,
			MaxWait:      500 * time.Millisecond,
			ErrorBackoff:   time.Second,
			DeadLetterSink: outbox.NewDeadLetterSink(db, "auth-service:profile_display_changed"),
		}),
		internalAuthSvc: internalAuthSvc,
		db:              db,
	}
}

// Start 启动资料展示字段变更消费循环。
func (c *ProfileDisplayChangedConsumer) Start(ctx context.Context) error {
	logger.Info(ctx, "Auth profile_display_changed 消费者启动中")
	return c.consumer.Start(ctx, c.handle)
}

// Close 关闭消费者。
func (c *ProfileDisplayChangedConsumer) Close() error {
	if c.consumer == nil {
		return nil
	}
	return c.consumer.Close()
}

func (c *ProfileDisplayChangedConsumer) handle(ctx context.Context, message []byte) error {
	payload, err := accountevent.DecodeProfileDisplayChanged(message)
	if err != nil {
		logger.Warn(ctx, "解析 profile_display_changed 事件失败，按不可重试消息跳过", logger.ErrorField("error", err))
		return nil
	}

	processed, err := outbox.CheckIdempotent(c.db, authProfileDisplayChangedIdempotentEventType, payload.EventID)
	if err != nil {
		return fmt.Errorf("检查 profile_display_changed 幂等记录失败: %w", err)
	}
	if processed {
		return nil
	}

	if _, err := c.internalAuthSvc.UpdateLoginDisplay(ctx, &authpb.UpdateLoginDisplayRequest{
		UserUuid: payload.UserUUID,
		Nickname: payload.Nickname,
		Avatar:   payload.Avatar,
	}); err != nil {
		return fmt.Errorf("回写 auth 登录展示字段失败: %w", err)
	}
	if err := outbox.MarkIdempotent(c.db, authProfileDisplayChangedIdempotentEventType, payload.EventID); err != nil {
		return fmt.Errorf("写入 profile_display_changed 幂等记录失败: %w", err)
	}
	return nil
}
