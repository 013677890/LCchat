package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/apps/group/internal/repository/projection"
	"github.com/013677890/LCchat-Backend/apps/group/internal/repository/store"
	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newCacheTestReader(t *testing.T) (*Reader, *projection.Repository, *goredis.Client, *gorm.DB) {
	t.Helper()
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	dsn := fmt.Sprintf("file:group_cache_reader_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.GroupInfo{}, &model.GroupMember{}, &model.GroupJoinRequest{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	projector := projection.New(db, client)
	mysqlStore := store.New(db, client)
	return New(client, mysqlStore, projector), projector, client, db
}

func TestGetGroupInfoAndMembersRejectDismissedProjection(t *testing.T) {
	reader, projector, _, db := newCacheTestReader(t)
	ctx := context.Background()
	now := time.UnixMilli(1710006250123)
	group := &model.GroupInfo{
		Uuid:         "deleted-group",
		Name:         "must-not-resurrect",
		OwnerUuid:    "owner-1",
		MemberCnt:    1,
		Status:       repository.GroupStatusNormal,
		CacheVersion: 6,
		UpdatedAt:    now,
		DeletedAt:    gorm.DeletedAt{Time: now, Valid: true},
	}
	require.NoError(t, db.Unscoped().Create(group).Error)
	require.NoError(t, projector.ReconcileGroupCache(ctx, group.Uuid))

	_, err := reader.GetGroupInfo(ctx, group.Uuid)
	assert.ErrorIs(t, err, repoerr.ErrRecordNotFound)
	_, err = reader.GetGroupMembers(ctx, group.Uuid)
	assert.ErrorIs(t, err, repository.ErrGroupDismissed,
		"有效空成员 tombstone 不能把不可用群降级成成功空列表")
}

func TestListUserGroupsReadyHitTakesReconcileLease(t *testing.T) {
	reader, projector, client, db := newCacheTestReader(t)
	ctx := context.Background()
	now := time.UnixMilli(1710006820123)
	group := &model.GroupInfo{
		Uuid:         "stray-on-hit",
		Name:         "group",
		OwnerUuid:    "user-1",
		Status:       repository.GroupStatusNormal,
		CacheVersion: 5,
		UpdatedAt:    now,
	}
	require.NoError(t, db.Create(group).Error)
	require.NoError(t, db.Create(&model.GroupMember{
		GroupUuid: group.Uuid,
		UserUuid:  "user-1",
		Role:      repository.MemberRoleOwner,
		Status:    repository.MemberStatusNormal,
		JoinedAt:  now,
	}).Error)
	require.NoError(t, projector.ReconcileGroupCache(ctx, group.Uuid))
	require.NoError(t, projector.ReconcileUserGroupsCache(ctx, "user-1"))
	require.NoError(t, db.Where("group_uuid = ? AND user_uuid = ?", group.Uuid, "user-1").Delete(&model.GroupMember{}).Error)

	groups, err := reader.ListUserGroups(ctx, "user-1")
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, group.Uuid, groups[0].Uuid)
	assert.Equal(t, "1", client.Get(ctx, rediskey.UserGroupReconcileLeaseKey("user-1")).Val(),
		"结构合法的 READY 命中必须取得租约，让 DB 已不存在的成员关系仍能被权威对账修复")
}
