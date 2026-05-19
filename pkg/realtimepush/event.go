package realtimepush

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// DefaultTopic 是非消息类实时提醒统一使用的默认 Kafka Topic。
const DefaultTopic = "realtime.push"

const (
	// TypeFriendApplyCreated 表示目标用户收到新的好友申请。
	TypeFriendApplyCreated = "FRIEND_APPLY_CREATED"
	// TypeFriendApplyHandled 表示好友申请已被同意或拒绝。
	TypeFriendApplyHandled = "FRIEND_APPLY_HANDLED"
	// TypeFriendRelationChanged 表示好友、拉黑等关系状态发生变化。
	TypeFriendRelationChanged = "FRIEND_RELATION_CHANGED"
	// TypeGroupJoinRequestCreated 表示群主或管理员收到新的入群申请。
	TypeGroupJoinRequestCreated = "GROUP_JOIN_REQUEST_CREATED"
	// TypeGroupJoinRequestReviewed 表示入群申请已完成审批。
	TypeGroupJoinRequestReviewed = "GROUP_JOIN_REQUEST_REVIEWED"
	// TypeGroupMemberRemoved 表示成员被移出群聊。
	TypeGroupMemberRemoved = "GROUP_MEMBER_REMOVED"
	// TypeGroupDismissed 表示群聊已解散。
	TypeGroupDismissed = "GROUP_DISMISSED"
	// TypeGroupInfoChanged 表示群资料、公告或加群方式发生变化。
	TypeGroupInfoChanged = "GROUP_INFO_CHANGED"
	// TypeGroupStateChanged 表示群聚合状态发生变化，客户端应按 changed 字段刷新本地视图。
	TypeGroupStateChanged = "GROUP_STATE_CHANGED"
	// TypeGroupRoleChanged 表示成员角色或群主身份发生变化。
	TypeGroupRoleChanged = "GROUP_ROLE_CHANGED"
	// TypeGroupMemberMuted 表示成员单人禁言状态发生变化。
	TypeGroupMemberMuted = "GROUP_MEMBER_MUTED"
	// TypeGroupMuteAllChanged 表示全员禁言开关发生变化。
	TypeGroupMuteAllChanged = "GROUP_MUTE_ALL_CHANGED"
)

const (
	// GroupChangedInfo 表示群名称、头像、加群方式等基础资料发生变化。
	GroupChangedInfo = "info"
	// GroupChangedNotice 表示群公告发生变化。
	GroupChangedNotice = "notice"
	// GroupChangedMembers 表示群成员集合发生变化。
	GroupChangedMembers = "members"
	// GroupChangedRoles 表示群成员角色或群主身份发生变化。
	GroupChangedRoles = "roles"
	// GroupChangedMuteAll 表示全员禁言开关发生变化。
	GroupChangedMuteAll = "mute_all"
	// GroupChangedMemberMute 表示成员单人禁言状态发生变化。
	GroupChangedMemberMute = "member_mute"
)

// TargetKind 描述实时提醒的目标类型。
type TargetKind string

const (
	// TargetKindUser 表示推送给单个用户的所有在线设备。
	TargetKindUser TargetKind = "user"
	// TargetKindDevice 表示推送给单个用户的指定在线设备。
	TargetKindDevice TargetKind = "device"
	// TargetKindUserList 表示推送给多个用户的所有在线设备。
	TargetKindUserList TargetKind = "user_list"
	// TargetKindGroupMembers 表示由 message-push 扩散到群内成员在线设备。
	TargetKindGroupMembers TargetKind = "group_members"
	// TargetKindGroupAdmins 表示由 message-push 扩散到群主和管理员在线设备。
	TargetKindGroupAdmins TargetKind = "group_admins"
)

// Target 描述实时提醒的投递目标。
type Target struct {
	Kind      TargetKind `json:"kind"`
	UserUUID  string     `json:"user_uuid,omitempty"`
	DeviceID  string     `json:"device_id,omitempty"`
	UserUUIDs []string   `json:"user_uuids,omitempty"`
	GroupUUID string     `json:"group_uuid,omitempty"`
}

// NewUserTarget 创建单用户目标。
func NewUserTarget(userUUID string) Target {
	return Target{Kind: TargetKindUser, UserUUID: userUUID}
}

// NewDeviceTarget 创建单设备目标。
func NewDeviceTarget(userUUID, deviceID string) Target {
	return Target{Kind: TargetKindDevice, UserUUID: userUUID, DeviceID: deviceID}
}

// NewUserListTarget 创建多用户目标。
func NewUserListTarget(userUUIDs []string) Target {
	return Target{Kind: TargetKindUserList, UserUUIDs: userUUIDs}
}

// NewGroupMembersTarget 创建群成员聚合目标。
func NewGroupMembersTarget(groupUUID string) Target {
	return Target{Kind: TargetKindGroupMembers, GroupUUID: groupUUID}
}

// NewGroupAdminsTarget 创建群管理员聚合目标。
func NewGroupAdminsTarget(groupUUID string) Target {
	return Target{Kind: TargetKindGroupAdmins, GroupUUID: groupUUID}
}

// Normalize 清理目标字段并对用户列表做去重。
func (t *Target) Normalize() {
	if t == nil {
		return
	}
	t.UserUUID = strings.TrimSpace(t.UserUUID)
	t.DeviceID = strings.TrimSpace(t.DeviceID)
	t.GroupUUID = strings.TrimSpace(t.GroupUUID)
	t.UserUUIDs = normalizeUserUUIDs(t.UserUUIDs)
}

// Validate 校验目标是否满足最小投递条件。
func (t Target) Validate() error {
	t.Normalize()
	switch t.Kind {
	case TargetKindUser:
		if t.UserUUID == "" {
			return errors.New("realtimepush: user target 缺少 user_uuid")
		}
	case TargetKindDevice:
		if t.UserUUID == "" || t.DeviceID == "" {
			return errors.New("realtimepush: device target 缺少 user_uuid 或 device_id")
		}
	case TargetKindUserList:
		if len(t.UserUUIDs) == 0 {
			return errors.New("realtimepush: user_list target 不能为空")
		}
	case TargetKindGroupMembers, TargetKindGroupAdmins:
		if t.GroupUUID == "" {
			return errors.New("realtimepush: group target 缺少 group_uuid")
		}
	default:
		return fmt.Errorf("realtimepush: 不支持的 target kind %q", t.Kind)
	}
	return nil
}

// PartitionKey 返回默认 Kafka 分区键，尽量让同一用户或同一群提醒保持相对有序。
func (t Target) PartitionKey() string {
	t.Normalize()
	if t.UserUUID != "" {
		return t.UserUUID
	}
	if t.GroupUUID != "" {
		return t.GroupUUID
	}
	if len(t.UserUUIDs) > 0 {
		return t.UserUUIDs[0]
	}
	return string(t.Kind)
}

// Event 是非消息类实时提醒统一事件。
type Event struct {
	Type        string `json:"type"`
	Target      Target `json:"target"`
	Data        []byte `json:"data,omitempty"`
	TraceID     string `json:"trace_id,omitempty"`
	ServerTs    int64  `json:"server_ts"`
	AckRequired bool   `json:"ack_required,omitempty"`
	Seq         int64  `json:"seq,omitempty"`
}

// GroupStateChangedPayload 是 GROUP_STATE_CHANGED 的推荐负载。
type GroupStateChangedPayload struct {
	GroupUUID string   `json:"group_uuid"`
	Changed   []string `json:"changed"`
	Version   int64    `json:"version,omitempty"`
}

// NewGroupStateChangedPayload 创建群状态刷新负载，并对 changed 做清理去重。
func NewGroupStateChangedPayload(groupUUID string, changed []string, version int64) GroupStateChangedPayload {
	return GroupStateChangedPayload{
		GroupUUID: strings.TrimSpace(groupUUID),
		Changed:   normalizeStrings(changed),
		Version:   version,
	}
}

// NewEvent 创建实时提醒事件。
func NewEvent(eventType string, target Target, data []byte) Event {
	return Event{Type: eventType, Target: target, Data: cloneBytes(data)}
}

// Normalize 清理事件字段，便于生产端统一生成稳定消息。
func (e *Event) Normalize() {
	if e == nil {
		return
	}
	e.Type = strings.TrimSpace(e.Type)
	e.TraceID = strings.TrimSpace(e.TraceID)
	e.Target.Normalize()
	e.Data = cloneBytes(e.Data)
}

// Validate 校验事件是否满足投递所需的最小字段。
func (e Event) Validate() error {
	e.Normalize()
	if e.Type == "" {
		return errors.New("realtimepush: type 不能为空")
	}
	if err := e.Target.Validate(); err != nil {
		return err
	}
	if e.Seq < 0 {
		return errors.New("realtimepush: seq 不能为负数")
	}
	if e.AckRequired && e.Seq <= 0 {
		return errors.New("realtimepush: ack_required=true 时 seq 必须大于 0")
	}
	return nil
}

// PartitionKey 返回事件默认 Kafka 分区键。
func (e Event) PartitionKey() string {
	e.Normalize()
	if key := e.Target.PartitionKey(); key != "" {
		return key
	}
	return e.Type
}

// Marshal 将事件编码为 Kafka 消息体。
func (e Event) Marshal() ([]byte, error) {
	e.Normalize()
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(e)
}

// Decode 解析 Kafka 中的实时提醒事件。
func Decode(message []byte) (Event, error) {
	if len(bytes.TrimSpace(message)) == 0 {
		return Event{}, errors.New("realtimepush: message 不能为空")
	}
	var event Event
	if err := json.Unmarshal(message, &event); err != nil {
		return Event{}, err
	}
	event.Normalize()
	if err := event.Validate(); err != nil {
		return Event{}, err
	}
	return event, nil
}

// EncodePayload 把业务负载编码成 Event.Data。
func EncodePayload(payload any) ([]byte, error) {
	switch value := payload.(type) {
	case nil:
		return nil, nil
	case []byte:
		return cloneBytes(value), nil
	case json.RawMessage:
		return cloneBytes(value), nil
	case string:
		return []byte(value), nil
	default:
		return json.Marshal(payload)
	}
}

// cloneBytes 复制字节切片，避免调用方后续修改影响事件内容。
func cloneBytes(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	cloned := make([]byte, len(data))
	copy(cloned, data)
	return cloned
}

// normalizeUserUUIDs 清理用户 UUID 列表并保持首次出现顺序去重。
func normalizeUserUUIDs(userUUIDs []string) []string {
	return normalizeStrings(userUUIDs)
}

// normalizeStrings 清理字符串列表并保持首次出现顺序去重。
func normalizeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}
