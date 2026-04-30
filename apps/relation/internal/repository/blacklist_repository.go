package repository

import (
	"context"
	"errors"
	"time"

	"github.com/013677890/LCchat-Backend/model"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type blacklistRepositoryImpl struct {
	db          *gorm.DB
	redisClient *goredis.Client
}

// NewBlacklistRepository 创建黑名单仓储实例。
//
// 当前 relation-service 的最小闭环采用 DB-first 策略，redisClient 仅作为后续缓存增强
// 的预留依赖传入；即使 Redis 缺失，黑名单核心读写也应能够正常工作。
func NewBlacklistRepository(db *gorm.DB, redisClient *goredis.Client) IBlacklistRepository {
	return &blacklistRepositoryImpl{db: db, redisClient: redisClient}
}

// AddBlacklist 将 targetUUID 加入 userUUID 的黑名单。
//
// 该方法会保留“拉黑前是否为好友”的信息：
//   - status=1 表示拉黑前存在好友关系，取消拉黑时需要恢复为好友；
//   - status=3 表示拉黑前不是好友，取消拉黑时恢复为删除状态。
//
// 写入使用 Upsert，保证重复拉黑或软删除记录恢复时仍然幂等。
func (r *blacklistRepositoryImpl) AddBlacklist(ctx context.Context, userUUID, targetUUID string) error {
	now := time.Now()

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		status := int8(3)

		// 先查历史关系，决定取消拉黑时应恢复为好友还是删除状态。
		var existing model.UserRelation
		if err := tx.Unscoped().
			Select("status").
			Where("user_uuid = ? AND peer_uuid = ?", userUUID, targetUUID).
			First(&existing).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		} else if existing.Status == 0 || existing.Status == 1 || existing.Status == 2 {
			// 曾经是好友或删除好友，都按“拉黑前为好友”处理，便于取消拉黑时恢复关系。
			status = 1
		}

		relation := &model.UserRelation{
			UserUuid:      userUUID,
			PeerUuid:      targetUUID,
			Status:        status,
			BlacklistedAt: &now,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		// 使用唯一键 (user_uuid, peer_uuid) 做 Upsert，避免并发重复拉黑产生冲突。
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_uuid"}, {Name: "peer_uuid"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"status":         status,
				"deleted_at":     nil,
				"blacklisted_at": now,
				"updated_at":     now,
			}),
		}).Create(relation).Error
	})
	if err != nil {
		return WrapDBError(err)
	}
	return nil
}

// RemoveBlacklist 将 targetUUID 从 userUUID 的黑名单中移除。
//
// 根据拉黑状态恢复关系：status=1 恢复为好友；status=3 恢复为已删除的非好友关系。
// 这样可以避免“非好友取消拉黑后错误变成好友”。
func (r *blacklistRepositoryImpl) RemoveBlacklist(ctx context.Context, userUUID, targetUUID string) error {
	if userUUID == "" || targetUUID == "" {
		return ErrRecordNotFound
	}

	var relation model.UserRelation
	if err := r.db.WithContext(ctx).
		Unscoped().
		Select("status").
		Where("user_uuid = ? AND peer_uuid = ? AND status IN ?", userUUID, targetUUID, []int{1, 3}).
		First(&relation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRecordNotFound
		}
		return WrapDBError(err)
	}

	now := time.Now()
	updates := map[string]interface{}{
		"blacklisted_at": nil,
		"updated_at":     now,
	}
	if relation.Status == 1 {
		// 拉黑前是好友：取消拉黑后恢复好友关系。
		updates["status"] = 0
		updates["deleted_at"] = nil
	} else {
		// 拉黑前不是好友：取消拉黑后恢复为软删除关系，仅保留历史记录。
		updates["status"] = 2
		updates["deleted_at"] = gorm.DeletedAt{Time: now, Valid: true}
	}

	result := r.db.WithContext(ctx).
		Unscoped().
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ?", userUUID, targetUUID).
		Updates(updates)
	if result.Error != nil {
		return WrapDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}
	return nil
}

// GetBlacklistList 分页查询 userUUID 的黑名单列表。
//
// 本阶段只返回关系域拥有的数据（peer_uuid、blacklisted_at 等），头像昵称后续应通过
// user-service 的 InternalProfileService 批量补齐，避免 relation 直查 profile 表。
func (r *blacklistRepositoryImpl) GetBlacklistList(ctx context.Context, userUUID string, page, pageSize int) ([]*model.UserRelation, int64, error) {
	_, pageSize, offset := normalizePage(page, pageSize)

	query := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND status IN ? AND deleted_at IS NULL", userUUID, []int{1, 3})

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}

	var relations []*model.UserRelation
	if err := query.
		Order("blacklisted_at DESC, id DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&relations).Error; err != nil {
		return nil, 0, WrapDBError(err)
	}
	return relations, total, nil
}

// IsBlocked 判断 userUUID 是否已拉黑 targetUUID。
//
// 该方法是消息权限校验和好友申请校验的高频路径；当前先使用 MySQL 保证拆分闭环正确，
// 后续可在不改变接口语义的前提下补回 Redis cache-aside。
func (r *blacklistRepositoryImpl) IsBlocked(ctx context.Context, userUUID, targetUUID string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ? AND status IN ? AND deleted_at IS NULL", userUUID, targetUUID, []int{1, 3}).
		Count(&count).Error; err != nil {
		return false, WrapDBError(err)
	}
	return count > 0, nil
}

// GetBlacklistRelation 查询一条有效黑名单关系。
//
// 当关系不存在时返回 ErrRecordNotFound，调用方可据此映射为“不在黑名单中”的业务错误。
func (r *blacklistRepositoryImpl) GetBlacklistRelation(ctx context.Context, userUUID, targetUUID string) (*model.UserRelation, error) {
	var relation model.UserRelation
	if err := r.db.WithContext(ctx).
		Where("user_uuid = ? AND peer_uuid = ? AND status IN ? AND deleted_at IS NULL", userUUID, targetUUID, []int{1, 3}).
		First(&relation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, WrapDBError(err)
	}
	return &relation, nil
}
