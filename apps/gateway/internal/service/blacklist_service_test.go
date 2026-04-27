package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	gatewaypb "github.com/013677890/LCchat-Backend/apps/gateway/internal/pb"
	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/pkg/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var gatewayBlacklistLoggerOnce sync.Once

func initGatewayBlacklistTestLogger() {
	gatewayBlacklistLoggerOnce.Do(func() {
		logger.ReplaceGlobal(zap.NewNop())
	})
}

type fakeGatewayUserClient struct {
	gatewaypb.UserServiceClient

	addBlacklistFn     func(context.Context, *userpb.AddBlacklistRequest) (*userpb.AddBlacklistResponse, error)
	removeBlacklistFn  func(context.Context, *userpb.RemoveBlacklistRequest) (*userpb.RemoveBlacklistResponse, error)
	getBlacklistListFn func(context.Context, *userpb.GetBlacklistListRequest) (*userpb.GetBlacklistListResponse, error)
	batchGetProfileFn  func(context.Context, *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error)
	checkIsBlacklistFn func(context.Context, *userpb.CheckIsBlacklistRequest) (*userpb.CheckIsBlacklistResponse, error)
}

func (f *fakeGatewayUserClient) AddBlacklist(ctx context.Context, req *userpb.AddBlacklistRequest) (*userpb.AddBlacklistResponse, error) {
	if f.addBlacklistFn == nil {
		return nil, errors.New("unexpected AddBlacklist call")
	}
	return f.addBlacklistFn(ctx, req)
}

func (f *fakeGatewayUserClient) RemoveBlacklist(ctx context.Context, req *userpb.RemoveBlacklistRequest) (*userpb.RemoveBlacklistResponse, error) {
	if f.removeBlacklistFn == nil {
		return nil, errors.New("unexpected RemoveBlacklist call")
	}
	return f.removeBlacklistFn(ctx, req)
}

func (f *fakeGatewayUserClient) GetBlacklistList(ctx context.Context, req *userpb.GetBlacklistListRequest) (*userpb.GetBlacklistListResponse, error) {
	if f.getBlacklistListFn == nil {
		return nil, errors.New("unexpected GetBlacklistList call")
	}
	return f.getBlacklistListFn(ctx, req)
}

func (f *fakeGatewayUserClient) BatchGetProfile(ctx context.Context, req *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
	if f.batchGetProfileFn == nil {
		return nil, errors.New("unexpected BatchGetProfile call")
	}
	return f.batchGetProfileFn(ctx, req)
}

func (f *fakeGatewayUserClient) CheckIsBlacklist(ctx context.Context, req *userpb.CheckIsBlacklistRequest) (*userpb.CheckIsBlacklistResponse, error) {
	if f.checkIsBlacklistFn == nil {
		return nil, errors.New("unexpected CheckIsBlacklist call")
	}
	return f.checkIsBlacklistFn(ctx, req)
}

func TestGatewayBlacklistServiceAddBlacklist(t *testing.T) {
	initGatewayBlacklistTestLogger()

	t.Run("success", func(t *testing.T) {
		client := &fakeGatewayUserClient{
			addBlacklistFn: func(_ context.Context, req *userpb.AddBlacklistRequest) (*userpb.AddBlacklistResponse, error) {
				require.Equal(t, "target-1", req.TargetUuid)
				return &userpb.AddBlacklistResponse{}, nil
			},
		}
		svc := NewBlacklistService(client)

		resp, err := svc.AddBlacklist(context.Background(), &dto.AddBlacklistRequest{TargetUUID: "target-1"})
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("grpc_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc unavailable")
		client := &fakeGatewayUserClient{
			addBlacklistFn: func(_ context.Context, _ *userpb.AddBlacklistRequest) (*userpb.AddBlacklistResponse, error) {
				return nil, wantErr
			},
		}
		svc := NewBlacklistService(client)

		resp, err := svc.AddBlacklist(context.Background(), &dto.AddBlacklistRequest{TargetUUID: "target-1"})
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestGatewayBlacklistServiceRemoveBlacklist(t *testing.T) {
	initGatewayBlacklistTestLogger()

	t.Run("success", func(t *testing.T) {
		client := &fakeGatewayUserClient{
			removeBlacklistFn: func(_ context.Context, req *userpb.RemoveBlacklistRequest) (*userpb.RemoveBlacklistResponse, error) {
				require.Equal(t, "target-1", req.UserUuid)
				return &userpb.RemoveBlacklistResponse{}, nil
			},
		}
		svc := NewBlacklistService(client)

		resp, err := svc.RemoveBlacklist(context.Background(), &dto.RemoveBlacklistRequest{UserUUID: "target-1"})
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("grpc_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc unavailable")
		client := &fakeGatewayUserClient{
			removeBlacklistFn: func(_ context.Context, _ *userpb.RemoveBlacklistRequest) (*userpb.RemoveBlacklistResponse, error) {
				return nil, wantErr
			},
		}
		svc := NewBlacklistService(client)

		resp, err := svc.RemoveBlacklist(context.Background(), &dto.RemoveBlacklistRequest{UserUUID: "target-1"})
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestGatewayBlacklistServiceGetBlacklistList(t *testing.T) {
	initGatewayBlacklistTestLogger()

	t.Run("empty_list_skips_profile_enrichment", func(t *testing.T) {
		batchCalls := 0
		client := &fakeGatewayUserClient{
			getBlacklistListFn: func(_ context.Context, _ *userpb.GetBlacklistListRequest) (*userpb.GetBlacklistListResponse, error) {
				return &userpb.GetBlacklistListResponse{
					Items:      []*userpb.BlacklistItem{},
					Pagination: &userpb.PaginationInfo{Page: 1, PageSize: 20, Total: 0, TotalPages: 0},
				}, nil
			},
			batchGetProfileFn: func(_ context.Context, _ *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
				batchCalls++
				return &userpb.BatchGetProfileResponse{}, nil
			},
		}
		svc := NewBlacklistService(client)

		resp, err := svc.GetBlacklistList(context.Background(), &dto.GetBlacklistListRequest{Page: 1, PageSize: 20})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Empty(t, resp.Items)
		assert.Equal(t, 0, batchCalls)
	})

	t.Run("enrich_user_info_and_deduplicate", func(t *testing.T) {
		var batchCalls [][]string
		client := &fakeGatewayUserClient{
			getBlacklistListFn: func(_ context.Context, req *userpb.GetBlacklistListRequest) (*userpb.GetBlacklistListResponse, error) {
				require.Equal(t, int32(1), req.Page)
				require.Equal(t, int32(20), req.PageSize)
				return &userpb.GetBlacklistListResponse{
					Items: []*userpb.BlacklistItem{
						{Uuid: "u1", BlacklistedAt: 1001},
						nil,
						{Uuid: "u2", BlacklistedAt: 1002},
						{Uuid: "u1", BlacklistedAt: 1003},
						{Uuid: "", BlacklistedAt: 1004},
					},
					Pagination: &userpb.PaginationInfo{Page: 1, PageSize: 20, Total: 5, TotalPages: 1},
				}, nil
			},
			batchGetProfileFn: func(_ context.Context, req *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
				copied := append([]string(nil), req.UserUuids...)
				batchCalls = append(batchCalls, copied)
				return &userpb.BatchGetProfileResponse{
					Users: []*userpb.SimpleUserInfo{
						{Uuid: "u1", Nickname: "nick-1", Avatar: "avatar-1"},
						{Uuid: "u2", Nickname: "nick-2", Avatar: "avatar-2"},
					},
				}, nil
			},
		}
		svc := NewBlacklistService(client)

		resp, err := svc.GetBlacklistList(context.Background(), &dto.GetBlacklistListRequest{Page: 1, PageSize: 20})
		require.NoError(t, err)
		require.Len(t, batchCalls, 1)
		assert.Equal(t, []string{"u1", "u2"}, batchCalls[0])

		require.Len(t, resp.Items, 5)
		assert.Equal(t, "nick-1", resp.Items[0].Nickname)
		assert.Equal(t, "avatar-1", resp.Items[0].Avatar)
		assert.Nil(t, resp.Items[1])
		assert.Equal(t, "nick-2", resp.Items[2].Nickname)
		assert.Equal(t, "avatar-2", resp.Items[2].Avatar)
		assert.Equal(t, "nick-1", resp.Items[3].Nickname)
		assert.Equal(t, "avatar-1", resp.Items[3].Avatar)
		assert.Equal(t, "", resp.Items[4].Nickname)
		assert.Equal(t, "", resp.Items[4].Avatar)
	})

	t.Run("batch_profile_failed_then_degrade", func(t *testing.T) {
		client := &fakeGatewayUserClient{
			getBlacklistListFn: func(_ context.Context, _ *userpb.GetBlacklistListRequest) (*userpb.GetBlacklistListResponse, error) {
				return &userpb.GetBlacklistListResponse{
					Items: []*userpb.BlacklistItem{
						{Uuid: "u1", Nickname: "from-user-service", Avatar: "raw-avatar", BlacklistedAt: 1001},
					},
					Pagination: &userpb.PaginationInfo{Page: 1, PageSize: 20, Total: 1, TotalPages: 1},
				}, nil
			},
			batchGetProfileFn: func(_ context.Context, _ *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
				return nil, errors.New("downstream timeout")
			},
		}
		svc := NewBlacklistService(client)

		resp, err := svc.GetBlacklistList(context.Background(), &dto.GetBlacklistListRequest{Page: 1, PageSize: 20})
		require.NoError(t, err)
		require.Len(t, resp.Items, 1)
		assert.Equal(t, "from-user-service", resp.Items[0].Nickname)
		assert.Equal(t, "raw-avatar", resp.Items[0].Avatar)
	})

	t.Run("grpc_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("rpc failed")
		client := &fakeGatewayUserClient{
			getBlacklistListFn: func(_ context.Context, _ *userpb.GetBlacklistListRequest) (*userpb.GetBlacklistListResponse, error) {
				return nil, wantErr
			},
		}
		svc := NewBlacklistService(client)

		resp, err := svc.GetBlacklistList(context.Background(), &dto.GetBlacklistListRequest{Page: 1, PageSize: 20})
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}

func TestGatewayBlacklistServiceBatchGetSimpleUserInfoChunking(t *testing.T) {
	initGatewayBlacklistTestLogger()

	uuids := make([]string, 0, 208)
	for i := 0; i < 205; i++ {
		uuids = append(uuids, fmt.Sprintf("u-%03d", i))
	}
	uuids = append(uuids, "u-001", "u-020", "")

	var calls [][]string
	client := &fakeGatewayUserClient{
		batchGetProfileFn: func(_ context.Context, req *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
			copied := append([]string(nil), req.UserUuids...)
			calls = append(calls, copied)

			users := make([]*userpb.SimpleUserInfo, 0, len(req.UserUuids))
			for _, id := range req.UserUuids {
				users = append(users, &userpb.SimpleUserInfo{Uuid: id, Nickname: "nick-" + id, Avatar: "avatar-" + id})
			}
			return &userpb.BatchGetProfileResponse{Users: users}, nil
		},
	}
	svc := &BlacklistServiceImpl{userClient: client}

	result, err := svc.batchGetSimpleUserInfo(context.Background(), uuids)
	require.NoError(t, err)
	require.Len(t, calls, 3)
	assert.Len(t, calls[0], 100)
	assert.Len(t, calls[1], 100)
	assert.Len(t, calls[2], 5)
	assert.Len(t, result, 205)
	assert.Equal(t, "nick-u-001", result["u-001"].Nickname)
	assert.Equal(t, "avatar-u-204", result["u-204"].Avatar)
}

func TestGatewayBlacklistServiceBatchGetSimpleUserInfoEmptyInput(t *testing.T) {
	initGatewayBlacklistTestLogger()

	calls := 0
	client := &fakeGatewayUserClient{
		batchGetProfileFn: func(_ context.Context, _ *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
			calls++
			return &userpb.BatchGetProfileResponse{}, nil
		},
	}
	svc := &BlacklistServiceImpl{userClient: client}

	result, err := svc.batchGetSimpleUserInfo(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, result)
	assert.Equal(t, 0, calls)
}

func TestGatewayBlacklistServiceBatchGetSimpleUserInfoPartialError(t *testing.T) {
	initGatewayBlacklistTestLogger()

	uuids := make([]string, 0, 150)
	for i := 0; i < 150; i++ {
		uuids = append(uuids, fmt.Sprintf("u-%03d", i))
	}

	calls := 0
	client := &fakeGatewayUserClient{
		batchGetProfileFn: func(_ context.Context, req *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
			calls++
			if calls == 2 {
				return nil, errors.New("second batch failed")
			}

			users := make([]*userpb.SimpleUserInfo, 0, len(req.UserUuids))
			for _, id := range req.UserUuids {
				users = append(users, &userpb.SimpleUserInfo{Uuid: id, Nickname: "nick-" + id})
			}
			return &userpb.BatchGetProfileResponse{Users: users}, nil
		},
	}
	svc := &BlacklistServiceImpl{userClient: client}

	result, err := svc.batchGetSimpleUserInfo(context.Background(), uuids)
	require.Error(t, err)
	assert.Len(t, result, 100)
	assert.Equal(t, "nick-u-000", result["u-000"].Nickname)
	assert.Nil(t, result["u-149"])
}

func TestGatewayBlacklistServiceCheckIsBlacklist(t *testing.T) {
	initGatewayBlacklistTestLogger()

	t.Run("success", func(t *testing.T) {
		client := &fakeGatewayUserClient{
			checkIsBlacklistFn: func(_ context.Context, req *userpb.CheckIsBlacklistRequest) (*userpb.CheckIsBlacklistResponse, error) {
				require.Equal(t, "u1", req.UserUuid)
				require.Equal(t, "u2", req.TargetUuid)
				return &userpb.CheckIsBlacklistResponse{IsBlacklist: true}, nil
			},
		}
		svc := NewBlacklistService(client)

		resp, err := svc.CheckIsBlacklist(context.Background(), &dto.CheckIsBlacklistRequest{UserUUID: "u1", TargetUUID: "u2"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.IsBlacklist)
	})

	t.Run("grpc_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc unavailable")
		client := &fakeGatewayUserClient{
			checkIsBlacklistFn: func(_ context.Context, _ *userpb.CheckIsBlacklistRequest) (*userpb.CheckIsBlacklistResponse, error) {
				return nil, wantErr
			},
		}
		svc := NewBlacklistService(client)

		resp, err := svc.CheckIsBlacklist(context.Background(), &dto.CheckIsBlacklistRequest{UserUUID: "u1", TargetUUID: "u2"})
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})
}
