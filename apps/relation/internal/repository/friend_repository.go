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

type friendRepositoryImpl struct {
	db          *gorm.DB
	redisClient *goredis.Client
}

// NewFriendRepository 创建好友关系仓储实例。
//
// 当前拆分阶段优先保证 relation-service 能独立运行，因此仓储实现采用 MySQL 为主、
// Redis 为辅的策略。redisClient 先保留在结构体中，为后续恢复缓存优化预留注入点。
func NewFriendRepository(db *gorm.DB, redisClient *goredis.Client) IFriendRepository {
	return &friendRepositoryImpl{db: db, redisClient: redisClient}
}

// normalizePage 对分页参数做统一兜底，并返回分页 offset。
//
// relation-service 的多个查询接口都共享相同的分页兜底规则，因此抽成仓储内帮助函数，
// 避免在每个查询方法中重复编写相同逻辑。
func normalizePage(page, pageSize int) (int, int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	return page, pageSize, (page - 1) * pageSize
}

// GetFriendList 分页查询用户好友列表。
//
// 该实现只返回 relation 域拥有的关系数据（peer_uuid、remark、group_tag 等），
// 头像昵称等资料字段后续由 gateway 或 relation 内部聚合 user-service 补齐。
func (r *friendRepositoryImpl) GetFriendList(ctx context.Context, userUUID, groupTag string, page, pageSize int) ([]*model.UserRelation, int64, int64, error) {
	page, pageSize, offset := normalizePage(page, pageSize)

	// 仅查询当前用户视角下仍然有效的好友关系。
	query := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, 0)
	if groupTag != "" {
		query = query.Where("group_tag = ?", groupTag)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, 0, WrapDBError(err)
	}

	var relations []*model.UserRelation
	if err := query.
		Order("created_at DESC, id DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&relations).Error; err != nil {
		return nil, 0, 0, WrapDBError(err)
	}

	// version 先使用服务端当前毫秒时间，供上层做下一次增量同步锚点。
	return relations, total, time.Now().UnixMilli(), nil
}

// GetFriendRelation 查询一条有效好友关系。
//
// 当关系不存在时返回 ErrRecordNotFound，方便 service 层统一映射为“不是好友”。
func (r *friendRepositoryImpl) GetFriendRelation(ctx context.Context, userUUID, friendUUID string) (*model.UserRelation, error) {
	var relation model.UserRelation
	if err := r.db.WithContext(ctx).
		Where("user_uuid = ? AND peer_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, friendUUID, 0).
		First(&relation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, WrapDBError(err)
	}
	return &relation, nil
}

// CreateFriendRelation 创建双向好友关系。
//
// 使用 Upsert 而不是“先查再插”的方式，目的是：
//  1. 避免并发下重复插入导致唯一键冲突；
//  2. 支持软删除或黑名单历史记录在重新加好友时被安全恢复；
//  3. 在重复同意好友申请时保持幂等。
func (r *friendRepositoryImpl) CreateFriendRelation(ctx context.Context, userUUID, friendUUID string) error {
	now := time.Now()
	relations := []*model.UserRelation{
		{UserUuid: userUUID, PeerUuid: friendUUID, Status: 0, CreatedAt: now, UpdatedAt: now},
		{UserUuid: friendUUID, PeerUuid: userUUID, Status: 0, CreatedAt: now, UpdatedAt: now},
	}

	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_uuid"}, {Name: "peer_uuid"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"status":         0,
			"deleted_at":     nil,
			"blacklisted_at": nil,
			"updated_at":     now,
		}),
	}).Create(&relations).Error; err != nil {
		return WrapDBError(err)
	}
	return nil
}

// DeleteFriendRelation 将一条单向好友关系标记为删除。
//
// relation 表是单向关系模型，因此删除好友时需要由上层分别处理 A->B 与 B->A 两条记录。
func (r *friendRepositoryImpl) DeleteFriendRelation(ctx context.Context, userUUID, friendUUID string) error {
	now := time.Now()
	result := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, friendUUID, 0).
		Updates(map[string]interface{}{
			"status":     2,
			"deleted_at": gorm.DeletedAt{Time: now, Valid: true},
			"updated_at": now,
		})
	if result.Error != nil {
		return WrapDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}
	return nil
}

// SetFriendRemark 更新好友备注。
//
// 备注是单向属性，只影响当前 userUUID 视角下看到的 peerUUID 展示信息。
func (r *friendRepositoryImpl) SetFriendRemark(ctx context.Context, userUUID, friendUUID, remark string) error {
	result := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, friendUUID, 0).
		Updates(map[string]interface{}{"remark": remark, "updated_at": time.Now()})
	if result.Error != nil {
		return WrapDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}
	return nil
}

// SetFriendTag 更新好友分组标签。
//
// 标签同样是单向属性，仅影响当前用户的好友分组视图，不修改对方视角下的数据。
func (r *friendRepositoryImpl) SetFriendTag(ctx context.Context, userUUID, friendUUID, groupTag string) error {
	result := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, friendUUID, 0).
		Updates(map[string]interface{}{"group_tag": groupTag, "updated_at": time.Now()})
	if result.Error != nil {
		return WrapDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}
	return nil
}

// GetTagList 查询当前用户已使用的好友标签列表。
//
// 当前最小闭环版本只返回标签名去重结果，不统计每个标签下的好友数量；如果上层需要
// count，可在下一步将仓储接口扩展为聚合查询。
func (r *friendRepositoryImpl) GetTagList(ctx context.Context, userUUID string) ([]string, error) {
	var tags []string
	if err := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND status = ? AND deleted_at IS NULL AND group_tag <> ''", userUUID, 0).
		Distinct().
		Pluck("group_tag", &tags).Error; err != nil {
		return nil, WrapDBError(err)
	}
	return tags, nil
}

// IsFriend 判断两人是否为好友。
//
// 这里直接复用 CheckIsFriendRelation，保持外部语义清晰，同时避免重复 SQL。
func (r *friendRepositoryImpl) IsFriend(ctx context.Context, userUUID, friendUUID string) (bool, error) {
	return r.CheckIsFriendRelation(ctx, userUUID, friendUUID)
}

// CheckIsFriendRelation 判断当前 userUUID 视角下是否存在有效好友关系。
//
// 由于 relation 表是单向建模，因此这里只检查单侧关系是否存在，不隐式推断对向记录。
func (r *friendRepositoryImpl) CheckIsFriendRelation(ctx context.Context, userUUID, peerUUID string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, peerUUID, 0).
		Count(&count).Error; err != nil {
		return false, WrapDBError(err)
	}
	return count > 0, nil
}

// BatchCheckIsFriend 批量判断多名用户是否为当前用户的好友。
//
// 返回 map 的好处是上层可以按请求顺序重新组织结果，同时对未命中的 peer_uuid 自动视为 false。
func (r *friendRepositoryImpl) BatchCheckIsFriend(ctx context.Context, userUUID string, peerUUIDs []string) (map[string]bool, error) {
	result := make(map[string]bool, len(peerUUIDs))
	if len(peerUUIDs) == 0 {
		return result, nil
	}
	for _, peerUUID := range peerUUIDs {
		result[peerUUID] = false
	}

	var relations []model.UserRelation
	if err := r.db.WithContext(ctx).
		Select("peer_uuid").
		Where("user_uuid = ? AND peer_uuid IN ? AND status = ? AND deleted_at IS NULL", userUUID, peerUUIDs, 0).
		Find(&relations).Error; err != nil {
		return nil, WrapDBError(err)
	}
	for _, relation := range relations {
		result[relation.PeerUuid] = true
	}
	return result, nil
}

// GetRelationStatus 查询一条历史关系状态。
//
// 这里使用 Unscoped 是为了让 service 层能够区分：
//  1. 从未存在过关系；
//  2. 曾经是好友但已删除；
//  3. 当前处于黑名单状态。
func (r *friendRepositoryImpl) GetRelationStatus(ctx context.Context, userUUID, peerUUID string) (*model.UserRelation, error) {
	var relation model.UserRelation
	if err := r.db.WithContext(ctx).
		Unscoped().
		Where("user_uuid = ? AND peer_uuid = ?", userUUID, peerUUID).
		First(&relation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, WrapDBError(err)
	}
	return &relation, nil
}

// SyncFriendList 按更新时间做好友关系增量同步。
//
// 该方法返回按 updated_at 升序排列的变更集，供 service 层转成 add/update/delete 三类
// 变化事件。limit 额外多查一条以判断是否还有下一页，从而避免单独执行 count 查询。
func (r *friendRepositoryImpl) SyncFriendList(ctx context.Context, userUUID string, version int64, limit int) ([]*model.UserRelation, int64, bool, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	changedAfter := time.UnixMilli(0)
	if version > 0 {
		changedAfter = time.UnixMilli(version)
	}

	var relations []*model.UserRelation
	// 使用 Unscoped 保留已删除关系，这样客户端才能收到 delete 类型的增量事件。
	if err := r.db.WithContext(ctx).
		Unscoped().
		Where("user_uuid = ? AND updated_at > ?", userUUID, changedAfter).
		Order("updated_at ASC, id ASC").
		Limit(limit + 1).
		Find(&relations).Error; err != nil {
		return nil, version, false, WrapDBError(err)
	}

	// 多查一条判断是否还有更多，避免追加一次 count 查询。
	hasMore := len(relations) > limit
	if hasMore {
		relations = relations[:limit]
	}

	// latestVersion 取当前批次中最大的 updated_at，供上层作为下一次同步游标。
	latestVersion := version
	for _, relation := range relations {
		if relation != nil && relation.UpdatedAt.UnixMilli() > latestVersion {
			latestVersion = relation.UpdatedAt.UnixMilli()
		}
	}

	return relations, latestVersion, hasMore, nil
}
