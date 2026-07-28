package groupevent

import (
	"errors"
	"fmt"
)

// ErrInvalidGroupCachePayload 表示事件不符合当前且唯一的 group.cache v2 语义。
//
// 该错误属于生产者/协议错误，重试不会自行恢复。所有 group.cache projector 必须共用
// ValidateGroupCachePayload，禁止每个消费者各自保留一套宽严不同的字段推断或兼容规则。
var ErrInvalidGroupCachePayload = errors.New("groupevent: invalid group.cache payload")

const (
	projectedGroupStatusNormal    int32 = 0
	projectedGroupStatusDisabled  int32 = 1
	projectedGroupStatusDismissed int32 = 2

	projectedMemberRoleMember int32 = 0
	projectedMemberRoleAdmin  int32 = 1
	projectedMemberRoleOwner  int32 = 2
)

// ValidateGroupCachePayload 校验 projector 可安全执行的完整 v2 终态语义。
//
// DecodeGroupCache 负责 JSON 结构、未知字段和 schema 基础校验；本函数继续校验 action
// 必需快照、目标集合一致性、角色和群状态。它不从 members 推导 user_uuids，不接受旧
// wrapper/schema，也不为缺失字段填默认值，因此 group Redis 与 msg 会话投影对同一事件
// 只有一种解释。
func ValidateGroupCachePayload(payload GroupCacheEventPayload) error {
	if payload.SchemaVersion != GroupCacheSchemaVersion {
		return invalidGroupCachePayload("unsupported schema_version %d", payload.SchemaVersion)
	}
	if payload.ProjectionVersion <= 0 {
		return invalidGroupCachePayload("projection_version must be positive")
	}
	if payload.EventID == "" || payload.GroupUUID == "" || payload.Action == "" {
		return invalidGroupCachePayload("missing base fields")
	}
	if payload.Group != nil {
		if payload.Group.GroupUUID != payload.GroupUUID {
			return invalidGroupCachePayload("group snapshot uuid mismatch")
		}
		if payload.Group.GroupID <= 0 ||
			payload.Group.OwnerUUID == "" ||
			payload.Group.MemberCount < 0 ||
			(payload.Group.AddMode != 0 && payload.Group.AddMode != 1) ||
			(payload.Group.Status != projectedGroupStatusNormal &&
				payload.Group.Status != projectedGroupStatusDisabled &&
				payload.Group.Status != projectedGroupStatusDismissed) ||
			payload.Group.UpdatedAtUnixMs <= 0 {
			return invalidGroupCachePayload("group snapshot contains invalid required fields")
		}
	}
	if len(payload.Members) > 0 && !validProjectedMemberSnapshots(payload.Members) {
		return invalidGroupCachePayload("member snapshots contain invalid required fields")
	}

	switch payload.Action {
	case ActionGroupCreated:
		if payload.Group == nil {
			return invalidGroupCachePayload("group_created missing group snapshot")
		}
		if len(payload.Members) == 0 {
			return invalidGroupCachePayload("group_created missing member snapshots")
		}
		if !sameProjectedMemberSet(payload.Members, payload.UserUUIDs) {
			return invalidGroupCachePayload("group_created user_uuids must exactly match members")
		}
		if payload.Group.Status != projectedGroupStatusNormal ||
			payload.Group.MemberCount != int32(len(payload.Members)) ||
			!validProjectedGroupOwnership(payload.Group, payload.Members) {
			return invalidGroupCachePayload("group_created final state is inconsistent")
		}
	case ActionMemberAdded:
		if payload.Group == nil {
			return invalidGroupCachePayload("member_added missing group snapshot")
		}
		if len(payload.Members) == 0 || len(payload.UserUUIDs) == 0 {
			return invalidGroupCachePayload("member_added missing target members")
		}
		if !sameProjectedMemberSet(payload.Members, payload.UserUUIDs) {
			return invalidGroupCachePayload("member_added user_uuids must exactly match members")
		}
		if payload.Group.Status != projectedGroupStatusNormal ||
			!allProjectedMembersHaveRole(payload.Members, projectedMemberRoleMember) {
			return invalidGroupCachePayload("member_added final state is inconsistent")
		}
	case ActionMemberRemoved:
		if payload.Group == nil || payload.UserUUID == "" {
			return invalidGroupCachePayload("member_removed missing required fields")
		}
		if payload.Group.Status != projectedGroupStatusNormal {
			return invalidGroupCachePayload("member_removed group must be normal")
		}
	case ActionGroupDismissed, ActionGroupInfoUpdated, ActionGroupMuteSettingUpdated:
		if payload.Group == nil {
			return invalidGroupCachePayload("%s missing group snapshot", payload.Action)
		}
		if payload.Action == ActionGroupDismissed {
			if payload.Group.Status != projectedGroupStatusDismissed {
				return invalidGroupCachePayload("group_dismissed snapshot must be dismissed")
			}
			if !validUniqueProjectedUUIDs(payload.UserUUIDs) {
				return invalidGroupCachePayload("group_dismissed contains invalid historical member uuids")
			}
		} else if payload.Group.Status != projectedGroupStatusNormal {
			return invalidGroupCachePayload("%s group must be normal", payload.Action)
		}
	case ActionOwnerTransferred, ActionMemberRoleUpdated, ActionMemberProfileUpdated, ActionMemberMuted:
		if payload.Group == nil {
			return invalidGroupCachePayload("%s missing group snapshot", payload.Action)
		}
		if payload.Group.Status != projectedGroupStatusNormal {
			return invalidGroupCachePayload("%s group must be normal", payload.Action)
		}
		if len(payload.Members) == 0 {
			return invalidGroupCachePayload("%s missing member snapshots", payload.Action)
		}
		if !sameProjectedMemberSet(payload.Members, payload.UserUUIDs) {
			return invalidGroupCachePayload("%s user_uuids must exactly match members", payload.Action)
		}
		switch payload.Action {
		case ActionOwnerTransferred:
			if !validProjectedOwnerTransfer(payload.Group, payload.Members) {
				return invalidGroupCachePayload("owner_transferred final state is inconsistent")
			}
		case ActionMemberRoleUpdated:
			if len(payload.Members) != 1 ||
				(payload.Members[0].Role != projectedMemberRoleMember &&
					payload.Members[0].Role != projectedMemberRoleAdmin) {
				return invalidGroupCachePayload("member_role_updated final state is inconsistent")
			}
		default:
			if len(payload.Members) != 1 {
				return invalidGroupCachePayload("%s must contain exactly one member", payload.Action)
			}
		}
	case ActionJoinRequestCreated, ActionJoinRequestReviewed, ActionJoinRequestCanceled:
		if payload.JoinRequest == nil ||
			payload.JoinRequest.ApplyID <= 0 ||
			payload.JoinRequest.ApplicantUUID == "" ||
			payload.JoinRequest.CreatedAtUnixMs <= 0 {
			return invalidGroupCachePayload("%s missing join request snapshot", payload.Action)
		}
	default:
		return invalidGroupCachePayload("unsupported action %s", payload.Action)
	}
	return nil
}

func invalidGroupCachePayload(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidGroupCachePayload, fmt.Sprintf(format, args...))
}

func validProjectedGroupOwnership(group *GroupSnapshot, members []GroupMemberSnapshot) bool {
	if group == nil || group.OwnerUUID == "" || len(members) == 0 {
		return false
	}
	ownerCount := 0
	for _, member := range members {
		if member.Role != projectedMemberRoleOwner {
			continue
		}
		if member.UserUUID != group.OwnerUUID {
			return false
		}
		ownerCount++
	}
	return ownerCount == 1
}

func allProjectedMembersHaveRole(members []GroupMemberSnapshot, role int32) bool {
	if len(members) == 0 {
		return false
	}
	for _, member := range members {
		if member.Role != role {
			return false
		}
	}
	return true
}

func validProjectedOwnerTransfer(group *GroupSnapshot, members []GroupMemberSnapshot) bool {
	if group == nil || len(members) != 2 || !validProjectedGroupOwnership(group, members) {
		return false
	}
	// 领域规则固定把旧群主降为普通成员，所以最终事件必须恰好是新群主 + 旧普通成员。
	memberCount := 0
	for _, member := range members {
		if member.Role == projectedMemberRoleMember {
			memberCount++
		}
	}
	return memberCount == 1
}

func validProjectedMemberSnapshots(members []GroupMemberSnapshot) bool {
	if len(members) == 0 {
		return false
	}
	for _, member := range members {
		if member.UserUUID == "" ||
			member.Role < projectedMemberRoleMember ||
			member.Role > projectedMemberRoleOwner ||
			member.MuteUntilUnixMs < 0 ||
			member.JoinedAtUnixMs <= 0 {
			return false
		}
	}
	return true
}

func validUniqueProjectedUUIDs(userUUIDs []string) bool {
	if len(userUUIDs) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(userUUIDs))
	for _, userUUID := range userUUIDs {
		if userUUID == "" {
			return false
		}
		if _, duplicate := seen[userUUID]; duplicate {
			return false
		}
		seen[userUUID] = struct{}{}
	}
	return true
}

// sameProjectedMemberSet 强制 members 与 user_uuids 显式表达同一集合。
func sameProjectedMemberSet(members []GroupMemberSnapshot, userUUIDs []string) bool {
	if len(members) == 0 || len(members) != len(userUUIDs) {
		return false
	}
	memberSet := make(map[string]struct{}, len(members))
	for _, member := range members {
		if member.UserUUID == "" {
			return false
		}
		if _, duplicate := memberSet[member.UserUUID]; duplicate {
			return false
		}
		memberSet[member.UserUUID] = struct{}{}
	}
	userSet := make(map[string]struct{}, len(userUUIDs))
	for _, userUUID := range userUUIDs {
		if userUUID == "" {
			return false
		}
		if _, duplicate := userSet[userUUID]; duplicate {
			return false
		}
		userSet[userUUID] = struct{}{}
	}
	for userUUID := range memberSet {
		if _, exists := userSet[userUUID]; !exists {
			return false
		}
	}
	return true
}
