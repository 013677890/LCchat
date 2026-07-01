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
// INSERT IGNORE 语义：仅当 (owner_uuid, target_uuid) 不存在时插入，已存在的跳过。
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
			Columns:   []clause.Column{{Name: "owner_uuid"}, {Name: "target_uuid"}},
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
		// max_seq 必须单调递增：即使异常 seq 或乱序 workflow 到达，也不能把会话清空位点的基准写小。
		// 使用 CASE 而非 GREATEST 是为了兼容 SQLite 单元测试，同时 MySQL 语义等价。
		"max_seq":          gorm.Expr("CASE WHEN max_seq > ? THEN max_seq ELSE ? END", conv.MaxSeq, conv.MaxSeq),
		"last_msg_id":      conv.LastMsgId,
		"last_msg_at":      conv.LastMsgAt,
		"last_msg_preview": conv.LastMsgPrev,
		"status":           0,          // 重新激活已删除会话
		"updated_at":       time.Now(), // 强制更新，用于增量拉取排序
	}

	if isSender {
		// 发送方：read_seq 只能向前追平到当前消息，避免旧设备/旧 workflow 把已读位点写小。
		updates["read_seq"] = gorm.Expr("CASE WHEN read_seq > ? THEN read_seq ELSE ? END", conv.MaxSeq, conv.MaxSeq)
	} else {
		// 接收方：在数据库层面 unread_count + 1（极端并发下也安全）
		updates["unread_count"] = gorm.Expr("unread_count + 1")
	}

	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "owner_uuid"}, {Name: "target_uuid"}},
			DoUpdates: clause.Assignments(updates),
		}).Create(conv).Error
	if err != nil {
		return fmt.Errorf("Upsert: db upsert failed: %w", err)
	}
	return nil
}

// RepairForMessage 幂等修复个人会话投影。
//
// 这条路径用于“消息事实已存在，但 conversation 派生视图可能漏写”的补偿场景：
//   - 不使用接收方 unread_count + 1，避免幂等重试重复加未读；
//   - 接收方未读按当前 max_seq-read_seq 重算；
//   - 只有会话行落后于当前消息时才推进 last_msg/max_seq/status。
func (r *repositoryImpl) RepairForMessage(ctx context.Context, conv *model.Conversation, isSender bool) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		insertResult := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "owner_uuid"}, {Name: "target_uuid"}},
			DoNothing: true,
		}).Create(conv)
		if insertResult.Error != nil {
			return fmt.Errorf("RepairForMessage: insert conversation failed: %w", insertResult.Error)
		}
		if insertResult.RowsAffected > 0 {
			return nil
		}

		staleUpdates := map[string]interface{}{
			"max_seq":          conv.MaxSeq,
			"last_msg_id":      conv.LastMsgId,
			"last_msg_at":      conv.LastMsgAt,
			"last_msg_preview": conv.LastMsgPrev,
			"status":           0,
			"updated_at":       time.Now(),
		}
		if isSender {
			staleUpdates["read_seq"] = gorm.Expr("CASE WHEN read_seq > ? THEN read_seq ELSE ? END", conv.MaxSeq, conv.MaxSeq)
		}

		if err := tx.Model(&model.Conversation{}).
			Where("owner_uuid = ? AND conv_id = ? AND max_seq < ?", conv.OwnerUuid, conv.ConvId, conv.MaxSeq).
			Updates(staleUpdates).Error; err != nil {
			return fmt.Errorf("RepairForMessage: update stale conversation failed: %w", err)
		}

		if isSender {
			if err := tx.Model(&model.Conversation{}).
				Where("owner_uuid = ? AND conv_id = ?", conv.OwnerUuid, conv.ConvId).
				UpdateColumn("read_seq", gorm.Expr("CASE WHEN read_seq > ? THEN read_seq ELSE ? END", conv.MaxSeq, conv.MaxSeq)).Error; err != nil {
				return fmt.Errorf("RepairForMessage: repair sender read_seq failed: %w", err)
			}
			return nil
		}

		if err := tx.Model(&model.Conversation{}).
			Where("owner_uuid = ? AND conv_id = ?", conv.OwnerUuid, conv.ConvId).
			UpdateColumn("unread_count", gorm.Expr("CASE WHEN read_seq >= max_seq THEN 0 ELSE max_seq - read_seq END")).Error; err != nil {
			return fmt.Errorf("RepairForMessage: recompute receiver unread failed: %w", err)
		}
		return nil
	})
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

// UpsertGroupReadSeqWithOutbox 群会话专用：本地无行时按需建行并写入已读位点 + outbox（同事务）。
func (r *repositoryImpl) UpsertGroupReadSeqWithOutbox(ctx context.Context, ownerUuid, groupUuid string, readSeq int64, events []OutboxEvent) (*model.Conversation, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("UpsertGroupReadSeqWithOutbox: empty outbox events")
	}
	for _, event := range events {
		if event.EventType == "" || event.EntityID == "" || event.Payload == "" {
			return nil, fmt.Errorf("UpsertGroupReadSeqWithOutbox: invalid outbox event")
		}
	}

	var conv model.Conversation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := &model.Conversation{
			ConvId:      groupUuid,
			Type:        2, // GROUP
			OwnerUuid:   ownerUuid,
			TargetUuid:  groupUuid,
			ReadSeq:     readSeq,
			UnreadCount: 0, // 群未读由会话列表用 group_conversation.max_seq 现算，这里不维护
			Status:      0,
		}
		// 冲突（行已存在）时只单调推进 read_seq，绝不触碰 status/clear_seq/mute/pin。
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "owner_uuid"}, {Name: "target_uuid"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"read_seq": gorm.Expr("GREATEST(read_seq, ?)", readSeq),
			}),
		}).Create(row).Error; err != nil {
			return fmt.Errorf("UpsertGroupReadSeqWithOutbox: upsert failed: %w", err)
		}
		for _, event := range events {
			if err := outbox.InsertEvent(tx, event.EventType, event.EntityID, event.Payload); err != nil {
				return fmt.Errorf("UpsertGroupReadSeqWithOutbox: outbox insert failed: %w", err)
			}
		}
		if err := tx.Where("owner_uuid = ? AND conv_id = ?", ownerUuid, groupUuid).First(&conv).Error; err != nil {
			return fmt.Errorf("UpsertGroupReadSeqWithOutbox: db query failed: %w", err)
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
			DoUpdates: clause.Assignments(map[string]interface{}{
				// 群会话热数据同样只允许 max_seq 前进，防止群列表活跃位点被异常小 seq 回退。
				"max_seq":          gorm.Expr("CASE WHEN max_seq > ? THEN max_seq ELSE ? END", gc.MaxSeq, gc.MaxSeq),
				"last_msg_id":      gc.LastMsgId,
				"last_msg_preview": gc.LastMsgPrev,
				"last_msg_at":      gc.LastMsgAt,
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
