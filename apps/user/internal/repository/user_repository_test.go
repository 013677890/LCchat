package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/model"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newUserRepositoryForTest(t *testing.T) *userRepositoryImpl {
	t.Helper()
	dsn := fmt.Sprintf("file:user_repository_%d?mode=memory&cache=shared&_busy_timeout=5000", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserProfile{}))

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	return &userRepositoryImpl{db: db}
}

func seedUserProfile(t *testing.T, repo *userRepositoryImpl, userUUID, nickname string) {
	t.Helper()
	now := time.Now()
	require.NoError(t, repo.db.Create(&model.UserProfile{
		UserUuid:  userUUID,
		Nickname:  nickname,
		Gender:    3,
		CreatedAt: now,
		UpdatedAt: now,
	}).Error)
}

func TestUserRepositoryProfileReadFallsBackToDBWhenRedisMissing(t *testing.T) {
	repo := newUserRepositoryForTest(t)
	ctx := context.Background()
	seedUserProfile(t, repo, "u1", "alice")

	profile, err := repo.GetByUUID(ctx, "u1")
	require.NoError(t, err)
	require.NotNil(t, profile)
	require.Equal(t, "alice", profile.Nickname)

	missing, err := repo.GetByUUID(ctx, "missing")
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestUserRepositoryBatchReadFallsBackToDBWhenRedisMissing(t *testing.T) {
	repo := newUserRepositoryForTest(t)
	ctx := context.Background()
	seedUserProfile(t, repo, "u1", "alice")
	seedUserProfile(t, repo, "u2", "bob")

	profiles, err := repo.BatchGetByUUIDs(ctx, []string{"u1", "missing", "u2"})
	require.NoError(t, err)
	require.Len(t, profiles, 2)
	require.Equal(t, "u1", profiles[0].UserUuid)
	require.Equal(t, "u2", profiles[1].UserUuid)
}

func TestUserRepositoryCreateProfileIsIdempotent(t *testing.T) {
	repo := newUserRepositoryForTest(t)
	ctx := context.Background()

	first, err := repo.CreateProfile(ctx, "user-idem-1", "alice", "avatar-a")
	require.NoError(t, err)
	require.NotNil(t, first)

	second, err := repo.CreateProfile(ctx, "user-idem-1", "bob", "avatar-b")
	require.NoError(t, err)
	require.NotNil(t, second)
	require.Equal(t, first.Id, second.Id)
	require.Equal(t, "alice", second.Nickname)
	require.Equal(t, "avatar-a", second.Avatar)

	var count int64
	require.NoError(t, repo.db.Model(&model.UserProfile{}).
		Where("user_uuid = ?", "user-idem-1").
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestUserRepositoryCreateProfileConcurrentDuplicateSucceeds(t *testing.T) {
	repo := newUserRepositoryForTest(t)
	ctx := context.Background()
	const workers = 8

	start := make(chan struct{})
	errs := make(chan error, workers)
	profiles := make(chan *model.UserProfile, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			profile, err := repo.CreateProfile(ctx, "user-concurrent-1", fmt.Sprintf("name-%d", index), fmt.Sprintf("avatar-%d", index))
			errs <- err
			if err == nil {
				profiles <- profile
			}
		}(i)
	}

	close(start)
	wg.Wait()
	close(errs)
	close(profiles)

	for err := range errs {
		require.NoError(t, err)
	}
	for profile := range profiles {
		require.NotNil(t, profile)
		require.Equal(t, "user-concurrent-1", profile.UserUuid)
	}

	var count int64
	require.NoError(t, repo.db.Model(&model.UserProfile{}).
		Where("user_uuid = ?", "user-concurrent-1").
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestUserRepositoryQRCodeRequiresRedis(t *testing.T) {
	repo := newUserRepositoryForTest(t)
	ctx := context.Background()

	require.ErrorIs(t, repo.SaveQRCode(ctx, "u1", "token1"), ErrRedis)

	_, err := repo.GetUUIDByQRCodeToken(ctx, "token1")
	require.True(t, errors.Is(err, ErrRedis))

	_, _, err = repo.GetQRCodeTokenByUserUUID(ctx, "u1")
	require.True(t, errors.Is(err, ErrRedis))
}
