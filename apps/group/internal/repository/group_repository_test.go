package repository

import (
	"testing"
	"time"

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
		{name: "群主可移除管理员", operatorRole: memberRoleOwner, targetRole: memberRoleAdmin, want: true},
		{name: "群主可移除普通成员", operatorRole: memberRoleOwner, targetRole: memberRoleMember, want: true},
		{name: "群主不能移除群主", operatorRole: memberRoleOwner, targetRole: memberRoleOwner, want: false},
		{name: "管理员可移除普通成员", operatorRole: memberRoleAdmin, targetRole: memberRoleMember, want: true},
		{name: "管理员不能移除管理员", operatorRole: memberRoleAdmin, targetRole: memberRoleAdmin, want: false},
		{name: "管理员不能移除群主", operatorRole: memberRoleAdmin, targetRole: memberRoleOwner, want: false},
		{name: "普通成员不能移除普通成员", operatorRole: memberRoleMember, targetRole: memberRoleMember, want: false},
		{name: "未知角色无权限", operatorRole: -1, targetRole: memberRoleMember, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, canRemoveGroupMember(tc.operatorRole, tc.targetRole))
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
		{name: "正常成员", member: &model.GroupMember{Status: memberStatusNormal}, want: true},
		{name: "已退出", member: &model.GroupMember{Status: memberStatusQuit}, want: false},
		{name: "已踢出", member: &model.GroupMember{Status: memberStatusKicked}, want: false},
		{name: "软删成员", member: &model.GroupMember{Status: memberStatusNormal, DeletedAt: deletedAt(now)}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isActiveGroupMember(tc.member))
		})
	}
}

func deletedAt(t time.Time) gorm.DeletedAt {
	return gorm.DeletedAt{Time: t, Valid: true}
}
