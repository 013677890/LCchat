package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/model"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newUserRepositoryForTest(t *testing.T) *userRepositoryImpl {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserProfile{}))
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

func TestUserRepositoryQRCodeRequiresRedis(t *testing.T) {
	repo := newUserRepositoryForTest(t)
	ctx := context.Background()

	require.ErrorIs(t, repo.SaveQRCode(ctx, "u1", "token1"), ErrRedis)

	_, err := repo.GetUUIDByQRCodeToken(ctx, "token1")
	require.True(t, errors.Is(err, ErrRedis))

	_, _, err = repo.GetQRCodeTokenByUserUUID(ctx, "u1")
	require.True(t, errors.Is(err, ErrRedis))
}
