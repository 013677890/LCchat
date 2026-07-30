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

// ListGroup 专属查询群聊（个人视图 + 群成员投影 + 群共享热数据）。
//
// 群是否应出现在用户列表中由两条彼此独立的状态共同决定：
//   - c.membership_status=ACTIVE：group-service 投影确认用户仍是群成员；
//   - c.status=0：用户没有主动隐藏/删除自己的会话。
//
// group_conversation.group_status 再提供群级终态栅栏，群解散只需更新一条共享行，
// 无需为了让列表消失而同步 UPDATE 全体成员的 conversation。
//
// 全量查询只返回当前可见会话；增量查询必须额外返回退群、群解散和个人删除记录，
// 并把它们映射为 status=1 tombstone。否则离线客户端只会看到会话“从结果中消失”，
// 无法判断应从本地列表删除它。
func (r *repositoryImpl) ListGroup(ctx context.Context, ownerUuid string, updatedSince, cursorTimeMs, cursorId int64, pageSize int) ([]*model.Conversation, error) {
	// 不把 SQL CASE 的 datetime 结果直接扫描进 time.Time：SQLite 会把该表达式返回为
	// string，而 MySQL 返回时间类型。显式扫描两条原始时间列后在 Go 中合并，既保持测试
	// 与生产语义一致，也避免为数据库方言增加分支。
	type groupListRow struct {
		model.Conversation `gorm:"embedded"`
		GroupMaxSeq        int64      `gorm:"column:group_max_seq"`
		GroupLastMsgID     string     `gorm:"column:group_last_msg_id"`
		GroupLastMsgPrev   string     `gorm:"column:group_last_msg_preview"`
		GroupLastMsgAt     *time.Time `gorm:"column:group_last_msg_at"`
		GroupUpdatedAt     time.Time  `gorm:"column:group_updated_at"`
		GroupStatus        int8       `gorm:"column:group_status"`
	}
	var rows []*groupListRow

	// 全量查询按最后消息/建群时间排序；增量查询按个人行或群共享行中较新的变更时间排序。
	// 查询完成后把同一个排序时间映射到 UpdatedAt，保证上层合并 P2P/GROUP 和生成联合
	// 游标时使用的字段与本查询的 WHERE/ORDER BY 完全一致。
	orderExpr := "gc.last_msg_at"
	if updatedSince > 0 {
		orderExpr = "CASE WHEN c.updated_at >= gc.updated_at THEN c.updated_at ELSE gc.updated_at END"
	}
	query := r.db.WithContext(ctx).Table("conversation c").
		Select(
			"c.*, gc.max_seq as group_max_seq, gc.last_msg_id as group_last_msg_id, "+
				"gc.last_msg_preview as group_last_msg_preview, gc.last_msg_at as group_last_msg_at, "+
				"gc.updated_at as group_updated_at, gc.group_status as group_status",
		).
		Joins("INNER JOIN group_conversation gc ON c.target_uuid = gc.group_uuid").
		Where("c.owner_uuid = ? AND c.type = 2", ownerUuid)

	// 增量同步同时观察个人状态与群共享热数据。群消息只更新 gc，不再写扩散更新
	// N 个成员的 c.updated_at；成员退出更新 c，群解散更新 gc，二者都必须作为删除态返回。
	if updatedSince > 0 {
		since := time.UnixMilli(updatedSince)
		query = query.Where("(c.updated_at > ? OR gc.updated_at > ?)", since, since)
	} else {
		// 全量拉取只返回当前可见会话，不返回历史 tombstone。
		query = query.Where(
			"(c.status = 0 OR gc.max_seq > c.clear_seq) AND c.membership_status = ? AND gc.group_status = ?",
			model.ConversationMembershipActive,
			model.GroupConversationStatusNormal,
		)
	}

	if cursorTimeMs > 0 && cursorId > 0 {
		curTime := time.UnixMilli(cursorTimeMs)
		query = query.Where(
			fmt.Sprintf("(%s < ?) OR (%s = ? AND c.id < ?)", orderExpr, orderExpr),
			curTime,
			curTime,
			cursorId,
		)
	}

	err := query.Order(orderExpr + " DESC, c.id DESC").
		Limit(pageSize).
		Find(&rows).Error

	if err != nil {
		return nil, err
	}

	convs := make([]*model.Conversation, 0, len(rows))
	for _, row := range rows {
		if row.GroupLastMsgAt == nil {
			// group_created 必须建立首次可见时间；出现 NULL 代表当前写路径违反模型
			// 不变量，禁止猜测替代时间后继续返回不稳定游标。
			return nil, fmt.Errorf("ListGroup: group_uuid=%s 缺少 last_msg_at", row.TargetUuid)
		}
		conv := &row.Conversation
		conv.MaxSeq = row.GroupMaxSeq
		conv.LastMsgId = row.GroupLastMsgID
		conv.LastMsgPrev = row.GroupLastMsgPrev
		conv.LastMsgAt = row.GroupLastMsgAt
		if updatedSince > 0 {
			if row.GroupUpdatedAt.After(conv.UpdatedAt) {
				conv.UpdatedAt = row.GroupUpdatedAt
			}
		} else {
			conv.UpdatedAt = *row.GroupLastMsgAt
		}
		if conv.MembershipStatus != model.ConversationMembershipActive ||
			row.GroupStatus != model.GroupConversationStatusNormal {
			conv.Status = 1
		} else if conv.Status == 1 && conv.MaxSeq > conv.ClearSeq {
			// 群聊不再向所有成员写扩散，所以新群消息无法逐行把 status 改回 0。
			// 用共享 max_seq 与个人 clear_seq 派生“删除后有新消息”，即可在 O(1) 写路径下
			// 恢复会话可见性；这里只改返回视图，不覆盖用户持久化设置。
			conv.Status = 0
		}

		convs = append(convs, conv)
	}

	return convs, nil
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

			// 只有消息 seq 真正越过用户的删除位点才能恢复可见。群个人行在读扩散
			// 模型下可能仍是 max_seq=0，而 clear_seq 已来自共享群行；不能仅凭
			// “个人 max_seq 落后”就把同一条旧消息重新显示出来。
			"status":     gorm.Expr("CASE WHEN clear_seq < ? THEN 0 ELSE status END", conv.MaxSeq),
			"updated_at": time.Now(),
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
			ConvId:            groupUuid,
			Type:              2, // GROUP
			OwnerUuid:         ownerUuid,
			TargetUuid:        groupUuid,
			ReadSeq:           readSeq,
			UnreadCount:       0, // 群未读由会话列表用 group_conversation.max_seq 现算，这里不维护
			Status:            0,
			MembershipStatus:  model.ConversationMembershipActive,
			MembershipVersion: 0, // 权威点查先于建行；Kafka 投影到达后以正版本覆盖。
		}

		// 冲突（行已存在）时只单调推进 read_seq，绝不触碰 status/clear_seq/mute/pin。
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "owner_uuid"}, {Name: "target_uuid"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"read_seq": gorm.Expr(
					"CASE WHEN read_seq > ? THEN read_seq ELSE ? END",
					readSeq,
					readSeq,
				),
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
	effectiveReadSeq := "CASE WHEN read_seq > ? THEN read_seq ELSE ? END"
	result := db.
		Model(&model.Conversation{}).
		Where("owner_uuid = ? AND conv_id = ?", ownerUuid, convId).
		Updates(map[string]interface{}{
			"read_seq": gorm.Expr(effectiveReadSeq, readSeq, readSeq),
			"unread_count": gorm.Expr(
				"CASE WHEN max_seq > ("+effectiveReadSeq+") THEN max_seq - ("+effectiveReadSeq+") ELSE 0 END",
				readSeq,
				readSeq,
				readSeq,
				readSeq,
			),
		})
	if result.Error != nil {
		return fmt.Errorf("UpdateReadSeq: db update failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrConversationNotFound
	}
	return nil
}

// Delete 逻辑删除会话。
//
// P2P 的 max_seq 就在个人行；GROUP 的权威 max_seq 在 group_conversation。删除群会话时
// 用相关子查询取得共享位点，避免成员个人行因读扩散保持 max_seq=0 而无法真正清空历史。
func (r *repositoryImpl) Delete(ctx context.Context, ownerUuid, convId string) error {
	clearSeqExpr := gorm.Expr(
		"CASE WHEN type = 2 THEN COALESCE((SELECT max_seq FROM group_conversation WHERE group_uuid = conversation.target_uuid), max_seq) ELSE max_seq END",
	)
	result := r.db.WithContext(ctx).
		Model(&model.Conversation{}).
		Where("owner_uuid = ? AND conv_id = ?", ownerUuid, convId).
		Updates(map[string]interface{}{
			"status":       1,
			"clear_seq":    clearSeqExpr,
			"read_seq":     clearSeqExpr,
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

// UpsertGroupConv 创建或更新群会话热数据。
//
// 这里只允许消息发送路径写 max_seq/last_msg_*。group_status/projection_version
// 完全属于 group.cache projector；两个写路径共享一行但不覆盖彼此字段。
func (r *repositoryImpl) UpsertGroupConv(ctx context.Context, gc *model.GroupConversation) error {
	updatedAt := time.Now()
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "group_uuid"}},
			// 显式使用有序 clause.Set：MySQL 的单表 UPDATE 赋值按从左到右求值，
			// 因此 last_msg_* / updated_at 必须在 max_seq 之前比较“原 max_seq”。
			// SQLite 会基于旧行同时求值，同一顺序也保持等价，测试与生产无需分支。
			DoUpdates: clause.Set{
				{
					Column: clause.Column{Name: "last_msg_id"},
					Value:  gorm.Expr("CASE WHEN max_seq < ? THEN ? ELSE last_msg_id END", gc.MaxSeq, gc.LastMsgId),
				},
				{
					Column: clause.Column{Name: "last_msg_preview"},
					Value:  gorm.Expr("CASE WHEN max_seq < ? THEN ? ELSE last_msg_preview END", gc.MaxSeq, gc.LastMsgPrev),
				},
				{
					Column: clause.Column{Name: "last_msg_at"},
					Value:  gorm.Expr("CASE WHEN max_seq < ? THEN ? ELSE last_msg_at END", gc.MaxSeq, gc.LastMsgAt),
				},
				{
					Column: clause.Column{Name: "updated_at"},
					Value:  gorm.Expr("CASE WHEN max_seq < ? THEN ? ELSE updated_at END", gc.MaxSeq, updatedAt),
				},
				{
					// max_seq 最后赋值，保证上面四个表达式比较的是冲突行原值。
					Column: clause.Column{Name: "max_seq"},
					Value:  gorm.Expr("CASE WHEN max_seq < ? THEN ? ELSE max_seq END", gc.MaxSeq, gc.MaxSeq),
				},
			},
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
