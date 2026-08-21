package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnsureActiveMemberRole(t *testing.T) {
	// 使用内存数据库覆盖权限查询的真实 ORM 条件，避免只测试角色比较而遗漏状态或软删除规则。
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.GroupMember{}))

	now := time.Now()
	members := []*model.GroupMember{
		{GroupUuid: "group-1", UserUuid: "owner-1", Role: repository.MemberRoleOwner, Status: repository.MemberStatusNormal},
		{GroupUuid: "group-1", UserUuid: "admin-1", Role: repository.MemberRoleAdmin, Status: repository.MemberStatusNormal},
		{GroupUuid: "group-1", UserUuid: "member-1", Role: repository.MemberRoleMember, Status: repository.MemberStatusNormal},
		{GroupUuid: "group-1", UserUuid: "quit-admin", Role: repository.MemberRoleAdmin, Status: repository.MemberStatusQuit},
		{GroupUuid: "group-1", UserUuid: "deleted-admin", Role: repository.MemberRoleAdmin, Status: repository.MemberStatusNormal, DeletedAt: deletedAt(now)},
	}
	for _, member := range members {
		require.NoError(t, db.Unscoped().Create(member).Error)
	}

	mysqlStore := New(db, nil)
	tests := []struct {
		name     string
		userUUID string
		minRole  int8
		wantRole int8
		wantErr  error
	}{
		{name: "群主满足管理员权限", userUUID: "owner-1", minRole: repository.MemberRoleAdmin, wantRole: repository.MemberRoleOwner},
		{name: "管理员满足管理员权限", userUUID: "admin-1", minRole: repository.MemberRoleAdmin, wantRole: repository.MemberRoleAdmin},
		{name: "普通成员权限不足", userUUID: "member-1", minRole: repository.MemberRoleAdmin, wantErr: repository.ErrNoPermission},
		{name: "已退出管理员不再有效", userUUID: "quit-admin", minRole: repository.MemberRoleAdmin, wantErr: repository.ErrNoPermission},
		{name: "软删除管理员不再有效", userUUID: "deleted-admin", minRole: repository.MemberRoleAdmin, wantErr: repository.ErrNoPermission},
		{name: "非成员无权限", userUUID: "missing-user", minRole: repository.MemberRoleAdmin, wantErr: repository.ErrNoPermission},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			member, err := mysqlStore.EnsureActiveMemberRole(context.Background(), "group-1", tt.userUUID, tt.minRole)
			if tt.wantErr != nil {
				assert.True(t, errors.Is(err, tt.wantErr))
				assert.Nil(t, member)
				return
			}
			require.NoError(t, err)
			if assert.NotNil(t, member) {
				assert.Equal(t, tt.wantRole, member.Role)
			}
		})
	}
}
