package repository

import (
	"encoding/json"

	"github.com/013677890/LCchat-Backend/model"

	"github.com/013677890/LCchat-Backend/pkg/groupevent"

	"math/rand"

	"sort"

	"strings"

	"time"
)

const (

	// groupInfoEmptyValue 是群资料空值缓存占位。

	//

	// 这里显式缓存“查无此群”，可以避免恶意探测或热点 miss

	// 在短时间内持续穿透到 MySQL。

	groupInfoEmptyValue = "{}"

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
	GroupID int64 `json:"group_id,omitempty"`

	GroupUUID string `json:"group_uuid"`

	Name string `json:"name"`

	Avatar string `json:"avatar"`

	Notice string `json:"notice"`

	OwnerUUID string `json:"owner_uuid"`

	MemberCount int `json:"member_count"`

	AddMode int8 `json:"add_mode"`

	MuteAll bool `json:"mute_all"`

	Status int8 `json:"status"`

	UpdatedAtUnix int64 `json:"updated_at_unix,omitempty"`

	UpdatedAtUnixMs int64 `json:"updated_at_unix_ms,omitempty"`
}

// groupMemberCacheEntry 是 group:members:{group_uuid} Hash 中单个 field 的值结构。

//

// 当前权限判断和成员展示依赖 role / remark / mute_until / joined_at，

// 因此缓存成员快照只保留这些稳定事实字段，不冗余用户昵称头像。

type groupMemberCacheEntry struct {
	Role int8 `json:"role"`

	Remark string `json:"remark"`

	MuteUntilUnixMs int64 `json:"mute_until_unix_ms"`

	JoinedAtUnix int64 `json:"joined_at_unix"`
}

// groupJoinRequestCacheEntry 是 group:join_requests:{group_uuid} Hash 中单条申请的缓存值结构。

//

// 这里缓存申请事实本身，而不是连同昵称头像一起缓存，原因是：

//  1. 申请人的展示资料仍以 user_profile 为准；

//  2. 申请列表缓存主要目标是减少 group_join_requests 热点回源；

//  3. 资料更新频率和申请流转频率不同，拆开缓存更容易保持职责清晰。

type groupJoinRequestCacheEntry struct {
	ApplyID int64 `json:"apply_id"`

	ApplicantUUID string `json:"applicant_uuid"`

	Reason string `json:"reason"`

	CreatedAtUnixM int64 `json:"created_at_unix_ms"`
}

// encodeGroupInfoCacheValue 把群模型编码成 Redis String 值。

//

// 即便 JSON 编码异常，这里也返回一个稳定的空对象字符串，

// 避免上层把编码失败传播成业务错误并打断主流程。

func encodeGroupInfoCacheValue(group *model.GroupInfo) string {

	entry := groupInfoCacheEntry{}

	if group != nil {
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

		entry.UpdatedAtUnix = group.UpdatedAt.Unix()

		entry.UpdatedAtUnixMs = group.UpdatedAt.UnixMilli()

	}

	data, err := json.Marshal(&entry)

	if err != nil {

		return groupInfoEmptyValue

	}

	return string(data)

}

// decodeGroupInfoCacheValue 解析群资料缓存 JSON。

func decodeGroupInfoCacheValue(raw string) (*groupInfoCacheEntry, error) {

	var entry groupInfoCacheEntry

	if err := json.Unmarshal([]byte(raw), &entry); err != nil {

		return nil, err

	}

	return &entry, nil

}

// buildGroupInfoFromCache 把缓存 entry 还原为读路径使用的群模型。

func buildGroupInfoFromCache(entry *groupInfoCacheEntry) *model.GroupInfo {

	if entry == nil || entry.GroupUUID == "" {

		return nil

	}

	group := &model.GroupInfo{
		Id: entry.GroupID,

		Uuid: entry.GroupUUID,

		Name: entry.Name,

		Avatar: entry.Avatar,

		Notice: entry.Notice,

		OwnerUuid: entry.OwnerUUID,

		MemberCnt: entry.MemberCount,

		AddMode: entry.AddMode,

		MuteAll: entry.MuteAll,

		Status: entry.Status,
	}

	if entry.UpdatedAtUnixMs > 0 {

		group.UpdatedAt = time.UnixMilli(entry.UpdatedAtUnixMs)

	} else if entry.UpdatedAtUnix > 0 {

		group.UpdatedAt = time.Unix(entry.UpdatedAtUnix, 0)

	}

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
		Id: snapshot.GroupID,

		Uuid: snapshot.GroupUUID,

		Name: snapshot.Name,

		Avatar: snapshot.Avatar,

		Notice: snapshot.Notice,

		OwnerUuid: snapshot.OwnerUUID,

		MemberCnt: int(snapshot.MemberCount),

		AddMode: int8(snapshot.AddMode),

		MuteAll: snapshot.MuteAll,

		Status: int8(snapshot.Status),
	}

	if snapshot.UpdatedAtUnixMs > 0 {

		group.UpdatedAt = time.UnixMilli(snapshot.UpdatedAtUnixMs)

	} else if snapshot.UpdatedAtUnix > 0 {

		group.UpdatedAt = time.Unix(snapshot.UpdatedAtUnix, 0)

	}

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

		entry.JoinedAtUnix = member.JoinedAt.Unix()

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

	if err := json.Unmarshal([]byte(raw), &entry); err != nil {

		return nil, err

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

	if entry.JoinedAtUnix > 0 {

		member.JoinedAt = time.Unix(entry.JoinedAtUnix, 0)

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

		UserUuid: snapshot.UserUUID,

		Role: int8(snapshot.Role),

		Remark: snapshot.Remark,
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

// 这里统一复用 member model，是为了让 projector 直接调用现有的

// rebuild / upsert / insert 等缓存 helper，而不是重复维护第二套结构。

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

	if err := json.Unmarshal([]byte(raw), &entry); err != nil {

		return nil, err

	}

	return &entry, nil

}

// buildGroupJoinRequestFromCache 把缓存 entry 还原为仓储层申请模型。

func buildGroupJoinRequestFromCache(entry *groupJoinRequestCacheEntry) *model.GroupJoinRequest {

	if entry == nil || entry.ApplyID <= 0 || entry.ApplicantUUID == "" {

		return nil

	}

	request := &model.GroupJoinRequest{

		Id: entry.ApplyID,

		ApplicantUuid: entry.ApplicantUUID,

		Reason: entry.Reason,

		Status: joinRequestStatusPending,
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

		Id: snapshot.ApplyID,

		ApplicantUuid: snapshot.ApplicantUUID,

		Reason: snapshot.Reason,

		Status: joinRequestStatusPending,
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

// cloneGroupMembers 深拷贝成员切片，防止异步任务读到后续被复用/篡改的数据。

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

// cloneGroupInfos 深拷贝群资料切片，避免异步缓存重建和调用方共享底层对象。

func cloneGroupInfos(groups []*model.GroupInfo) []*model.GroupInfo {

	if len(groups) == 0 {

		return []*model.GroupInfo{}

	}

	cloned := make([]*model.GroupInfo, 0, len(groups))

	for _, group := range groups {

		if group == nil {

			continue

		}

		copyGroup := *group

		cloned = append(cloned, &copyGroup)

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

// 一旦发现类型错误，当前项目的策略是删除脏 key，

// 让后续读路径按正常 miss 逻辑回源并重建。

func isRedisWrongType(err error) bool {

	if err == nil {

		return false

	}

	return strings.Contains(err.Error(), "WRONGTYPE")

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
