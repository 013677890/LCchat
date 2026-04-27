package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/apps/user/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var userBlacklistLoggerOnce sync.Once

func initUserBlacklistTestLogger() {
	userBlacklistLoggerOnce.Do(func() {
		logger.ReplaceGlobal(zap.NewNop())
	})
}

type fakeBlacklistRepository struct {
	addBlacklistFn         func(ctx context.Context, userUUID, targetUUID string) error
	removeBlacklistFn      func(ctx context.Context, userUUID, targetUUID string) error
	getBlacklistListFn     func(ctx context.Context, userUUID string, page, pageSize int) ([]*model.UserRelation, int64, error)
	isBlockedFn            func(ctx context.Context, userUUID, targetUUID string) (bool, error)
	getBlacklistRelationFn func(ctx context.Context, userUUID, targetUUID string) (*model.UserRelation, error)
}

func (f *fakeBlacklistRepository) AddBlacklist(ctx context.Context, userUUID, targetUUID string) error {
	if f.addBlacklistFn == nil {
		return nil
	}
	return f.addBlacklistFn(ctx, userUUID, targetUUID)
}

func (f *fakeBlacklistRepository) RemoveBlacklist(ctx context.Context, userUUID, targetUUID string) error {
	if f.removeBlacklistFn == nil {
		return nil
	}
	return f.removeBlacklistFn(ctx, userUUID, targetUUID)
}

func (f *fakeBlacklistRepository) GetBlacklistList(ctx context.Context, userUUID string, page, pageSize int) ([]*model.UserRelation, int64, error) {
	if f.getBlacklistListFn == nil {
		return nil, 0, nil
	}
	return f.getBlacklistListFn(ctx, userUUID, page, pageSize)
}

func (f *fakeBlacklistRepository) IsBlocked(ctx context.Context, userUUID, targetUUID string) (bool, error) {
	if f.isBlockedFn == nil {
		return false, nil
	}
	return f.isBlockedFn(ctx, userUUID, targetUUID)
}

func (f *fakeBlacklistRepository) GetBlacklistRelation(ctx context.Context, userUUID, targetUUID string) (*model.UserRelation, error) {
	if f.getBlacklistRelationFn == nil {
		return nil, nil
	}
	return f.getBlacklistRelationFn(ctx, userUUID, targetUUID)
}

func withUserUUID(userUUID string) context.Context {
	return context.WithValue(context.Background(), "user_uuid", userUUID)
}

func requireStatusBizCode(t *testing.T, err error, wantGRPCCode codes.Code, wantBizCode int) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, wantBizCode, apperr.Code(err))

	stErr := apperr.ToStatus(err)
	converted := apperr.FromStatus(stErr)
	require.Equal(t, wantBizCode, apperr.Code(converted))
	st, ok := status.FromError(stErr)
	require.True(t, ok)
	require.Equal(t, wantGRPCCode, st.Code())
}

func TestUserBlacklistServiceAddBlacklist(t *testing.T) {
	initUserBlacklistTestLogger()

	repoErr := errors.New("repo failed")

	tests := []struct {
		name               string
		ctx                context.Context
		req                *pb.AddBlacklistRequest
		isBlockedResult    bool
		isBlockedErr       error
		addErr             error
		wantErr            bool
		wantGRPCCode       codes.Code
		wantBizCode        int
		wantIsBlockedCalls int
		wantAddCalls       int
	}{
		{
			name:         "missing_user_uuid_in_context",
			ctx:          context.Background(),
			req:          &pb.AddBlacklistRequest{TargetUuid: "u2"},
			wantErr:      true,
			wantGRPCCode: codes.Unauthenticated,
			wantBizCode:  consts.CodeUnauthorized,
		},
		{
			name:         "invalid_nil_request",
			ctx:          withUserUUID("u1"),
			req:          nil,
			wantErr:      true,
			wantGRPCCode: codes.InvalidArgument,
			wantBizCode:  consts.CodeParamError,
		},
		{
			name:         "invalid_empty_target_uuid",
			ctx:          withUserUUID("u1"),
			req:          &pb.AddBlacklistRequest{TargetUuid: ""},
			wantErr:      true,
			wantGRPCCode: codes.InvalidArgument,
			wantBizCode:  consts.CodeParamError,
		},
		{
			name:         "cannot_blacklist_self",
			ctx:          withUserUUID("u1"),
			req:          &pb.AddBlacklistRequest{TargetUuid: "u1"},
			wantErr:      true,
			wantGRPCCode: codes.InvalidArgument,
			wantBizCode:  consts.CodeCannotBlacklistSelf,
		},
		{
			name:               "already_in_blacklist",
			ctx:                withUserUUID("u1"),
			req:                &pb.AddBlacklistRequest{TargetUuid: "u2"},
			isBlockedResult:    true,
			wantErr:            true,
			wantGRPCCode:       codes.AlreadyExists,
			wantBizCode:        consts.CodeAlreadyInBlacklist,
			wantIsBlockedCalls: 1,
		},
		{
			name:               "repo_isblocked_error",
			ctx:                withUserUUID("u1"),
			req:                &pb.AddBlacklistRequest{TargetUuid: "u2"},
			isBlockedErr:       repoErr,
			wantErr:            true,
			wantGRPCCode:       codes.Internal,
			wantBizCode:        consts.CodeInternalError,
			wantIsBlockedCalls: 1,
		},
		{
			name:               "repo_add_error",
			ctx:                withUserUUID("u1"),
			req:                &pb.AddBlacklistRequest{TargetUuid: "u2"},
			addErr:             repoErr,
			wantErr:            true,
			wantGRPCCode:       codes.Internal,
			wantBizCode:        consts.CodeInternalError,
			wantIsBlockedCalls: 1,
			wantAddCalls:       1,
		},
		{
			name:               "success",
			ctx:                withUserUUID("u1"),
			req:                &pb.AddBlacklistRequest{TargetUuid: "u2"},
			wantErr:            false,
			wantIsBlockedCalls: 1,
			wantAddCalls:       1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var isBlockedCalls int
			var addCalls int

			repo := &fakeBlacklistRepository{
				isBlockedFn: func(_ context.Context, userUUID, targetUUID string) (bool, error) {
					isBlockedCalls++
					assert.Equal(t, "u1", userUUID)
					assert.Equal(t, "u2", targetUUID)
					return tt.isBlockedResult, tt.isBlockedErr
				},
				addBlacklistFn: func(_ context.Context, userUUID, targetUUID string) error {
					addCalls++
					assert.Equal(t, "u1", userUUID)
					assert.Equal(t, "u2", targetUUID)
					return tt.addErr
				},
			}

			svc := NewBlacklistService(repo)
			err := svc.AddBlacklist(tt.ctx, tt.req)

			if tt.wantErr {
				requireStatusBizCode(t, err, tt.wantGRPCCode, tt.wantBizCode)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantIsBlockedCalls, isBlockedCalls)
			assert.Equal(t, tt.wantAddCalls, addCalls)
		})
	}
}

func TestUserBlacklistServiceRemoveBlacklist(t *testing.T) {
	initUserBlacklistTestLogger()

	repoErr := errors.New("repo failed")

	tests := []struct {
		name               string
		ctx                context.Context
		req                *pb.RemoveBlacklistRequest
		isBlockedResult    bool
		isBlockedErr       error
		removeErr          error
		wantErr            bool
		wantGRPCCode       codes.Code
		wantBizCode        int
		wantIsBlockedCalls int
		wantRemoveCalls    int
	}{
		{
			name:         "missing_user_uuid_in_context",
			ctx:          context.Background(),
			req:          &pb.RemoveBlacklistRequest{UserUuid: "u2"},
			wantErr:      true,
			wantGRPCCode: codes.Unauthenticated,
			wantBizCode:  consts.CodeUnauthorized,
		},
		{
			name:         "invalid_nil_request",
			ctx:          withUserUUID("u1"),
			req:          nil,
			wantErr:      true,
			wantGRPCCode: codes.InvalidArgument,
			wantBizCode:  consts.CodeParamError,
		},
		{
			name:         "invalid_empty_user_uuid",
			ctx:          withUserUUID("u1"),
			req:          &pb.RemoveBlacklistRequest{UserUuid: ""},
			wantErr:      true,
			wantGRPCCode: codes.InvalidArgument,
			wantBizCode:  consts.CodeParamError,
		},
		{
			name:               "not_in_blacklist",
			ctx:                withUserUUID("u1"),
			req:                &pb.RemoveBlacklistRequest{UserUuid: "u2"},
			isBlockedResult:    false,
			wantErr:            true,
			wantGRPCCode:       codes.NotFound,
			wantBizCode:        consts.CodeNotInBlacklist,
			wantIsBlockedCalls: 1,
		},
		{
			name:               "repo_isblocked_error",
			ctx:                withUserUUID("u1"),
			req:                &pb.RemoveBlacklistRequest{UserUuid: "u2"},
			isBlockedErr:       repoErr,
			wantErr:            true,
			wantGRPCCode:       codes.Internal,
			wantBizCode:        consts.CodeInternalError,
			wantIsBlockedCalls: 1,
		},
		{
			name:               "repo_remove_record_not_found",
			ctx:                withUserUUID("u1"),
			req:                &pb.RemoveBlacklistRequest{UserUuid: "u2"},
			isBlockedResult:    true,
			removeErr:          repository.ErrRecordNotFound,
			wantErr:            true,
			wantGRPCCode:       codes.NotFound,
			wantBizCode:        consts.CodeNotInBlacklist,
			wantIsBlockedCalls: 1,
			wantRemoveCalls:    1,
		},
		{
			name:               "repo_remove_error",
			ctx:                withUserUUID("u1"),
			req:                &pb.RemoveBlacklistRequest{UserUuid: "u2"},
			isBlockedResult:    true,
			removeErr:          repoErr,
			wantErr:            true,
			wantGRPCCode:       codes.Internal,
			wantBizCode:        consts.CodeInternalError,
			wantIsBlockedCalls: 1,
			wantRemoveCalls:    1,
		},
		{
			name:               "success",
			ctx:                withUserUUID("u1"),
			req:                &pb.RemoveBlacklistRequest{UserUuid: "u2"},
			isBlockedResult:    true,
			wantErr:            false,
			wantIsBlockedCalls: 1,
			wantRemoveCalls:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var isBlockedCalls int
			var removeCalls int

			repo := &fakeBlacklistRepository{
				isBlockedFn: func(_ context.Context, userUUID, targetUUID string) (bool, error) {
					isBlockedCalls++
					assert.Equal(t, "u1", userUUID)
					assert.Equal(t, "u2", targetUUID)
					return tt.isBlockedResult, tt.isBlockedErr
				},
				removeBlacklistFn: func(_ context.Context, userUUID, targetUUID string) error {
					removeCalls++
					assert.Equal(t, "u1", userUUID)
					assert.Equal(t, "u2", targetUUID)
					return tt.removeErr
				},
			}

			svc := NewBlacklistService(repo)
			err := svc.RemoveBlacklist(tt.ctx, tt.req)

			if tt.wantErr {
				requireStatusBizCode(t, err, tt.wantGRPCCode, tt.wantBizCode)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantIsBlockedCalls, isBlockedCalls)
			assert.Equal(t, tt.wantRemoveCalls, removeCalls)
		})
	}
}

func TestUserBlacklistServiceGetBlacklistList(t *testing.T) {
	initUserBlacklistTestLogger()

	t.Run("unauthenticated", func(t *testing.T) {
		svc := NewBlacklistService(&fakeBlacklistRepository{})
		resp, err := svc.GetBlacklistList(context.Background(), &pb.GetBlacklistListRequest{Page: 1, PageSize: 20})
		require.Nil(t, resp)
		requireStatusBizCode(t, err, codes.Unauthenticated, consts.CodeUnauthorized)
	})

	t.Run("repo_error", func(t *testing.T) {
		repo := &fakeBlacklistRepository{
			getBlacklistListFn: func(_ context.Context, _ string, _, _ int) ([]*model.UserRelation, int64, error) {
				return nil, 0, errors.New("db error")
			},
		}
		svc := NewBlacklistService(repo)

		resp, err := svc.GetBlacklistList(withUserUUID("u1"), &pb.GetBlacklistListRequest{Page: 1, PageSize: 20})
		require.Nil(t, resp)
		requireStatusBizCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("default_pagination_and_time_mapping", func(t *testing.T) {
		updatedAt := time.Unix(1700000000, 0)
		blacklistedAt := time.Unix(1700001111, 0)
		repo := &fakeBlacklistRepository{
			getBlacklistListFn: func(_ context.Context, userUUID string, page, pageSize int) ([]*model.UserRelation, int64, error) {
				assert.Equal(t, "u1", userUUID)
				assert.Equal(t, 1, page)
				assert.Equal(t, 20, pageSize)
				return []*model.UserRelation{
					nil,
					{PeerUuid: "u2", UpdatedAt: updatedAt},
					{PeerUuid: "u3", UpdatedAt: updatedAt, BlacklistedAt: &blacklistedAt},
				}, 21, nil
			},
		}
		svc := NewBlacklistService(repo)

		resp, err := svc.GetBlacklistList(withUserUUID("u1"), &pb.GetBlacklistListRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.Pagination)
		assert.Equal(t, int32(1), resp.Pagination.Page)
		assert.Equal(t, int32(20), resp.Pagination.PageSize)
		assert.Equal(t, int64(21), resp.Pagination.Total)
		assert.Equal(t, int32(2), resp.Pagination.TotalPages)

		require.Len(t, resp.Items, 2)
		assert.Equal(t, "u2", resp.Items[0].Uuid)
		assert.Equal(t, updatedAt.UnixMilli(), resp.Items[0].BlacklistedAt)
		assert.Equal(t, "u3", resp.Items[1].Uuid)
		assert.Equal(t, blacklistedAt.UnixMilli(), resp.Items[1].BlacklistedAt)
	})

	t.Run("empty_result_with_pagination", func(t *testing.T) {
		repo := &fakeBlacklistRepository{
			getBlacklistListFn: func(_ context.Context, userUUID string, page, pageSize int) ([]*model.UserRelation, int64, error) {
				assert.Equal(t, "u1", userUUID)
				assert.Equal(t, 2, page)
				assert.Equal(t, 5, pageSize)
				return []*model.UserRelation{}, 0, nil
			},
		}
		svc := NewBlacklistService(repo)

		resp, err := svc.GetBlacklistList(withUserUUID("u1"), &pb.GetBlacklistListRequest{Page: 2, PageSize: 5})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.Pagination)
		assert.Empty(t, resp.Items)
		assert.Equal(t, int32(2), resp.Pagination.Page)
		assert.Equal(t, int32(5), resp.Pagination.PageSize)
		assert.Equal(t, int64(0), resp.Pagination.Total)
		assert.Equal(t, int32(0), resp.Pagination.TotalPages)
	})
}

func TestUserBlacklistServiceCheckIsBlacklist(t *testing.T) {
	initUserBlacklistTestLogger()

	t.Run("invalid_nil_request", func(t *testing.T) {
		svc := NewBlacklistService(&fakeBlacklistRepository{})
		resp, err := svc.CheckIsBlacklist(context.Background(), nil)
		require.Nil(t, resp)
		requireStatusBizCode(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("invalid_missing_user_uuid", func(t *testing.T) {
		svc := NewBlacklistService(&fakeBlacklistRepository{})
		resp, err := svc.CheckIsBlacklist(context.Background(), &pb.CheckIsBlacklistRequest{UserUuid: "", TargetUuid: "u2"})
		require.Nil(t, resp)
		requireStatusBizCode(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("invalid_missing_target_uuid", func(t *testing.T) {
		svc := NewBlacklistService(&fakeBlacklistRepository{})
		resp, err := svc.CheckIsBlacklist(context.Background(), &pb.CheckIsBlacklistRequest{UserUuid: "u1", TargetUuid: ""})
		require.Nil(t, resp)
		requireStatusBizCode(t, err, codes.InvalidArgument, consts.CodeParamError)
	})

	t.Run("repo_error", func(t *testing.T) {
		repo := &fakeBlacklistRepository{
			isBlockedFn: func(_ context.Context, _, _ string) (bool, error) {
				return false, errors.New("repo error")
			},
		}
		svc := NewBlacklistService(repo)

		resp, err := svc.CheckIsBlacklist(context.Background(), &pb.CheckIsBlacklistRequest{UserUuid: "u1", TargetUuid: "u2"})
		require.Nil(t, resp)
		requireStatusBizCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("success", func(t *testing.T) {
		repo := &fakeBlacklistRepository{
			isBlockedFn: func(_ context.Context, userUUID, targetUUID string) (bool, error) {
				assert.Equal(t, "u1", userUUID)
				assert.Equal(t, "u2", targetUUID)
				return true, nil
			},
		}
		svc := NewBlacklistService(repo)

		resp, err := svc.CheckIsBlacklist(context.Background(), &pb.CheckIsBlacklistRequest{UserUuid: "u1", TargetUuid: "u2"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.IsBlacklist)
	})

	t.Run("success_false", func(t *testing.T) {
		repo := &fakeBlacklistRepository{
			isBlockedFn: func(_ context.Context, _, _ string) (bool, error) {
				return false, nil
			},
		}
		svc := NewBlacklistService(repo)

		resp, err := svc.CheckIsBlacklist(context.Background(), &pb.CheckIsBlacklistRequest{UserUuid: "u1", TargetUuid: "u2"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.IsBlacklist)
	})
}
