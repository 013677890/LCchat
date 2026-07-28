package consumer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/013677890/LCchat-Backend/apps/msg/internal/domain/conversation"
	"github.com/013677890/LCchat-Backend/pkg/groupevent"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/outbox"

	"gorm.io/gorm"
)

// GroupMembershipProjector 以独立 consumer group 消费 group.cache，
// 把 group-service 的成员事实增量投影到 msg 拥有的会话表。
//
// 它和 group-service 自己的 Redis cache projector 订阅同一 topic 但使用不同 group ID：
// 两个消费者都会收到每一条事件，既不会互相抢消息，也不需要 message-push 中转。
type GroupMembershipProjector struct {
	consumer *kafka.Consumer
	repo     conversation.GroupMembershipProjectorRepository
}

// NewGroupMembershipProjector 创建 msg 群成员会话投影消费者。
func NewGroupMembershipProjector(
	brokers []string,
	topic, groupID string,
	repo conversation.GroupMembershipProjectorRepository,
	db *gorm.DB,
) *GroupMembershipProjector {
	return &GroupMembershipProjector{
		// NewManualCommitConsumer 在消费组没有已提交位点时使用 kafka-go 的 FirstOffset
		// 默认值。成员投影必须从该消费组可见的最早事件开始，不能从最新位置猜当前全集。
		consumer: kafka.NewManualCommitConsumer(brokers, topic, groupID, kafka.ManualConsumerConfig{
			MinBytes:       1,
			MaxBytes:       10 << 20,
			MaxWait:        500 * time.Millisecond,
			ErrorBackoff:   time.Second,
			DeadLetterSink: outbox.NewDeadLetterSink(db, "msg-service:group-membership"),
		}),
		repo: repo,
	}
}

// Start 启动阻塞式消费循环。
func (p *GroupMembershipProjector) Start(ctx context.Context) error {
	if p == nil || p.consumer == nil || p.repo == nil {
		return fmt.Errorf("msg group membership projector 未完整初始化")
	}
	logger.Info(ctx, "Msg group membership projector 启动中")
	return p.consumer.Start(ctx, p.handle)
}

// Close 关闭底层 Kafka reader。
func (p *GroupMembershipProjector) Close() error {
	if p == nil || p.consumer == nil {
		return nil
	}
	return p.consumer.Close()
}

func (p *GroupMembershipProjector) handle(ctx context.Context, message []byte) error {
	payload, err := groupevent.DecodeGroupCache(message)
	if err != nil {
		return kafka.Permanent(fmt.Errorf("解析 group.cache 当前严格事件失败: %w", err))
	}
	if err := p.repo.ApplyGroupCacheEvent(ctx, payload); err != nil {
		if errors.Is(err, conversation.ErrInvalidGroupProjectionEvent) ||
			errors.Is(err, conversation.ErrGroupProjectionVersionGap) {
			return kafka.Permanent(err)
		}
		return fmt.Errorf("投影 msg 群成员会话状态失败: %w", err)
	}
	return nil
}
