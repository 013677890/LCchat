package conversation

import (
	"context"

	"github.com/013677890/LCchat-Backend/model"
)

// Repository 会话领域仓储接口
// 职责：conversation 表 + group_conversation 表 CRUD
type Repository interface {
	// ==================== 个人会话 (conversation 表) ====================

	// GetByOwnerAndConvId 查询单个会话
	GetByOwnerAndConvId(ctx context.Context, ownerUuid, convId string) (*model.Conversation, error)

	// ListP2P 专属查询单聊（极速，纯走 B+ 树索引）
	ListP2P(ctx context.Context, ownerUuid string, updatedSince, cursorTimeMs, cursorId int64, pageSize int) ([]*model.Conversation, error)

	// ListGroup 专属查询群聊（核心魔法：联表 JOIN 替换真实时间）
	ListGroup(ctx context.Context, ownerUuid string, updatedSince, cursorTimeMs, cursorId int64, pageSize int) ([]*model.Conversation, error)

	// Upsert 创建或更新个人会话（发消息时调用）
	//   - 按 (owner_uuid, conv_id) 唯一键 upsert
	//   - isSender: 发送方不增加未读数；接收方在 DB 层面 unread_count + 1
	//   - 只更新核心字段 (max_seq, last_msg_*, status)，绝不碰 mute/pin/read_seq/clear_seq
	Upsert(ctx context.Context, conv *model.Conversation, isSender bool) error

	// UpdateReadSeq 更新已读位点（单调递增）
	//   - 实现：UPDATE SET read_seq = GREATEST(read_seq, ?),
	//           unread_count = GREATEST(0, max_seq - GREATEST(read_seq, ?))
	UpdateReadSeq(ctx context.Context, ownerUuid, convId string, readSeq int64) error

	// Delete 逻辑删除会话
	//   - 实现：status=1, clear_seq=max_seq, read_seq=max_seq, unread_count=0
	Delete(ctx context.Context, ownerUuid, convId string) error

	// UpdateSettings 更新会话设置（免打扰/置顶）
	//   - optional 语义：nil 表示不修改该字段
	UpdateSettings(ctx context.Context, ownerUuid, convId string, mute *bool, pin *bool) error

	// BatchInitGroupMemberConv 批量初始化群成员的 conversation 行（INSERT IGNORE 语义）。
	// 仅当 (owner_uuid, target_uuid) 不存在时插入，已存在的跳过。
	BatchInitGroupMemberConv(ctx context.Context, memberUUIDs []string, groupUUID string) error

	// ==================== 群会话热数据 (group_conversation 表) ====================

	// UpsertGroupConv 创建或更新群会话热数据
	//   - 每发一条群消息就 UPDATE 一次 max_seq, last_msg_*
	UpsertGroupConv(ctx context.Context, gc *model.GroupConversation) error

	// GetGroupConv 查询单个群的热数据
	GetGroupConv(ctx context.Context, groupUuid string) (*model.GroupConversation, error)
}
