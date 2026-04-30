package repository

import (
	"context"
	"time"

	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/async"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type applyRepositoryImpl struct {
	db          *gorm.DB
	redisClient *goredis.Client
}

// NewApplyRepository 创建好友申请仓储实例。
//
// 当前拆分阶段先保证申请读写、状态流转、幂等同意流程在 relation-service 内闭环，
// redisClient 暂时仅作为后续恢复缓存和未读数优化的依赖预留。
func NewApplyRepository(db *gorm.DB, redisClient *goredis.Client) IApplyRepository {
	return &applyRepositoryImpl{db: db, redisClient: redisClient}
}

// Create 创建一条好友申请记录。
//
// 当前最小实现直接落 MySQL；后续若恢复 Redis 待处理列表缓存，可在此处追加异步回填。
func (r *applyRepositoryImpl) Create(ctx context.Context, apply *model.ApplyRequest) (*model.ApplyRequest, error) {
	if err := r.db.WithContext(ctx).Create(apply).Error; err != nil {
		return nil, WrapDBError(err)
	}
	return apply, nil
}

// GetByID 按主键查询一条好友申请。
//
// 仅查询好友申请（apply_type=0），避免与未来加群申请共用表后的数据混淆。
func (r *applyRepositoryImpl) GetByID(ctx context.Context, id int64) (*model.ApplyRequest, error) {
	var apply model.ApplyRequest
	if err := r.db.WithContext(ctx).
		Where("id = ? AND apply_type = ? AND deleted_at IS NULL", id, 0).
		First(&apply).Error; err != nil {
		return nil, WrapDBError(err)
	}
	return &apply, nil
}

// GetPendingList 分页查询当前用户收到的好友申请列表。
//
// status >= 0 时按指定状态过滤；status < 0 时返回待处理/已同意/已拒绝的全部状态。
func (r *applyRepositoryImpl) GetPendingList(ctx context.Context, targetUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error) {
	page, pageSize, offset := normalizePage(page, pageSize)
	query := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("apply_type = ? AND target_uuid = ? AND deleted_at IS NULL", 0, targetUUID)
	if status >= 0 {
		query = query.Where("status = ?", status)
	} else {
		query = query.Where("status IN ?", []int{0, 1, 2})
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}

	var applies []*model.ApplyRequest
	if err := query.
		Order("created_at DESC, id DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&applies).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}
	return applies, total, nil
}

// GetSentList 分页查询当前用户发出的好友申请列表。
//
// 该查询与 GetPendingList 对称，只是视角从 target_uuid 切换为 applicant_uuid。
func (r *applyRepositoryImpl) GetSentList(ctx context.Context, applicantUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error) {
	page, pageSize, offset := normalizePage(page, pageSize)
	query := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("apply_type = ? AND applicant_uuid = ? AND deleted_at IS NULL", 0, applicantUUID)
	if status >= 0 {
		query = query.Where("status = ?", status)
	} else {
		query = query.Where("status IN ?", []int{0, 1, 2})
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}

	var applies []*model.ApplyRequest
	if err := query.
		Order("created_at DESC, id DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&applies).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}
	return applies, total, nil
}

// UpdateStatus 更新申请状态。
//
// 这里只允许更新当前仍为待处理的申请，依靠 WHERE status=0 保证幂等：
// 重复同意或拒绝时不会再次修改历史记录。
func (r *applyRepositoryImpl) UpdateStatus(ctx context.Context, id int64, status int, remark string) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}
	if remark != "" {
		updates["handle_remark"] = remark
	}

	result := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("id = ? AND apply_type = ? AND status = ?", id, 0, 0).
		Updates(updates)
	if result.Error != nil {
		return WrapDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrApplyNotFound
	}
	return nil
}

// AcceptApplyAndCreateRelation 同意好友申请并在同事务内建立双向好友关系。
//
// 事务内包含两类写操作：
//  1. 将申请从待处理 CAS 更新为已同意；
//  2. 对 A->B / B->A 两条关系执行 Upsert，保证重复请求下仍然安全。
//
// alreadyProcessed=true 表示申请已被其他并发请求处理，调用方应按幂等成功处理。
func (r *applyRepositoryImpl) AcceptApplyAndCreateRelation(ctx context.Context, applyId int64, userUUID, friendUUID, remark string) (bool, error) {
	now := time.Now()
	var alreadyProcessed bool

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 第一步：仅允许从待处理状态流转到已同意状态。
		updates := map[string]interface{}{
			"status":           1,
			"handle_user_uuid": userUUID,
			"updated_at":       now,
		}
		if remark != "" {
			updates["handle_remark"] = remark
		}

		result := tx.Model(&model.ApplyRequest{}).
			Where("id = ? AND apply_type = ? AND status = ?", applyId, 0, 0).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			alreadyProcessed = true
			return nil
		}

		// 第二步：写入 userUUID -> friendUUID 方向的关系，保留当前处理人填写的 remark。
		relationAB := &model.UserRelation{
			UserUuid:  userUUID,
			PeerUuid:  friendUUID,
			Status:    0,
			Remark:    remark,
			CreatedAt: now,
			UpdatedAt: now,
		}
		abUpdates := map[string]interface{}{
			"status":         0,
			"deleted_at":     nil,
			"blacklisted_at": nil,
			"updated_at":     now,
		}
		if remark != "" {
			abUpdates["remark"] = remark
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_uuid"}, {Name: "peer_uuid"}},
			DoUpdates: clause.Assignments(abUpdates),
		}).Create(relationAB).Error; err != nil {
			return err
		}

		// 第三步：写入对向关系，但不覆盖对方未来可能单独维护的 remark。
		relationBA := &model.UserRelation{
			UserUuid:  friendUUID,
			PeerUuid:  userUUID,
			Status:    0,
			CreatedAt: now,
			UpdatedAt: now,
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_uuid"}, {Name: "peer_uuid"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"status":         0,
				"deleted_at":     nil,
				"blacklisted_at": nil,
				"updated_at":     now,
			}),
		}).Create(relationBA).Error
	})
	if err != nil {
		return false, WrapDBError(err)
	}
	return alreadyProcessed, nil
}

// MarkAsRead 将指定申请标记为已读。
//
// 这里仍限定 target_uuid，避免用户越权把其他人的申请记录改成已读。
func (r *applyRepositoryImpl) MarkAsRead(ctx context.Context, targetUUID string, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("apply_type = ? AND target_uuid = ? AND id IN ? AND deleted_at IS NULL", 0, targetUUID, ids).
		Updates(map[string]interface{}{"is_read": true, "updated_at": time.Now()})
	if result.Error != nil {
		return 0, WrapDBError(result.Error)
	}
	return result.RowsAffected, nil
}

// MarkAllAsRead 将当前用户收到的全部未读好友申请标记为已读。
func (r *applyRepositoryImpl) MarkAllAsRead(ctx context.Context, targetUUID string) (int64, error) {
	result := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("apply_type = ? AND target_uuid = ? AND is_read = ? AND deleted_at IS NULL", 0, targetUUID, false).
		Updates(map[string]interface{}{"is_read": true, "updated_at": time.Now()})
	if result.Error != nil {
		return 0, WrapDBError(result.Error)
	}
	return result.RowsAffected, nil
}

// MarkAsReadAsync 异步将申请标记为已读。
//
// 该接口用于“读取申请列表后顺手消已读”的尽力而为场景：失败不影响主流程返回。
func (r *applyRepositoryImpl) MarkAsReadAsync(ctx context.Context, ids []int64) {
	if len(ids) == 0 {
		return
	}
	async.RunSafe(ctx, func(runCtx context.Context) {
		// 这里不返回错误给调用方，只做后台 best-effort 更新。
		_ = r.db.WithContext(runCtx).
			Model(&model.ApplyRequest{}).
			Where("id IN ? AND apply_type = ?", ids, 0).
			Updates(map[string]interface{}{"is_read": true, "updated_at": time.Now()}).Error
	}, async.AsyncDBTimeout)
}

// GetUnreadCount 统计当前用户未读的好友申请数量。
func (r *applyRepositoryImpl) GetUnreadCount(ctx context.Context, targetUUID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("apply_type = ? AND target_uuid = ? AND is_read = ? AND deleted_at IS NULL", 0, targetUUID, false).
		Count(&count).Error; err != nil {
		return 0, WrapDBError(err)
	}
	return count, nil
}

// ClearUnreadCount 清理未读状态。
//
// 在当前 DB-first 实现里，清理未读等价于将所有未读申请批量置为已读。
func (r *applyRepositoryImpl) ClearUnreadCount(ctx context.Context, targetUUID string) error {
	_, err := r.MarkAllAsRead(ctx, targetUUID)
	return err
}

// ExistsPendingRequest 判断同一申请人对同一目标用户是否已存在待处理申请。
//
// 该检查用于发送申请前去重，避免用户短时间内重复创建多条待处理记录。
func (r *applyRepositoryImpl) ExistsPendingRequest(ctx context.Context, applicantUUID, targetUUID string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.ApplyRequest{}).
		Where("apply_type = ? AND applicant_uuid = ? AND target_uuid = ? AND status = ? AND deleted_at IS NULL", 0, applicantUUID, targetUUID, 0).
		Count(&count).Error; err != nil {
		return false, WrapDBError(err)
	}
	return count > 0, nil
}

// GetByIDWithInfo 返回带基础申请信息的记录。
//
// 目前 relation-service 还未引入 profile 聚合逻辑，因此先直接复用 GetByID，保证接口语义完整。
func (r *applyRepositoryImpl) GetByIDWithInfo(ctx context.Context, id int64) (*model.ApplyRequest, error) {
	return r.GetByID(ctx, id)
}
