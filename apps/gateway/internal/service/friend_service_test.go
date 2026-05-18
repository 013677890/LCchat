package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	gatewaypb "github.com/013677890/LCchat-Backend/apps/gateway/internal/pb"
	relationpb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var gatewayFriendLoggerOnce sync.Once

func initGatewayFriendServiceTestLogger() {
	gatewayFriendLoggerOnce.Do(func() {
		logger.ReplaceGlobal(zap.NewNop())
	})
}

type fakeGatewayFriendClient struct {
	gatewaypb.UserServiceClient

	sendFriendApplyFn    func(context.Context, *relationpb.SendFriendApplyRequest) (*relationpb.SendFriendApplyResponse, error)
	getFriendApplyListFn func(context.Context, *relationpb.GetFriendApplyListRequest) (*relationpb.GetFriendApplyListResponse, error)
	getSentApplyListFn   func(context.Context, *relationpb.GetSentApplyListRequest) (*relationpb.GetSentApplyListResponse, error)
	handleFriendApplyFn  func(context.Context, *relationpb.HandleFriendApplyRequest) (*relationpb.HandleFriendApplyResponse, error)
	getUnreadCountFn     func(context.Context, *relationpb.GetUnreadApplyCountRequest) (*relationpb.GetUnreadApplyCountResponse, error)
	markApplyAsReadFn    func(context.Context, *relationpb.MarkApplyAsReadRequest) (*relationpb.MarkApplyAsReadResponse, error)
	getFriendListFn      func(context.Context, *relationpb.GetFriendListRequest) (*relationpb.GetFriendListResponse, error)
	syncFriendListFn     func(context.Context, *relationpb.SyncFriendListRequest) (*relationpb.SyncFriendListResponse, error)
	deleteFriendFn       func(context.Context, *relationpb.DeleteFriendRequest) (*relationpb.DeleteFriendResponse, error)
	setFriendRemarkFn    func(context.Context, *relationpb.SetFriendRemarkRequest) (*relationpb.SetFriendRemarkResponse, error)
	setFriendTagFn       func(context.Context, *relationpb.SetFriendTagRequest) (*relationpb.SetFriendTagResponse, error)
	checkIsFriendFn      func(context.Context, *relationpb.CheckIsFriendRequest) (*relationpb.CheckIsFriendResponse, error)
	getRelationStatusFn  func(context.Context, *relationpb.GetRelationStatusRequest) (*relationpb.GetRelationStatusResponse, error)
	batchGetProfileFn    func(context.Context, *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error)
}

func (f *fakeGatewayFriendClient) SendFriendApply(ctx context.Context, req *relationpb.SendFriendApplyRequest) (*relationpb.SendFriendApplyResponse, error) {
	if f.sendFriendApplyFn == nil {
		return nil, errors.New("unexpected SendFriendApply call")
	}
	return f.sendFriendApplyFn(ctx, req)
}

func (f *fakeGatewayFriendClient) GetFriendApplyList(ctx context.Context, req *relationpb.GetFriendApplyListRequest) (*relationpb.GetFriendApplyListResponse, error) {
	if f.getFriendApplyListFn == nil {
		return nil, errors.New("unexpected GetFriendApplyList call")
	}
	return f.getFriendApplyListFn(ctx, req)
}

func (f *fakeGatewayFriendClient) GetSentApplyList(ctx context.Context, req *relationpb.GetSentApplyListRequest) (*relationpb.GetSentApplyListResponse, error) {
	if f.getSentApplyListFn == nil {
		return nil, errors.New("unexpected GetSentApplyList call")
	}
	return f.getSentApplyListFn(ctx, req)
}

func (f *fakeGatewayFriendClient) HandleFriendApply(ctx context.Context, req *relationpb.HandleFriendApplyRequest) (*relationpb.HandleFriendApplyResponse, error) {
	if f.handleFriendApplyFn == nil {
		return nil, errors.New("unexpected HandleFriendApply call")
	}
	return f.handleFriendApplyFn(ctx, req)
}

func (f *fakeGatewayFriendClient) GetUnreadApplyCount(ctx context.Context, req *relationpb.GetUnreadApplyCountRequest) (*relationpb.GetUnreadApplyCountResponse, error) {
	if f.getUnreadCountFn == nil {
		return nil, errors.New("unexpected GetUnreadApplyCount call")
	}
	return f.getUnreadCountFn(ctx, req)
}

func (f *fakeGatewayFriendClient) MarkApplyAsRead(ctx context.Context, req *relationpb.MarkApplyAsReadRequest) (*relationpb.MarkApplyAsReadResponse, error) {
	if f.markApplyAsReadFn == nil {
		return nil, errors.New("unexpected MarkApplyAsRead call")
	}
	return f.markApplyAsReadFn(ctx, req)
}

func (f *fakeGatewayFriendClient) GetFriendList(ctx context.Context, req *relationpb.GetFriendListRequest) (*relationpb.GetFriendListResponse, error) {
	if f.getFriendListFn == nil {
		return nil, errors.New("unexpected GetFriendList call")
	}
	return f.getFriendListFn(ctx, req)
}

func (f *fakeGatewayFriendClient) SyncFriendList(ctx context.Context, req *relationpb.SyncFriendListRequest) (*relationpb.SyncFriendListResponse, error) {
	if f.syncFriendListFn == nil {
		return nil, errors.New("unexpected SyncFriendList call")
	}
	return f.syncFriendListFn(ctx, req)
}

func (f *fakeGatewayFriendClient) DeleteFriend(ctx context.Context, req *relationpb.DeleteFriendRequest) (*relationpb.DeleteFriendResponse, error) {
	if f.deleteFriendFn == nil {
		return nil, errors.New("unexpected DeleteFriend call")
	}
	return f.deleteFriendFn(ctx, req)
}

func (f *fakeGatewayFriendClient) SetFriendRemark(ctx context.Context, req *relationpb.SetFriendRemarkRequest) (*relationpb.SetFriendRemarkResponse, error) {
	if f.setFriendRemarkFn == nil {
		return nil, errors.New("unexpected SetFriendRemark call")
	}
	return f.setFriendRemarkFn(ctx, req)
}

func (f *fakeGatewayFriendClient) SetFriendTag(ctx context.Context, req *relationpb.SetFriendTagRequest) (*relationpb.SetFriendTagResponse, error) {
	if f.setFriendTagFn == nil {
		return nil, errors.New("unexpected SetFriendTag call")
	}
	return f.setFriendTagFn(ctx, req)
}

func (f *fakeGatewayFriendClient) CheckIsFriend(ctx context.Context, req *relationpb.CheckIsFriendRequest) (*relationpb.CheckIsFriendResponse, error) {
	if f.checkIsFriendFn == nil {
		return nil, errors.New("unexpected CheckIsFriend call")
	}
	return f.checkIsFriendFn(ctx, req)
}

func (f *fakeGatewayFriendClient) GetRelationStatus(ctx context.Context, req *relationpb.GetRelationStatusRequest) (*relationpb.GetRelationStatusResponse, error) {
	if f.getRelationStatusFn == nil {
		return nil, errors.New("unexpected GetRelationStatus call")
	}
	return f.getRelationStatusFn(ctx, req)
}

func (f *fakeGatewayFriendClient) BatchGetProfile(ctx context.Context, req *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
	if f.batchGetProfileFn == nil {
		return nil, errors.New("unexpected BatchGetProfile call")
	}
	return f.batchGetProfileFn(ctx, req)
}

func TestGatewayFriendServiceSendFriendApply(t *testing.T) {
	initGatewayFriendServiceTestLogger()

	t.Run("success_mapping", func(t *testing.T) {
		svc := NewFriendService(&fakeGatewayFriendClient{
			sendFriendApplyFn: func(_ context.Context, req *relationpb.SendFriendApplyRequest) (*relationpb.SendFriendApplyResponse, error) {
				require.Equal(t, "u2", req.TargetUuid)
				require.Equal(t, "hi", req.Reason)
				require.Equal(t, "search", req.Source)
				return &relationpb.SendFriendApplyResponse{ApplyId: 1001}, nil
			},
		})

		resp, err := svc.SendFriendApply(context.Background(), &dto.SendFriendApplyRequest{
			TargetUUID: "u2",
			Reason:     "hi",
			Source:     "search",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int64(1001), resp.ApplyID)
	})

	t.Run("downstream_error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc unavailable")
		svc := NewFriendService(&fakeGatewayFriendClient{
			sendFriendApplyFn: func(_ context.Context, _ *relationpb.SendFriendApplyRequest) (*relationpb.SendFriendApplyResponse, error) {
				return nil, wantErr
			},
		})

		resp, err := svc.SendFriendApply(context.Background(), &dto.SendFriendApplyRequest{TargetUUID: "u2"})
		require.Nil(t, resp)
		require.Equal(t, consts.CodeInternalError, apperr.Code(err))
	})
}

func TestGatewayFriendServiceGetFriendApplyList(t *testing.T) {
	initGatewayFriendServiceTestLogger()

	t.Run("error_passthrough", func(t *testing.T) {
		wantErr := errors.New("grpc unavailable")
		svc := NewFriendService(&fakeGatewayFriendClient{
			getFriendApplyListFn: func(_ context.Context, _ *relationpb.GetFriendApplyListRequest) (*relationpb.GetFriendApplyListResponse, error) {
				return nil, wantErr
			},
		})
		resp, err := svc.GetFriendApplyList(context.Background(), &dto.GetFriendApplyListRequest{Status: -1, Page: 1, PageSize: 20})
		require.Nil(t, resp)
		require.Equal(t, consts.CodeInternalError, apperr.Code(err))
	})

	t.Run("empty_items_do_not_call_batch_profile", func(t *testing.T) {
		batchCalled := false
		svc := NewFriendService(&fakeGatewayFriendClient{
			getFriendApplyListFn: func(_ context.Context, _ *relationpb.GetFriendApplyListRequest) (*relationpb.GetFriendApplyListResponse, error) {
				return &relationpb.GetFriendApplyListResponse{
					Items: []*relationpb.FriendApplyItem{},
				}, nil
			},
			batchGetProfileFn: func(_ context.Context, _ *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
				batchCalled = true
				return nil, nil
			},
		})
		resp, err := svc.GetFriendApplyList(context.Background(), &dto.GetFriendApplyListRequest{Status: -1, Page: 1, PageSize: 20})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, batchCalled)
	})

	t.Run("enrich_success_and_degrade_on_batch_error", func(t *testing.T) {
		t.Run("enrich_success", func(t *testing.T) {
			svc := NewFriendService(&fakeGatewayFriendClient{
				getFriendApplyListFn: func(_ context.Context, _ *relationpb.GetFriendApplyListRequest) (*relationpb.GetFriendApplyListResponse, error) {
					return &relationpb.GetFriendApplyListResponse{
						Items: []*relationpb.FriendApplyItem{
							{ApplyId: 1, ApplicantUuid: "u2"},
							nil,
							{ApplyId: 2, ApplicantUuid: "u3"},
						},
					}, nil
				},
				batchGetProfileFn: func(_ context.Context, req *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
					assert.ElementsMatch(t, []string{"u2", "u3"}, req.UserUuids)
					return &userpb.BatchGetProfileResponse{
						Users: []*userpb.SimpleUserInfo{
							{Uuid: "u2", Nickname: "n2", Avatar: "a2"},
							{Uuid: "u3", Nickname: "n3", Avatar: "a3"},
						},
					}, nil
				},
			})
			resp, err := svc.GetFriendApplyList(context.Background(), &dto.GetFriendApplyListRequest{Status: -1, Page: 1, PageSize: 20})
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Len(t, resp.Items, 3)
			assert.Equal(t, "n2", resp.Items[0].ApplicantNickname)
			assert.Equal(t, "a2", resp.Items[0].ApplicantAvatar)
			assert.Equal(t, "n3", resp.Items[2].ApplicantNickname)
			assert.Equal(t, "a3", resp.Items[2].ApplicantAvatar)
		})

		t.Run("batch_profile_failed_should_degrade", func(t *testing.T) {
			svc := NewFriendService(&fakeGatewayFriendClient{
				getFriendApplyListFn: func(_ context.Context, _ *relationpb.GetFriendApplyListRequest) (*relationpb.GetFriendApplyListResponse, error) {
					return &relationpb.GetFriendApplyListResponse{
						Items: []*relationpb.FriendApplyItem{{ApplyId: 1, ApplicantUuid: "u2"}},
					}, nil
				},
				batchGetProfileFn: func(_ context.Context, _ *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
					return nil, errors.New("batch failed")
				},
			})
			resp, err := svc.GetFriendApplyList(context.Background(), &dto.GetFriendApplyListRequest{Status: -1, Page: 1, PageSize: 20})
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Len(t, resp.Items, 1)
			assert.Equal(t, "", resp.Items[0].ApplicantNickname)
		})
	})
}

func TestGatewayFriendServiceGetSentApplyList(t *testing.T) {
	initGatewayFriendServiceTestLogger()

	t.Run("enrich_target_info", func(t *testing.T) {
		svc := NewFriendService(&fakeGatewayFriendClient{
			getSentApplyListFn: func(_ context.Context, _ *relationpb.GetSentApplyListRequest) (*relationpb.GetSentApplyListResponse, error) {
				return &relationpb.GetSentApplyListResponse{
					Items: []*relationpb.SentApplyItem{
						{ApplyId: 1, TargetUuid: "u2"},
						{ApplyId: 2, TargetUuid: "u3", TargetInfo: &relationpb.SimpleUserInfo{Uuid: "u3"}},
					},
				}, nil
			},
			batchGetProfileFn: func(_ context.Context, req *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
				assert.ElementsMatch(t, []string{"u2", "u3"}, req.UserUuids)
				return &userpb.BatchGetProfileResponse{
					Users: []*userpb.SimpleUserInfo{
						{Uuid: "u2", Nickname: "n2", Avatar: "a2", Gender: 1, Signature: "s2"},
						{Uuid: "u3", Nickname: "n3", Avatar: "a3", Gender: 2, Signature: "s3"},
					},
				}, nil
			},
		})
		resp, err := svc.GetSentApplyList(context.Background(), &dto.GetSentApplyListRequest{Status: -1, Page: 1, PageSize: 20})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Items, 2)
		assert.Equal(t, "n2", resp.Items[0].TargetInfo.Nickname)
		assert.Equal(t, "a3", resp.Items[1].TargetInfo.Avatar)
	})

	t.Run("batch_error_should_degrade", func(t *testing.T) {
		svc := NewFriendService(&fakeGatewayFriendClient{
			getSentApplyListFn: func(_ context.Context, _ *relationpb.GetSentApplyListRequest) (*relationpb.GetSentApplyListResponse, error) {
				return &relationpb.GetSentApplyListResponse{
					Items: []*relationpb.SentApplyItem{{ApplyId: 1, TargetUuid: "u2"}},
				}, nil
			},
			batchGetProfileFn: func(_ context.Context, _ *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
				return nil, errors.New("batch failed")
			},
		})
		resp, err := svc.GetSentApplyList(context.Background(), &dto.GetSentApplyListRequest{Status: -1, Page: 1, PageSize: 20})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Items, 1)
		assert.Nil(t, resp.Items[0].TargetInfo)
	})
}

func TestGatewayFriendServiceGetFriendListAndSync(t *testing.T) {
	initGatewayFriendServiceTestLogger()

	t.Run("get_friend_list_enrich", func(t *testing.T) {
		svc := NewFriendService(&fakeGatewayFriendClient{
			getFriendListFn: func(_ context.Context, _ *relationpb.GetFriendListRequest) (*relationpb.GetFriendListResponse, error) {
				return &relationpb.GetFriendListResponse{
					Items: []*relationpb.FriendItem{
						{Uuid: "u2"},
						{Uuid: "u3"},
					},
				}, nil
			},
			batchGetProfileFn: func(_ context.Context, req *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
				assert.ElementsMatch(t, []string{"u2", "u3"}, req.UserUuids)
				return &userpb.BatchGetProfileResponse{
					Users: []*userpb.SimpleUserInfo{
						{Uuid: "u2", Nickname: "n2", Avatar: "a2"},
						{Uuid: "u3", Nickname: "n3", Avatar: "a3"},
					},
				}, nil
			},
		})
		resp, err := svc.GetFriendList(context.Background(), &dto.GetFriendListRequest{Page: 1, PageSize: 20})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Items, 2)
		assert.Equal(t, "n2", resp.Items[0].Nickname)
		assert.Equal(t, "a3", resp.Items[1].Avatar)
	})

	t.Run("sync_friend_list_enrich_skip_delete", func(t *testing.T) {
		svc := NewFriendService(&fakeGatewayFriendClient{
			syncFriendListFn: func(_ context.Context, _ *relationpb.SyncFriendListRequest) (*relationpb.SyncFriendListResponse, error) {
				return &relationpb.SyncFriendListResponse{
					Changes: []*relationpb.FriendChange{
						{Uuid: "u2", ChangeType: "add"},
						{Uuid: "u3", ChangeType: "delete"},
						{Uuid: "u4", ChangeType: "update"},
					},
					LatestVersion: 100,
				}, nil
			},
			batchGetProfileFn: func(_ context.Context, req *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
				assert.ElementsMatch(t, []string{"u2", "u4"}, req.UserUuids)
				return &userpb.BatchGetProfileResponse{
					Users: []*userpb.SimpleUserInfo{
						{Uuid: "u2", Nickname: "n2", Avatar: "a2"},
						{Uuid: "u4", Nickname: "n4", Avatar: "a4"},
					},
				}, nil
			},
		})
		resp, err := svc.SyncFriendList(context.Background(), &dto.SyncFriendListRequest{Version: 0, Limit: 100})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Changes, 3)
		assert.Equal(t, "n2", resp.Changes[0].Nickname)
		assert.Equal(t, "", resp.Changes[1].Nickname)
		assert.Equal(t, "n4", resp.Changes[2].Nickname)
	})
}

func TestGatewayFriendServiceSimpleMethods(t *testing.T) {
	initGatewayFriendServiceTestLogger()

	t.Run("handle_friend_apply", func(t *testing.T) {
		wantErr := errors.New("handle failed")
		svc := NewFriendService(&fakeGatewayFriendClient{
			handleFriendApplyFn: func(_ context.Context, req *relationpb.HandleFriendApplyRequest) (*relationpb.HandleFriendApplyResponse, error) {
				if req.ApplyId == 1 {
					return &relationpb.HandleFriendApplyResponse{}, nil
				}
				return nil, wantErr
			},
		})
		okResp, okErr := svc.HandleFriendApply(context.Background(), &dto.HandleFriendApplyRequest{ApplyID: 1, Action: 1})
		require.NoError(t, okErr)
		require.Nil(t, okResp)

		errResp, err := svc.HandleFriendApply(context.Background(), &dto.HandleFriendApplyRequest{ApplyID: 2, Action: 1})
		require.Nil(t, errResp)
		require.Equal(t, consts.CodeInternalError, apperr.Code(err))
	})

	t.Run("other_simple_methods", func(t *testing.T) {
		wantErr := errors.New("downstream failed")
		svc := NewFriendService(&fakeGatewayFriendClient{
			getUnreadCountFn: func(_ context.Context, _ *relationpb.GetUnreadApplyCountRequest) (*relationpb.GetUnreadApplyCountResponse, error) {
				return &relationpb.GetUnreadApplyCountResponse{UnreadCount: 3}, nil
			},
			markApplyAsReadFn: func(_ context.Context, req *relationpb.MarkApplyAsReadRequest) (*relationpb.MarkApplyAsReadResponse, error) {
				if len(req.ApplyIds) == 0 {
					return nil, wantErr
				}
				return &relationpb.MarkApplyAsReadResponse{}, nil
			},
			deleteFriendFn: func(_ context.Context, req *relationpb.DeleteFriendRequest) (*relationpb.DeleteFriendResponse, error) {
				if req.UserUuid == "bad" {
					return nil, wantErr
				}
				return &relationpb.DeleteFriendResponse{}, nil
			},
			setFriendRemarkFn: func(_ context.Context, req *relationpb.SetFriendRemarkRequest) (*relationpb.SetFriendRemarkResponse, error) {
				if req.UserUuid == "bad" {
					return nil, wantErr
				}
				return &relationpb.SetFriendRemarkResponse{}, nil
			},
			setFriendTagFn: func(_ context.Context, req *relationpb.SetFriendTagRequest) (*relationpb.SetFriendTagResponse, error) {
				if req.UserUuid == "bad" {
					return nil, wantErr
				}
				return &relationpb.SetFriendTagResponse{}, nil
			},
			checkIsFriendFn: func(_ context.Context, req *relationpb.CheckIsFriendRequest) (*relationpb.CheckIsFriendResponse, error) {
				if req.PeerUuid == "bad" {
					return nil, wantErr
				}
				return &relationpb.CheckIsFriendResponse{IsFriend: true}, nil
			},
			getRelationStatusFn: func(_ context.Context, req *relationpb.GetRelationStatusRequest) (*relationpb.GetRelationStatusResponse, error) {
				if req.PeerUuid == "bad" {
					return nil, wantErr
				}
				return &relationpb.GetRelationStatusResponse{Relation: "friend", IsFriend: true}, nil
			},
		})

		unreadResp, unreadErr := svc.GetUnreadApplyCount(context.Background(), &dto.GetUnreadApplyCountRequest{})
		require.NoError(t, unreadErr)
		require.NotNil(t, unreadResp)
		assert.Equal(t, int32(3), unreadResp.UnreadCount)

		markResp, markErr := svc.MarkApplyAsRead(context.Background(), &dto.MarkApplyAsReadRequest{ApplyIDs: []int64{1}})
		require.NoError(t, markErr)
		require.Nil(t, markResp)
		_, markErrBad := svc.MarkApplyAsRead(context.Background(), &dto.MarkApplyAsReadRequest{})
		require.ErrorIs(t, markErrBad, wantErr)

		delResp, delErr := svc.DeleteFriend(context.Background(), &dto.DeleteFriendRequest{UserUUID: "u2"})
		require.NoError(t, delErr)
		require.Nil(t, delResp)
		_, delErrBad := svc.DeleteFriend(context.Background(), &dto.DeleteFriendRequest{UserUUID: "bad"})
		require.ErrorIs(t, delErrBad, wantErr)

		remarkResp, remarkErr := svc.SetFriendRemark(context.Background(), &dto.SetFriendRemarkRequest{UserUUID: "u2", Remark: "r"})
		require.NoError(t, remarkErr)
		require.Nil(t, remarkResp)
		_, remarkErrBad := svc.SetFriendRemark(context.Background(), &dto.SetFriendRemarkRequest{UserUUID: "bad", Remark: "r"})
		require.ErrorIs(t, remarkErrBad, wantErr)

		tagResp, tagErr := svc.SetFriendTag(context.Background(), &dto.SetFriendTagRequest{UserUUID: "u2", GroupTag: "g"})
		require.NoError(t, tagErr)
		require.Nil(t, tagResp)
		_, tagErrBad := svc.SetFriendTag(context.Background(), &dto.SetFriendTagRequest{UserUUID: "bad", GroupTag: "g"})
		require.ErrorIs(t, tagErrBad, wantErr)

		checkResp, checkErr := svc.CheckIsFriend(context.Background(), &dto.CheckIsFriendRequest{UserUUID: "u1", PeerUUID: "u2"})
		require.NoError(t, checkErr)
		require.NotNil(t, checkResp)
		assert.True(t, checkResp.IsFriend)
		_, checkErrBad := svc.CheckIsFriend(context.Background(), &dto.CheckIsFriendRequest{UserUUID: "u1", PeerUUID: "bad"})
		require.ErrorIs(t, checkErrBad, wantErr)

		relationResp, relationErr := svc.GetRelationStatus(context.Background(), &dto.GetRelationStatusRequest{UserUUID: "u1", PeerUUID: "u2"})
		require.NoError(t, relationErr)
		require.NotNil(t, relationResp)
		assert.Equal(t, "friend", relationResp.Relation)
		_, relationErrBad := svc.GetRelationStatus(context.Background(), &dto.GetRelationStatusRequest{UserUUID: "u1", PeerUUID: "bad"})
		require.ErrorIs(t, relationErrBad, wantErr)
	})
}

func TestGatewayFriendServiceBatchGetSimpleUserInfo(t *testing.T) {
	initGatewayFriendServiceTestLogger()

	t.Run("empty_input", func(t *testing.T) {
		svc := NewFriendService(&fakeGatewayFriendClient{})
		result, err := svc.(*FriendServiceImpl).batchGetSimpleUserInfo(context.Background(), nil)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("dedup_and_chunk", func(t *testing.T) {
		var calls int
		var totalRequested int
		svc := NewFriendService(&fakeGatewayFriendClient{
			batchGetProfileFn: func(_ context.Context, req *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
				calls++
				totalRequested += len(req.UserUuids)
				users := make([]*userpb.SimpleUserInfo, 0, len(req.UserUuids))
				for _, u := range req.UserUuids {
					users = append(users, &userpb.SimpleUserInfo{Uuid: u, Nickname: "n-" + u})
				}
				return &userpb.BatchGetProfileResponse{Users: users}, nil
			},
		})

		uuids := make([]string, 0, 230)
		uuids = append(uuids, "", "u-dup", "u-dup")
		for i := 0; i < 205; i++ {
			uuids = append(uuids, fmt.Sprintf("u-%03d", i))
		}

		result, err := svc.(*FriendServiceImpl).batchGetSimpleUserInfo(context.Background(), uuids)
		require.NoError(t, err)
		require.Len(t, result, 206) // u-dup + 205 unique
		assert.Equal(t, 3, calls)
		assert.Equal(t, 206, totalRequested)
		assert.Equal(t, "n-u-dup", result["u-dup"].Nickname)
		assert.Equal(t, "n-u-100", result["u-100"].Nickname)
	})

	t.Run("partial_result_when_batch_failed", func(t *testing.T) {
		var calls int
		svc := NewFriendService(&fakeGatewayFriendClient{
			batchGetProfileFn: func(_ context.Context, req *userpb.BatchGetProfileRequest) (*userpb.BatchGetProfileResponse, error) {
				calls++
				if calls == 2 {
					return nil, errors.New("second batch failed")
				}
				users := make([]*userpb.SimpleUserInfo, 0, len(req.UserUuids))
				for _, u := range req.UserUuids {
					users = append(users, &userpb.SimpleUserInfo{Uuid: u, Nickname: "n-" + u})
				}
				return &userpb.BatchGetProfileResponse{Users: users}, nil
			},
		})

		uuids := make([]string, 0, 130)
		for i := 0; i < 130; i++ {
			uuids = append(uuids, fmt.Sprintf("u-%03d", i))
		}

		result, err := svc.(*FriendServiceImpl).batchGetSimpleUserInfo(context.Background(), uuids)
		require.Error(t, err)
		require.Len(t, result, 100)
		assert.Equal(t, "n-u-000", result["u-000"].Nickname)
		_, ok := result["u-120"]
		assert.False(t, ok)
	})
}
