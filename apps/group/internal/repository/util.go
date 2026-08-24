package repository

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/event"
)

const (
	// GroupCacheSchemaVersion 是 Redis 群缓存当前且唯一支持的结构版本。
	//
	// 这里故意不把缺失字段解释为旧版本：任何没有显式 schema/version 元数据的
	// String、Hash 或 ZSet 都会被拒绝；点查 Lua 会原子删除，完整集合读则按 miss
	// 交给版本化对账覆盖，绝不把旧字段映射成当前结构。
	GroupCacheSchemaVersion = "2"
	// GroupProjectionSchemaField / GroupProjectionVersionField / GroupProjectionCompleteField
	// 是版本化 Hash 的保留字段。业务 UUID 和申请 ID 都不能使用 "__" 前缀。
	GroupProjectionSchemaField  = "__SCHEMA__"
	GroupProjectionVersionField = "__VERSION__"
	// GroupProjectionCompleteField 证明 Hash 来自一次完整 replace，而不是只写了
	// schema/version 的中间态。增量 Lua 必须先检查它，才能以 O(1) 成本安全 patch；
	// v2 之前或任何缺少该标记的 Hash 都直接失效，不做结构兼容。
	GroupProjectionCompleteField = "__COMPLETE__"
	// UserGroupsReadyField 只由一次完整的用户群关系对账写入。
	//
	// 单条 Kafka 事件可以先创建版本 tombstone，但不会把局部集合标成完整缓存；
	// 读路径仅在 READY=1 时命中，从而避免“先到的一条 member_added”制造残缺群列表。
	UserGroupsReadyField = "__READY__"
	// GroupInfoEmptyValue 是群资料空值缓存占位。
	//
	// 这里显式缓存“查无此群”，可以避免恶意探测或热点 miss
	// 在短时间内持续穿透到 MySQL。0 是唯一允许的非正版本，且只能与该固定
	// NOT_FOUND 载荷组合；正常群快照必须使用大于 0 的数据库投影版本。
	GroupInfoEmptyValue = GroupCacheSchemaVersion + "|0|__NOT_FOUND__"
	// GroupMembersEmptyField 是群成员 Hash 的空集合占位 field。
	//
	// Redis Hash 为空时会被 Redis 直接删除，因此如果需要表达
	// “这个群目前成员集合为空但缓存是有效的”，就必须保留一个哨兵 field。
	GroupMembersEmptyField = "__EMPTY__"
	// GroupMembersEmptyValue 是空集合占位 field 的固定值。
	GroupMembersEmptyValue = "{}"
	// UserGroupsEmptyValue 是用户群列表 ZSet 的空集合占位 member。
	UserGroupsEmptyValue = "__EMPTY__"
	// GroupJoinRequestsEmptyField 是待审批申请 Hash 的空集合占位 field。
	GroupJoinRequestsEmptyField = "__EMPTY__"
	// GroupJoinRequestsEmptyValue 是待审批申请空集合占位值。
	GroupJoinRequestsEmptyValue = "{}"
)

// GroupInfoCacheEntry 是 group:info:{group_uuid} 的缓存载体。
//
// 这里不直接缓存整个 GORM 模型，而是只保留读路径真正需要的字段，
// 这样可以：
//  1. 降低 JSON 体积；
//  2. 避免把 DB 内部字段泄露给缓存格式；
//  3. 给未来缓存结构调整留出独立演化空间。
type GroupInfoCacheEntry struct {
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

// GroupMemberCacheEntry 是 group:members:{group_uuid} Hash 中单个 field 的值结构。
//
// 当前权限判断和成员展示依赖 role / remark / mute_until / joined_at，
// 因此缓存成员快照只保留这些稳定事实字段，不冗余用户昵称头像。
type GroupMemberCacheEntry struct {
	Role            int8   `json:"role"`
	Remark          string `json:"remark"`
	MuteUntilUnixMs int64  `json:"mute_until_unix_ms"`
	JoinedAtUnixMs  int64  `json:"joined_at_unix_ms"`
}

// GroupJoinRequestCacheEntry 是 group:join_requests:{group_uuid} Hash 中单条申请的缓存值结构。
//
// 这里缓存申请事实本身，而不是连同昵称头像一起缓存，原因是：
//  1. 申请人的展示资料仍以 user_profile 为准；
//  2. 申请列表缓存主要目标是减少 group_join_requests 热点回源；
//  3. 资料更新频率和申请流转频率不同，拆开缓存更容易保持职责清晰。
type GroupJoinRequestCacheEntry struct {
	ApplyID        int64  `json:"apply_id"`
	ApplicantUUID  string `json:"applicant_uuid"`
	Reason         string `json:"reason"`
	CreatedAtUnixM int64  `json:"created_at_unix_ms"`
}

// EncodeGroupInfoCacheValue 把群模型编码成严格版本化 Redis String。
//
// 格式固定为 "<schema>|<projection_version>|<json>"。Lua 只需解析前两个十进制
// 字段就能在单次原子操作里比较版本，不依赖 Redis 内并不存在的通用 JSON 解析能力。
// 编码失败必须显式返回错误，禁止再用 "{}" 伪装成一条可用缓存。
func EncodeGroupInfoCacheValue(group *model.GroupInfo, projectionVersion int64) (string, error) {
	if group == nil ||
		group.Id <= 0 ||
		group.Uuid == "" ||
		group.OwnerUuid == "" ||
		group.MemberCnt < 0 ||
		(group.AddMode != 0 && group.AddMode != 1) ||
		(group.Status != GroupStatusNormal &&
			group.Status != GroupStatusDisabled &&
			group.Status != GroupStatusDismissed) ||
		projectionVersion <= 0 ||
		group.UpdatedAt.UnixMilli() <= 0 {
		return "", fmt.Errorf("群资料缓存包含非法必填字段或 projection_version")
	}
	entry := GroupInfoCacheEntry{
		GroupID:         group.Id,
		GroupUUID:       group.Uuid,
		Name:            group.Name,
		Avatar:          group.Avatar,
		Notice:          group.Notice,
		OwnerUUID:       group.OwnerUuid,
		MemberCount:     group.MemberCnt,
		AddMode:         group.AddMode,
		MuteAll:         group.MuteAll,
		Status:          group.Status,
		UpdatedAtUnixMs: group.UpdatedAt.UnixMilli(),
	}
	data, err := json.Marshal(&entry)
	if err != nil {
		return "", fmt.Errorf("编码群资料缓存失败: %w", err)
	}
	return GroupCacheSchemaVersion + "|" + strconv.FormatInt(projectionVersion, 10) + "|" + string(data), nil
}

// DecodeGroupInfoCacheValue 解析严格版本化的群资料缓存。
//
// 返回的 empty 仅表示命中当前格式的 NOT_FOUND 占位。旧 "{}"、缺少版本前缀、
// 未知 schema 或带未知 JSON 字段的值都会报错，调用方按 miss 回源并触发权威对账。
func DecodeGroupInfoCacheValue(raw string) (entry *GroupInfoCacheEntry, projectionVersion int64, empty bool, err error) {
	parts := strings.SplitN(raw, "|", 3)
	if len(parts) != 3 || parts[0] != GroupCacheSchemaVersion {
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
	var decoded GroupInfoCacheEntry
	if err := DecodeStrictCacheJSON(parts[2], &decoded); err != nil {
		return nil, 0, false, err
	}
	if decoded.GroupID <= 0 ||
		decoded.GroupUUID == "" ||
		decoded.OwnerUUID == "" ||
		decoded.MemberCount < 0 ||
		(decoded.AddMode != 0 && decoded.AddMode != 1) ||
		(decoded.Status != GroupStatusNormal &&
			decoded.Status != GroupStatusDisabled &&
			decoded.Status != GroupStatusDismissed) ||
		decoded.UpdatedAtUnixMs <= 0 {
		return nil, 0, false, fmt.Errorf("群资料缓存缺少必填字段")
	}

	return &decoded, projectionVersion, false, nil
}

// BuildGroupInfoFromCache 把缓存 entry 还原为读路径使用的群模型。
func BuildGroupInfoFromCache(entry *GroupInfoCacheEntry, projectionVersion int64) *model.GroupInfo {
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

// BuildGroupInfoFromSnapshot 把 outbox 事件里的群快照还原为仓储层模型。
//
// projector 侧不应为了回填缓存再回源 MySQL，
// 因此这里直接把事件携带的最终态快照转换成可复用的 model.GroupInfo。
func BuildGroupInfoFromSnapshot(snapshot *event.GroupSnapshot) *model.GroupInfo {
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

// EncodeGroupMemberCacheValue 把群成员模型编码成 Hash field 值。
//
// 编码失败必须显式返回错误，禁止再用 "{}" 伪装成一条可用缓存；
// 读路径的严格解码会拒绝这种假值，等于把投影失败藏成一次 miss。
func EncodeGroupMemberCacheValue(member *model.GroupMember) (string, error) {
	if member == nil ||
		member.Role < MemberRoleMember ||
		member.Role > MemberRoleOwner ||
		member.JoinedAt.UnixMilli() <= 0 {
		return "", fmt.Errorf("群成员缓存包含非法必填字段")
	}
	entry := GroupMemberCacheEntry{
		Role:           member.Role,
		Remark:         member.Remark,
		JoinedAtUnixMs: member.JoinedAt.UnixMilli(),
	}
	if member.MuteUntil != nil {
		muteUntilUnixMs := member.MuteUntil.UnixMilli()
		if muteUntilUnixMs < 0 {
			return "", fmt.Errorf("群成员缓存禁言时间非法")
		}
		entry.MuteUntilUnixMs = muteUntilUnixMs
	}
	data, err := json.Marshal(&entry)
	if err != nil {
		return "", fmt.Errorf("编码群成员缓存失败: %w", err)
	}
	return string(data), nil
}

// DecodeGroupMemberCacheValue 解析群成员缓存值。
func DecodeGroupMemberCacheValue(raw string) (*GroupMemberCacheEntry, error) {
	var entry GroupMemberCacheEntry
	if err := DecodeStrictCacheJSON(raw, &entry); err != nil {
		return nil, err
	}
	if entry.Role < MemberRoleMember ||
		entry.Role > MemberRoleOwner ||
		entry.MuteUntilUnixMs < 0 ||
		entry.JoinedAtUnixMs <= 0 {
		return nil, fmt.Errorf("群成员缓存字段值非法")
	}
	return &entry, nil
}

// BuildGroupMemberFromCache 把单个缓存 field 还原为成员模型。
func BuildGroupMemberFromCache(userUUID string, entry *GroupMemberCacheEntry) *model.GroupMember {
	if userUUID == "" || userUUID == GroupMembersEmptyField || entry == nil {
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

// BuildGroupMemberFromSnapshot 把事件快照还原为成员模型。
func BuildGroupMemberFromSnapshot(groupUUID string, snapshot event.GroupMemberSnapshot) *model.GroupMember {
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

// BuildGroupMembersFromSnapshots 批量把事件成员快照还原成成员模型。
//
// 这里统一复用 member model，是为了让 projector 和权威对账共用同一套严格缓存编码，
// 避免事件快照与数据库快照分别维护两份容易漂移的字段映射。
func BuildGroupMembersFromSnapshots(groupUUID string, snapshots []event.GroupMemberSnapshot) []*model.GroupMember {
	if groupUUID == "" || len(snapshots) == 0 {
		return []*model.GroupMember{}
	}
	members := make([]*model.GroupMember, 0, len(snapshots))
	for _, snapshot := range snapshots {
		member := BuildGroupMemberFromSnapshot(groupUUID, snapshot)
		if member == nil {
			continue
		}
		members = append(members, member)
	}
	return members
}

// EncodeGroupJoinRequestCacheValue 把入群申请模型编码成 Hash field 值。
//
// 与群资料编码相同：失败必须返回 error，禁止用 "{}" 占位后让读侧再当 miss。
func EncodeGroupJoinRequestCacheValue(request *model.GroupJoinRequest) (string, error) {
	if request == nil ||
		request.Id <= 0 ||
		request.ApplicantUuid == "" ||
		request.CreatedAt.UnixMilli() <= 0 {
		return "", fmt.Errorf("入群申请缓存包含非法必填字段")
	}
	entry := GroupJoinRequestCacheEntry{
		ApplyID:        request.Id,
		ApplicantUUID:  request.ApplicantUuid,
		Reason:         request.Reason,
		CreatedAtUnixM: request.CreatedAt.UnixMilli(),
	}
	data, err := json.Marshal(&entry)
	if err != nil {
		return "", fmt.Errorf("编码入群申请缓存失败: %w", err)
	}
	return string(data), nil
}

// DecodeGroupJoinRequestCacheValue 解析入群申请缓存值。
func DecodeGroupJoinRequestCacheValue(raw string) (*GroupJoinRequestCacheEntry, error) {
	var entry GroupJoinRequestCacheEntry
	if err := DecodeStrictCacheJSON(raw, &entry); err != nil {
		return nil, err
	}
	if entry.ApplyID <= 0 || entry.ApplicantUUID == "" || entry.CreatedAtUnixM <= 0 {
		return nil, fmt.Errorf("入群申请缓存字段值非法")
	}
	return &entry, nil
}

// BuildGroupJoinRequestFromCache 把缓存 entry 还原为仓储层申请模型。
func BuildGroupJoinRequestFromCache(entry *GroupJoinRequestCacheEntry) *model.GroupJoinRequest {
	if entry == nil || entry.ApplyID <= 0 || entry.ApplicantUUID == "" {
		return nil
	}
	request := &model.GroupJoinRequest{
		Id:            entry.ApplyID,
		ApplicantUuid: entry.ApplicantUUID,
		Reason:        entry.Reason,
		Status:        JoinRequestStatusPending,
	}
	if entry.CreatedAtUnixM > 0 {
		request.CreatedAt = time.UnixMilli(entry.CreatedAtUnixM)
	}
	return request
}

// BuildGroupJoinRequestFromSnapshot 把事件快照还原成申请模型。
func BuildGroupJoinRequestFromSnapshot(snapshot *event.GroupJoinRequestSnapshot) *model.GroupJoinRequest {
	if snapshot == nil || snapshot.ApplyID <= 0 || snapshot.ApplicantUUID == "" {
		return nil
	}
	request := &model.GroupJoinRequest{
		Id:            snapshot.ApplyID,
		ApplicantUuid: snapshot.ApplicantUUID,
		Reason:        snapshot.Reason,
		Status:        JoinRequestStatusPending,
	}
	if snapshot.CreatedAtUnixMs > 0 {
		request.CreatedAt = time.UnixMilli(snapshot.CreatedAtUnixMs)
	}
	return request
}

// SortGroupJoinRequests 统一待审批申请列表排序规则。
//
// 排序与 DB 查询保持一致：
//  1. 创建时间新的在前；
//  2. 创建时间相同时，ID 大的在前。
func SortGroupJoinRequests(items []*model.GroupJoinRequest) {
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

// CloneGroupMembers 深拷贝 singleflight 结果，避免多个请求共享可变成员指针。
func CloneGroupMembers(members []*model.GroupMember) []*model.GroupMember {
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

// SortGroupInfos 统一用户群列表排序规则。
//
// 排序与 DB 查询保持一致：
//  1. 群更新时间新的在前；
//  2. 更新时间相同时，ID 大的在前；
//  3. 最后用群 UUID 保证稳定顺序。
func SortGroupInfos(groups []*model.GroupInfo) {
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

// SortGroupMembers 统一群成员排序规则。
//
// 排序与 DB 查询保持一致：
//  1. 角色高的在前；
//  2. 入群早的在前；
//  3. 最后用 user_uuid 保证稳定顺序。
func SortGroupMembers(members []*model.GroupMember) {
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

// DecodeStrictCacheJSON 只接受当前结构声明过的字段，并拒绝尾随 JSON。
//
// 缓存不是跨版本兼容接口：新旧二进制混跑时，未知字段不会被静默忽略，而是让该
// key 失效并回源。这使结构升级失败能够快速暴露，也避免“看似命中、实则只解出一半”
// 的隐性数据错误。
func DecodeStrictCacheJSON(raw string, dst any) error {
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
