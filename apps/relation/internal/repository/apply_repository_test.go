package repository

import (
	"context"
	"testing"

	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/013677890/LCchat-Backend/model"
	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newApplyRepositoryTestRepo(t *testing.T) (*applyRepositoryImpl, *goredis.Client) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ApplyRequest{}, &model.UserRelation{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})

	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	return &applyRepositoryImpl{db: db, redisClient: client}, client
}

func seedPendingApplyCache(t *testing.T, client *goredis.Client, targetUUID, applicantUUID string) {
	t.Helper()
	require.NoError(t, client.ZAdd(context.Background(), rediskey.ApplyPendingKey(targetUUID), goredis.Z{
		Score:  1,
		Member: applicantUUID,
	}).Err())
}

func requirePendingApplyRemoved(t *testing.T, repo *applyRepositoryImpl, client *goredis.Client, targetUUID, applicantUUID string) {
	t.Helper()

	_, err := client.ZScore(context.Background(), rediskey.ApplyPendingKey(targetUUID), applicantUUID).Result()
	require.ErrorIs(t, err, goredis.Nil)

	exists, err := repo.ExistsPendingRequest(context.Background(), applicantUUID, targetUUID)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestApplyRepositoryUpdateStatusRemovesPendingCache(t *testing.T) {
	repo, client := newApplyRepositoryTestRepo(t)
	ctx := context.Background()

	apply := &model.ApplyRequest{
		ApplyType:     0,
		ApplicantUuid: "user-2",
		TargetUuid:    "user-1",
		Status:        0,
	}
	require.NoError(t, repo.db.WithContext(ctx).Create(apply).Error)
	seedPendingApplyCache(t, client, apply.TargetUuid, apply.ApplicantUuid)

	require.NoError(t, repo.UpdateStatus(ctx, apply.Id, 2, "not now"))

	requirePendingApplyRemoved(t, repo, client, apply.TargetUuid, apply.ApplicantUuid)
}

func TestApplyRepositoryAcceptApplyRemovesPendingCache(t *testing.T) {
	repo, client := newApplyRepositoryTestRepo(t)
	ctx := context.Background()

	apply := &model.ApplyRequest{
		ApplyType:     0,
		ApplicantUuid: "user-2",
		TargetUuid:    "user-1",
		Status:        0,
	}
	require.NoError(t, repo.db.WithContext(ctx).Create(apply).Error)
	seedPendingApplyCache(t, client, apply.TargetUuid, apply.ApplicantUuid)

	alreadyProcessed, err := repo.AcceptApplyAndCreateRelation(ctx, apply.Id, apply.TargetUuid, apply.ApplicantUuid, "buddy")
	require.NoError(t, err)
	require.False(t, alreadyProcessed)

	requirePendingApplyRemoved(t, repo, client, apply.TargetUuid, apply.ApplicantUuid)
}

func TestApplyRepositoryGetUnreadCountFallsBackToDBOnCacheMiss(t *testing.T) {
	repo, client := newApplyRepositoryTestRepo(t)
	ctx := context.Background()
	targetUUID := "user-1"

	require.NoError(t, repo.db.WithContext(ctx).Create([]*model.ApplyRequest{
		{ApplyType: 0, ApplicantUuid: "user-2", TargetUuid: targetUUID, Status: 0, IsRead: false},
		{ApplyType: 0, ApplicantUuid: "user-3", TargetUuid: targetUUID, Status: 1, IsRead: false},
		{ApplyType: 0, ApplicantUuid: "user-4", TargetUuid: targetUUID, Status: 2, IsRead: true},
	}).Error)

	count, err := repo.GetUnreadCount(ctx, targetUUID)

	require.NoError(t, err)
	require.EqualValues(t, 2, count)
	cached, err := client.Get(ctx, rediskey.ApplyUnreadNotifyKey(targetUUID)).Int64()
	require.NoError(t, err)
	require.EqualValues(t, 2, cached)
}

func TestApplyRepositoryGetUnreadCountRepairsMalformedCacheFromDB(t *testing.T) {
	repo, client := newApplyRepositoryTestRepo(t)
	ctx := context.Background()
	targetUUID := "user-1"
	notifyKey := rediskey.ApplyUnreadNotifyKey(targetUUID)

	require.NoError(t, repo.db.WithContext(ctx).Create(&model.ApplyRequest{
		ApplyType:     0,
		ApplicantUuid: "user-2",
		TargetUuid:    targetUUID,
		Status:        0,
		IsRead:        false,
	}).Error)
	require.NoError(t, client.Set(ctx, notifyKey, "invalid", 0).Err())

	count, err := repo.GetUnreadCount(ctx, targetUUID)

	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	cached, err := client.Get(ctx, notifyKey).Int64()
	require.NoError(t, err)
	require.EqualValues(t, 1, cached)
}
