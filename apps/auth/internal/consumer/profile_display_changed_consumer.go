package consumer

import (
	"context"
	"fmt"
	"time"

	"github.com/013677890/LCchat-Backend/apps/auth/internal/service"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/pkg/event"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	"gorm.io/gorm"
)

const authProfileDisplayChangedIdempotentEventType = event.EventTypeProfileDisplayChanged + ":auth-service"

// ProfileDisplayChangedConsumer 消费 profile_display_changed，回写 auth 域登录展示冗余字段。
// 编排使用 ManualConsumerPool；进程级由 AuthApp 通过 kafka.RunIsolatedPool 隔离，
// 消费故障不中断登录/鉴权 gRPC。
type ProfileDisplayChangedConsumer struct {
	pool            *kafka.ManualConsumerPool
	internalAuthSvc service.InternalAuthService
	db              *gorm.DB
}

// NewProfileDisplayChangedConsumer 创建 auth-service 的资料展示事件消费 Pool。
// workers 非法或 Pool 参数不完整时直接返回错误，调用方必须中止服务初始化，禁止回退单 Reader。
func NewProfileDisplayChangedConsumer(
	brokers []string,
	topic, groupID string,
	workers int,
	internalAuthSvc service.InternalAuthService,
	db *gorm.DB,
) (*ProfileDisplayChangedConsumer, error) {
	pool, err := kafka.NewManualConsumerPool(kafka.ManualConsumerPoolConfig{
		Name:    "auth-profile-display-changed",
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
		Workers: workers,
		Config: kafka.ManualConsumerConfig{
			MinBytes:       1,
			MaxBytes:       10 << 20,
			MaxWait:        500 * time.Millisecond,
			ErrorBackoff:   time.Second,
			DeadLetterSink: outbox.NewDeadLetterSink(db, "auth-service:profile_display_changed"),
		},
	})
	if err != nil {
		return nil, err
	}
	return &ProfileDisplayChangedConsumer{
		pool:            pool,
		internalAuthSvc: internalAuthSvc,
		db:              db,
	}, nil
}

// Start 启动资料展示字段变更消费循环并阻塞到取消或 Pool 致命失败。
// Pool 内 worker 致命退出会取消兄弟；返回的错误由 AuthApp 的 RunIsolatedPool 隔离处理。
func (c *ProfileDisplayChangedConsumer) Start(ctx context.Context) error {
	logger.Info(ctx, "Auth profile_display_changed 消费者启动中")
	return c.pool.Start(ctx, c.handle)
}

// Close 关闭 Pool 内全部 Reader。
// 应在取消 Start 所用 ctx 之后调用，避免与活跃 Fetch 竞态。
func (c *ProfileDisplayChangedConsumer) Close() error {
	if c == nil || c.pool == nil {
		return nil
	}
	return c.pool.Close()
}

// handle 解码并幂等回写 auth 登录展示字段。
// 契约/解码错误标记 Permanent，由公共层首轮写 dead_events 后再提交；
// 幂等命中直接成功；DB 或内部服务抖动返回普通 error 以原地重试，成功后才推进 offset。
func (c *ProfileDisplayChangedConsumer) handle(ctx context.Context, message []byte) error {
	payload, err := event.DecodeProfileDisplayChanged(message)
	if err != nil {
		return kafka.Permanent(fmt.Errorf("解析 profile_display_changed 事件失败: %w", err))
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
