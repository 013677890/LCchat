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

// UserCreatedConsumer 消费 user_created，在 user 域完成资料初始化闭环。
// 使用 ManualConsumerPool；由 UserApp 经 RunIsolatedPool 旁路运行，不拖死资料 gRPC。
type UserCreatedConsumer struct {
	pool               *kafka.ManualConsumerPool
	internalProfileSvc service.InternalProfileService
	db                 *gorm.DB
}

// NewUserCreatedConsumer 创建 user_created 事件消费 Pool。
// workers 非法或 Pool 参数不完整时直接返回错误，禁止回退为单 Reader。
func NewUserCreatedConsumer(
	brokers []string,
	topic, groupID string,
	workers int,
	internalProfileSvc service.InternalProfileService,
	db *gorm.DB,
) (*UserCreatedConsumer, error) {
	pool, err := kafka.NewManualConsumerPool(kafka.ManualConsumerPoolConfig{
		Name:    "user-created",
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
		Workers: workers,
		Config: kafka.ManualConsumerConfig{
			MinBytes:       1,
			MaxBytes:       10 << 20,
			MaxWait:        500 * time.Millisecond,
			ErrorBackoff:   time.Second,
			DeadLetterSink: outbox.NewDeadLetterSink(db, "user-service:user_created"),
		},
	})
	if err != nil {
		return nil, err
	}
	return &UserCreatedConsumer{
		pool:               pool,
		internalProfileSvc: internalProfileSvc,
		db:                 db,
	}, nil
}

// Start 启动 user_created 消费循环并阻塞到取消或 Pool 致命失败。
// Pool 内 worker 致命退出会取消兄弟；错误由 UserApp 的 RunIsolatedPool 隔离。
func (c *UserCreatedConsumer) Start(ctx context.Context) error {
	logger.Info(ctx, "User user_created 消费者启动中")
	return c.pool.Start(ctx, c.handle)
}

// Close 关闭 Pool 内全部 Reader；应在取消 Start 的 ctx 之后调用。
func (c *UserCreatedConsumer) Close() error {
	if c == nil || c.pool == nil {
		return nil
	}
	return c.pool.Close()
}

// handle 解码并幂等初始化 user 域资料。
// 契约错误标记 Permanent 进入 dead_events；DB/内部服务错误可重试；成功后才推进 offset。
func (c *UserCreatedConsumer) handle(ctx context.Context, message []byte) error {
	payload, err := accountevent.DecodeUserCreated(message)
	if err != nil {
		return kafka.Permanent(fmt.Errorf("解析 user_created 事件失败: %w", err))
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
