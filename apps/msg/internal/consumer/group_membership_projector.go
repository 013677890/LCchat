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
//
// 并行模型与 group Redis projector 一致：
//   - 进程内 N 个独立 Reader，同一 group ID，Kafka 分配不同 partition；
//   - Reader 内串行；不同群可并行；同群因 key=group_uuid 严格有序；
//   - 严格连续 projection_version、同事务幂等与死信语义保持不变。
type GroupMembershipProjector struct {
	pool *kafka.ManualConsumerPool
	repo conversation.GroupMembershipProjectorRepository
}

// NewGroupMembershipProjector 创建 msg 群成员会话分区级并行投影消费者。
//
// workers 必须由调用方用 kafka.ParsePoolWorkers 严格解析后传入。
// ManualConsumerPool 在消费组没有已提交位点时使用 kafka-go 的 FirstOffset
// 默认值。成员投影必须从该消费组可见的最早事件开始，不能从最新位置猜当前全集。
func NewGroupMembershipProjector(
	brokers []string,
	topic, groupID string,
	workers int,
	repo conversation.GroupMembershipProjectorRepository,
	db *gorm.DB,
) (*GroupMembershipProjector, error) {
	pool, err := kafka.NewManualConsumerPool(kafka.ManualConsumerPoolConfig{
		Name:    "msg-group-membership-projector",
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
		Workers: workers,
		Config: kafka.ManualConsumerConfig{
			MinBytes:       1,
			MaxBytes:       10 << 20,
			MaxWait:        500 * time.Millisecond,
			ErrorBackoff:   time.Second,
			DeadLetterSink: outbox.NewDeadLetterSink(db, "msg-service:group-membership"),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("创建 msg group membership projector consumer pool 失败: %w", err)
	}
	return &GroupMembershipProjector{
		pool: pool,
		repo: repo,
	}, nil
}

// Start 启动阻塞式分区并行消费。
// 任一 worker 致命退出会取消并等待其余 worker；MsgApp 通过 RunIsolatedPool 隔离该错误，
// 发送消息 gRPC 仍可继续（投影滞后由后续事件与客户端同步弥补）。
func (p *GroupMembershipProjector) Start(ctx context.Context) error {
	if p == nil || p.pool == nil || p.repo == nil {
		return fmt.Errorf("msg group membership projector 未完整初始化")
	}
	logger.Info(ctx, "Msg group membership projector 启动中",
		logger.Int("worker_count", p.pool.WorkerCount()),
	)
	return p.pool.Start(ctx, p.handle)
}

// Close 关闭全部底层 Kafka Reader；关停时应先 cancel Start 的 ctx。
func (p *GroupMembershipProjector) Close() error {
	if p == nil || p.pool == nil {
		return nil
	}
	return p.pool.Close()
}

// WorkerCount 返回并行 Reader 数量。
func (p *GroupMembershipProjector) WorkerCount() int {
	if p == nil || p.pool == nil {
		return 0
	}
	return p.pool.WorkerCount()
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
