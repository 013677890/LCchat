package conversation

import (
	"context"

	"github.com/013677890/LCchat-Backend/model"
)

// OutboxEvent 描述需要与会话事实同事务落库的 CDC 事件。
type OutboxEvent struct {
	EventType string
	EntityID  string
	Payload   string
}

// Repository 会话领域仓储接口
// 职责：conversation 表 + group_conversation 表 CRUD
type Repository interface {
	// ==================== 个人会话 (conversation 表) ====================

	// GetByOwnerAndConvId 查询单个会话
	GetByOwnerAndConvId(ctx context.Context, ownerUuid, convId string) (*model.Conversation, error)

	// ListP2P 专属查询单聊（极速，纯走 B+ 树索引）
	ListP2P(ctx context.Context, ownerUuid string, updatedSince, cursorTimeMs, cursorId int64, pageSize int) ([]*model.Conversation, error)

	// ListGroup 联表读取个人成员状态与群共享热数据。
	// 全量只返回当前可见会话；增量会返回退群/解散/个人删除 tombstone。
	ListGroup(ctx context.Context, ownerUuid string, updatedSince, cursorTimeMs, cursorId int64, pageSize int) ([]*model.Conversation, error)

	// Upsert 创建或更新个人会话（发消息时调用）
	//   - 按 (owner_uuid, target_uuid) 唯一键 upsert
	//   - isSender: 发送方不增加未读数；接收方在 DB 层面 unread_count + 1
	//   - 只更新核心字段 (max_seq, last_msg_*, status)，绝不碰 mute/pin/read_seq/clear_seq
	Upsert(ctx context.Context, conv *model.Conversation, isSender bool) error

	// RepairForMessage 幂等修复个人会话投影（幂等命中/补偿路径使用）
	//   - 缺行时按消息创建会话行
	//   - 已有行落后时推进 max_seq/last_msg/status
	//   - 仅 incoming seq > clear_seq 时恢复 status，幂等旧消息不能越过删除位点
	//   - 接收方 unread_count 按 max_seq-read_seq 重算，避免重复重试时 +1
	RepairForMessage(ctx context.Context, conv *model.Conversation, isSender bool) error

	// UpdateReadSeq 更新已读位点（单调递增）
	//   - 实现等价于 read_seq=max(read_seq, ?)，unread_count=max(0,max_seq-read_seq)。
	UpdateReadSeq(ctx context.Context, ownerUuid, convId string, readSeq int64) error

	// UpdateReadSeqWithOutbox 在同一 MySQL 事务中更新已读位点并写入 outbox 事件。
	UpdateReadSeqWithOutbox(ctx context.Context, ownerUuid, convId string, readSeq int64, events []OutboxEvent) (*model.Conversation, error)

	// UpsertGroupReadSeqWithOutbox 群会话专用：本地无行时按需建行并写入已读位点 + outbox（同事务）。
	//   - 冲突时只单调推进 read_seq，不触碰 status/clear_seq/mute/pin。
	//   - 仅用于群会话（type=2, target_uuid=group_uuid），调用方需先校验群成员资格。
	UpsertGroupReadSeqWithOutbox(ctx context.Context, ownerUuid, groupUuid string, readSeq int64, events []OutboxEvent) (*model.Conversation, error)

	// Delete 逻辑删除会话。
	//   - P2P 使用个人 max_seq；GROUP 使用 group_conversation.max_seq。
	//   - 写入 status=1, clear_seq/read_seq=当前权威 max_seq, unread_count=0。
	Delete(ctx context.Context, ownerUuid, convId string) error

	// UpdateSettings 更新会话设置（免打扰/置顶）
	//   - optional 语义：nil 表示不修改该字段
	UpdateSettings(ctx context.Context, ownerUuid, convId string, mute *bool, pin *bool) error

	// ==================== 群会话热数据 (group_conversation 表) ====================

	// UpsertGroupConv 创建或更新群会话热数据
	//   - 每发一条群消息只触碰一行，并按 seq 整组单调推进 max_seq/last_msg_*/updated_at
	UpsertGroupConv(ctx context.Context, gc *model.GroupConversation) error

	// GetGroupConv 查询单个群的热数据
	GetGroupConv(ctx context.Context, groupUuid string) (*model.GroupConversation, error)
}
