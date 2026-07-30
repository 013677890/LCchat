package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/cachex"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GroupCacheReconcileTarget 是周期对账扫描使用的轻量游标记录。
//
// 使用数据库自增 ID 做 keyset 游标，避免 OFFSET 在大表扫描时反复跳过前缀行；
// UUID 才是实际聚合标识，ID 只服务于本轮稳定分页。
type GroupCacheReconcileTarget struct {
	ID        int64  `gorm:"column:id"`
	GroupUUID string `gorm:"column:uuid"`
}

type groupCacheReconcileSnapshot struct {
	group           *model.GroupInfo
	activeMembers   []*model.GroupMember
	allMembers      []*model.GroupMember
	pendingRequests []*model.GroupJoinRequest
}

type userGroupReconcileTarget struct {
	group  *model.GroupInfo
	active bool
}

type userGroupProjectionReference struct {
	groupUUID string
	version   int64
}

const maxUserGroupReconcileSnapshotAttempts = 3

// groupInfoProjectionMode 明确区分三种写语义，避免两个 bool 参数在调用点被误传。
type groupInfoProjectionMode uint8

const (
	// groupInfoPatchExisting 是普通增量事件：只更新已存在的当前格式缓存，拒绝同版本。
	groupInfoPatchExisting groupInfoProjectionMode = iota
	// groupInfoCreateIfMissing 是 group_created：允许首次创建，但同版本重放仍保持幂等。
	groupInfoCreateIfMissing
	// groupInfoAuthoritativeRepair 是 MySQL 对账：允许创建，也允许用同版本修复损坏值。
	groupInfoAuthoritativeRepair
)

// setVersionedGroupInfoProjection 写入或 patch 群资料投影。
//
// mode 把“增量 patch”“事件首次创建”“权威修复”分成互斥语义。三种模式都在 Lua
// 内比较 projection_version：普通事件拒绝 <= 当前版本；权威修复仅允许覆盖相同版本，
// 仍然拒绝更低版本，因此晚到的 DB 快照不可能回滚并发新事件。
func (r *groupRepositoryImpl) setVersionedGroupInfoProjection(
	ctx context.Context,
	group *model.GroupInfo,
	projectionVersion int64,
	mode groupInfoProjectionMode,
) error {
	if r == nil || r.redisClient == nil {
		return nil
	}
	if group == nil || group.Uuid == "" || projectionVersion <= 0 {
		return fmt.Errorf("%w: invalid versioned group info projection", ErrInvalidProjectorPayload)
	}
	projectedGroup := group
	if group.DeletedAt.Valid {
		// Redis 群资料格式不暴露 GORM 的 DeletedAt，但软删除必须仍有一个正版本
		// tombstone 来挡住旧事件。复制后映射为解散终态，避免周期对账把
		// status=0、deleted_at!=NULL 的 DB 行重新发布成可读正常群。
		copyGroup := *group
		copyGroup.Status = groupStatusDismissed
		projectedGroup = &copyGroup
	}

	value, err := encodeGroupInfoCacheValue(projectedGroup, projectionVersion)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidProjectorPayload, err)
	}
	createFlag := "0"
	repairEqualFlag := "0"
	switch mode {
	case groupInfoPatchExisting:
	case groupInfoCreateIfMissing:
		createFlag = "1"
	case groupInfoAuthoritativeRepair:
		createFlag = "1"
		repairEqualFlag = "1"
	default:
		return fmt.Errorf("%w: unknown group info projection mode %d", ErrInvalidProjectorPayload, mode)
	}
	result, err := goredis.NewScript(luaSetVersionedGroupInfo).Run(
		ctx,
		r.redisClient,
		[]string{rediskey.GroupInfoKey(group.Uuid)},
		projectionVersion,
		value,
		int(cachex.JitterTTL(rediskey.GroupInfoTTL).Seconds()),
		createFlag,
		groupCacheSchemaVersion,
		repairEqualFlag,
	).Int64()
	if err != nil {
		return WrapRedisError(err)
	}
	// -2 表示旧格式已被主动清理。增量事件没有义务从局部 payload 重建一份此前
	// 不受信任的缓存；下一次读 miss 或周期对账会用 DB 完整快照重新创建。
	_ = result
	return nil
}

// replaceVersionedHashProjection 原子替换一个群维度 Hash。
func (r *groupRepositoryImpl) replaceVersionedHashProjection(
	ctx context.Context,
	cacheKey string,
	projectionVersion int64,
	ttlSeconds int,
	emptyField, emptyValue string,
	fields map[string]string,
	repairEqual bool,
) error {
	if r == nil || r.redisClient == nil {
		return nil
	}
	if cacheKey == "" || projectionVersion <= 0 || ttlSeconds <= 0 {
		return fmt.Errorf("%w: invalid versioned hash replacement", ErrInvalidProjectorPayload)
	}
	repairEqualFlag := "0"
	if repairEqual {
		repairEqualFlag = "1"
	}
	args := make([]interface{}, 0, 6+len(fields)*2)
	args = append(
		args,
		groupCacheSchemaVersion,
		projectionVersion,
		ttlSeconds,
		emptyField,
		emptyValue,
		repairEqualFlag,
	)
	keys := make([]string, 0, len(fields))
	for field := range fields {
		if field == "" ||
			field == groupProjectionSchemaField ||
			field == groupProjectionVersionField ||
			field == groupProjectionCompleteField ||
			field == emptyField {
			return fmt.Errorf("%w: hash projection contains reserved field %q", ErrInvalidProjectorPayload, field)
		}

		keys = append(keys, field)
	}

	// 排序不是 Redis 正确性的要求，但让脚本参数与测试/诊断日志保持确定性。
	sort.Strings(keys)
	for _, field := range keys {
		args = append(args, field, fields[field])
	}
	if _, err := goredis.NewScript(luaReplaceVersionedHash).
		Run(ctx, r.redisClient, []string{cacheKey}, args...).
		Int64(); err != nil {
		return WrapRedisError(err)
	}

	return nil
}

// upsertVersionedHashProjectionIfExists 在一个 Lua 调用里更新同事件的全部 field。
func (r *groupRepositoryImpl) upsertVersionedHashProjectionIfExists(
	ctx context.Context,
	cacheKey string,
	projectionVersion int64,
	ttlSeconds int,
	emptyField, emptyValue string,
	fields map[string]string,
) error {
	if r == nil || r.redisClient == nil {
		return nil
	}
	if cacheKey == "" || projectionVersion <= 0 || ttlSeconds <= 0 || len(fields) == 0 {
		return fmt.Errorf("%w: invalid versioned hash upsert", ErrInvalidProjectorPayload)
	}
	args := make([]interface{}, 0, 5+len(fields)*2)
	args = append(args, groupCacheSchemaVersion, projectionVersion, ttlSeconds, emptyField, emptyValue)
	keys := make([]string, 0, len(fields))
	for field := range fields {
		if field == "" ||
			field == groupProjectionSchemaField ||
			field == groupProjectionVersionField ||
			field == groupProjectionCompleteField ||
			field == emptyField {
			return fmt.Errorf("%w: hash projection contains reserved field %q", ErrInvalidProjectorPayload, field)
		}

		keys = append(keys, field)
	}

	sort.Strings(keys)
	for _, field := range keys {
		args = append(args, field, fields[field])
	}
	if _, err := goredis.NewScript(luaUpsertVersionedHash).
		Run(ctx, r.redisClient, []string{cacheKey}, args...).
		Int64(); err != nil {
		return WrapRedisError(err)
	}

	return nil
}

// removeVersionedHashProjectionIfExists 删除 field，同时把 key 版本推进到删除事件版本。
func (r *groupRepositoryImpl) removeVersionedHashProjectionIfExists(
	ctx context.Context,
	cacheKey string,
	projectionVersion int64,
	ttlSeconds int,
	emptyField, emptyValue string,
	fields ...string,
) error {
	if r == nil || r.redisClient == nil {
		return nil
	}
	if cacheKey == "" || projectionVersion <= 0 || ttlSeconds <= 0 || len(fields) == 0 {
		return fmt.Errorf("%w: invalid versioned hash removal", ErrInvalidProjectorPayload)
	}
	args := make([]interface{}, 0, 5+len(fields))
	args = append(args, groupCacheSchemaVersion, projectionVersion, ttlSeconds, emptyField, emptyValue)
	for _, field := range fields {
		if field == "" ||
			field == groupProjectionSchemaField ||
			field == groupProjectionVersionField ||
			field == groupProjectionCompleteField ||
			field == emptyField {
			return fmt.Errorf("%w: hash projection contains reserved field %q", ErrInvalidProjectorPayload, field)
		}

		args = append(args, field)
	}

	if _, err := goredis.NewScript(luaRemoveVersionedHash).
		Run(ctx, r.redisClient, []string{cacheKey}, args...).
		Int64(); err != nil {
		return WrapRedisError(err)
	}

	return nil
}

// readVersionedHashFieldProjection 在一个 Lua 快照里读取 Hash 元数据和目标 field。
//
// fieldExists=false、cacheHit=true 表示“缓存完整且确定没有这个业务 field”；它与
// cacheHit=false 的“缓存缺失/格式非法”必须区分，否则成员权限检查会把缓存 miss
// 错判为明确的非成员结论。
func (r *groupRepositoryImpl) readVersionedHashFieldProjection(
	ctx context.Context,
	cacheKey, field string,
	ttlSeconds int,
	renew bool,
) (raw string, fieldExists, cacheHit bool, err error) {
	if r == nil || r.redisClient == nil || cacheKey == "" || field == "" || ttlSeconds <= 0 {
		return "", false, false, nil
	}
	renewFlag := "0"
	if renew {
		renewFlag = "1"
	}
	result, err := goredis.NewScript(luaReadVersionedHashField).Run(
		ctx,
		r.redisClient,
		[]string{cacheKey},
		groupCacheSchemaVersion,
		field,
		ttlSeconds,
		renewFlag,
	).Slice()

	if err != nil {
		return "", false, false, WrapRedisError(err)
	}
	if len(result) == 0 {
		return "", false, false, fmt.Errorf("%w: empty versioned hash read response", ErrRedis)
	}

	status, err := redisLuaInt64(result[0])
	if err != nil {
		return "", false, false, fmt.Errorf("%w: invalid versioned hash read status: %v", ErrRedis, err)
	}
	switch status {
	case 0, -1:
		return "", false, false, nil
	case 1:
	default:
		return "", false, false, fmt.Errorf("%w: unknown versioned hash read status %d", ErrRedis, status)
	}

	if len(result) < 3 {
		return "", false, false, fmt.Errorf("%w: incomplete versioned hash read response", ErrRedis)
	}
	existsFlag, err := redisLuaString(result[1])
	if err != nil {
		return "", false, false, fmt.Errorf("%w: invalid versioned hash exists flag: %v", ErrRedis, err)
	}
	if _, err := redisLuaPositiveInt64(result[2]); err != nil {
		return "", false, false, fmt.Errorf("%w: invalid versioned hash version: %v", ErrRedis, err)
	}

	switch existsFlag {
	case "0":
		if len(result) != 3 {
			return "", false, false, fmt.Errorf("%w: malformed missing-field response", ErrRedis)
		}
		return "", false, true, nil
	case "1":
		if len(result) != 4 {
			return "", false, false, fmt.Errorf("%w: malformed present-field response", ErrRedis)
		}
		raw, err = redisLuaString(result[3])
		if err != nil {
			return "", false, false, fmt.Errorf("%w: invalid versioned hash field value: %v", ErrRedis, err)
		}
		return raw, true, true, nil
	default:
		return "", false, false, fmt.Errorf("%w: unknown versioned hash exists flag %q", ErrRedis, existsFlag)
	}
}

// readVersionedUserGroupsProjection 原子读取 READY 用户群列表及每个活跃群的版本。
func (r *groupRepositoryImpl) readVersionedUserGroupsProjection(
	ctx context.Context,
	userUUID string,
	renew bool,
) ([]userGroupProjectionReference, bool, error) {
	if r == nil || r.redisClient == nil || userUUID == "" {
		return nil, false, nil
	}
	renewFlag := "0"
	if renew {
		renewFlag = "1"
	}
	result, err := goredis.NewScript(luaReadVersionedUserGroups).Run(
		ctx,
		r.redisClient,
		[]string{rediskey.UserGroupListKey(userUUID), rediskey.UserGroupVersionKey(userUUID)},
		groupCacheSchemaVersion,
		int(cachex.JitterTTL(rediskey.UserGroupListTTL).Seconds()),
		renewFlag,
	).Slice()

	if err != nil {
		return nil, false, WrapRedisError(err)
	}
	if len(result) == 0 {
		return nil, false, fmt.Errorf("%w: empty user-group read response", ErrRedis)
	}

	status, err := redisLuaInt64(result[0])
	if err != nil {
		return nil, false, fmt.Errorf("%w: invalid user-group read status: %v", ErrRedis, err)
	}
	switch status {
	case 0, -1:
		return nil, false, nil
	case 1:
	default:
		return nil, false, fmt.Errorf("%w: unknown user-group read status %d", ErrRedis, status)
	}

	if len(result) < 2 {
		return nil, false, fmt.Errorf("%w: incomplete user-group read response", ErrRedis)
	}
	count, err := redisLuaInt64(result[1])
	if err != nil || count <= 0 || int64(len(result)) != 2+count*2 {
		return nil, false, fmt.Errorf("%w: malformed user-group read count", ErrRedis)
	}

	references := make([]userGroupProjectionReference, 0, count)
	for index := 0; index < int(count); index++ {
		groupUUID, stringErr := redisLuaString(result[2+index*2])
		if stringErr != nil || groupUUID == "" {
			return nil, false, fmt.Errorf("%w: invalid user-group uuid", ErrRedis)
		}
		version, versionErr := redisLuaInt64(result[3+index*2])
		if versionErr != nil {
			return nil, false, fmt.Errorf("%w: invalid user-group version: %v", ErrRedis, versionErr)
		}
		if groupUUID == userGroupsEmptyValue {
			if count != 1 || version != 0 {
				return nil, false, fmt.Errorf("%w: malformed user-group empty sentinel", ErrRedis)
			}
		} else if version <= 0 {
			return nil, false, fmt.Errorf("%w: non-positive user-group version", ErrRedis)
		}
		references = append(references, userGroupProjectionReference{groupUUID: groupUUID, version: version})
	}

	return references, true, nil
}

func redisLuaPositiveInt64(value interface{}) (int64, error) {
	parsed, err := redisLuaInt64(value)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("expected positive integer, got %d", parsed)
	}
	return parsed, nil
}

func redisLuaInt64(value interface{}) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("unsupported Lua integer type %T", value)
	}
}

func redisLuaString(value interface{}) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case []byte:
		return string(typed), nil
	default:
		return "", fmt.Errorf("unsupported Lua string type %T", value)
	}
}

func buildGroupMemberProjectionFields(members []*model.GroupMember) (map[string]string, error) {
	fields := make(map[string]string, len(members))
	for _, member := range members {
		if member == nil ||
			member.UserUuid == "" ||
			member.Role < memberRoleMember ||
			member.Role > memberRoleOwner ||
			member.JoinedAt.UnixMilli() <= 0 {
			return nil, fmt.Errorf("%w: invalid member in cache projection", ErrInvalidProjectorPayload)
		}
		if _, duplicate := fields[member.UserUuid]; duplicate {
			return nil, fmt.Errorf(
				"%w: duplicate member %s in cache projection",
				ErrInvalidProjectorPayload,
				member.UserUuid,
			)
		}
		fields[member.UserUuid] = encodeGroupMemberCacheValue(member)
	}
	return fields, nil
}

func buildGroupJoinRequestProjectionFields(requests []*model.GroupJoinRequest) (map[string]string, error) {
	fields := make(map[string]string, len(requests))
	for _, request := range requests {
		if request == nil ||
			request.Id <= 0 ||
			request.ApplicantUuid == "" ||
			request.CreatedAt.UnixMilli() <= 0 {
			return nil, fmt.Errorf("%w: invalid join request in cache projection", ErrInvalidProjectorPayload)
		}
		field := strconv.FormatInt(request.Id, 10)
		if _, duplicate := fields[field]; duplicate {
			return nil, fmt.Errorf(
				"%w: duplicate join request %s in cache projection",
				ErrInvalidProjectorPayload,
				field,
			)
		}
		fields[field] = encodeGroupJoinRequestCacheValue(request)
	}
	return fields, nil
}

func (r *groupRepositoryImpl) replaceGroupMembersProjection(
	ctx context.Context,
	groupUUID string,
	members []*model.GroupMember,
	projectionVersion int64,
	repairEqual bool,
) error {
	fields, err := buildGroupMemberProjectionFields(members)
	if err != nil {
		return err
	}
	return r.replaceVersionedHashProjection(
		ctx,
		rediskey.GroupMembersKey(groupUUID),
		projectionVersion,
		int(cachex.JitterTTL(rediskey.GroupMembersTTL).Seconds()),
		groupMembersEmptyField,
		groupMembersEmptyValue,
		fields,
		repairEqual,
	)
}

func (r *groupRepositoryImpl) upsertGroupMembersProjectionIfExists(
	ctx context.Context,
	groupUUID string,
	members []*model.GroupMember,
	projectionVersion int64,
) error {
	fields, err := buildGroupMemberProjectionFields(members)
	if err != nil {
		return err
	}
	return r.upsertVersionedHashProjectionIfExists(
		ctx,
		rediskey.GroupMembersKey(groupUUID),
		projectionVersion,
		int(cachex.JitterTTL(rediskey.GroupMembersTTL).Seconds()),
		groupMembersEmptyField,
		groupMembersEmptyValue,
		fields,
	)
}

func (r *groupRepositoryImpl) removeGroupMemberProjectionIfExists(
	ctx context.Context,
	groupUUID, userUUID string,
	projectionVersion int64,
) error {
	return r.removeVersionedHashProjectionIfExists(
		ctx,
		rediskey.GroupMembersKey(groupUUID),
		projectionVersion,
		int(cachex.JitterTTL(rediskey.GroupMembersTTL).Seconds()),
		groupMembersEmptyField,
		groupMembersEmptyValue,
		userUUID,
	)
}

func (r *groupRepositoryImpl) replaceGroupJoinRequestsProjection(
	ctx context.Context,
	groupUUID string,
	requests []*model.GroupJoinRequest,
	projectionVersion int64,
	repairEqual bool,
) error {
	fields, err := buildGroupJoinRequestProjectionFields(requests)
	if err != nil {
		return err
	}
	return r.replaceVersionedHashProjection(
		ctx,
		rediskey.GroupJoinRequestPendingKey(groupUUID),
		projectionVersion,
		int(cachex.JitterTTL(rediskey.GroupJoinRequestTTL).Seconds()),
		groupJoinRequestsEmptyField,
		groupJoinRequestsEmptyValue,
		fields,
		repairEqual,
	)
}

func (r *groupRepositoryImpl) upsertGroupJoinRequestProjectionIfExists(
	ctx context.Context,
	groupUUID string,
	request *model.GroupJoinRequest,
	projectionVersion int64,
) error {
	fields, err := buildGroupJoinRequestProjectionFields([]*model.GroupJoinRequest{request})
	if err != nil {
		return err
	}
	return r.upsertVersionedHashProjectionIfExists(
		ctx,
		rediskey.GroupJoinRequestPendingKey(groupUUID),
		projectionVersion,
		int(cachex.JitterTTL(rediskey.GroupJoinRequestTTL).Seconds()),
		groupJoinRequestsEmptyField,
		groupJoinRequestsEmptyValue,
		fields,
	)
}

func (r *groupRepositoryImpl) removeGroupJoinRequestProjectionIfExists(
	ctx context.Context,
	groupUUID string,
	applyID, projectionVersion int64,
) error {
	return r.removeVersionedHashProjectionIfExists(
		ctx,
		rediskey.GroupJoinRequestPendingKey(groupUUID),
		projectionVersion,
		int(cachex.JitterTTL(rediskey.GroupJoinRequestTTL).Seconds()),
		groupJoinRequestsEmptyField,
		groupJoinRequestsEmptyValue,
		strconv.FormatInt(applyID, 10),
	)
}

// patchUserGroupProjection 原子更新 ZSet 与逐群版本 tombstone。
func (r *groupRepositoryImpl) patchUserGroupProjection(
	ctx context.Context,
	userUUID string,
	group *model.GroupInfo,
	active bool,
	projectionVersion int64,
	repairEqual bool,
) error {
	if r == nil || r.redisClient == nil {
		return nil
	}
	if userUUID == "" || group == nil || group.Uuid == "" || projectionVersion <= 0 {
		return fmt.Errorf("%w: invalid user-group projection", ErrInvalidProjectorPayload)
	}
	activeFlag := "0"
	if active {
		activeFlag = "1"
	}
	repairEqualFlag := "0"
	if repairEqual {
		repairEqualFlag = "1"
	}
	if _, err := goredis.NewScript(luaPatchVersionedUserGroup).Run(
		ctx,
		r.redisClient,
		[]string{rediskey.UserGroupListKey(userUUID), rediskey.UserGroupVersionKey(userUUID)},
		groupCacheSchemaVersion,
		projectionVersion,
		int(cachex.JitterTTL(rediskey.UserGroupListTTL).Seconds()),
		group.Uuid,
		group.UpdatedAt.UnixMilli(),
		activeFlag,
		repairEqualFlag,
	).Int64(); err != nil {
		return WrapRedisError(err)
	}
	return nil
}

// reconcileUserGroupProjection 把一个数据库完整快照合并进用户群反向索引。
func (r *groupRepositoryImpl) reconcileUserGroupProjection(
	ctx context.Context,
	userUUID string,
	targets []userGroupReconcileTarget,
) error {
	if r == nil || r.redisClient == nil {
		return nil
	}
	if userUUID == "" {
		return fmt.Errorf("%w: empty user uuid for user-group reconciliation", ErrInvalidProjectorPayload)
	}
	args := make([]interface{}, 0, 2+len(targets)*4)
	args = append(args, groupCacheSchemaVersion, int(cachex.JitterTTL(rediskey.UserGroupListTTL).Seconds()))
	for _, target := range targets {
		if target.group == nil || target.group.Uuid == "" || target.group.CacheVersion <= 0 {
			return fmt.Errorf("%w: user-group reconcile target missing version", ErrInvalidProjectorPayload)
		}
	}
	sort.SliceStable(targets, func(i, j int) bool {
		return targets[i].group.Uuid < targets[j].group.Uuid
	})
	for _, target := range targets {
		activeFlag := "0"
		if target.active {
			activeFlag = "1"
		}
		args = append(
			args,
			target.group.Uuid,
			target.group.UpdatedAt.UnixMilli(),
			target.group.CacheVersion,
			activeFlag,
		)
	}

	if _, err := goredis.NewScript(luaReconcileVersionedUserGroups).Run(
		ctx,
		r.redisClient,
		[]string{rediskey.UserGroupListKey(userUUID), rediskey.UserGroupVersionKey(userUUID)},
		args...,
	).Int64(); err != nil {
		return WrapRedisError(err)
	}
	return nil
}

// ListGroupCacheReconcileTargets 分页列出需要进行缓存对账的群。
func (r *groupRepositoryImpl) ListGroupCacheReconcileTargets(
	ctx context.Context,
	afterID int64,
	limit int,
) ([]GroupCacheReconcileTarget, error) {
	if r == nil || r.db == nil {
		return nil, ErrDatabase
	}
	if afterID < 0 || limit <= 0 {
		return nil, fmt.Errorf("%w: invalid group cache reconcile cursor", ErrDatabase)
	}
	var targets []GroupCacheReconcileTarget
	if err := r.db.WithContext(ctx).
		Unscoped().
		Model(&model.GroupInfo{}).
		Select("id, uuid").
		Where("id > ?", afterID).
		Order("id ASC").
		Limit(limit).
		Scan(&targets).Error; err != nil {
		return nil, WrapDBError(err)
	}
	return targets, nil
}

// ReconcileGroupCache 按 group_uuid 从 MySQL 一致性快照重建全部群维度缓存。
//
// 这是 projector 之外唯一允许写群缓存的修复入口。它不复用请求线程刚读到的对象，
// 而是在一个 DB 事务里同时读取 group、全部历史成员和待审批申请，再以
// groups.cache_version 写入版本化 Lua。即使读取期间 Kafka 正在投影，较低版本快照
// 也只会被脚本拒绝，不会把新缓存回滚。
func (r *groupRepositoryImpl) ReconcileGroupCache(ctx context.Context, groupUUID string) error {
	if r == nil || r.db == nil || groupUUID == "" {
		return fmt.Errorf("%w: invalid group cache reconcile request", ErrDatabase)
	}
	if r.redisClient == nil {
		return nil
	}
	snapshot, err := r.loadGroupCacheReconcileSnapshot(ctx, groupUUID)
	if err != nil {
		return err
	}
	version := snapshot.group.CacheVersion
	if version <= 0 {
		return fmt.Errorf("%w: group %s cache_version is not initialized", ErrDatabase, groupUUID)
	}

	r.addGroupUUIDToBloomBestEffort(ctx, groupUUID)
	if err := r.setVersionedGroupInfoProjection(
		ctx,
		snapshot.group,
		version,
		groupInfoAuthoritativeRepair,
	); err != nil {
		return err
	}
	if err := r.replaceGroupMembersProjection(ctx, groupUUID, snapshot.activeMembers, version, true); err != nil {
		return err
	}
	if err := r.replaceGroupJoinRequestsProjection(ctx, groupUUID, snapshot.pendingRequests, version, true); err != nil {
		return err
	}

	groupActive := snapshot.group.Status == groupStatusNormal && !snapshot.group.DeletedAt.Valid
	for _, member := range snapshot.allMembers {
		if member == nil || member.UserUuid == "" {
			continue
		}
		memberActive := groupActive && member.Status == memberStatusNormal && !member.DeletedAt.Valid
		if err := r.patchUserGroupProjection(
			ctx,
			member.UserUuid,
			snapshot.group,
			memberActive,
			version,
			true,
		); err != nil {
			return err
		}
	}

	return nil
}

func (r *groupRepositoryImpl) loadGroupCacheReconcileSnapshot(
	ctx context.Context,
	groupUUID string,
) (groupCacheReconcileSnapshot, error) {
	var snapshot groupCacheReconcileSnapshot
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var group model.GroupInfo
		// 所有群写事务最终都要更新同一 groups 行领取 cache_version。这里持共享锁到
		// 快照读取结束，可在 READ COMMITTED 环境下同样阻止成员/申请与版本跨事务拼接；
		// Redis 写入仍在事务提交后执行，不把网络 I/O 带进数据库锁区。
		if err := tx.Unscoped().
			Clauses(clause.Locking{Strength: "SHARE"}).
			Where("uuid = ?", groupUUID).
			Take(&group).Error; err != nil {
			return WrapDBError(err)
		}
		var allMembers []*model.GroupMember
		if err := tx.Unscoped().
			Where("group_uuid = ?", groupUUID).
			Order("id ASC").
			Find(&allMembers).Error; err != nil {
			return WrapDBError(err)
		}

		activeMembers := make([]*model.GroupMember, 0, len(allMembers))
		if group.Status == groupStatusNormal && !group.DeletedAt.Valid {
			for _, member := range allMembers {
				if member != nil && member.Status == memberStatusNormal && !member.DeletedAt.Valid {
					activeMembers = append(activeMembers, member)
				}
			}
		}
		var pendingRequests []*model.GroupJoinRequest
		if group.Status == groupStatusNormal && !group.DeletedAt.Valid {
			if err := tx.Where("group_uuid = ? AND status = ?", groupUUID, joinRequestStatusPending).
				Order("created_at DESC, id DESC").
				Find(&pendingRequests).Error; err != nil {
				return WrapDBError(err)
			}
		}
		snapshot = groupCacheReconcileSnapshot{
			group:           &group,
			activeMembers:   activeMembers,
			allMembers:      allMembers,
			pendingRequests: pendingRequests,
		}

		return nil
	})

	if err != nil {
		return groupCacheReconcileSnapshot{}, err
	}
	return snapshot, nil
}

// ReconcileUserGroupsCache 从 MySQL 历史成员关系重建单个用户的群反向索引。
//
// 历史退出/被踢记录也必须读出并写入 tombstone；只查询当前有效群会失去删除版本，
// 在“读快照 vN 与删除事件 vN+1 交错”时可能把已退出群重新加回缓存。
func (r *groupRepositoryImpl) ReconcileUserGroupsCache(ctx context.Context, userUUID string) error {
	if r == nil || r.db == nil || userUUID == "" {
		return fmt.Errorf("%w: invalid user-group reconcile request", ErrDatabase)
	}
	if r.redisClient == nil {
		return nil
	}

	// 现有版本 Hash 只作为“可能存在的脏 group_uuid”提示。把这些 UUID 也带入
	// MySQL 快照后，没有任何成员历史的多余 ZSet 项也能得到一个权威删除版本；
	// 若读取提示后并发新增成员，事件版本会更高，Lua 会保留该新事实。
	cachedVersions, err := r.loadCachedUserGroupProjectionVersions(ctx, userUUID)
	if err != nil {
		return err
	}
	targets, err := r.loadUserGroupReconcileTargets(ctx, userUUID, cachedVersions)
	if err != nil {
		return err
	}
	if err := r.reconcileUserGroupProjection(ctx, userUUID, targets); err != nil {
		return err
	}

	// 用户群列表读命中后还要读取各 group:info。这里复用同一个 DB 快照做版本化
	// 回填；若某群已被更高版本事件更新，Lua 会拒绝这份较旧快照。
	for _, target := range targets {
		if target.group == nil || !target.active {
			continue
		}
		if err := r.setVersionedGroupInfoProjection(
			ctx,
			target.group,
			target.group.CacheVersion,
			groupInfoAuthoritativeRepair,
		); err != nil {
			return err
		}
	}

	return nil
}

func (r *groupRepositoryImpl) loadUserGroupReconcileTargets(
	ctx context.Context,
	userUUID string,
	cachedVersions map[string]int64,
) ([]userGroupReconcileTarget, error) {
	candidateUUIDs, err := r.discoverUserGroupReconcileCandidates(ctx, userUUID, cachedVersions)
	if err != nil {
		return nil, err
	}
	for attempt := 1; attempt <= maxUserGroupReconcileSnapshotAttempts; attempt++ {
		targets, newlyDiscovered, loadErr := r.loadUserGroupReconcileTargetsOnce(
			ctx,
			userUUID,
			candidateUUIDs,
		)
		if loadErr != nil {
			return nil, loadErr
		}
		if len(newlyDiscovered) == 0 {
			return targets, nil
		}

		// 有用户在候选集合发现后加入了另一个群。当前事务没有预先锁住那个群，
		// 不能把这次读取包装成权威快照；扩充候选集合后从头重试，仍保持
		// “groups 行锁 -> memberships 读取”的全局锁顺序。
		candidateUUIDs = mergeSortedUniqueStrings(candidateUUIDs, newlyDiscovered)
	}

	return nil, fmt.Errorf(
		"%w: user %s memberships 在 %d 次快照尝试内持续变化",
		ErrDatabase,
		userUUID,
		maxUserGroupReconcileSnapshotAttempts,
	)
}

// discoverUserGroupReconcileCandidates 只负责发现需要加锁的 group_uuid 候选集。
//
// 这次查询不是权威快照：它可以与成员写事务并发，结果仅用于下一阶段按固定顺序锁
// groups 行。真正的 active 状态必须在群锁全部取得后重新读取，绝不能把这里看到的
// 旧 membership 与随后读到的新 cache_version 拼在一起。
func (r *groupRepositoryImpl) discoverUserGroupReconcileCandidates(
	ctx context.Context,
	userUUID string,
	cachedVersions map[string]int64,
) ([]string, error) {
	var membershipUUIDs []string
	if err := r.db.WithContext(ctx).
		Unscoped().
		Model(&model.GroupMember{}).
		Distinct("group_uuid").
		Where("user_uuid = ?", userUUID).
		Pluck("group_uuid", &membershipUUIDs).Error; err != nil {
		return nil, WrapDBError(err)
	}
	candidates := make([]string, 0, len(membershipUUIDs)+len(cachedVersions))
	candidates = append(candidates, membershipUUIDs...)
	for groupUUID := range cachedVersions {
		candidates = append(candidates, groupUUID)
	}
	return mergeSortedUniqueStrings(nil, candidates), nil
}

// loadUserGroupReconcileTargetsOnce 在同一个锁边界内构造用户群权威快照。
//
// 锁顺序必须与所有群写事务一致：先锁 groups，再读 group_members。旧实现反过来先读
// memberships、再锁 groups，在 MySQL READ COMMITTED 和 REPEATABLE READ 下都可能得到
// “旧 active 状态 + 新 cache_version”；同版本权威 Lua 随后会写错状态，并挡住对应
// Kafka 事件。这里取得全部群共享锁后重新读取成员关系，群写事务要么完整发生在快照
// 之前，要么等本事务结束后再提交，不再出现跨事务拼接。
func (r *groupRepositoryImpl) loadUserGroupReconcileTargetsOnce(
	ctx context.Context,
	userUUID string,
	candidateUUIDs []string,
) (targets []userGroupReconcileTarget, newlyDiscovered []string, err error) {
	candidateSet := make(map[string]struct{}, len(candidateUUIDs))
	for _, groupUUID := range candidateUUIDs {
		if groupUUID != "" {
			candidateSet[groupUUID] = struct{}{}
		}
	}

	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var groups []*model.GroupInfo
		if len(candidateUUIDs) > 0 {
			if err := tx.Unscoped().
				Clauses(clause.Locking{Strength: "SHARE"}).
				Where("uuid IN ?", candidateUUIDs).
				Order("uuid ASC").
				Find(&groups).Error; err != nil {
				return WrapDBError(err)
			}
		}

		// 这才是本轮用于裁决 active 的权威成员读取。它必须位于 groups 共享锁之后；
		// 上面的候选发现结果只能用于定位锁，不能复用为业务状态。
		var memberships []*model.GroupMember
		if err := tx.Unscoped().
			Where("user_uuid = ?", userUUID).
			Order("id ASC").
			Find(&memberships).Error; err != nil {
			return WrapDBError(err)
		}

		for _, membership := range memberships {
			if membership == nil || membership.GroupUuid == "" {
				continue
			}
			if _, locked := candidateSet[membership.GroupUuid]; !locked {
				newlyDiscovered = append(newlyDiscovered, membership.GroupUuid)
			}
		}

		if len(newlyDiscovered) > 0 {
			newlyDiscovered = mergeSortedUniqueStrings(nil, newlyDiscovered)
			return nil
		}

		membershipByGroup := make(map[string]*model.GroupMember, len(memberships))
		for _, membership := range memberships {
			if membership == nil || membership.GroupUuid == "" {
				continue
			}
			membershipByGroup[membership.GroupUuid] = membership
		}

		if len(candidateUUIDs) == 0 {
			targets = []userGroupReconcileTarget{}
			return nil
		}
		groupByUUID := make(map[string]*model.GroupInfo, len(groups))
		for _, group := range groups {
			if group != nil {
				groupByUUID[group.Uuid] = group
			}
		}

		targets = make([]userGroupReconcileTarget, 0, len(candidateUUIDs))
		for _, groupUUID := range candidateUUIDs {
			group := groupByUUID[groupUUID]
			if group == nil {
				// 正常投影版本不可能脱离 groups 权威行存在；这里不凭缓存自造版本，
				// 避免把随机脏值包装成可信 tombstone。此类极端污染交给 TTL 清理。
				continue
			}
			membership := membershipByGroup[groupUUID]
			active := membership != nil &&
				membership.Status == memberStatusNormal &&
				!membership.DeletedAt.Valid &&
				group.Status == groupStatusNormal &&
				!group.DeletedAt.Valid
			targets = append(targets, userGroupReconcileTarget{group: group, active: active})
		}

		return nil
	})

	if err != nil {
		return nil, nil, err
	}
	return targets, newlyDiscovered, nil
}

func mergeSortedUniqueStrings(base, additions []string) []string {
	seen := make(map[string]struct{}, len(base)+len(additions))
	merged := make([]string, 0, len(base)+len(additions))
	for _, values := range [][]string{base, additions} {
		for _, value := range values {
			if value == "" {
				continue
			}
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			merged = append(merged, value)
		}
	}
	sort.Strings(merged)
	return merged
}

// loadCachedUserGroupProjectionVersions 读取当前格式版本 Hash 中的业务 group_uuid。
//
// 这不是权威读，也不参与最终裁决，只用于让用户全量对账发现“DB 从未有过该成员关系，
// 但 ZSet 里多出一个群”的污染项。读取后发生的并发变化仍由每群 cache_version 保护。
func (r *groupRepositoryImpl) loadCachedUserGroupProjectionVersions(
	ctx context.Context,
	userUUID string,
) (map[string]int64, error) {
	result := make(map[string]int64)
	values, err := r.redisClient.HGetAll(ctx, rediskey.UserGroupVersionKey(userUUID)).Result()

	if err != nil {
		if cachex.IsRedisWrongType(err) {
			return result, nil
		}
		return nil, WrapRedisError(err)
	}

	if len(values) == 0 || values[groupProjectionSchemaField] != groupCacheSchemaVersion {
		return result, nil
	}
	for field, raw := range values {
		if field == groupProjectionSchemaField || field == userGroupsReadyField || field == "" {
			continue
		}
		version, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || version <= 0 {
			continue
		}
		result[field] = version
	}

	return result, nil
}

// scheduleGroupCacheReconcile / scheduleUserGroupsCacheReconcile 是读 miss 的异步修复入口。
//
// 任务只传聚合标识，不携带请求线程读出的对象；后台会重新读取一致性 DB 快照。
// 这样“查询旧快照后晚写 Redis”的经典 cache-aside 竞态会被彻底消除。
func (r *groupRepositoryImpl) scheduleGroupCacheReconcile(ctx context.Context, groupUUID string) {
	if r == nil || r.redisClient == nil || groupUUID == "" {
		return
	}
	async.RunSafe(ctx, func(runCtx context.Context) {
		_, err, _ := r.memberGroup.Do("cache-reconcile-group:"+groupUUID, func() (interface{}, error) {
			return nil, r.ReconcileGroupCache(runCtx, groupUUID)
		})
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && !errors.Is(err, ErrRecordNotFound) {
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRetryTimeout)
}

func (r *groupRepositoryImpl) scheduleUserGroupsCacheReconcile(ctx context.Context, userUUID string) {
	if r == nil || r.redisClient == nil || userUUID == "" {
		return
	}
	async.RunSafe(ctx, func(runCtx context.Context) {
		_, err, _ := r.memberGroup.Do("cache-reconcile-user:"+userUUID, func() (interface{}, error) {
			return nil, r.ReconcileUserGroupsCache(runCtx, userUUID)
		})
		if err != nil {
			LogRedisError(runCtx, err)
		}
	}, async.AsyncRetryTimeout)
}

// scheduleUserGroupsCacheAuditAfterHit 让“结构合法但语义错误”的反向索引也有修复入口。
//
// 缓存 miss 会自然触发 ReconcileUserGroupsCache，但一个错误地包含多余群的 READY 缓存
// 可以持续命中：群级周期对账又只知道该群历史上出现过的用户，无法发现一个 DB 中从未
// 加入该群的陌生用户。这里用 Redis SET NX 租约给命中路径增加低频权威对账，使这类污染
// 最迟在活跃用户下一次租约窗口内被清除，同时避免每个请求都访问 MySQL。
func (r *groupRepositoryImpl) scheduleUserGroupsCacheAuditAfterHit(ctx context.Context, userUUID string) {
	claimed, err := r.claimUserGroupsReconcileLease(ctx, userUUID)
	if err != nil {
		// 对账是读路径的自愈增强项。Redis 短暂异常不能把已经成功完成的业务读降级为失败。
		LogRedisError(ctx, err)
		return
	}
	if claimed {
		r.scheduleUserGroupsCacheReconcile(ctx, userUUID)
	}
}

func (r *groupRepositoryImpl) claimUserGroupsReconcileLease(ctx context.Context, userUUID string) (bool, error) {
	if r == nil || r.redisClient == nil || userUUID == "" {
		return false, nil
	}
	claimed, err := r.redisClient.SetNX(
		ctx,
		rediskey.UserGroupReconcileLeaseKey(userUUID),
		"1",
		rediskey.UserGroupReconcileLeaseTTL,
	).Result()
	if err != nil {
		return false, WrapRedisError(err)
	}
	return claimed, nil
}
