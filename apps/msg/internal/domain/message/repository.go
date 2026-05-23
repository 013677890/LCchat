package message

import (
	"context"

	"github.com/013677890/LCchat-Backend/model"
)

// OutboxEvent 描述需要与消息事实同事务落库的 CDC 事件。
type OutboxEvent struct {
	EventType string
	EntityID  string
	Payload   string
}

// Repository 消息领域仓储接口
// 职责：消息表 CRUD + Redis seq 分配 + 幂等锁
type Repository interface {
	// ==================== seq 分配 ====================

	// AllocSeq 原子分配会话内递增序号
	// 实现：Redis INCR msg:seq:{conv_id}
	AllocSeq(ctx context.Context, convId string) (int64, error)

	// ==================== 幂等 (SETNX 锁 + 结果缓存) ====================

	// TryAcquireIdempotent 尝试获取幂等锁（Redis SETNX）
	// 返回值：
	//   - (nil, nil):  锁获取成功，可以继续执行消息创建
	//   - (msg, nil):  已有处理结果（命中缓存），直接返回 msg
	//   - (nil, ErrIdempotentProcessing): 另一个请求正在处理中
	//   - (nil, other): Redis 异常（降级到 DB 唯一索引兜底）
	TryAcquireIdempotent(ctx context.Context, fromUuid, deviceId, clientMsgId string) (*model.Message, error)

	// SetIdempotentResult 将 "PROCESSING" 标记替换为实际结果，TTL 延长至 10 分钟
	SetIdempotentResult(ctx context.Context, fromUuid, deviceId, clientMsgId string, msg *model.Message) error

	// ==================== 消息 CRUD ====================

	// Create 插入一条消息
	// 如果触发 uidx_sender_client 唯一索引冲突，返回 ErrDuplicateMessage
	Create(ctx context.Context, msg *model.Message) error

	// CreateWithOutbox 在同一 MySQL 事务中插入消息与 outbox 事件。
	// 如果消息唯一索引冲突，返回 ErrDuplicateMessage，且不会写 outbox。
	CreateWithOutbox(ctx context.Context, msg *model.Message, event OutboxEvent) error

	// GetByDuplicateKey 通过幂等三元组查询已存在的消息（DB 唯一索引兜底）
	GetByDuplicateKey(ctx context.Context, fromUuid, deviceId, clientMsgId string) (*model.Message, error)

	// GetBySeqRange 按 seq 范围拉取消息（支持双向 + clear_seq 过滤）
	// direction: 1=FORWARD(seq > anchor), 2=BACKWARD(seq < anchor)
	GetBySeqRange(ctx context.Context, convId string, anchorSeq int64, direction int, limit int, clearSeq int64) ([]*model.Message, error)

	// GetByIds 批量按消息 ID 查询
	GetByIds(ctx context.Context, convId string, msgIds []string) ([]*model.Message, error)

	// GetById 查单条消息
	GetById(ctx context.Context, convId string, msgId string) (*model.Message, error)

	// GetMaxSeq 查询会话当前最大已落库 seq
	GetMaxSeq(ctx context.Context, convId string) (int64, error)

	// ==================== 撤回 ====================

	// UpdateStatus 更新消息状态和内容（撤回场景：status=1, content=提示JSON）
	UpdateStatus(ctx context.Context, convId string, msgId string, status int8, content string) error
}
