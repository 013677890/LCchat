package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/apps/relation/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRelationBlacklistServiceAddBlacklist(t *testing.T) {
	initRelationServiceTestLogger()

	repoErr := errors.New("repo failed")

	tests := []struct {
		name        string
		ctx         context.Context
		req         *pb.AddBlacklistRequest
		isBlocked   bool
		isBlockedErr error
		addErr      error
		wantErr     int
	}{
		{name: "unauthenticated", ctx: context.Background(), req: &pb.AddBlacklistRequest{TargetUuid: "u2"}, wantErr: consts.CodeUnauthorized},
		{name: "param_error", ctx: withRelationUserUUID("u1"), req: nil, wantErr: consts.CodeParamError},
		{name: "cannot_blacklist_self", ctx: withRelationUserUUID("u1"), req: &pb.AddBlacklistRequest{TargetUuid: "u1"}, wantErr: consts.CodeCannotBlacklistSelf},
		{name: "already_in_blacklist", ctx: withRelationUserUUID("u1"), req: &pb.AddBlacklistRequest{TargetUuid: "u2"}, isBlocked: true, wantErr: consts.CodeAlreadyInBlacklist},
		{name: "is_blocked_error", ctx: withRelationUserUUID("u1"), req: &pb.AddBlacklistRequest{TargetUuid: "u2"}, isBlockedErr: repoErr, wantErr: consts.CodeInternalError},
		{name: "add_error", ctx: withRelationUserUUID("u1"), req: &pb.AddBlacklistRequest{TargetUuid: "u2"}, addErr: repoErr, wantErr: consts.CodeInternalError},
		{name: "success", ctx: withRelationUserUUID("u1"), req: &pb.AddBlacklistRequest{TargetUuid: "u2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeBlacklistRepoForService{
				isBlockedFn: func(_ context.Context, userUUID, targetUUID string) (bool, error) {
					assert.Equal(t, "u1", userUUID)
					assert.Equal(t, "u2", targetUUID)
					return tt.isBlocked, tt.isBlockedErr
				},
				addBlacklistFn: func(_ context.Context, userUUID, targetUUID string) error {
					assert.Equal(t, "u1", userUUID)
					assert.Equal(t, "u2", targetUUID)
					return tt.addErr
				},
			}
			svc := NewBlacklistService((*gorm.DB)(nil), nil, repo, &fakeFriendRepoForService{})
			err := svc.AddBlacklist(tt.ctx, tt.req)
			if tt.wantErr != 0 {
				requireRelationBizCode(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestRelationBlacklistServiceRemoveBlacklist(t *testing.T) {
	initRelationServiceTestLogger()

	repoErr := errors.New("repo failed")

	tests := []struct {
		name         string
		ctx          context.Context
		req          *pb.RemoveBlacklistRequest
		isBlocked    bool
		isBlockedErr error
		removeErr    error
		wantErr      int
	}{
		{name: "unauthenticated", ctx: context.Background(), req: &pb.RemoveBlacklistRequest{UserUuid: "u2"}, wantErr: consts.CodeUnauthorized},
		{name: "param_error", ctx: withRelationUserUUID("u1"), req: nil, wantErr: consts.CodeParamError},
		{name: "not_in_blacklist", ctx: withRelationUserUUID("u1"), req: &pb.RemoveBlacklistRequest{UserUuid: "u2"}, wantErr: consts.CodeNotInBlacklist},
		{name: "is_blocked_error", ctx: withRelationUserUUID("u1"), req: &pb.RemoveBlacklistRequest{UserUuid: "u2"}, isBlockedErr: repoErr, wantErr: consts.CodeInternalError},
		{name: "remove_not_found", ctx: withRelationUserUUID("u1"), req: &pb.RemoveBlacklistRequest{UserUuid: "u2"}, isBlocked: true, removeErr: repository.ErrRecordNotFound, wantErr: consts.CodeNotInBlacklist},
		{name: "remove_error", ctx: withRelationUserUUID("u1"), req: &pb.RemoveBlacklistRequest{UserUuid: "u2"}, isBlocked: true, removeErr: repoErr, wantErr: consts.CodeInternalError},
		{name: "success", ctx: withRelationUserUUID("u1"), req: &pb.RemoveBlacklistRequest{UserUuid: "u2"}, isBlocked: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeBlacklistRepoForService{
				isBlockedFn: func(_ context.Context, userUUID, targetUUID string) (bool, error) {
					assert.Equal(t, "u1", userUUID)
					assert.Equal(t, "u2", targetUUID)
					return tt.isBlocked, tt.isBlockedErr
				},
				removeBlacklistFn: func(_ context.Context, userUUID, targetUUID string) error {
					assert.Equal(t, "u1", userUUID)
					assert.Equal(t, "u2", targetUUID)
					return tt.removeErr
				},
			}
			svc := NewBlacklistService((*gorm.DB)(nil), nil, repo, &fakeFriendRepoForService{})
			err := svc.RemoveBlacklist(tt.ctx, tt.req)
			if tt.wantErr != 0 {
				requireRelationBizCode(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestRelationBlacklistServiceGetBlacklistList(t *testing.T) {
	initRelationServiceTestLogger()

	now := int64(1710000000000)
	repo := &fakeBlacklistRepoForService{
		getBlacklistListFn: func(_ context.Context, userUUID string, page, pageSize int) ([]*model.UserRelation, int64, error) {
			assert.Equal(t, "u1", userUUID)
			assert.Equal(t, 1, page)
			assert.Equal(t, 20, pageSize)
			return []*model.UserRelation{{PeerUuid: "u2", UpdatedAt: mustUnixMilli(now)}}, 1, nil
		},
	}
	svc := NewBlacklistService((*gorm.DB)(nil), nil, repo, &fakeFriendRepoForService{})
	resp, err := svc.GetBlacklistList(withRelationUserUUID("u1"), &pb.GetBlacklistListRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "u2", resp.Items[0].Uuid)
	assert.Equal(t, now, resp.Items[0].BlacklistedAt)
}

func TestRelationBlacklistServiceCheckIsBlacklist(t *testing.T) {
	initRelationServiceTestLogger()

	repo := &fakeBlacklistRepoForService{
		isBlockedFn: func(_ context.Context, userUUID, targetUUID string) (bool, error) {
			assert.Equal(t, "u1", userUUID)
			assert.Equal(t, "u2", targetUUID)
			return true, nil
		},
	}
	svc := NewBlacklistService((*gorm.DB)(nil), nil, repo, &fakeFriendRepoForService{})
	resp, err := svc.CheckIsBlacklist(context.Background(), &pb.CheckIsBlacklistRequest{UserUuid: "u1", TargetUuid: "u2"})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.IsBlacklist)
}

func mustUnixMilli(ms int64) time.Time {
	return time.UnixMilli(ms)
}
