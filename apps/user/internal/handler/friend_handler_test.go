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

type fakeFriendHandlerService struct {
	sendApplyFn      func(context.Context, *pb.SendFriendApplyRequest) (*pb.SendFriendApplyResponse, error)
	applyListFn      func(context.Context, *pb.GetFriendApplyListRequest) (*pb.GetFriendApplyListResponse, error)
	sentApplyListFn  func(context.Context, *pb.GetSentApplyListRequest) (*pb.GetSentApplyListResponse, error)
	handleApplyFn    func(context.Context, *pb.HandleFriendApplyRequest) error
	unreadCountFn    func(context.Context, *pb.GetUnreadApplyCountRequest) (*pb.GetUnreadApplyCountResponse, error)
	markReadFn       func(context.Context, *pb.MarkApplyAsReadRequest) error
	friendListFn     func(context.Context, *pb.GetFriendListRequest) (*pb.GetFriendListResponse, error)
	syncFn           func(context.Context, *pb.SyncFriendListRequest) (*pb.SyncFriendListResponse, error)
	deleteFn         func(context.Context, *pb.DeleteFriendRequest) error
	remarkFn         func(context.Context, *pb.SetFriendRemarkRequest) error
	tagFn            func(context.Context, *pb.SetFriendTagRequest) error
	getTagListFn     func(context.Context, *pb.GetTagListRequest) (*pb.GetTagListResponse, error)
	checkFn          func(context.Context, *pb.CheckIsFriendRequest) (*pb.CheckIsFriendResponse, error)
	batchCheckFn     func(context.Context, *pb.BatchCheckIsFriendRequest) (*pb.BatchCheckIsFriendResponse, error)
	getRelationFn    func(context.Context, *pb.GetRelationStatusRequest) (*pb.GetRelationStatusResponse, error)
}

var _ service.IFriendService = (*fakeFriendHandlerService)(nil)

func (f *fakeFriendHandlerService) SendFriendApply(ctx context.Context, req *pb.SendFriendApplyRequest) (*pb.SendFriendApplyResponse, error) {
	if f.sendApplyFn == nil {
		return &pb.SendFriendApplyResponse{}, nil
	}
	return f.sendApplyFn(ctx, req)
}

func (f *fakeFriendHandlerService) GetFriendApplyList(ctx context.Context, req *pb.GetFriendApplyListRequest) (*pb.GetFriendApplyListResponse, error) {
	if f.applyListFn == nil {
		return &pb.GetFriendApplyListResponse{}, nil
	}
	return f.applyListFn(ctx, req)
}

func (f *fakeFriendHandlerService) GetSentApplyList(ctx context.Context, req *pb.GetSentApplyListRequest) (*pb.GetSentApplyListResponse, error) {
	if f.sentApplyListFn == nil {
		return &pb.GetSentApplyListResponse{}, nil
	}
	return f.sentApplyListFn(ctx, req)
}

func (f *fakeFriendHandlerService) HandleFriendApply(ctx context.Context, req *pb.HandleFriendApplyRequest) error {
	if f.handleApplyFn == nil {
		return nil
	}
	return f.handleApplyFn(ctx, req)
}

func (f *fakeFriendHandlerService) GetUnreadApplyCount(ctx context.Context, req *pb.GetUnreadApplyCountRequest) (*pb.GetUnreadApplyCountResponse, error) {
	if f.unreadCountFn == nil {
		return &pb.GetUnreadApplyCountResponse{}, nil
	}
	return f.unreadCountFn(ctx, req)
}

func (f *fakeFriendHandlerService) MarkApplyAsRead(ctx context.Context, req *pb.MarkApplyAsReadRequest) error {
	if f.markReadFn == nil {
		return nil
	}
	return f.markReadFn(ctx, req)
}

func (f *fakeFriendHandlerService) GetFriendList(ctx context.Context, req *pb.GetFriendListRequest) (*pb.GetFriendListResponse, error) {
	if f.friendListFn == nil {
		return &pb.GetFriendListResponse{}, nil
	}
	return f.friendListFn(ctx, req)
}

func (f *fakeFriendHandlerService) SyncFriendList(ctx context.Context, req *pb.SyncFriendListRequest) (*pb.SyncFriendListResponse, error) {
	if f.syncFn == nil {
		return &pb.SyncFriendListResponse{}, nil
	}
	return f.syncFn(ctx, req)
}

func (f *fakeFriendHandlerService) DeleteFriend(ctx context.Context, req *pb.DeleteFriendRequest) error {
	if f.deleteFn == nil {
		return nil
	}
	return f.deleteFn(ctx, req)
}

func (f *fakeFriendHandlerService) SetFriendRemark(ctx context.Context, req *pb.SetFriendRemarkRequest) error {
	if f.remarkFn == nil {
		return nil
	}
	return f.remarkFn(ctx, req)
}

func (f *fakeFriendHandlerService) SetFriendTag(ctx context.Context, req *pb.SetFriendTagRequest) error {
	if f.tagFn == nil {
		return nil
	}
	return f.tagFn(ctx, req)
}

func (f *fakeFriendHandlerService) GetTagList(ctx context.Context, req *pb.GetTagListRequest) (*pb.GetTagListResponse, error) {
	if f.getTagListFn == nil {
		return &pb.GetTagListResponse{}, nil
	}
	return f.getTagListFn(ctx, req)
}

func (f *fakeFriendHandlerService) CheckIsFriend(ctx context.Context, req *pb.CheckIsFriendRequest) (*pb.CheckIsFriendResponse, error) {
	if f.checkFn == nil {
		return &pb.CheckIsFriendResponse{}, nil
	}
	return f.checkFn(ctx, req)
}

func (f *fakeFriendHandlerService) BatchCheckIsFriend(ctx context.Context, req *pb.BatchCheckIsFriendRequest) (*pb.BatchCheckIsFriendResponse, error) {
	if f.batchCheckFn == nil {
		return &pb.BatchCheckIsFriendResponse{}, nil
	}
	return f.batchCheckFn(ctx, req)
}

func (f *fakeFriendHandlerService) GetRelationStatus(ctx context.Context, req *pb.GetRelationStatusRequest) (*pb.GetRelationStatusResponse, error) {
	if f.getRelationFn == nil {
		return &pb.GetRelationStatusResponse{}, nil
	}
	return f.getRelationFn(ctx, req)
}

func TestUserFriendHandlerPassThroughMethods(t *testing.T) {
	t.Run("send_friend_apply", func(t *testing.T) {
		want := &pb.SendFriendApplyResponse{ApplyId: 1001}
		h := NewFriendHandler(&fakeFriendHandlerService{
			sendApplyFn: func(_ context.Context, req *pb.SendFriendApplyRequest) (*pb.SendFriendApplyResponse, error) {
				require.Equal(t, "u2", req.TargetUuid)
				return want, nil
			},
		})
		resp, err := h.SendFriendApply(context.Background(), &pb.SendFriendApplyRequest{TargetUuid: "u2"})
		require.NoError(t, err)
		assert.Equal(t, want, resp)
	})

	t.Run("get_apply_list", func(t *testing.T) {
		want := &pb.GetFriendApplyListResponse{Items: []*pb.FriendApplyItem{{ApplyId: 1}}}
		h := NewFriendHandler(&fakeFriendHandlerService{
			applyListFn: func(_ context.Context, req *pb.GetFriendApplyListRequest) (*pb.GetFriendApplyListResponse, error) {
				require.Equal(t, int32(1), req.Page)
				return want, nil
			},
		})
		resp, err := h.GetFriendApplyList(context.Background(), &pb.GetFriendApplyListRequest{Page: 1})
		require.NoError(t, err)
		assert.Equal(t, want, resp)
	})

	t.Run("get_sent_apply_list_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("failed")
		h := NewFriendHandler(&fakeFriendHandlerService{
			sentApplyListFn: func(_ context.Context, _ *pb.GetSentApplyListRequest) (*pb.GetSentApplyListResponse, error) {
				return nil, wantErr
			},
		})
		resp, err := h.GetSentApplyList(context.Background(), &pb.GetSentApplyListRequest{})
		assert.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("get_unread_count", func(t *testing.T) {
		want := &pb.GetUnreadApplyCountResponse{UnreadCount: 3}
		h := NewFriendHandler(&fakeFriendHandlerService{
			unreadCountFn: func(_ context.Context, _ *pb.GetUnreadApplyCountRequest) (*pb.GetUnreadApplyCountResponse, error) {
				return want, nil
			},
		})
		resp, err := h.GetUnreadApplyCount(context.Background(), &pb.GetUnreadApplyCountRequest{})
		require.NoError(t, err)
		assert.Equal(t, want, resp)
	})

	t.Run("get_friend_list", func(t *testing.T) {
		want := &pb.GetFriendListResponse{Version: 10}
		h := NewFriendHandler(&fakeFriendHandlerService{
			friendListFn: func(_ context.Context, _ *pb.GetFriendListRequest) (*pb.GetFriendListResponse, error) {
				return want, nil
			},
		})
		resp, err := h.GetFriendList(context.Background(), &pb.GetFriendListRequest{})
		require.NoError(t, err)
		assert.Equal(t, want, resp)
	})

	t.Run("sync_friend_list", func(t *testing.T) {
		want := &pb.SyncFriendListResponse{LatestVersion: 20}
		h := NewFriendHandler(&fakeFriendHandlerService{
			syncFn: func(_ context.Context, _ *pb.SyncFriendListRequest) (*pb.SyncFriendListResponse, error) {
				return want, nil
			},
		})
		resp, err := h.SyncFriendList(context.Background(), &pb.SyncFriendListRequest{})
		require.NoError(t, err)
		assert.Equal(t, want, resp)
	})

	t.Run("get_tag_list", func(t *testing.T) {
		want := &pb.GetTagListResponse{Tags: []*pb.TagItem{{TagName: "work"}}}
		h := NewFriendHandler(&fakeFriendHandlerService{
			getTagListFn: func(_ context.Context, _ *pb.GetTagListRequest) (*pb.GetTagListResponse, error) {
				return want, nil
			},
		})
		resp, err := h.GetTagList(context.Background(), &pb.GetTagListRequest{})
		require.NoError(t, err)
		assert.Equal(t, want, resp)
	})

	t.Run("check_and_batch_and_relation", func(t *testing.T) {
		h := NewFriendHandler(&fakeFriendHandlerService{
			checkFn: func(_ context.Context, req *pb.CheckIsFriendRequest) (*pb.CheckIsFriendResponse, error) {
				require.Equal(t, "u1", req.UserUuid)
				return &pb.CheckIsFriendResponse{IsFriend: true}, nil
			},
			batchCheckFn: func(_ context.Context, req *pb.BatchCheckIsFriendRequest) (*pb.BatchCheckIsFriendResponse, error) {
				require.Equal(t, "u1", req.UserUuid)
				return &pb.BatchCheckIsFriendResponse{
					Items: []*pb.FriendCheckItem{{PeerUuid: "u2", IsFriend: true}},
				}, nil
			},
			getRelationFn: func(_ context.Context, _ *pb.GetRelationStatusRequest) (*pb.GetRelationStatusResponse, error) {
				return &pb.GetRelationStatusResponse{Relation: "friend", IsFriend: true}, nil
			},
		})

		checkResp, checkErr := h.CheckIsFriend(context.Background(), &pb.CheckIsFriendRequest{UserUuid: "u1", PeerUuid: "u2"})
		require.NoError(t, checkErr)
		assert.True(t, checkResp.IsFriend)

		batchResp, batchErr := h.BatchCheckIsFriend(context.Background(), &pb.BatchCheckIsFriendRequest{UserUuid: "u1", PeerUuids: []string{"u2"}})
		require.NoError(t, batchErr)
		require.Len(t, batchResp.Items, 1)
		assert.True(t, batchResp.Items[0].IsFriend)

		relationResp, relationErr := h.GetRelationStatus(context.Background(), &pb.GetRelationStatusRequest{UserUuid: "u1", PeerUuid: "u2"})
		require.NoError(t, relationErr)
		assert.Equal(t, "friend", relationResp.Relation)
	})
}

func TestUserFriendHandlerEmptyResponseContracts(t *testing.T) {
	t.Run("handle_friend_apply", func(t *testing.T) {
		h := NewFriendHandler(&fakeFriendHandlerService{
			handleApplyFn: func(_ context.Context, req *pb.HandleFriendApplyRequest) error {
				require.Equal(t, int64(1), req.ApplyId)
				return nil
			},
		})
		resp, err := h.HandleFriendApply(context.Background(), &pb.HandleFriendApplyRequest{ApplyId: 1, Action: 1})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.IsType(t, &pb.HandleFriendApplyResponse{}, resp)

		wantErr := errors.New("handle failed")
		h = NewFriendHandler(&fakeFriendHandlerService{
			handleApplyFn: func(_ context.Context, _ *pb.HandleFriendApplyRequest) error {
				return wantErr
			},
		})
		resp, err = h.HandleFriendApply(context.Background(), &pb.HandleFriendApplyRequest{ApplyId: 1, Action: 1})
		require.ErrorIs(t, err, wantErr)
		require.NotNil(t, resp)
		assert.IsType(t, &pb.HandleFriendApplyResponse{}, resp)
	})

	t.Run("mark_read_delete_remark_tag", func(t *testing.T) {
		h := NewFriendHandler(&fakeFriendHandlerService{
			markReadFn: func(_ context.Context, _ *pb.MarkApplyAsReadRequest) error { return nil },
			deleteFn:   func(_ context.Context, _ *pb.DeleteFriendRequest) error { return nil },
			remarkFn:   func(_ context.Context, _ *pb.SetFriendRemarkRequest) error { return nil },
			tagFn:      func(_ context.Context, _ *pb.SetFriendTagRequest) error { return nil },
		})

		markResp, markErr := h.MarkApplyAsRead(context.Background(), &pb.MarkApplyAsReadRequest{ApplyIds: []int64{1}})
		require.NoError(t, markErr)
		assert.IsType(t, &pb.MarkApplyAsReadResponse{}, markResp)

		deleteResp, deleteErr := h.DeleteFriend(context.Background(), &pb.DeleteFriendRequest{UserUuid: "u2"})
		require.NoError(t, deleteErr)
		assert.IsType(t, &pb.DeleteFriendResponse{}, deleteResp)

		remarkResp, remarkErr := h.SetFriendRemark(context.Background(), &pb.SetFriendRemarkRequest{UserUuid: "u2", Remark: "r"})
		require.NoError(t, remarkErr)
		assert.IsType(t, &pb.SetFriendRemarkResponse{}, remarkResp)

		tagResp, tagErr := h.SetFriendTag(context.Background(), &pb.SetFriendTagRequest{UserUuid: "u2", GroupTag: "work"})
		require.NoError(t, tagErr)
		assert.IsType(t, &pb.SetFriendTagResponse{}, tagResp)
	})
}
