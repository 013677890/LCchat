package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/013677890/LCchat-Backend/apps/user/internal/service"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBlacklistHandlerService struct {
	addFn    func(context.Context, *pb.AddBlacklistRequest) error
	removeFn func(context.Context, *pb.RemoveBlacklistRequest) error
	listFn   func(context.Context, *pb.GetBlacklistListRequest) (*pb.GetBlacklistListResponse, error)
	checkFn  func(context.Context, *pb.CheckIsBlacklistRequest) (*pb.CheckIsBlacklistResponse, error)
}

var _ service.IBlacklistService = (*fakeBlacklistHandlerService)(nil)

func (f *fakeBlacklistHandlerService) AddBlacklist(ctx context.Context, req *pb.AddBlacklistRequest) error {
	if f.addFn == nil {
		return nil
	}
	return f.addFn(ctx, req)
}

func (f *fakeBlacklistHandlerService) RemoveBlacklist(ctx context.Context, req *pb.RemoveBlacklistRequest) error {
	if f.removeFn == nil {
		return nil
	}
	return f.removeFn(ctx, req)
}

func (f *fakeBlacklistHandlerService) GetBlacklistList(ctx context.Context, req *pb.GetBlacklistListRequest) (*pb.GetBlacklistListResponse, error) {
	if f.listFn == nil {
		return &pb.GetBlacklistListResponse{}, nil
	}
	return f.listFn(ctx, req)
}

func (f *fakeBlacklistHandlerService) CheckIsBlacklist(ctx context.Context, req *pb.CheckIsBlacklistRequest) (*pb.CheckIsBlacklistResponse, error) {
	if f.checkFn == nil {
		return &pb.CheckIsBlacklistResponse{}, nil
	}
	return f.checkFn(ctx, req)
}

func TestUserBlacklistHandlerAddBlacklist(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := &fakeBlacklistHandlerService{
			addFn: func(_ context.Context, req *pb.AddBlacklistRequest) error {
				require.Equal(t, "u2", req.TargetUuid)
				return nil
			},
		}
		h := NewBlacklistHandler(svc)

		resp, err := h.AddBlacklist(context.Background(), &pb.AddBlacklistRequest{TargetUuid: "u2"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.IsType(t, &pb.AddBlacklistResponse{}, resp)
	})

	t.Run("service_error", func(t *testing.T) {
		wantErr := errors.New("service error")
		svc := &fakeBlacklistHandlerService{
			addFn: func(_ context.Context, _ *pb.AddBlacklistRequest) error {
				return wantErr
			},
		}
		h := NewBlacklistHandler(svc)

		resp, err := h.AddBlacklist(context.Background(), &pb.AddBlacklistRequest{TargetUuid: "u2"})
		require.ErrorIs(t, err, wantErr)
		require.NotNil(t, resp)
		assert.IsType(t, &pb.AddBlacklistResponse{}, resp)
	})
}

func TestUserBlacklistHandlerRemoveBlacklist(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := &fakeBlacklistHandlerService{
			removeFn: func(_ context.Context, req *pb.RemoveBlacklistRequest) error {
				require.Equal(t, "u2", req.UserUuid)
				return nil
			},
		}
		h := NewBlacklistHandler(svc)

		resp, err := h.RemoveBlacklist(context.Background(), &pb.RemoveBlacklistRequest{UserUuid: "u2"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.IsType(t, &pb.RemoveBlacklistResponse{}, resp)
	})

	t.Run("service_error", func(t *testing.T) {
		wantErr := errors.New("service error")
		svc := &fakeBlacklistHandlerService{
			removeFn: func(_ context.Context, _ *pb.RemoveBlacklistRequest) error {
				return wantErr
			},
		}
		h := NewBlacklistHandler(svc)

		resp, err := h.RemoveBlacklist(context.Background(), &pb.RemoveBlacklistRequest{UserUuid: "u2"})
		require.ErrorIs(t, err, wantErr)
		require.NotNil(t, resp)
		assert.IsType(t, &pb.RemoveBlacklistResponse{}, resp)
	})
}

func TestUserBlacklistHandlerGetBlacklistList(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := &pb.GetBlacklistListResponse{
			Items: []*pb.BlacklistItem{{Uuid: "u2", BlacklistedAt: 1000}},
			Pagination: &pb.PaginationInfo{
				Page:       1,
				PageSize:   20,
				Total:      1,
				TotalPages: 1,
			},
		}
		svc := &fakeBlacklistHandlerService{
			listFn: func(_ context.Context, req *pb.GetBlacklistListRequest) (*pb.GetBlacklistListResponse, error) {
				require.Equal(t, int32(1), req.Page)
				require.Equal(t, int32(20), req.PageSize)
				return want, nil
			},
		}
		h := NewBlacklistHandler(svc)

		resp, err := h.GetBlacklistList(context.Background(), &pb.GetBlacklistListRequest{Page: 1, PageSize: 20})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, want, resp)
	})

	t.Run("service_error", func(t *testing.T) {
		wantErr := errors.New("service error")
		svc := &fakeBlacklistHandlerService{
			listFn: func(_ context.Context, _ *pb.GetBlacklistListRequest) (*pb.GetBlacklistListResponse, error) {
				return nil, wantErr
			},
		}
		h := NewBlacklistHandler(svc)

		resp, err := h.GetBlacklistList(context.Background(), &pb.GetBlacklistListRequest{Page: 1, PageSize: 20})
		require.ErrorIs(t, err, wantErr)
		assert.Nil(t, resp)
	})
}

func TestUserBlacklistHandlerCheckIsBlacklist(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := &pb.CheckIsBlacklistResponse{IsBlacklist: true}
		svc := &fakeBlacklistHandlerService{
			checkFn: func(_ context.Context, req *pb.CheckIsBlacklistRequest) (*pb.CheckIsBlacklistResponse, error) {
				require.Equal(t, "u1", req.UserUuid)
				require.Equal(t, "u2", req.TargetUuid)
				return want, nil
			},
		}
		h := NewBlacklistHandler(svc)

		resp, err := h.CheckIsBlacklist(context.Background(), &pb.CheckIsBlacklistRequest{UserUuid: "u1", TargetUuid: "u2"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, want, resp)
	})

	t.Run("service_error", func(t *testing.T) {
		wantErr := errors.New("service error")
		svc := &fakeBlacklistHandlerService{
			checkFn: func(_ context.Context, _ *pb.CheckIsBlacklistRequest) (*pb.CheckIsBlacklistResponse, error) {
				return nil, wantErr
			},
		}
		h := NewBlacklistHandler(svc)

		resp, err := h.CheckIsBlacklist(context.Background(), &pb.CheckIsBlacklistRequest{UserUuid: "u1", TargetUuid: "u2"})
		require.ErrorIs(t, err, wantErr)
		assert.Nil(t, resp)
	})
}
