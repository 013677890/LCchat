package service

import (
	"context"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/apps/auth/internal/repository"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/model"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newInternalAuthServiceForTest(t *testing.T) (*internalAuthServiceImpl, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserAccount{}))
	repo := repository.NewAuthRepository(db, nil)
	return &internalAuthServiceImpl{authRepo: repo}, db
}

func TestInternalAuthServiceUpdateLoginDisplayMissingAccountIsIdempotent(t *testing.T) {
	svc, _ := newInternalAuthServiceForTest(t)

	resp, err := svc.UpdateLoginDisplay(context.Background(), &authpb.UpdateLoginDisplayRequest{
		UserUuid: "missing",
		Nickname: "new-nick",
		Avatar:   "new-avatar",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestInternalAuthServiceUpdateLoginDisplayUpdatesExistingAccount(t *testing.T) {
	svc, db := newInternalAuthServiceForTest(t)
	ctx := context.Background()
	now := time.Now()
	require.NoError(t, db.Create(&model.UserAccount{
		UserUuid:      "u1",
		Email:         "u1@test.com",
		PasswordHash:  "hash",
		Status:        0,
		IsAdmin:       0,
		LoginNickname: "old-nick",
		LoginAvatar:   "old-avatar",
		CreatedAt:     now,
		UpdatedAt:     now,
	}).Error)

	resp, err := svc.UpdateLoginDisplay(ctx, &authpb.UpdateLoginDisplayRequest{
		UserUuid: "u1",
		Nickname: "new-nick",
		Avatar:   "new-avatar",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)

	var account model.UserAccount
	require.NoError(t, db.Where("user_uuid = ?", "u1").First(&account).Error)
	require.Equal(t, "new-nick", account.LoginNickname)
	require.Equal(t, "new-avatar", account.LoginAvatar)
}
