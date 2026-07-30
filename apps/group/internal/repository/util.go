package repository

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/groupevent"
)

const (
	// groupCacheSchemaVersion 是 Redis 群缓存当前且唯一支持的结构版本。
	//
	// 这里故意不把缺失字段解释为旧版本：任何没有显式 schema/version 元数据的
	// String、Hash 或 ZSet 都会被拒绝；点查 Lua 会原子删除，完整集合读则按 miss
	// 交给版本化对账覆盖，绝不把旧字段映射成当前结构。
	groupCacheSchemaVersion = "2"
	// groupProjectionSchemaField / groupProjectionVersionField / groupProjectionCompleteField
	// 是版本化 Hash 的保留字段。业务 UUID 和申请 ID 都不能使用 "__" 前缀。
	groupProjectionSchemaField  = "__SCHEMA__"
	groupProjectionVersionField = "__VERSION__"
	// groupProjectionCompleteField 证明 Hash 来自一次完整 replace，而不是只写了
	// schema/version 的中间态。增量 Lua 必须先检查它，才能以 O(1) 成本安全 patch；
	// v2 之前或任何缺少该标记的 Hash 都直接失效，不做结构兼容。
	groupProjectionCompleteField = "__COMPLETE__"
	// userGroupsReadyField 只由一次完整的用户群关系对账写入。
	//
	// 单条 Kafka 事件可以先创建版本 tombstone，但不会把局部集合标成完整缓存；
	// 读路径仅在 READY=1 时命中，从而避免“先到的一条 member_added”制造残缺群列表。
	userGroupsReadyField = "__READY__"
	// groupInfoEmptyValue 是群资料空值缓存占位。
	//
	// 这里显式缓存“查无此群”，可以避免恶意探测或热点 miss
	// 在短时间内持续穿透到 MySQL。0 是唯一允许的非正版本，且只能与该固定
	// NOT_FOUND 载荷组合；正常群快照必须使用大于 0 的数据库投影版本。
	groupInfoEmptyValue = groupCacheSchemaVersion + "|0|__NOT_FOUND__"
	// groupMembersEmptyField 是群成员 Hash 的空集合占位 field。
	//
	// Redis Hash 为空时会被 Redis 直接删除，因此如果需要表达
	// “这个群目前成员集合为空但缓存是有效的”，就必须保留一个哨兵 field。
	groupMembersEmptyField = "__EMPTY__"
	// groupMembersEmptyValue 是空集合占位 field 的固定值。
	groupMembersEmptyValue = "{}"
	// userGroupsEmptyValue 是用户群列表 ZSet 的空集合占位 member。
	userGroupsEmptyValue = "__EMPTY__"
	// groupJoinRequestsEmptyField 是待审批申请 Hash 的空集合占位 field。
	groupJoinRequestsEmptyField = "__EMPTY__"
	// groupJoinRequestsEmptyValue 是待审批申请空集合占位值。
	groupJoinRequestsEmptyValue = "{}"
)

// groupInfoCacheEntry 是 group:info:{group_uuid} 的缓存载体。
//
// 这里不直接缓存整个 GORM 模型，而是只保留读路径真正需要的字段，
// 这样可以：
//  1. 降低 JSON 体积；
//  2. 避免把 DB 内部字段泄露给缓存格式；
//  3. 给未来缓存结构调整留出独立演化空间。
type groupInfoCacheEntry struct {
	GroupID         int64  `json:"group_id,omitempty"`
	GroupUUID       string `json:"group_uuid"`
	Name            string `json:"name"`
	Avatar          string `json:"avatar"`
	Notice          string `json:"notice"`
	OwnerUUID       string `json:"owner_uuid"`
	MemberCount     int    `json:"member_count"`
	AddMode         int8   `json:"add_mode"`
	MuteAll         bool   `json:"mute_all"`
	Status          int8   `json:"status"`
	UpdatedAtUnixMs int64  `json:"updated_at_unix_ms"`
}

// groupMemberCacheEntry 是 group:members:{group_uuid} Hash 中单个 field 的值结构。
//
// 当前权限判断和成员展示依赖 role / remark / mute_until / joined_at，
// 因此缓存成员快照只保留这些稳定事实字段，不冗余用户昵称头像。
type groupMemberCacheEntry struct {
	Role            int8   `json:"role"`
	Remark          string `json:"remark"`
	MuteUntilUnixMs int64  `json:"mute_until_unix_ms"`
	JoinedAtUnixMs  int64  `json:"joined_at_unix_ms"`
}

// groupJoinRequestCacheEntry 是 group:join_requests:{group_uuid} Hash 中单条申请的缓存值结构。
//
// 这里缓存申请事实本身，而不是连同昵称头像一起缓存，原因是：
//  1. 申请人的展示资料仍以 user_profile 为准；
//  2. 申请列表缓存主要目标是减少 group_join_requests 热点回源；
//  3. 资料更新频率和申请流转频率不同，拆开缓存更容易保持职责清晰。
type groupJoinRequestCacheEntry struct {
	ApplyID        int64  `json:"apply_id"`
	ApplicantUUID  string `json:"applicant_uuid"`
	Reason         string `json:"reason"`
	CreatedAtUnixM int64  `json:"created_at_unix_ms"`
}

// encodeGroupInfoCacheValue 把群模型编码成严格版本化 Redis String。
//
// 格式固定为 "<schema>|<projection_version>|<json>"。Lua 只需解析前两个十进制
// 字段就能在单次原子操作里比较版本，不依赖 Redis 内并不存在的通用 JSON 解析能力。
// 编码失败必须显式返回错误，禁止再用 "{}" 伪装成一条可用缓存。
func encodeGroupInfoCacheValue(group *model.GroupInfo, projectionVersion int64) (string, error) {
	if group == nil ||
		group.Id <= 0 ||
		group.Uuid == "" ||
		group.OwnerUuid == "" ||
		group.MemberCnt < 0 ||
		(group.AddMode != 0 && group.AddMode != 1) ||
		(group.Status != groupStatusNormal &&
			group.Status != groupStatusDisabled &&
			group.Status != groupStatusDismissed) ||
		projectionVersion <= 0 ||
		group.UpdatedAt.UnixMilli() <= 0 {
		return "", fmt.Errorf("群资料缓存包含非法必填字段或 projection_version")
	}
	entry := groupInfoCacheEntry{}
	entry.GroupID = group.Id
	entry.GroupUUID = group.Uuid
	entry.Name = group.Name
	entry.Avatar = group.Avatar
	entry.Notice = group.Notice
	entry.OwnerUUID = group.OwnerUuid
	entry.MemberCount = group.MemberCnt
	entry.AddMode = group.AddMode
	entry.MuteAll = group.MuteAll
	entry.Status = group.Status
	entry.UpdatedAtUnixMs = group.UpdatedAt.UnixMilli()
	data, err := json.Marshal(&entry)
	if err != nil {
		return "", fmt.Errorf("编码群资料缓存失败: %w", err)
	}
	return groupCacheSchemaVersion + "|" + strconv.FormatInt(projectionVersion, 10) + "|" + string(data), nil
}

// decodeGroupInfoCacheValue 解析严格版本化的群资料缓存。
//
// 返回的 empty 仅表示命中当前格式的 NOT_FOUND 占位。旧 "{}"、缺少版本前缀、
// 未知 schema 或带未知 JSON 字段的值都会报错，调用方按 miss 回源并触发权威对账。
func decodeGroupInfoCacheValue(raw string) (entry *groupInfoCacheEntry, projectionVersion int64, empty bool, err error) {
	parts := strings.SplitN(raw, "|", 3)
	if len(parts) != 3 || parts[0] != groupCacheSchemaVersion {
		return nil, 0, false, fmt.Errorf("群资料缓存 schema/version 前缀非法")
	}
	projectionVersion, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil || projectionVersion < 0 {
		return nil, 0, false, fmt.Errorf("群资料缓存 projection_version 非法")
	}
	if projectionVersion == 0 {
		if parts[2] != "__NOT_FOUND__" {
			return nil, 0, false, fmt.Errorf("群资料缓存 0 版本只能表示 NOT_FOUND")
		}
		return nil, 0, true, nil
	}
	var decoded groupInfoCacheEntry
	if err := decodeStrictCacheJSON(parts[2], &decoded); err != nil {
		return nil, 0, false, err
	}
	if decoded.GroupID <= 0 ||
		decoded.GroupUUID == "" ||
		decoded.OwnerUUID == "" ||
		decoded.MemberCount < 0 ||
		(decoded.AddMode != 0 && decoded.AddMode != 1) ||
		(decoded.Status != groupStatusNormal &&
			decoded.Status != groupStatusDisabled &&
			decoded.Status != groupStatusDismissed) ||
		decoded.UpdatedAtUnixMs <= 0 {
		return nil, 0, false, fmt.Errorf("群资料缓存缺少必填字段")
	}

	return &decoded, projectionVersion, false, nil
}

// buildGroupInfoFromCache 把缓存 entry 还原为读路径使用的群模型。
func buildGroupInfoFromCache(entry *groupInfoCacheEntry, projectionVersion int64) *model.GroupInfo {
	if entry == nil || entry.GroupUUID == "" || projectionVersion <= 0 {
		return nil
	}
	group := &model.GroupInfo{
		Id:           entry.GroupID,
		Uuid:         entry.GroupUUID,
		Name:         entry.Name,
		Avatar:       entry.Avatar,
		Notice:       entry.Notice,
		OwnerUuid:    entry.OwnerUUID,
		MemberCnt:    entry.MemberCount,
		AddMode:      entry.AddMode,
		MuteAll:      entry.MuteAll,
		Status:       entry.Status,
		CacheVersion: projectionVersion,
	}
	group.UpdatedAt = time.UnixMilli(entry.UpdatedAtUnixMs)
	return group
}

// buildGroupInfoFromSnapshot 把 outbox 事件里的群快照还原为仓储层模型。
//
// projector 侧不应为了回填缓存再回源 MySQL，
// 因此这里直接把事件携带的最终态快照转换成可复用的 model.GroupInfo。
func buildGroupInfoFromSnapshot(snapshot *groupevent.GroupSnapshot) *model.GroupInfo {
	if snapshot == nil || snapshot.GroupUUID == "" {
		return nil
	}
	group := &model.GroupInfo{
		Id:        snapshot.GroupID,
		Uuid:      snapshot.GroupUUID,
		Name:      snapshot.Name,
		Avatar:    snapshot.Avatar,
		Notice:    snapshot.Notice,
		OwnerUuid: snapshot.OwnerUUID,
		MemberCnt: int(snapshot.MemberCount),
		AddMode:   int8(snapshot.AddMode),
		MuteAll:   snapshot.MuteAll,
		Status:    int8(snapshot.Status),
	}
	group.UpdatedAt = time.UnixMilli(snapshot.UpdatedAtUnixMs)
	return group
}

// encodeGroupMemberCacheValue 把群成员模型编码成 Hash field 值。
func encodeGroupMemberCacheValue(member *model.GroupMember) string {
	entry := groupMemberCacheEntry{}
	if member != nil {
		entry.Role = member.Role
		entry.Remark = member.Remark
		if member.MuteUntil != nil {
			entry.MuteUntilUnixMs = member.MuteUntil.UnixMilli()
		}
		entry.JoinedAtUnixMs = member.JoinedAt.UnixMilli()
	}
	data, err := json.Marshal(&entry)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// decodeGroupMemberCacheValue 解析群成员缓存值。
func decodeGroupMemberCacheValue(raw string) (*groupMemberCacheEntry, error) {
	var entry groupMemberCacheEntry
	if err := decodeStrictCacheJSON(raw, &entry); err != nil {
		return nil, err
	}
	if entry.Role < memberRoleMember ||
		entry.Role > memberRoleOwner ||
		entry.MuteUntilUnixMs < 0 ||
		entry.JoinedAtUnixMs <= 0 {
		return nil, fmt.Errorf("群成员缓存字段值非法")
	}
	return &entry, nil
}

// buildGroupMemberFromCache 把单个缓存 field 还原为成员模型。
func buildGroupMemberFromCache(userUUID string, entry *groupMemberCacheEntry) *model.GroupMember {
	if userUUID == "" || userUUID == groupMembersEmptyField || entry == nil {
		return nil
	}
	member := &model.GroupMember{UserUuid: userUUID, Role: entry.Role, Remark: entry.Remark}
	if entry.MuteUntilUnixMs > 0 {
		muteUntil := time.UnixMilli(entry.MuteUntilUnixMs)
		member.MuteUntil = &muteUntil
	}
	if entry.JoinedAtUnixMs > 0 {
		member.JoinedAt = time.UnixMilli(entry.JoinedAtUnixMs)
	}
	return member
}

// buildGroupMemberFromSnapshot 把事件快照还原为成员模型。
func buildGroupMemberFromSnapshot(groupUUID string, snapshot groupevent.GroupMemberSnapshot) *model.GroupMember {
	if groupUUID == "" || snapshot.UserUUID == "" {
		return nil
	}
	member := &model.GroupMember{
		GroupUuid: groupUUID,
		UserUuid:  snapshot.UserUUID,
		Role:      int8(snapshot.Role),
		Remark:    snapshot.Remark,
	}
	if snapshot.MuteUntilUnixMs > 0 {
		muteUntil := time.UnixMilli(snapshot.MuteUntilUnixMs)
		member.MuteUntil = &muteUntil
	}
	if snapshot.JoinedAtUnixMs > 0 {
		member.JoinedAt = time.UnixMilli(snapshot.JoinedAtUnixMs)
	}
	return member
}

// buildGroupMembersFromSnapshots 批量把事件成员快照还原成成员模型。
//
// 这里统一复用 member model，是为了让 projector 和权威对账共用同一套严格缓存编码，
// 避免事件快照与数据库快照分别维护两份容易漂移的字段映射。
func buildGroupMembersFromSnapshots(groupUUID string, snapshots []groupevent.GroupMemberSnapshot) []*model.GroupMember {
	if groupUUID == "" || len(snapshots) == 0 {
		return []*model.GroupMember{}
	}
	members := make([]*model.GroupMember, 0, len(snapshots))
	for _, snapshot := range snapshots {
		member := buildGroupMemberFromSnapshot(groupUUID, snapshot)
		if member == nil {
			continue
		}
		members = append(members, member)
	}
	return members
}

// encodeGroupJoinRequestCacheValue 把入群申请模型编码成 Hash field 值。
func encodeGroupJoinRequestCacheValue(request *model.GroupJoinRequest) string {
	entry := groupJoinRequestCacheEntry{}
	if request != nil {
		entry.ApplyID = request.Id
		entry.ApplicantUUID = request.ApplicantUuid
		entry.Reason = request.Reason
		entry.CreatedAtUnixM = request.CreatedAt.UnixMilli()
	}
	data, err := json.Marshal(&entry)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// decodeGroupJoinRequestCacheValue 解析入群申请缓存值。
func decodeGroupJoinRequestCacheValue(raw string) (*groupJoinRequestCacheEntry, error) {
	var entry groupJoinRequestCacheEntry
	if err := decodeStrictCacheJSON(raw, &entry); err != nil {
		return nil, err
	}
	if entry.ApplyID <= 0 || entry.ApplicantUUID == "" || entry.CreatedAtUnixM <= 0 {
		return nil, fmt.Errorf("入群申请缓存字段值非法")
	}
	return &entry, nil
}

// buildGroupJoinRequestFromCache 把缓存 entry 还原为仓储层申请模型。
func buildGroupJoinRequestFromCache(entry *groupJoinRequestCacheEntry) *model.GroupJoinRequest {
	if entry == nil || entry.ApplyID <= 0 || entry.ApplicantUUID == "" {
		return nil
	}
	request := &model.GroupJoinRequest{
		Id:            entry.ApplyID,
		ApplicantUuid: entry.ApplicantUUID,
		Reason:        entry.Reason,
		Status:        joinRequestStatusPending,
	}
	if entry.CreatedAtUnixM > 0 {
		request.CreatedAt = time.UnixMilli(entry.CreatedAtUnixM)
	}
	return request
}

// buildGroupJoinRequestFromSnapshot 把事件快照还原成申请模型。
func buildGroupJoinRequestFromSnapshot(snapshot *groupevent.GroupJoinRequestSnapshot) *model.GroupJoinRequest {
	if snapshot == nil || snapshot.ApplyID <= 0 || snapshot.ApplicantUUID == "" {
		return nil
	}
	request := &model.GroupJoinRequest{
		Id:            snapshot.ApplyID,
		ApplicantUuid: snapshot.ApplicantUUID,
		Reason:        snapshot.Reason,
		Status:        joinRequestStatusPending,
	}
	if snapshot.CreatedAtUnixMs > 0 {
		request.CreatedAt = time.UnixMilli(snapshot.CreatedAtUnixMs)
	}
	return request
}

// sortGroupJoinRequests 统一待审批申请列表排序规则。
//
// 排序与 DB 查询保持一致：
//  1. 创建时间新的在前；
//  2. 创建时间相同时，ID 大的在前。
func sortGroupJoinRequests(items []*model.GroupJoinRequest) {
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		if left == nil || right == nil {
			return right == nil
		}
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.After(right.CreatedAt)
		}
		return left.Id > right.Id
	})
}

// cloneGroupMembers 深拷贝 singleflight 结果，避免多个请求共享可变成员指针。
func cloneGroupMembers(members []*model.GroupMember) []*model.GroupMember {
	if len(members) == 0 {
		return []*model.GroupMember{}
	}
	cloned := make([]*model.GroupMember, 0, len(members))
	for _, member := range members {
		if member == nil {
			continue
		}
		copyMember := *member
		cloned = append(cloned, &copyMember)
	}
	return cloned
}

// sortGroupInfos 统一用户群列表排序规则。
//
// 排序与 DB 查询保持一致：
//  1. 群更新时间新的在前；
//  2. 更新时间相同时，ID 大的在前；
//  3. 最后用群 UUID 保证稳定顺序。
func sortGroupInfos(groups []*model.GroupInfo) {
	sort.SliceStable(groups, func(i, j int) bool {
		left := groups[i]
		right := groups[j]
		if left == nil || right == nil {
			return right == nil
		}
		if !left.UpdatedAt.Equal(right.UpdatedAt) {
			return left.UpdatedAt.After(right.UpdatedAt)
		}
		if left.Id != right.Id {
			return left.Id > right.Id
		}
		return left.Uuid > right.Uuid
	})
}

// sortGroupMembers 统一群成员排序规则。
//
// 排序与 DB 查询保持一致：
//  1. 角色高的在前；
//  2. 入群早的在前；
//  3. 最后用 user_uuid 保证稳定顺序。
func sortGroupMembers(members []*model.GroupMember) {
	sort.SliceStable(members, func(i, j int) bool {
		left := members[i]
		right := members[j]
		if left == nil || right == nil {
			return right == nil
		}
		if left.Role != right.Role {
			return left.Role > right.Role
		}
		if !left.JoinedAt.Equal(right.JoinedAt) {
			return left.JoinedAt.Before(right.JoinedAt)
		}
		return left.UserUuid < right.UserUuid
	})
}

// isRedisWrongType 用于识别 Redis key 类型污染。
//
// 一旦发现类型错误，调用方按 miss 回源；需要清理时必须放进同一段 Lua，
// 或交给版本化对账覆盖，避免“先读到旧值、后 DEL 掉并发新值”的删除竞态。
func isRedisWrongType(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "WRONGTYPE")
}

// decodeStrictCacheJSON 只接受当前结构声明过的字段，并拒绝尾随 JSON。
//
// 缓存不是跨版本兼容接口：新旧二进制混跑时，未知字段不会被静默忽略，而是让该
// key 失效并回源。这使结构升级失败能够快速暴露，也避免“看似命中、实则只解出一半”
// 的隐性数据错误。
func decodeStrictCacheJSON(raw string, dst any) error {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("缓存值包含多个 JSON")
		}
		return fmt.Errorf("缓存值包含尾随数据: %w", err)
	}
	return nil
}

// getRandomExpireTime 为基础 TTL 增加 10% 抖动，降低缓存雪崩概率。
func getRandomExpireTime(baseExpire time.Duration) time.Duration {
	jitterRange := float64(baseExpire) * 0.1
	jitter := time.Duration(rand.Float64()*float64(jitterRange)*2 - float64(jitterRange))
	return baseExpire + jitter
}

// getRandomBool 用于执行低概率续期等轻量后台优化。
func getRandomBool(probability float64) bool {
	return rand.Float64() < probability
}
