package conversation

import (
	"context"
	"fmt"
	"time"

	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/outbox"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// repositoryImpl 会话领域仓储实现
type repositoryImpl struct {
	db *gorm.DB
}

// NewRepository 创建会话仓储实例
func NewRepository(db *gorm.DB) Repository {
	return &repositoryImpl{db: db}
}

// ==================== 个人会话 (conversation 表) ====================

// GetByOwnerAndConvId 查询单个会话
func (r *repositoryImpl) GetByOwnerAndConvId(ctx context.Context, ownerUuid, convId string) (*model.Conversation, error) {
	var conv model.Conversation
	err := r.db.WithContext(ctx).
		Where("owner_uuid = ? AND conv_id = ?", ownerUuid, convId).
		First(&conv).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrConversationNotFound
		}
		return nil, fmt.Errorf("GetByOwnerAndConvId: db query failed: %w", err)
	}
	return &conv, nil
}

// ListP2P 专属查询单聊
func (r *repositoryImpl) ListP2P(ctx context.Context, ownerUuid string, updatedSince, cursorTimeMs, cursorId int64, pageSize int) ([]*model.Conversation, error) {
	var convs []*model.Conversation
	query := r.db.WithContext(ctx).
		Where("owner_uuid = ? AND type = 1", ownerUuid) // type=1 为单聊

	// 增量同步：返回所有变更记录（包括 status=1 已删除的，用于多端同步删除状态）
	if updatedSince > 0 {
		query = query.Where("updated_at > ?", time.UnixMilli(updatedSince))
	} else {
		// 全量拉取：只返回活跃会话
		query = query.Where("status = 0")
	}

	if cursorTimeMs > 0 && cursorId > 0 {
		curTime := time.UnixMilli(cursorTimeMs)
		query = query.Where("(updated_at < ?) OR (updated_at = ? AND id < ?)", curTime, curTime, cursorId)
	}

	err := query.Order("updated_at DESC, id DESC").
		Limit(pageSize).
		Find(&convs).Error

	return convs, err
}

// ListGroup 专属查询群聊（联表 JOIN 替换真实时间）
func (r *repositoryImpl) ListGroup(ctx context.Context, ownerUuid string, updatedSince, cursorTimeMs, cursorId int64, pageSize int) ([]*model.Conversation, error) {
	var convs []*model.Conversation

	// 核心：Select 中将 gc.last_msg_at 别名赋给 updated_at，让 GORM 自动映射到 model 的 UpdatedAt 用于后续内存排序
	query := r.db.WithContext(ctx).Table("conversation c").
		Select("c.*, gc.max_seq as max_seq, gc.last_msg_id as last_msg_id, gc.last_msg_preview as last_msg_preview, gc.last_msg_at as updated_at").
		Joins("INNER JOIN group_conversation gc ON c.target_uuid = gc.group_uuid").
		Where("c.owner_uuid = ? AND c.type = 2", ownerUuid) // type=2 为群聊

	// 增量同步必须看个人表 c.updated_at（设置变更、删除会话等）
	if updatedSince > 0 {
		query = query.Where("c.updated_at > ?", time.UnixMilli(updatedSince))
	} else {
		// 全量拉取：只返回活跃会话
		query = query.Where("c.status = 0")
	}

	if cursorTimeMs > 0 && cursorId > 0 {
		curTime := time.UnixMilli(cursorTimeMs)
		// 注意：游标必须依赖 gc 的真实活跃时间
		query = query.Where("(gc.last_msg_at < ?) OR (gc.last_msg_at = ? AND c.id < ?)", curTime, curTime, cursorId)
	}

	// 按照真实的群活跃时间排序
	err := query.Order("gc.last_msg_at DESC, c.id DESC").
		Limit(pageSize).
		Find(&convs).Error

	return convs, err
}

// BatchInitGroupMemberConv 批量初始化群成员 conversation 行。
// INSERT IGNORE 语义：仅当 (owner_uuid, conv_id) 不存在时插入，已存在的跳过。
func (r *repositoryImpl) BatchInitGroupMemberConv(ctx context.Context, memberUUIDs []string, groupUUID string) error {
	if len(memberUUIDs) == 0 {
		return nil
	}
	convs := make([]*model.Conversation, 0, len(memberUUIDs))
	for _, uid := range memberUUIDs {
		convs = append(convs, &model.Conversation{
			ConvId:     groupUUID,
			Type:       2, // GROUP
			OwnerUuid:  uid,
			TargetUuid: groupUUID,
			Status:     0,
		})
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "owner_uuid"}, {Name: "conv_id"}},
			DoNothing: true,
		}).CreateInBatches(convs, 100).Error
}

// Upsert 创建或更新个人会话
//
// 【Bug1 修复】
// - 只更新核心字段 (max_seq, last_msg_*, status)
// - 绝不碰 mute / pin / read_seq / clear_seq（这些由专门的方法维护）
// - 接收方 unread_count 在 DB 层面 +1，而非 Go 层覆盖
// - 发送方 read_seq = max_seq，unread_count 不变
func (r *repositoryImpl) Upsert(ctx context.Context, conv *model.Conversation, isSender bool) error {
	// 构造更新 map，只更新发消息时需要变更的字段
	updates := map[string]interface{}{
		"max_seq":          conv.MaxSeq,
		"last_msg_id":      conv.LastMsgId,
		"last_msg_at":      conv.LastMsgAt,
		"last_msg_preview": conv.LastMsgPrev,
		"status":           0,          // 重新激活已删除会话
		"updated_at":       time.Now(), // 强制更新，用于增量拉取排序
	}

	if isSender {
		// 发送方：read_seq 追平到当前消息（自己发的消息不算未读）
		updates["read_seq"] = conv.MaxSeq
	} else {
		// 接收方：在数据库层面 unread_count + 1（极端并发下也安全）
		updates["unread_count"] = gorm.Expr("unread_count + 1")
	}

	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "owner_uuid"}, {Name: "conv_id"}},
			DoUpdates: clause.Assignments(updates),
		}).Create(conv).Error
	if err != nil {
		return fmt.Errorf("Upsert: db upsert failed: %w", err)
	}
	return nil
}

// UpdateReadSeq 更新已读位点（单调递增）
//
// GREATEST 保证 read_seq 只增不减，防止旧设备覆盖新设备的已读位点
func (r *repositoryImpl) UpdateReadSeq(ctx context.Context, ownerUuid, convId string, readSeq int64) error {
	return updateConversationReadSeq(r.db.WithContext(ctx), ownerUuid, convId, readSeq)
}

// UpdateReadSeqWithOutbox 在同一事务中更新已读位点并追加 CDC Outbox 事件。
func (r *repositoryImpl) UpdateReadSeqWithOutbox(ctx context.Context, ownerUuid, convId string, readSeq int64, events []OutboxEvent) (*model.Conversation, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("UpdateReadSeqWithOutbox: empty outbox events")
	}
	for _, event := range events {
		if event.EventType == "" || event.EntityID == "" || event.Payload == "" {
			return nil, fmt.Errorf("UpdateReadSeqWithOutbox: invalid outbox event")
		}
	}

	var conv model.Conversation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := updateConversationReadSeq(tx, ownerUuid, convId, readSeq); err != nil {
			return err
		}
		for _, event := range events {
			if err := outbox.InsertEvent(tx, event.EventType, event.EntityID, event.Payload); err != nil {
				return fmt.Errorf("UpdateReadSeqWithOutbox: outbox insert failed: %w", err)
			}
		}
		if err := tx.Where("owner_uuid = ? AND conv_id = ?", ownerUuid, convId).First(&conv).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrConversationNotFound
			}
			return fmt.Errorf("UpdateReadSeqWithOutbox: db query failed: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

// updateConversationReadSeq 执行已读位点单调更新。
func updateConversationReadSeq(db *gorm.DB, ownerUuid, convId string, readSeq int64) error {
	result := db.
		Model(&model.Conversation{}).
		Where("owner_uuid = ? AND conv_id = ?", ownerUuid, convId).
		Updates(map[string]interface{}{
			"read_seq":     gorm.Expr("GREATEST(read_seq, ?)", readSeq),
			"unread_count": gorm.Expr("GREATEST(0, max_seq - GREATEST(read_seq, ?))", readSeq),
		})
	if result.Error != nil {
		return fmt.Errorf("UpdateReadSeq: db update failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrConversationNotFound
	}
	return nil
}

// Delete 逻辑删除会话
func (r *repositoryImpl) Delete(ctx context.Context, ownerUuid, convId string) error {
	result := r.db.WithContext(ctx).
		Model(&model.Conversation{}).
		Where("owner_uuid = ? AND conv_id = ?", ownerUuid, convId).
		Updates(map[string]interface{}{
			"status":       1,
			"clear_seq":    gorm.Expr("max_seq"),
			"read_seq":     gorm.Expr("max_seq"),
			"unread_count": 0,
		})
	if result.Error != nil {
		return fmt.Errorf("Delete: db update failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrConversationNotFound
	}
	return nil
}

// UpdateSettings 更新会话设置（optional bool 语义）
func (r *repositoryImpl) UpdateSettings(ctx context.Context, ownerUuid, convId string, mute *bool, pin *bool) error {
	updates := map[string]interface{}{}
	if mute != nil {
		updates["mute"] = *mute
	}
	if pin != nil {
		updates["pin"] = *pin
	}
	if len(updates) == 0 {
		return nil
	}

	result := r.db.WithContext(ctx).
		Model(&model.Conversation{}).
		Where("owner_uuid = ? AND conv_id = ?", ownerUuid, convId).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("UpdateSettings: db update failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrConversationNotFound
	}
	return nil
}

// ==================== 群会话热数据 (group_conversation 表) ====================

// UpsertGroupConv 创建或更新群会话热数据
func (r *repositoryImpl) UpsertGroupConv(ctx context.Context, gc *model.GroupConversation) error {
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "group_uuid"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"max_seq", "last_msg_id", "last_msg_preview", "last_msg_at",
			}),
		}).Create(gc).Error
	if err != nil {
		return fmt.Errorf("UpsertGroupConv: db upsert failed: %w", err)
	}
	return nil
}

// GetGroupConv 查询单个群的热数据
func (r *repositoryImpl) GetGroupConv(ctx context.Context, groupUuid string) (*model.GroupConversation, error) {
	var gc model.GroupConversation
	err := r.db.WithContext(ctx).
		Where("group_uuid = ?", groupUuid).
		First(&gc).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrConversationNotFound
		}
		return nil, fmt.Errorf("GetGroupConv: db query failed: %w", err)
	}
	return &gc, nil
}
