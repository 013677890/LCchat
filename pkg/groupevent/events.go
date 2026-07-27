package groupevent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// EventTypeGroupCache 是群缓存投影统一使用的 outbox 事件类型。
const (
	EventTypeGroupCache = "group.cache"
	// GroupCacheSchemaVersion 是当前且唯一可接受的 group.cache 事件结构版本。
	//
	// 消费端不会把缺失版本当作 v0，也不会递归拆 Debezium/字符串包装来兼容旧消息；
	// CDC 必须按当前 connector 契约直接输出 payload JSON，否则消息会立即进入死信。
	GroupCacheSchemaVersion int32 = 2
)

const (
	// ActionGroupCreated 表示群聚合首次创建后的全量初始化事件。
	ActionGroupCreated = "group_created"
	// ActionMemberAdded 表示群成员新增或恢复入群事件。
	ActionMemberAdded = "member_added"
	// ActionMemberRemoved 表示成员退群或被移除事件。
	ActionMemberRemoved = "member_removed"
	// ActionGroupDismissed 表示群已被解散，读写都应转入终态。
	ActionGroupDismissed = "group_dismissed"
	// ActionGroupInfoUpdated 表示群资料字段被更新。
	ActionGroupInfoUpdated = "group_info_updated"
	// ActionOwnerTransferred 表示群主身份已在成员之间完成转移。
	ActionOwnerTransferred = "owner_transferred"
	// ActionMemberRoleUpdated 表示普通成员与管理员之间的角色切换。
	ActionMemberRoleUpdated = "member_role_updated"
	// ActionMemberProfileUpdated 表示成员群名片等群内个人资料已更新。
	ActionMemberProfileUpdated = "member_profile_updated"
	// ActionMemberMuted 表示成员单人禁言截止时间已更新。
	ActionMemberMuted = "member_muted"
	// ActionGroupMuteSettingUpdated 表示群全员禁言开关已更新。
	ActionGroupMuteSettingUpdated = "group_mute_setting_updated"
	// ActionJoinRequestCreated 表示新增了一条待审批入群申请。
	ActionJoinRequestCreated = "join_request_created"
	// ActionJoinRequestReviewed 表示一条待审批入群申请已被处理，应从待审批缓存移除。
	ActionJoinRequestReviewed = "join_request_reviewed"
	// ActionJoinRequestCanceled 表示申请人主动撤销了待审批入群申请，应从待审批缓存移除。
	ActionJoinRequestCanceled = "join_request_canceled"
)

// GroupSnapshot 描述单条群聚合快照。
type GroupSnapshot struct {
	GroupID         int64  `json:"group_id"`
	GroupUUID       string `json:"group_uuid"`
	Name            string `json:"name"`
	Avatar          string `json:"avatar"`
	Notice          string `json:"notice"`
	OwnerUUID       string `json:"owner_uuid"`
	MemberCount     int32  `json:"member_count"`
	AddMode         int32  `json:"add_mode"`
	MuteAll         bool   `json:"mute_all"`
	Status          int32  `json:"status"`
	UpdatedAtUnixMs int64  `json:"updated_at_unix_ms"`
}

// GroupMemberSnapshot 描述单条群成员快照。
type GroupMemberSnapshot struct {
	UserUUID        string `json:"user_uuid"`
	Role            int32  `json:"role"`
	Remark          string `json:"remark"`
	MuteUntilUnixMs int64  `json:"mute_until_unix_ms"`
	JoinedAtUnixMs  int64  `json:"joined_at_unix_ms"`
}

// GroupJoinRequestSnapshot 描述单条待审批入群申请快照。
type GroupJoinRequestSnapshot struct {
	ApplyID         int64  `json:"apply_id"`
	ApplicantUUID   string `json:"applicant_uuid"`
	Reason          string `json:"reason"`
	CreatedAtUnixMs int64  `json:"created_at_unix_ms"`
}

// GroupCacheEventPayload 是 group.cache 主题的统一事件载体。
//
// 所有群写事实都收敛到同一 topic，再由 action 决定投影器如何 patch Redis，
// 这样可以复用 Kafka 同 key 有序语义，避免同一群事件散落到多个 topic 后失序。
type GroupCacheEventPayload struct {
	SchemaVersion     int32                     `json:"schema_version"`
	ProjectionVersion int64                     `json:"projection_version"`
	EventID           string                    `json:"event_id"`
	Action            string                    `json:"action"`
	GroupUUID         string                    `json:"group_uuid"`
	OperatorUUID      string                    `json:"operator_uuid,omitempty"`
	Group             *GroupSnapshot            `json:"group,omitempty"`
	Members           []GroupMemberSnapshot     `json:"members,omitempty"`
	JoinRequest       *GroupJoinRequestSnapshot `json:"join_request,omitempty"`
	UserUUID          string                    `json:"user_uuid,omitempty"`
	UserUUIDs         []string                  `json:"user_uuids,omitempty"`
}

// Encode 把群缓存事件编码成 JSON 字符串。
func Encode(payload any) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// DecodeGroupCache 解析 group.cache 事件消息。
func DecodeGroupCache(message []byte) (GroupCacheEventPayload, error) {
	var payload GroupCacheEventPayload
	trimmed := bytes.TrimSpace(message)
	if len(trimmed) == 0 {
		return payload, fmt.Errorf("group.cache payload 为空")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return GroupCacheEventPayload{}, fmt.Errorf("group.cache payload 不是当前严格 JSON 结构: %w", err)
	}
	// 第二次 Decode 必须直接到 EOF，明确拒绝尾随第二个 JSON 值或其他脏数据。
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return GroupCacheEventPayload{}, fmt.Errorf("group.cache payload 含多个 JSON 值")
		}
		return GroupCacheEventPayload{}, fmt.Errorf("group.cache payload 含尾随数据: %w", err)
	}
	if payload.SchemaVersion != GroupCacheSchemaVersion {
		return GroupCacheEventPayload{}, fmt.Errorf(
			"group.cache schema_version=%d，当前只接受 %d",
			payload.SchemaVersion,
			GroupCacheSchemaVersion,
		)
	}
	if payload.ProjectionVersion <= 0 {
		return GroupCacheEventPayload{}, fmt.Errorf("group.cache projection_version 必须大于 0")
	}
	if payload.EventID == "" || payload.GroupUUID == "" || payload.Action == "" {
		return GroupCacheEventPayload{}, fmt.Errorf("group.cache payload 缺少 event_id/action/group_uuid")
	}
	return payload, nil
}
