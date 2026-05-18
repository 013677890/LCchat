package repository

import (
	"context"
	"errors"
	"time"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/async"
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
// relation-service 中好友关系属于高频读路径，因此这里恢复为“Redis Hash + MySQL”双层
// 结构：Redis 负责常见存在性判断与元数据读取，MySQL 继续作为最终权威来源兜底。
func NewFriendRepository(db *gorm.DB, redisClient *goredis.Client) IFriendRepository {
	return &friendRepositoryImpl{db: db, redisClient: redisClient}
}

// normalizePage 对分页参数做统一兜底，并返回分页 offset。
//
// relation-service 的多个查询接口都共享相同的分页兜底规则，因此抽成仓储内帮助函数，
// 避免在每个查询方法中重复编写相同逻辑。
func normalizePage(page, pageSize int) (int, int, int) {
	// page/pageSize 都采用最小兜底值，避免上层遗漏参数时生成负 offset。
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
	// group_tag 非空时继续收窄到某个分组视图。
	if groupTag != "" {
		query = query.Where("group_tag = ?", groupTag)
	}

	var total int64
	// 先 count 再分页查询，让上层能直接返回完整分页信息。
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, 0, WrapDBError(err)
	}

	var relations []*model.UserRelation
	// 按 created_at/id 倒序返回，兼容旧单体好友列表的展示顺序。
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
	// 好友关系是双向模型，因此一次性准备 A->B 与 B->A 两条记录。
	relations := []*model.UserRelation{
		{UserUuid: userUUID, PeerUuid: friendUUID, Status: 0, CreatedAt: now, UpdatedAt: now},
		{UserUuid: friendUUID, PeerUuid: userUUID, Status: 0, CreatedAt: now, UpdatedAt: now},
	}

	// Upsert 时统一恢复 status / deleted_at / blacklisted_at，兼容“删好友后重新添加”场景。
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

	// 事务成功后只做“缓存存在时”的增量补丁；若 key 已过期，则后续读路径会负责全量重建。
	r.invalidateFriendCacheAsync(ctx, userUUID, friendUUID)
	return nil
}

// DeleteFriendRelation 将一条单向好友关系标记为删除。
//
// relation 表是单向关系模型，因此删除好友时需要由上层分别处理 A->B 与 B->A 两条记录。
func (r *friendRepositoryImpl) DeleteFriendRelation(ctx context.Context, userUUID, friendUUID string) error {
	now := time.Now()
	// 删除好友本质上是把单向关系标记为 status=2 + deleted_at，有历史可追溯但不再算有效好友。
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
	// 没有命中任何有效好友关系时，返回 ErrRecordNotFound 供 service 层映射业务错误。
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}

	// 删除好友只影响当前用户单侧视角，因此这里只移除当前 userUUID 对应的 Hash field。
	r.removeFriendCacheAsync(ctx, userUUID, friendUUID)
	return nil
}

// CleanupAccountRelations 在账号注销后清理 relation 域中与该用户相关的全部关系记录。
//
// relation 域当前统一使用 user_relations 维护好友/黑名单关系，这里按“软删除 + 缓存失效”处理，
// 避免继续向外暴露已注销账号的关系数据。
func (r *friendRepositoryImpl) CleanupAccountRelations(ctx context.Context, userUUID string) error {
	if userUUID == "" {
		return nil
	}

	type affectedRow struct {
		UserUUID string `gorm:"column:user_uuid"`
	}

	var rows []affectedRow
	if err := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Select("DISTINCT user_uuid").
		Where("(user_uuid = ? OR peer_uuid = ?) AND deleted_at IS NULL", userUUID, userUUID).
		Find(&rows).Error; err != nil {
		return WrapDBError(err)
	}

	now := time.Now()
	if err := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("(user_uuid = ? OR peer_uuid = ?) AND deleted_at IS NULL", userUUID, userUUID).
		Updates(map[string]interface{}{
			"status":         2,
			"blacklisted_at": nil,
			"deleted_at":     gorm.DeletedAt{Time: now, Valid: true},
			"updated_at":     now,
		}).Error; err != nil {
		return WrapDBError(err)
	}

	affectedUsers := make([]string, 0, len(rows)+1)
	affectedUsers = append(affectedUsers, userUUID)
	for _, row := range rows {
		if row.UserUUID == "" || row.UserUUID == userUUID {
			continue
		}
		affectedUsers = append(affectedUsers, row.UserUUID)
	}
	r.invalidateRelationCachesAsync(ctx, affectedUsers)
	return nil
}

// SetFriendRemark 更新好友备注。
//
// 备注是单向属性，只影响当前 userUUID 视角下看到的 peerUUID 展示信息。
func (r *friendRepositoryImpl) SetFriendRemark(ctx context.Context, userUUID, friendUUID, remark string) error {
	now := time.Now()
	result := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, friendUUID, 0).
		Updates(map[string]interface{}{"remark": remark, "updated_at": now})
	if result.Error != nil {
		return WrapDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}

	// 备注是单条元数据修改，优先走单 field 增量回写，避免整份好友列表重建。
	r.updateFriendRemarkCacheAsync(ctx, userUUID, friendUUID, remark, now.UnixMilli())
	return nil
}

// SetFriendTag 更新好友分组标签。
//
// 标签同样是单向属性，仅影响当前用户的好友分组视图，不修改对方视角下的数据。
func (r *friendRepositoryImpl) SetFriendTag(ctx context.Context, userUUID, friendUUID, groupTag string) error {
	now := time.Now()
	result := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, friendUUID, 0).
		Updates(map[string]interface{}{"group_tag": groupTag, "updated_at": now})
	if result.Error != nil {
		return WrapDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}

	// 标签与备注一样都是好友元数据的一部分，可以在已有 Hash 上做细粒度更新。
	r.updateFriendTagCacheAsync(ctx, userUUID, friendUUID, groupTag, now.UnixMilli())
	return nil
}

// IsFriend 判断两人是否为好友。
//
// 对外语义仍保持不变，但内部会优先命中 Redis Hash，只有缓存未命中时才回源 MySQL。
func (r *friendRepositoryImpl) IsFriend(ctx context.Context, userUUID, friendUUID string) (bool, error) {
	return r.CheckIsFriendRelation(ctx, userUUID, friendUUID)
}

// checkFriendCache 检查单侧好友缓存命中情况。
//
// 返回值依次表示：
//  1. 当前用户的好友 Hash 是否存在；
//  2. 若 Hash 存在，对方是否在该 Hash 中。
//
// 之所以区分“缓存存在但字段不存在”和“缓存整体不存在”，是为了让调用方能把前者视为
// 权威的 false，后者则继续回源数据库，避免把缓存未命中误判成业务不存在。
func (r *friendRepositoryImpl) checkFriendCache(ctx context.Context, userUUID, friendUUID string) (bool, bool) {
	if r.redisClient == nil || userUUID == "" || friendUUID == "" {
		return false, false
	}

	cacheKey := rediskey.FriendRelationKey(userUUID)
	pipe := r.redisClient.Pipeline()
	// exists 用来区分“整个 Hash 不存在”和“Hash 存在但 field 缺失”两种情况。
	existsCmd := pipe.Exists(ctx, cacheKey)
	metaCmd := pipe.HGet(ctx, cacheKey, friendUUID)

	// 热点 key 使用小概率续期，避免每次读取都追加 EXPIRE 带来额外 RTT。
	if getRandomBool(0.01) {
		pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.FriendRelationTTL))
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != goredis.Nil {
		// key 类型错乱时直接删除，让后续读路径重新回源重建正确结构。
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
		} else {
			LogRedisError(ctx, err)
		}
		return false, false
	}

	if existsCmd.Val() == 0 {
		return false, false
	}

	if metaCmd.Err() == nil {
		// 这里只需要“该 field 是否存在”的结论；即使 JSON 解析失败，也仍按好友存在处理。
		_, _ = parseFriendMetaJSON(metaCmd.Val())
		return true, true
	}
	if metaCmd.Err() == goredis.Nil {
		return true, false
	}
	if isRedisWrongType(metaCmd.Err()) {
		_ = r.redisClient.Del(ctx, cacheKey).Err()
		return false, false
	}

	LogRedisError(ctx, metaCmd.Err())
	return false, false
}

// getFriendMetaCache 获取某个好友 field 的缓存元数据。
//
// 与 checkFriendCache 类似，该方法同样区分“Hash 存在但 field 缺失”和“整个 Hash 不存在”；
// 这样 GetRelationStatus 既能利用缓存补 remark/group_tag/source，又不会误把冷数据查成空。
func (r *friendRepositoryImpl) getFriendMetaCache(ctx context.Context, userUUID, friendUUID string) (bool, *friendMeta, bool) {
	if r.redisClient == nil || userUUID == "" || friendUUID == "" {
		return false, nil, false
	}

	cacheKey := rediskey.FriendRelationKey(userUUID)
	pipe := r.redisClient.Pipeline()
	existsCmd := pipe.Exists(ctx, cacheKey)
	metaCmd := pipe.HGet(ctx, cacheKey, friendUUID)

	if getRandomBool(0.01) {
		pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.FriendRelationTTL))
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != goredis.Nil {
		// 和 checkFriendCache 一样，类型错乱时直接删 key，让后续读路径做全量修复。
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
		} else {
			LogRedisError(ctx, err)
		}
		return false, nil, false
	}

	if existsCmd.Val() == 0 {
		return false, nil, false
	}

	if metaCmd.Err() == nil {
		// field 存在时进一步解析出 remark/group_tag/source 等元数据供状态接口复用。
		meta, parseErr := parseFriendMetaJSON(metaCmd.Val())
		if parseErr != nil {
			// JSON 坏掉时仍把“是好友”这个事实返回给上层，避免状态判断被缓存脏数据完全打断。
			return true, nil, true
		}
		return true, meta, true
	}
	if metaCmd.Err() == goredis.Nil {
		return true, nil, false
	}
	if isRedisWrongType(metaCmd.Err()) {
		_ = r.redisClient.Del(ctx, cacheKey).Err()
		return false, nil, false
	}

	LogRedisError(ctx, metaCmd.Err())
	return false, nil, false
}

// checkBlacklistCache 检查黑名单缓存命中情况。
//
// 好友关系状态查询需要同时感知好友 Hash 与黑名单 ZSet，因此这里直接复用黑名单缓存，避免
// 为 GetRelationStatus 额外追加一次数据库访问。
func (r *friendRepositoryImpl) checkBlacklistCache(ctx context.Context, userUUID, peerUUID string) (bool, bool) {
	if r.redisClient == nil || userUUID == "" || peerUUID == "" {
		return false, false
	}

	cacheKey := rediskey.BlacklistRelationKey(userUUID)
	pipe := r.redisClient.Pipeline()
	existsCmd := pipe.Exists(ctx, cacheKey)
	scoreCmd := pipe.ZScore(ctx, cacheKey, peerUUID)

	if getRandomBool(0.01) {
		pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.BlacklistTTL))
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != goredis.Nil {
		// blacklist ZSet 类型异常时同样删 key，让后续读路径重建。
		if isRedisWrongType(err) {
			_ = r.redisClient.Del(ctx, cacheKey).Err()
		} else {
			LogRedisError(ctx, err)
		}
		return false, false
	}

	if existsCmd.Val() == 0 {
		return false, false
	}
	if scoreCmd.Err() == nil {
		return true, true
	}
	if scoreCmd.Err() == goredis.Nil {
		return true, false
	}
	if isRedisWrongType(scoreCmd.Err()) {
		_ = r.redisClient.Del(ctx, cacheKey).Err()
		return false, false
	}

	LogRedisError(ctx, scoreCmd.Err())
	return false, false
}

// CheckIsFriendRelation 判断当前 userUUID 视角下是否存在有效好友关系。
//
// 由于 relation 表是单向建模，因此这里只检查单侧关系是否存在，不隐式推断对向记录。
func (r *friendRepositoryImpl) CheckIsFriendRelation(ctx context.Context, userUUID, peerUUID string) (bool, error) {
	cacheHit, isFriend := r.checkFriendCache(ctx, userUUID, peerUUID)
	if cacheHit {
		return isFriend, nil
	}

	var count int64
	// 缓存整体缺失时才回源 DB；单侧关系判断仍只看 userUUID 当前视角这条记录。
	if err := r.db.WithContext(ctx).
		Model(&model.UserRelation{}).
		Where("user_uuid = ? AND peer_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, peerUUID, 0).
		Count(&count).Error; err != nil {
		return false, WrapDBError(err)
	}

	// 缓存未命中时异步重建整份好友 Hash，避免后续同用户的高频关系判断持续打 DB。
	r.rebuildFriendCacheFromDBAsync(ctx, userUUID)
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
	// 先把所有请求对象初始化成 false，后续命中缓存或数据库时再改成 true。
	for _, peerUUID := range peerUUIDs {
		result[peerUUID] = false
	}

	if r.redisClient != nil {
		cacheKey := rediskey.FriendRelationKey(userUUID)
		fields := make([]string, 0, len(peerUUIDs))
		for _, peerUUID := range peerUUIDs {
			fields = append(fields, peerUUID)
		}

		pipe := r.redisClient.Pipeline()
		existsCmd := pipe.Exists(ctx, cacheKey)
		valuesCmd := pipe.HMGet(ctx, cacheKey, fields...)
		if getRandomBool(0.01) {
			pipe.Expire(ctx, cacheKey, getRandomExpireTime(rediskey.FriendRelationTTL))
		}

		_, err := pipe.Exec(ctx)
		if err == nil {
			if existsCmd.Val() > 0 {
				// Hash 已存在时，HMGet 中非 nil 的 field 统一视为好友命中。
				for index, value := range valuesCmd.Val() {
					if value != nil {
						result[peerUUIDs[index]] = true
					}
				}
				return result, nil
			}
		} else if err != goredis.Nil {
			if isRedisWrongType(err) {
				_ = r.redisClient.Del(ctx, cacheKey).Err()
			} else {
				LogRedisError(ctx, err)
			}
		}
	}

	var relations []model.UserRelation
	// 缓存没命中时，按 peer_uuid IN 批量回源数据库，再把命中的关系回填到结果 map。
	if err := r.db.WithContext(ctx).
		Select("peer_uuid").
		Where("user_uuid = ? AND peer_uuid IN ? AND status = ? AND deleted_at IS NULL", userUUID, peerUUIDs, 0).
		Find(&relations).Error; err != nil {
		return nil, WrapDBError(err)
	}
	for _, relation := range relations {
		result[relation.PeerUuid] = true
	}

	// 批量校验通常紧跟着大量会话/资料卡展示，命中 DB 后顺手重建一次缓存最划算。
	r.rebuildFriendCacheFromDBAsync(ctx, userUUID)
	return result, nil
}

// GetRelationStatus 查询一条历史关系状态。
//
// 这里使用 Unscoped 是为了让 service 层能够区分：
//  1. 从未存在过关系；
//  2. 曾经是好友但已删除；
//  3. 当前处于黑名单状态。
func (r *friendRepositoryImpl) GetRelationStatus(ctx context.Context, userUUID, peerUUID string) (*model.UserRelation, error) {
	friendHit, meta, isFriend := r.getFriendMetaCache(ctx, userUUID, peerUUID)
	if friendHit && isFriend {
		// 好友缓存命中时直接构造 status=0 的关系对象，避免继续访问数据库。
		relation := &model.UserRelation{
			UserUuid: userUUID,
			PeerUuid: peerUUID,
			Status:   0,
		}
		if meta != nil {
			relation.Remark = meta.Remark
			relation.GroupTag = meta.GroupTag
			relation.Source = meta.Source
		}
		return relation, nil
	}

	blacklistHit, isBlacklist := r.checkBlacklistCache(ctx, userUUID, peerUUID)
	if blacklistHit && isBlacklist {
		// 黑名单缓存命中时只需要返回 status=1 的最小关系视图。
		return &model.UserRelation{UserUuid: userUUID, PeerUuid: peerUUID, Status: 1}, nil
	}

	// 当好友和黑名单缓存都明确给出“不存在”时，可直接认定两者没有有效关系。
	if friendHit && !isFriend && blacklistHit && !isBlacklist {
		return nil, nil
	}

	var relation model.UserRelation
	// 缓存无法给出确定答案时，最后再 Unscoped 回源数据库拿历史关系真值。
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
	// 同步窗口默认值与上限都在仓储内收口，避免上层把过大 limit 直接打进数据库。
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	changedAfter := time.UnixMilli(0)
	if version > 0 {
		// version>0 时表示增量同步，从该时间点之后开始找变化。
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

// rebuildFriendCacheFromDBAsync 异步从数据库重建某个用户的整份好友 Hash。
//
// 只有整份重建才能保证 Hash 中的 field 集合完整，因此缓存缺失时不做“单条补丁式写入”，
// 而是统一回源当前用户全部好友关系后一次性替换。
func (r *friendRepositoryImpl) rebuildFriendCacheFromDBAsync(ctx context.Context, userUUID string) {
	if r.redisClient == nil || userUUID == "" {
		return
	}

	async.RunSafe(ctx, func(runCtx context.Context) {
		var relations []model.UserRelation
		// 全量取出当前用户所有有效好友关系，作为重建整份 Hash 的唯一事实来源。
		err := r.db.WithContext(runCtx).
			Select("peer_uuid", "remark", "group_tag", "source", "updated_at").
			Where("user_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, 0).
			Find(&relations).Error
		if err != nil {
			return
		}
		r.rebuildFriendCacheAsync(runCtx, userUUID, relations)
	}, async.AsyncDBTimeout)
}

// invalidateFriendCacheAsync 异步把新好友补进双方的好友 Hash。
//
// 这里只在目标 Hash 已存在时做增量插入；如果整个 key 已过期，则让下一次读请求回源 DB
// 并全量重建，避免把“不完整的好友集合”误写成缓存事实。
func (r *friendRepositoryImpl) invalidateFriendCacheAsync(ctx context.Context, userUUID, friendUUID string) {
	if r.redisClient == nil || userUUID == "" || friendUUID == "" {
		return
	}

	async.RunSafe(ctx, func(runCtx context.Context) {
		pairs := []struct {
			userKey   string
			newFriend string
		}{
			{userKey: rediskey.FriendRelationKey(userUUID), newFriend: friendUUID},
			{userKey: rediskey.FriendRelationKey(friendUUID), newFriend: userUUID},
		}

		metaJSON := buildFriendMetaJSON("", "", "", time.Now().UnixMilli())
		expireSeconds := int(getRandomExpireTime(rediskey.FriendRelationTTL).Seconds())
		luaScript := goredis.NewScript(luaInsertFriendMetaIfExists)

		for _, pair := range pairs {
			// 只有 key 已存在时才补丁插入 field；否则让下一次读路径统一做全量重建。
			_, err := luaScript.Run(runCtx, r.redisClient,
				[]string{pair.userKey},
				pair.newFriend,
				metaJSON,
				expireSeconds,
			).Result()
			if err != nil && err != goredis.Nil {
				if isRedisWrongType(err) {
					_ = r.redisClient.Del(runCtx, pair.userKey).Err()
					continue
				}
				LogRedisError(runCtx, err)
			}
		}
	}, async.AsyncRedisTimeout)
}

// removeFriendCacheAsync 异步删除单向好友缓存。
//
// 删除好友只影响当前用户视角，因此这里只移除 userUUID -> friendUUID 这一侧的 Hash field。
func (r *friendRepositoryImpl) removeFriendCacheAsync(ctx context.Context, userUUID, friendUUID string) {
	if r.redisClient == nil || userUUID == "" || friendUUID == "" {
		return
	}

	cacheKey := rediskey.FriendRelationKey(userUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		luaScript := goredis.NewScript(luaRemoveFriendMetaIfExists)
		placeholderJSON := buildFriendMetaJSON("", "", "", 0)
		expireSeconds := int(getRandomExpireTime(rediskey.FriendRelationTTL).Seconds())
		_, err := luaScript.Run(runCtx, r.redisClient,
			[]string{cacheKey},
			friendUUID,
			placeholderJSON,
			expireSeconds,
		).Result()

		if err != nil && err != goredis.Nil {
			if isRedisWrongType(err) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
				return
			}
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)
}

// rebuildFriendCacheAsync 异步重建某个用户的好友 Hash。
//
// 该方法只负责把调用方已经拿到的完整关系集合写入 Redis；真正决定“是否需要全量回源”
// 的逻辑由上层 read path 控制，以便把缓存成本放在首次 miss 上摊平。
func (r *friendRepositoryImpl) rebuildFriendCacheAsync(ctx context.Context, userUUID string, relations []model.UserRelation) {
	if r.redisClient == nil || userUUID == "" {
		return
	}

	cacheKey := rediskey.FriendRelationKey(userUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		pipe := r.redisClient.Pipeline()
		pipe.Del(runCtx, cacheKey)

		if len(relations) == 0 {
			pipe.HSet(runCtx, cacheKey, "__EMPTY__", buildFriendMetaJSON("", "", "", 0))
			pipe.Expire(runCtx, cacheKey, rediskey.FriendRelationEmptyTTL)
		} else {
			fields := make(map[string]interface{}, len(relations))
			for _, relation := range relations {
				if relation.PeerUuid == "" {
					continue
				}
				fields[relation.PeerUuid] = buildFriendMetaJSON(
					relation.Remark,
					relation.GroupTag,
					relation.Source,
					relation.UpdatedAt.UnixMilli(),
				)
			}
			if len(fields) > 0 {
				pipe.HSet(runCtx, cacheKey, fields)
			}
			// 非空好友集使用常规 TTL，后续靠小概率续期维持热点 key。
			pipe.Expire(runCtx, cacheKey, getRandomExpireTime(rediskey.FriendRelationTTL))
		}

		if _, err := pipe.Exec(runCtx); err != nil && err != goredis.Nil {
			if isRedisWrongType(err) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
				return
			}
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisPipelineTimeout)
}

func (r *friendRepositoryImpl) invalidateRelationCachesAsync(ctx context.Context, userUUIDs []string) {
	if r.redisClient == nil || len(userUUIDs) == 0 {
		return
	}

	async.RunSafe(ctx, func(runCtx context.Context) {
		seen := make(map[string]struct{}, len(userUUIDs))
		pipe := r.redisClient.Pipeline()
		for _, userUUID := range userUUIDs {
			if userUUID == "" {
				continue
			}
			if _, ok := seen[userUUID]; ok {
				continue
			}
			seen[userUUID] = struct{}{}
			pipe.Del(runCtx, rediskey.FriendRelationKey(userUUID))
			pipe.Del(runCtx, rediskey.BlacklistRelationKey(userUUID))
		}
		if _, err := pipe.Exec(runCtx); err != nil && err != goredis.Nil {
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisPipelineTimeout)
}

// updateFriendMetaCacheAsync 异步更新单个好友的元数据缓存。
//
// 该方法用于 remark/group_tag/source 等单条关系属性变更后做增量回写，前提仍然是目标 Hash
// 已存在；若 key 不存在，说明缓存已过期，后续读路径会全量重建。
func (r *friendRepositoryImpl) updateFriendMetaCacheAsync(ctx context.Context, userUUID string, relation *model.UserRelation) {
	if r.redisClient == nil || userUUID == "" || relation == nil || relation.PeerUuid == "" {
		return
	}

	cacheKey := rediskey.FriendRelationKey(userUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		metaJSON := buildFriendMetaJSON(
			relation.Remark,
			relation.GroupTag,
			relation.Source,
			relation.UpdatedAt.UnixMilli(),
		)
		expireSeconds := int(getRandomExpireTime(rediskey.FriendRelationTTL).Seconds())
		luaScript := goredis.NewScript(luaUpsertFriendMetaIfExists)
		_, err := luaScript.Run(runCtx, r.redisClient,
			[]string{cacheKey},
			relation.PeerUuid,
			metaJSON,
			expireSeconds,
		).Result()
		if err != nil && err != goredis.Nil {
			if isRedisWrongType(err) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
				return
			}
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRedisTimeout)
}

// updateFriendRemarkCacheAsync 异步更新单个好友的备注缓存。
//
// 如果 Hash 已存在但 field 缺失，说明缓存已不是最新完整集合，此时会回源 DB 读取整条关系
// 元数据后再尝试回写，避免把只含 remark 的“半条记录”塞进缓存。
func (r *friendRepositoryImpl) updateFriendRemarkCacheAsync(ctx context.Context, userUUID, friendUUID, remark string, updatedAt int64) {
	if r.redisClient == nil || userUUID == "" || friendUUID == "" {
		return
	}

	cacheKey := rediskey.FriendRelationKey(userUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		pipe := r.redisClient.Pipeline()
		existsCmd := pipe.Exists(runCtx, cacheKey)
		metaCmd := pipe.HGet(runCtx, cacheKey, friendUUID)
		_, err := pipe.Exec(runCtx)

		if err != nil && err != goredis.Nil {
			if isRedisWrongType(err) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
				return
			}
			LogRedisError(runCtx, err)
			return
		}
		if existsCmd.Val() == 0 {
			return
		}

		if metaCmd.Err() == nil {
			meta, parseErr := parseFriendMetaJSON(metaCmd.Val())
			if parseErr != nil {
				return
			}
			meta.Remark = remark
			meta.UpdatedAt = updatedAt

			luaScript := goredis.NewScript(luaUpsertFriendMetaIfExists)
			expireSeconds := int(getRandomExpireTime(rediskey.FriendRelationTTL).Seconds())
			_, err = luaScript.Run(runCtx, r.redisClient,
				[]string{cacheKey},
				friendUUID,
				buildFriendMetaJSON(meta.Remark, meta.GroupTag, meta.Source, meta.UpdatedAt),
				expireSeconds,
			).Result()
			if err != nil && err != goredis.Nil {
				if isRedisWrongType(err) {
					_ = r.redisClient.Del(runCtx, cacheKey).Err()
					return
				}
				LogRedisError(runCtx, err)
			}
			return
		}

		if metaCmd.Err() != goredis.Nil {
			if isRedisWrongType(metaCmd.Err()) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
			} else {
				LogRedisError(runCtx, metaCmd.Err())
			}
			return
		}

		var relation model.UserRelation
		if err := r.db.WithContext(runCtx).
			Select("peer_uuid", "remark", "group_tag", "source", "updated_at").
			Where("user_uuid = ? AND peer_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, friendUUID, 0).
			First(&relation).Error; err != nil {
			return
		}
		r.updateFriendMetaCacheAsync(runCtx, userUUID, &relation)
	}, async.AsyncRedisPipelineTimeout)
}

// updateFriendTagCacheAsync 异步更新单个好友的标签缓存。
//
// 与备注更新类似，这里也优先走“在已有 Hash 上增量改 field”的策略；只有 field 缺失时才
// 回源 DB，避免把缓存写放大成整份列表重建。
func (r *friendRepositoryImpl) updateFriendTagCacheAsync(ctx context.Context, userUUID, friendUUID, groupTag string, updatedAt int64) {
	if r.redisClient == nil || userUUID == "" || friendUUID == "" {
		return
	}

	cacheKey := rediskey.FriendRelationKey(userUUID)
	async.RunSafe(ctx, func(runCtx context.Context) {
		pipe := r.redisClient.Pipeline()
		existsCmd := pipe.Exists(runCtx, cacheKey)
		metaCmd := pipe.HGet(runCtx, cacheKey, friendUUID)
		_, err := pipe.Exec(runCtx)

		if err != nil && err != goredis.Nil {
			if isRedisWrongType(err) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
				return
			}
			LogRedisError(runCtx, err)
			return
		}
		if existsCmd.Val() == 0 {
			return
		}

		if metaCmd.Err() == nil {
			meta, parseErr := parseFriendMetaJSON(metaCmd.Val())
			if parseErr != nil {
				return
			}
			meta.GroupTag = groupTag
			meta.UpdatedAt = updatedAt

			luaScript := goredis.NewScript(luaUpsertFriendMetaIfExists)
			expireSeconds := int(getRandomExpireTime(rediskey.FriendRelationTTL).Seconds())
			_, err = luaScript.Run(runCtx, r.redisClient,
				[]string{cacheKey},
				friendUUID,
				buildFriendMetaJSON(meta.Remark, meta.GroupTag, meta.Source, meta.UpdatedAt),
				expireSeconds,
			).Result()
			if err != nil && err != goredis.Nil {
				if isRedisWrongType(err) {
					_ = r.redisClient.Del(runCtx, cacheKey).Err()
					return
				}
				LogRedisError(runCtx, err)
			}
			return
		}

		if metaCmd.Err() != goredis.Nil {
			if isRedisWrongType(metaCmd.Err()) {
				_ = r.redisClient.Del(runCtx, cacheKey).Err()
			} else {
				LogRedisError(runCtx, metaCmd.Err())
			}
			return
		}

		var relation model.UserRelation
		if err := r.db.WithContext(runCtx).
			Select("peer_uuid", "remark", "group_tag", "source", "updated_at").
			Where("user_uuid = ? AND peer_uuid = ? AND status = ? AND deleted_at IS NULL", userUUID, friendUUID, 0).
			First(&relation).Error; err != nil {
			return
		}
		r.updateFriendMetaCacheAsync(runCtx, userUUID, &relation)
	}, async.AsyncRedisPipelineTimeout)
}
