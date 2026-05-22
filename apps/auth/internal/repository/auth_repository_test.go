package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/outbox"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newAuthRepositoryForTest(t *testing.T) *authRepositoryImpl {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserAccount{}, &outbox.Event{}))
	return &authRepositoryImpl{db: db}
}

func seedAuthAccount(t *testing.T, repo *authRepositoryImpl, userUUID string) {
	t.Helper()
	now := time.Now()
	require.NoError(t, repo.db.Create(&model.UserAccount{
		UserUuid:      userUUID,
		Email:         userUUID + "@test.com",
		PasswordHash:  "old-hash",
		Status:        0,
		IsAdmin:       0,
		LoginNickname: "old-nick",
		LoginAvatar:   "old-avatar",
		CreatedAt:     now,
		UpdatedAt:     now,
	}).Error)
}

func TestAuthRepositoryWriteReturnsNotFoundOnZeroRows(t *testing.T) {
	repo := newAuthRepositoryForTest(t)
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{name: "update_password", call: func() error {
			return repo.UpdatePassword(ctx, "missing", "new-hash")
		}},
		{name: "update_email", call: func() error {
			return repo.UpdateEmail(ctx, "missing", "new@test.com")
		}},
		{name: "update_login_display", call: func() error {
			return repo.UpdateLoginDisplay(ctx, "missing", "nick", "avatar")
		}},
		{name: "delete", call: func() error {
			return repo.Delete(ctx, "missing")
		}},
		{name: "delete_with_outbox", call: func() error {
			return repo.DeleteWithOutboxEvent(ctx, "missing", "account_deleted", `{"event_id":"e1"}`)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			require.ErrorIs(t, err, ErrRecordNotFound)
		})
	}
}

func TestAuthRepositoryDeleteWithOutboxDoesNotEmitEventWhenAccountMissing(t *testing.T) {
	repo := newAuthRepositoryForTest(t)
	ctx := context.Background()

	err := repo.DeleteWithOutboxEvent(ctx, "missing", "account_deleted", `{"event_id":"e1"}`)
	require.ErrorIs(t, err, ErrRecordNotFound)

	var count int64
	require.NoError(t, repo.db.Model(&outbox.Event{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestAuthRepositoryWritesExistingAccount(t *testing.T) {
	repo := newAuthRepositoryForTest(t)
	ctx := context.Background()
	seedAuthAccount(t, repo, "u1")

	require.NoError(t, repo.UpdatePassword(ctx, "u1", "new-hash"))
	require.NoError(t, repo.UpdateEmail(ctx, "u1", "new@test.com"))
	require.NoError(t, repo.UpdateLoginDisplay(ctx, "u1", "new-nick", "new-avatar"))
	require.NoError(t, repo.DeleteWithOutboxEvent(ctx, "u1", "account_deleted", `{"event_id":"e1"}`))

	_, err := repo.GetByUserUUID(ctx, "u1")
	require.True(t, errors.Is(err, ErrRecordNotFound))

	var count int64
	require.NoError(t, repo.db.Model(&outbox.Event{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}
