package groupevent

import (
	"bytes"
	"encoding/json"
	"errors"
)

// EventTypeGroupCache 是群缓存投影统一使用的 outbox 事件类型。
const EventTypeGroupCache = "group.cache"
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
	// ActionJoinRequestCreated 表示新增了一条待审批入群申请。
	ActionJoinRequestCreated = "join_request_created"
	// ActionJoinRequestReviewed 表示一条待审批入群申请已被处理，应从待审批缓存移除。
	ActionJoinRequestReviewed = "join_request_reviewed"
	// ActionJoinRequestCanceled 表示申请人主动撤销了待审批入群申请，应从待审批缓存移除。
	ActionJoinRequestCanceled = "join_request_canceled"
)

// GroupSnapshot 描述单条群聚合快照。
type GroupSnapshot struct {
	GroupUUID     string `json:"group_uuid"`
	Name          string `json:"name"`
	Avatar        string `json:"avatar"`
	Notice        string `json:"notice"`
	OwnerUUID     string `json:"owner_uuid"`
	MemberCount   int32  `json:"member_count"`
	AddMode       int32  `json:"add_mode"`
	Status        int32  `json:"status"`
	UpdatedAtUnix int64  `json:"updated_at_unix"`
}

// GroupMemberSnapshot 描述单条群成员快照。
type GroupMemberSnapshot struct {
	UserUUID       string `json:"user_uuid"`
	Role           int32  `json:"role"`
	JoinedAtUnixMs int64  `json:"joined_at_unix_ms"`
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
	EventID        string                    `json:"event_id"`
	Action         string                    `json:"action"`
	GroupUUID      string                    `json:"group_uuid"`
	OperatorUUID   string                    `json:"operator_uuid,omitempty"`
	Group          *GroupSnapshot            `json:"group,omitempty"`
	Members        []GroupMemberSnapshot     `json:"members,omitempty"`
	JoinRequest    *GroupJoinRequestSnapshot `json:"join_request,omitempty"`
	UserUUID       string                    `json:"user_uuid,omitempty"`
	UserUUIDs      []string                  `json:"user_uuids,omitempty"`
	JoinedAtUnixMs int64                     `json:"joined_at_unix_ms,omitempty"`
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
	return decodeEventPayload(message, func(payload *GroupCacheEventPayload) bool {
		return payload.EventID != "" && payload.GroupUUID != "" && payload.Action != ""
	})
}

// decodeEventPayload 在多种嵌套形态里提取事件 payload。
func decodeEventPayload[T any](message []byte, isValid func(*T) bool) (T, error) {
	var zero T
	var lastErr error
	for _, candidate := range collectPayloadCandidates(message, 0, map[string]struct{}{}) {
		var payload T
		if err := json.Unmarshal(candidate, &payload); err != nil {
			lastErr = err
			continue
		}
		if isValid(&payload) {
			return payload, nil
		}
	}
	if lastErr != nil {
		return zero, lastErr
	}
	return zero, errors.New("event payload missing required fields")
}

// collectPayloadCandidates 递归展开 Debezium/字符串包裹后的候选消息体。
func collectPayloadCandidates(raw []byte, depth int, visited map[string]struct{}) [][]byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || depth > 4 {
		return nil
	}
	key := string(trimmed)
	if _, exists := visited[key]; exists {
		return nil
	}
	visited[key] = struct{}{}
	results := [][]byte{trimmed}
	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		results = append(results, collectPayloadCandidates([]byte(text), depth+1, visited)...)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err != nil {
		return results
	}
	for _, field := range []string{"payload", "after", "data"} {
		candidate, ok := object[field]
		if !ok {
			continue
		}
		results = append(results, collectPayloadCandidates(candidate, depth+1, visited)...)
	}
	return results
}
