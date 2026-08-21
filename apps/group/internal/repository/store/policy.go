package store

import (
	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/model"
)

// canRemoveGroupMember 判断当前角色矩阵下是否允许移除目标成员。
//
// 规则保持最小闭环：
//  1. 群主可移除管理员和普通成员，不能移除自己；
//  2. 管理员只能移除普通成员；
//  3. 普通成员不能移除任何人。
func canRemoveGroupMember(operatorRole, targetRole int8) bool {
	switch operatorRole {
	case repository.MemberRoleOwner:
		return targetRole != repository.MemberRoleOwner
	case repository.MemberRoleAdmin:
		return targetRole == repository.MemberRoleMember
	default:
		return false
	}
}

// canUpdateGroupMemberRole 判断是否允许把目标成员调整为指定角色。
//
// 当前规则：
//  1. 只有群主可以设置管理员；
//  2. 目标必须是当前有效的普通成员或管理员；
//  3. 不能通过该接口直接产生第二个群主。
func canUpdateGroupMemberRole(operatorRole, currentTargetRole, nextTargetRole int8) bool {
	if operatorRole != repository.MemberRoleOwner {
		return false
	}
	if currentTargetRole != repository.MemberRoleMember && currentTargetRole != repository.MemberRoleAdmin {
		return false
	}
	return nextTargetRole == repository.MemberRoleMember || nextTargetRole == repository.MemberRoleAdmin
}

// canMuteGroupMember 判断操作者是否可以调整目标成员禁言状态。
func canMuteGroupMember(operatorRole, targetRole int8) bool {
	switch operatorRole {
	case repository.MemberRoleOwner:
		return targetRole == repository.MemberRoleAdmin || targetRole == repository.MemberRoleMember
	case repository.MemberRoleAdmin:
		return targetRole == repository.MemberRoleMember
	default:
		return false
	}
}

// canUpdateGroupMemberNickname 判断操作者是否可以代改目标成员群名片。
func canUpdateGroupMemberNickname(operatorRole, targetRole int8) bool {
	switch operatorRole {
	case repository.MemberRoleOwner:
		return targetRole == repository.MemberRoleAdmin || targetRole == repository.MemberRoleMember
	case repository.MemberRoleAdmin:
		return targetRole == repository.MemberRoleMember
	default:
		return false
	}
}

// actionToJoinRequestStatus 把审批动作值映射成申请状态值。
func actionToJoinRequestStatus(action int8) int8 {
	if action == repository.JoinRequestStatusRejected {
		return repository.JoinRequestStatusRejected
	}
	return repository.JoinRequestStatusApproved
}

// collectWriteMemberUUIDs 提取写事务需要锁定的去重用户集合。
func collectWriteMemberUUIDs(members []*model.GroupMember) []string {
	if len(members) == 0 {
		return []string{}
	}
	userUUIDs := make([]string, 0, len(members))
	seen := make(map[string]struct{}, len(members))
	for _, member := range members {
		if member == nil || member.UserUuid == "" {
			continue
		}
		if _, exists := seen[member.UserUuid]; exists {
			continue
		}
		seen[member.UserUuid] = struct{}{}
		userUUIDs = append(userUUIDs, member.UserUuid)
	}
	return userUUIDs
}
