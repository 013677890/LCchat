package store

import (
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestCanRemoveGroupMemberRoleMatrix(t *testing.T) {
	cases := []struct {
		name         string
		operatorRole int8
		targetRole   int8
		want         bool
	}{
		{name: "群主可移除管理员", operatorRole: repository.MemberRoleOwner, targetRole: repository.MemberRoleAdmin, want: true},
		{name: "群主可移除普通成员", operatorRole: repository.MemberRoleOwner, targetRole: repository.MemberRoleMember, want: true},
		{name: "群主不能移除群主", operatorRole: repository.MemberRoleOwner, targetRole: repository.MemberRoleOwner, want: false},
		{name: "管理员可移除普通成员", operatorRole: repository.MemberRoleAdmin, targetRole: repository.MemberRoleMember, want: true},
		{name: "管理员不能移除管理员", operatorRole: repository.MemberRoleAdmin, targetRole: repository.MemberRoleAdmin, want: false},
		{name: "管理员不能移除群主", operatorRole: repository.MemberRoleAdmin, targetRole: repository.MemberRoleOwner, want: false},
		{name: "普通成员不能移除普通成员", operatorRole: repository.MemberRoleMember, targetRole: repository.MemberRoleMember, want: false},
		{name: "未知角色无权限", operatorRole: -1, targetRole: repository.MemberRoleMember, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, canRemoveGroupMember(tc.operatorRole, tc.targetRole))
		})
	}
}

func TestCanUpdateGroupMemberRoleMatrix(t *testing.T) {
	cases := []struct {
		name              string
		operatorRole      int8
		currentTargetRole int8
		nextTargetRole    int8
		want              bool
	}{
		{name: "群主可设置管理员", operatorRole: repository.MemberRoleOwner, currentTargetRole: repository.MemberRoleMember, nextTargetRole: repository.MemberRoleAdmin, want: true},
		{name: "群主可取消管理员", operatorRole: repository.MemberRoleOwner, currentTargetRole: repository.MemberRoleAdmin, nextTargetRole: repository.MemberRoleMember, want: true},
		{name: "群主不能设置群主", operatorRole: repository.MemberRoleOwner, currentTargetRole: repository.MemberRoleMember, nextTargetRole: repository.MemberRoleOwner, want: false},
		{name: "管理员不能设置角色", operatorRole: repository.MemberRoleAdmin, currentTargetRole: repository.MemberRoleMember, nextTargetRole: repository.MemberRoleAdmin, want: false},
		{name: "普通成员不能设置角色", operatorRole: repository.MemberRoleMember, currentTargetRole: repository.MemberRoleMember, nextTargetRole: repository.MemberRoleAdmin, want: false},
		{name: "不能更新群主角色", operatorRole: repository.MemberRoleOwner, currentTargetRole: repository.MemberRoleOwner, nextTargetRole: repository.MemberRoleMember, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, canUpdateGroupMemberRole(tc.operatorRole, tc.currentTargetRole, tc.nextTargetRole))
		})
	}
}

func TestIsActiveGroupMember(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name   string
		member *model.GroupMember
		want   bool
	}{
		{name: "空成员", member: nil, want: false},
		{name: "正常成员", member: &model.GroupMember{Status: repository.MemberStatusNormal}, want: true},
		{name: "已退出", member: &model.GroupMember{Status: repository.MemberStatusQuit}, want: false},
		{name: "已踢出", member: &model.GroupMember{Status: repository.MemberStatusKicked}, want: false},
		{name: "软删成员", member: &model.GroupMember{Status: repository.MemberStatusNormal, DeletedAt: deletedAt(now)}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isActiveGroupMember(tc.member))
		})
	}
}

func deletedAt(value time.Time) gorm.DeletedAt {
	return gorm.DeletedAt{Time: value, Valid: true}
}
