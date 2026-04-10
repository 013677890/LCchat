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
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/apperr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

var userFriendLoggerOnce sync.Once

func initUserFriendTestLogger() {
	userFriendLoggerOnce.Do(func() {
		logger.ReplaceGlobal(zap.NewNop())
	})
}

func withFriendUserUUID(userUUID string) context.Context {
	return context.WithValue(context.Background(), "user_uuid", userUUID)
}

func requireFriendStatusCode(t *testing.T, err error, _ codes.Code, wantBizCode int) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, wantBizCode, apperr.Code(err))
	stErr := apperr.ToStatus(err)
	st, ok := status.FromError(stErr)
	require.True(t, ok)
	require.Equal(t, wantBizCode, apperr.Code(apperr.FromStatus(stErr)))
	require.NotEmpty(t, st.Message())
}

type fakeFriendRepoForService struct {
	getFriendListFn      func(context.Context, string, string, int, int) ([]*model.UserRelation, int64, int64, error)
	getFriendRelationFn  func(context.Context, string, string) (*model.UserRelation, error)
	createRelationFn     func(context.Context, string, string) error
	deleteRelationFn     func(context.Context, string, string) error
	setRemarkFn          func(context.Context, string, string, string) error
	setTagFn             func(context.Context, string, string, string) error
	getTagListFn         func(context.Context, string) ([]string, error)
	isFriendFn           func(context.Context, string, string) (bool, error)
	checkIsFriendFn      func(context.Context, string, string) (bool, error)
	batchCheckIsFriendFn func(context.Context, string, []string) (map[string]bool, error)
	getRelationStatusFn  func(context.Context, string, string) (*model.UserRelation, error)
	syncFriendListFn     func(context.Context, string, int64, int) ([]*model.UserRelation, int64, bool, error)
}

func (f *fakeFriendRepoForService) GetFriendList(ctx context.Context, userUUID, groupTag string, page, pageSize int) ([]*model.UserRelation, int64, int64, error) {
	if f.getFriendListFn == nil {
		return nil, 0, 0, nil
	}
	return f.getFriendListFn(ctx, userUUID, groupTag, page, pageSize)
}

func (f *fakeFriendRepoForService) GetFriendRelation(ctx context.Context, userUUID, friendUUID string) (*model.UserRelation, error) {
	if f.getFriendRelationFn == nil {
		return nil, nil
	}
	return f.getFriendRelationFn(ctx, userUUID, friendUUID)
}

func (f *fakeFriendRepoForService) CreateFriendRelation(ctx context.Context, userUUID, friendUUID string) error {
	if f.createRelationFn == nil {
		return nil
	}
	return f.createRelationFn(ctx, userUUID, friendUUID)
}

func (f *fakeFriendRepoForService) DeleteFriendRelation(ctx context.Context, userUUID, friendUUID string) error {
	if f.deleteRelationFn == nil {
		return nil
	}
	return f.deleteRelationFn(ctx, userUUID, friendUUID)
}

func (f *fakeFriendRepoForService) SetFriendRemark(ctx context.Context, userUUID, friendUUID, remark string) error {
	if f.setRemarkFn == nil {
		return nil
	}
	return f.setRemarkFn(ctx, userUUID, friendUUID, remark)
}

func (f *fakeFriendRepoForService) SetFriendTag(ctx context.Context, userUUID, friendUUID, groupTag string) error {
	if f.setTagFn == nil {
		return nil
	}
	return f.setTagFn(ctx, userUUID, friendUUID, groupTag)
}

func (f *fakeFriendRepoForService) GetTagList(ctx context.Context, userUUID string) ([]string, error) {
	if f.getTagListFn == nil {
		return nil, nil
	}
	return f.getTagListFn(ctx, userUUID)
}

func (f *fakeFriendRepoForService) IsFriend(ctx context.Context, userUUID, friendUUID string) (bool, error) {
	if f.isFriendFn == nil {
		return false, nil
	}
	return f.isFriendFn(ctx, userUUID, friendUUID)
}

func (f *fakeFriendRepoForService) CheckIsFriendRelation(ctx context.Context, userUUID, peerUUID string) (bool, error) {
	if f.checkIsFriendFn == nil {
		return false, nil
	}
	return f.checkIsFriendFn(ctx, userUUID, peerUUID)
}

func (f *fakeFriendRepoForService) BatchCheckIsFriend(ctx context.Context, userUUID string, peerUUIDs []string) (map[string]bool, error) {
	if f.batchCheckIsFriendFn == nil {
		return map[string]bool{}, nil
	}
	return f.batchCheckIsFriendFn(ctx, userUUID, peerUUIDs)
}

func (f *fakeFriendRepoForService) GetRelationStatus(ctx context.Context, userUUID, peerUUID string) (*model.UserRelation, error) {
	if f.getRelationStatusFn == nil {
		return nil, nil
	}
	return f.getRelationStatusFn(ctx, userUUID, peerUUID)
}

func (f *fakeFriendRepoForService) SyncFriendList(ctx context.Context, userUUID string, version int64, limit int) ([]*model.UserRelation, int64, bool, error) {
	if f.syncFriendListFn == nil {
		return nil, 0, false, nil
	}
	return f.syncFriendListFn(ctx, userUUID, version, limit)
}

type fakeApplyRepoForService struct {
	createFn           func(context.Context, *model.ApplyRequest) (*model.ApplyRequest, error)
	getByIDFn          func(context.Context, int64) (*model.ApplyRequest, error)
	getPendingListFn   func(context.Context, string, int, int, int) ([]*model.ApplyRequest, int64, error)
	getSentListFn      func(context.Context, string, int, int, int) ([]*model.ApplyRequest, int64, error)
	updateStatusFn     func(context.Context, int64, int, string) error
	acceptApplyFn      func(context.Context, int64, string, string, string) (bool, error)
	markAsReadFn       func(context.Context, string, []int64) (int64, error)
	markAllAsReadFn    func(context.Context, string) (int64, error)
	markAsReadAsyncFn  func(context.Context, []int64)
	getUnreadCountFn   func(context.Context, string) (int64, error)
	clearUnreadCountFn func(context.Context, string) error
	existsPendingReqFn func(context.Context, string, string) (bool, error)
	getByIDWithInfoFn  func(context.Context, int64) (*model.ApplyRequest, error)
}

func (f *fakeApplyRepoForService) Create(ctx context.Context, apply *model.ApplyRequest) (*model.ApplyRequest, error) {
	if f.createFn == nil {
		return apply, nil
	}
	return f.createFn(ctx, apply)
}

func (f *fakeApplyRepoForService) GetByID(ctx context.Context, id int64) (*model.ApplyRequest, error) {
	if f.getByIDFn == nil {
		return nil, repository.ErrRecordNotFound
	}
	return f.getByIDFn(ctx, id)
}

func (f *fakeApplyRepoForService) GetPendingList(ctx context.Context, targetUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error) {
	if f.getPendingListFn == nil {
		return nil, 0, nil
	}
	return f.getPendingListFn(ctx, targetUUID, status, page, pageSize)
}

func (f *fakeApplyRepoForService) GetSentList(ctx context.Context, applicantUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error) {
	if f.getSentListFn == nil {
		return nil, 0, nil
	}
	return f.getSentListFn(ctx, applicantUUID, status, page, pageSize)
}

func (f *fakeApplyRepoForService) UpdateStatus(ctx context.Context, id int64, status int, remark string) error {
	if f.updateStatusFn == nil {
		return nil
	}
	return f.updateStatusFn(ctx, id, status, remark)
}

func (f *fakeApplyRepoForService) AcceptApplyAndCreateRelation(ctx context.Context, applyID int64, userUUID, friendUUID, remark string) (bool, error) {
	if f.acceptApplyFn == nil {
		return false, nil
	}
	return f.acceptApplyFn(ctx, applyID, userUUID, friendUUID, remark)
}

func (f *fakeApplyRepoForService) MarkAsRead(ctx context.Context, targetUUID string, ids []int64) (int64, error) {
	if f.markAsReadFn == nil {
		return int64(len(ids)), nil
	}
	return f.markAsReadFn(ctx, targetUUID, ids)
}

func (f *fakeApplyRepoForService) MarkAllAsRead(ctx context.Context, targetUUID string) (int64, error) {
	if f.markAllAsReadFn == nil {
		return 0, nil
	}
	return f.markAllAsReadFn(ctx, targetUUID)
}

func (f *fakeApplyRepoForService) MarkAsReadAsync(ctx context.Context, ids []int64) {
	if f.markAsReadAsyncFn != nil {
		f.markAsReadAsyncFn(ctx, ids)
	}
}

func (f *fakeApplyRepoForService) GetUnreadCount(ctx context.Context, targetUUID string) (int64, error) {
	if f.getUnreadCountFn == nil {
		return 0, nil
	}
	return f.getUnreadCountFn(ctx, targetUUID)
}

func (f *fakeApplyRepoForService) ClearUnreadCount(ctx context.Context, targetUUID string) error {
	if f.clearUnreadCountFn == nil {
		return nil
	}
	return f.clearUnreadCountFn(ctx, targetUUID)
}

func (f *fakeApplyRepoForService) ExistsPendingRequest(ctx context.Context, applicantUUID, targetUUID string) (bool, error) {
	if f.existsPendingReqFn == nil {
		return false, nil
	}
	return f.existsPendingReqFn(ctx, applicantUUID, targetUUID)
}

func (f *fakeApplyRepoForService) GetByIDWithInfo(ctx context.Context, id int64) (*model.ApplyRequest, error) {
	if f.getByIDWithInfoFn == nil {
		return nil, nil
	}
	return f.getByIDWithInfoFn(ctx, id)
}

type fakeBlacklistRepoForService struct {
	isBlockedFn        func(context.Context, string, string) (bool, error)
	addBlacklistFn     func(context.Context, string, string) error
	removeBlacklistFn  func(context.Context, string, string) error
	getBlacklistListFn func(context.Context, string, int, int) ([]*model.UserRelation, int64, error)
	getBlacklistRelFn  func(context.Context, string, string) (*model.UserRelation, error)
}

func (f *fakeBlacklistRepoForService) AddBlacklist(ctx context.Context, userUUID, targetUUID string) error {
	if f.addBlacklistFn == nil {
		return nil
	}
	return f.addBlacklistFn(ctx, userUUID, targetUUID)
}

func (f *fakeBlacklistRepoForService) RemoveBlacklist(ctx context.Context, userUUID, targetUUID string) error {
	if f.removeBlacklistFn == nil {
		return nil
	}
	return f.removeBlacklistFn(ctx, userUUID, targetUUID)
}

func (f *fakeBlacklistRepoForService) GetBlacklistList(ctx context.Context, userUUID string, page, pageSize int) ([]*model.UserRelation, int64, error) {
	if f.getBlacklistListFn == nil {
		return nil, 0, nil
	}
	return f.getBlacklistListFn(ctx, userUUID, page, pageSize)
}

func (f *fakeBlacklistRepoForService) IsBlocked(ctx context.Context, userUUID, targetUUID string) (bool, error) {
	if f.isBlockedFn == nil {
		return false, nil
	}
	return f.isBlockedFn(ctx, userUUID, targetUUID)
}

func (f *fakeBlacklistRepoForService) GetBlacklistRelation(ctx context.Context, userUUID, targetUUID string) (*model.UserRelation, error) {
	if f.getBlacklistRelFn == nil {
		return nil, nil
	}
	return f.getBlacklistRelFn(ctx, userUUID, targetUUID)
}

func TestUserFriendServiceSendFriendApply(t *testing.T) {
	initUserFriendTestLogger()

	t.Run("unauthenticated", func(t *testing.T) {
		svc := NewFriendService(&fakeFriendRepoForService{}, &fakeApplyRepoForService{}, &fakeBlacklistRepoForService{})
		resp, err := svc.SendFriendApply(context.Background(), &pb.SendFriendApplyRequest{TargetUuid: "u2"})
		require.Nil(t, resp)
		requireFriendStatusCode(t, err, codes.Unauthenticated, consts.CodeUnauthorized)
	})

	t.Run("cannot_add_self", func(t *testing.T) {
		svc := NewFriendService(&fakeFriendRepoForService{}, &fakeApplyRepoForService{}, &fakeBlacklistRepoForService{})
		resp, err := svc.SendFriendApply(withFriendUserUUID("u1"), &pb.SendFriendApplyRequest{TargetUuid: "u1"})
		require.Nil(t, resp)
		requireFriendStatusCode(t, err, codes.InvalidArgument, consts.CodeCannotAddSelf)
	})

	t.Run("already_friend", func(t *testing.T) {
		svc := NewFriendService(&fakeFriendRepoForService{
			isFriendFn: func(_ context.Context, userUUID, friendUUID string) (bool, error) {
				assert.Equal(t, "u1", userUUID)
				assert.Equal(t, "u2", friendUUID)
				return true, nil
			},
		}, &fakeApplyRepoForService{}, &fakeBlacklistRepoForService{})
		resp, err := svc.SendFriendApply(withFriendUserUUID("u1"), &pb.SendFriendApplyRequest{TargetUuid: "u2"})
		require.Nil(t, resp)
		requireFriendStatusCode(t, err, codes.AlreadyExists, consts.CodeAlreadyFriend)
	})

	t.Run("pending_request_exists", func(t *testing.T) {
		svc := NewFriendService(&fakeFriendRepoForService{
			isFriendFn: func(_ context.Context, _, _ string) (bool, error) { return false, nil },
		}, &fakeApplyRepoForService{
			existsPendingReqFn: func(_ context.Context, _, _ string) (bool, error) {
				return true, nil
			},
		}, &fakeBlacklistRepoForService{})

		resp, err := svc.SendFriendApply(withFriendUserUUID("u1"), &pb.SendFriendApplyRequest{TargetUuid: "u2"})
		require.Nil(t, resp)
		requireFriendStatusCode(t, err, codes.AlreadyExists, consts.CodeFriendRequestSent)
	})

	t.Run("blocked_checks_and_create", func(t *testing.T) {
		var createCalled bool
		svc := NewFriendService(
			&fakeFriendRepoForService{
				isFriendFn: func(_ context.Context, _, _ string) (bool, error) { return false, nil },
			},
			&fakeApplyRepoForService{
				existsPendingReqFn: func(_ context.Context, _, _ string) (bool, error) { return false, nil },
				createFn: func(_ context.Context, apply *model.ApplyRequest) (*model.ApplyRequest, error) {
					createCalled = true
					require.Equal(t, "u1", apply.ApplicantUuid)
					require.Equal(t, "u2", apply.TargetUuid)
					require.Equal(t, "hello", apply.Reason)
					require.Equal(t, "search", apply.Source)
					return &model.ApplyRequest{Id: 101}, nil
				},
			},
			&fakeBlacklistRepoForService{
				isBlockedFn: func(_ context.Context, userUUID, targetUUID string) (bool, error) {
					if userUUID == "u2" && targetUUID == "u1" {
						return false, nil
					}
					if userUUID == "u1" && targetUUID == "u2" {
						return false, nil
					}
					return false, nil
				},
			},
		)

		resp, err := svc.SendFriendApply(withFriendUserUUID("u1"), &pb.SendFriendApplyRequest{
			TargetUuid: "u2",
			Reason:     "hello",
			Source:     "search",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int64(101), resp.ApplyId)
		assert.True(t, createCalled)
	})

	t.Run("blocked_by_target", func(t *testing.T) {
		svc := NewFriendService(
			&fakeFriendRepoForService{
				isFriendFn: func(_ context.Context, _, _ string) (bool, error) { return false, nil },
			},
			&fakeApplyRepoForService{
				existsPendingReqFn: func(_ context.Context, _, _ string) (bool, error) { return false, nil },
			},
			&fakeBlacklistRepoForService{
				isBlockedFn: func(_ context.Context, userUUID, targetUUID string) (bool, error) {
					if userUUID == "u2" && targetUUID == "u1" {
						return true, nil
					}
					return false, nil
				},
			},
		)
		resp, err := svc.SendFriendApply(withFriendUserUUID("u1"), &pb.SendFriendApplyRequest{TargetUuid: "u2"})
		require.Nil(t, resp)
		requireFriendStatusCode(t, err, codes.FailedPrecondition, consts.CodePeerBlacklistYou)
	})

	t.Run("blacklist_check_error", func(t *testing.T) {
		svc := NewFriendService(
			&fakeFriendRepoForService{isFriendFn: func(_ context.Context, _, _ string) (bool, error) { return false, nil }},
			&fakeApplyRepoForService{existsPendingReqFn: func(_ context.Context, _, _ string) (bool, error) { return false, nil }},
			&fakeBlacklistRepoForService{
				isBlockedFn: func(_ context.Context, _, _ string) (bool, error) {
					return false, errors.New("redis failed")
				},
			},
		)
		resp, err := svc.SendFriendApply(withFriendUserUUID("u1"), &pb.SendFriendApplyRequest{TargetUuid: "u2"})
		require.Nil(t, resp)
		requireFriendStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})
}

func TestUserFriendServiceGetFriendApplyList(t *testing.T) {
	initUserFriendTestLogger()

	t.Run("unauthenticated", func(t *testing.T) {
		svc := NewFriendService(&fakeFriendRepoForService{}, &fakeApplyRepoForService{}, &fakeBlacklistRepoForService{})
		resp, err := svc.GetFriendApplyList(context.Background(), &pb.GetFriendApplyListRequest{})
		require.Nil(t, resp)
		requireFriendStatusCode(t, err, codes.Unauthenticated, consts.CodeUnauthorized)
	})

	t.Run("repo_error", func(t *testing.T) {
		svc := NewFriendService(&fakeFriendRepoForService{}, &fakeApplyRepoForService{
			getPendingListFn: func(_ context.Context, _ string, _ int, _ int, _ int) ([]*model.ApplyRequest, int64, error) {
				return nil, 0, errors.New("db failed")
			},
		}, &fakeBlacklistRepoForService{})
		resp, err := svc.GetFriendApplyList(withFriendUserUUID("u1"), &pb.GetFriendApplyListRequest{Page: 1, PageSize: 20})
		require.Nil(t, resp)
		requireFriendStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("default_pagination_and_mark_read_async", func(t *testing.T) {
		createdAt := time.Unix(1700000000, 0)
		var asyncIDs []int64
		var clearCalled bool
		svc := NewFriendService(&fakeFriendRepoForService{}, &fakeApplyRepoForService{
			getPendingListFn: func(_ context.Context, userUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error) {
				assert.Equal(t, "u1", userUUID)
				assert.Equal(t, 0, status)
				assert.Equal(t, 1, page)
				assert.Equal(t, 20, pageSize)
				return []*model.ApplyRequest{
					nil,
					{Id: 1, ApplicantUuid: "u2", Status: 0, IsRead: false, Reason: "hi", Source: "search", CreatedAt: createdAt},
					{Id: 2, ApplicantUuid: "u3", Status: 1, IsRead: true, Reason: "ok", Source: "qrcode", CreatedAt: createdAt},
				}, 22, nil
			},
			markAsReadAsyncFn: func(_ context.Context, ids []int64) {
				asyncIDs = append(asyncIDs, ids...)
			},
			clearUnreadCountFn: func(_ context.Context, userUUID string) error {
				clearCalled = true
				assert.Equal(t, "u1", userUUID)
				return nil
			},
		}, &fakeBlacklistRepoForService{})

		resp, err := svc.GetFriendApplyList(withFriendUserUUID("u1"), &pb.GetFriendApplyListRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Items, 2)
		assert.Equal(t, int64(1), resp.Items[0].ApplyId)
		assert.Equal(t, "u2", resp.Items[0].ApplicantInfo.Uuid)
		assert.Equal(t, int32(1), resp.Pagination.Page)
		assert.Equal(t, int32(20), resp.Pagination.PageSize)
		assert.Equal(t, int64(22), resp.Pagination.Total)
		assert.Equal(t, []int64{1}, asyncIDs)
		assert.True(t, clearCalled)
	})

	t.Run("empty_result_still_clears_unread_best_effort", func(t *testing.T) {
		clearCalled := false
		svc := NewFriendService(&fakeFriendRepoForService{}, &fakeApplyRepoForService{
			getPendingListFn: func(_ context.Context, _ string, _ int, _ int, _ int) ([]*model.ApplyRequest, int64, error) {
				return []*model.ApplyRequest{}, 0, nil
			},
			clearUnreadCountFn: func(_ context.Context, _ string) error {
				clearCalled = true
				return errors.New("redis down")
			},
		}, &fakeBlacklistRepoForService{})

		resp, err := svc.GetFriendApplyList(withFriendUserUUID("u1"), &pb.GetFriendApplyListRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Empty(t, resp.Items)
		assert.True(t, clearCalled)
	})
}

func TestUserFriendServiceHandleFriendApply(t *testing.T) {
	initUserFriendTestLogger()

	t.Run("unauthenticated", func(t *testing.T) {
		svc := NewFriendService(&fakeFriendRepoForService{}, &fakeApplyRepoForService{}, &fakeBlacklistRepoForService{})
		err := svc.HandleFriendApply(context.Background(), &pb.HandleFriendApplyRequest{ApplyId: 1, Action: 1})
		requireFriendStatusCode(t, err, codes.Unauthenticated, consts.CodeUnauthorized)
	})

	t.Run("apply_not_found", func(t *testing.T) {
		svc := NewFriendService(&fakeFriendRepoForService{}, &fakeApplyRepoForService{
			getByIDFn: func(_ context.Context, _ int64) (*model.ApplyRequest, error) {
				return nil, repository.ErrRecordNotFound
			},
		}, &fakeBlacklistRepoForService{})
		err := svc.HandleFriendApply(withFriendUserUUID("u1"), &pb.HandleFriendApplyRequest{ApplyId: 1, Action: 1})
		requireFriendStatusCode(t, err, codes.NotFound, consts.CodeApplyNotFoundOrHandle)
	})

	t.Run("apply_nil_without_error", func(t *testing.T) {
		svc := NewFriendService(&fakeFriendRepoForService{}, &fakeApplyRepoForService{
			getByIDFn: func(_ context.Context, _ int64) (*model.ApplyRequest, error) {
				return nil, nil
			},
		}, &fakeBlacklistRepoForService{})
		err := svc.HandleFriendApply(withFriendUserUUID("u1"), &pb.HandleFriendApplyRequest{ApplyId: 1, Action: 2})
		requireFriendStatusCode(t, err, codes.NotFound, consts.CodeApplyNotFoundOrHandle)
	})

	t.Run("no_permission", func(t *testing.T) {
		svc := NewFriendService(&fakeFriendRepoForService{}, &fakeApplyRepoForService{
			getByIDFn: func(_ context.Context, _ int64) (*model.ApplyRequest, error) {
				return &model.ApplyRequest{Id: 1, TargetUuid: "u2", ApplicantUuid: "u3"}, nil
			},
		}, &fakeBlacklistRepoForService{})
		err := svc.HandleFriendApply(withFriendUserUUID("u1"), &pb.HandleFriendApplyRequest{ApplyId: 1, Action: 1})
		requireFriendStatusCode(t, err, codes.PermissionDenied, consts.CodeNoPermission)
	})

	t.Run("accept_success_and_error", func(t *testing.T) {
		var accepted bool
		svc := NewFriendService(&fakeFriendRepoForService{}, &fakeApplyRepoForService{
			getByIDFn: func(_ context.Context, _ int64) (*model.ApplyRequest, error) {
				return &model.ApplyRequest{Id: 1, TargetUuid: "u1", ApplicantUuid: "u2"}, nil
			},
			acceptApplyFn: func(_ context.Context, applyID int64, userUUID, friendUUID, remark string) (bool, error) {
				accepted = true
				assert.Equal(t, int64(1), applyID)
				assert.Equal(t, "u1", userUUID)
				assert.Equal(t, "u2", friendUUID)
				assert.Equal(t, "ok", remark)
				return false, nil
			},
		}, &fakeBlacklistRepoForService{})
		err := svc.HandleFriendApply(withFriendUserUUID("u1"), &pb.HandleFriendApplyRequest{ApplyId: 1, Action: 1, Remark: "ok"})
		require.NoError(t, err)
		assert.True(t, accepted)

		svc = NewFriendService(&fakeFriendRepoForService{}, &fakeApplyRepoForService{
			getByIDFn: func(_ context.Context, _ int64) (*model.ApplyRequest, error) {
				return &model.ApplyRequest{Id: 1, TargetUuid: "u1", ApplicantUuid: "u2"}, nil
			},
			acceptApplyFn: func(_ context.Context, _ int64, _, _, _ string) (bool, error) {
				return false, errors.New("tx failed")
			},
		}, &fakeBlacklistRepoForService{})
		err = svc.HandleFriendApply(withFriendUserUUID("u1"), &pb.HandleFriendApplyRequest{ApplyId: 1, Action: 1})
		requireFriendStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("reject_idempotent_and_error", func(t *testing.T) {
		svc := NewFriendService(&fakeFriendRepoForService{}, &fakeApplyRepoForService{
			getByIDFn: func(_ context.Context, _ int64) (*model.ApplyRequest, error) {
				return &model.ApplyRequest{Id: 1, TargetUuid: "u1", ApplicantUuid: "u2"}, nil
			},
			updateStatusFn: func(_ context.Context, _ int64, _ int, _ string) error {
				return repository.ErrApplyNotFound
			},
		}, &fakeBlacklistRepoForService{})
		err := svc.HandleFriendApply(withFriendUserUUID("u1"), &pb.HandleFriendApplyRequest{ApplyId: 1, Action: 2})
		require.NoError(t, err)

		svc = NewFriendService(&fakeFriendRepoForService{}, &fakeApplyRepoForService{
			getByIDFn: func(_ context.Context, _ int64) (*model.ApplyRequest, error) {
				return &model.ApplyRequest{Id: 1, TargetUuid: "u1", ApplicantUuid: "u2"}, nil
			},
			updateStatusFn: func(_ context.Context, _ int64, _ int, _ string) error {
				return errors.New("update failed")
			},
		}, &fakeBlacklistRepoForService{})
		err = svc.HandleFriendApply(withFriendUserUUID("u1"), &pb.HandleFriendApplyRequest{ApplyId: 1, Action: 2})
		requireFriendStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})
}

func TestUserFriendServiceUnreadMarkAndListSync(t *testing.T) {
	initUserFriendTestLogger()

	t.Run("get_unread_count_degrade", func(t *testing.T) {
		svc := NewFriendService(&fakeFriendRepoForService{}, &fakeApplyRepoForService{
			getUnreadCountFn: func(_ context.Context, _ string) (int64, error) {
				return 0, errors.New("redis unavailable")
			},
		}, &fakeBlacklistRepoForService{})
		resp, err := svc.GetUnreadApplyCount(withFriendUserUUID("u1"), &pb.GetUnreadApplyCountRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(0), resp.UnreadCount)
	})

	t.Run("mark_apply_as_read_paths", func(t *testing.T) {
		var markAllCalled bool
		var markSomeCalled bool
		svc := NewFriendService(&fakeFriendRepoForService{}, &fakeApplyRepoForService{
			markAllAsReadFn: func(_ context.Context, userUUID string) (int64, error) {
				markAllCalled = true
				assert.Equal(t, "u1", userUUID)
				return 10, nil
			},
			markAsReadFn: func(_ context.Context, userUUID string, ids []int64) (int64, error) {
				markSomeCalled = true
				assert.Equal(t, "u1", userUUID)
				assert.Equal(t, []int64{1, 2}, ids)
				return int64(len(ids)), nil
			},
			clearUnreadCountFn: func(_ context.Context, _ string) error {
				return errors.New("ignore")
			},
		}, &fakeBlacklistRepoForService{})

		require.NoError(t, svc.MarkApplyAsRead(withFriendUserUUID("u1"), &pb.MarkApplyAsReadRequest{}))
		require.NoError(t, svc.MarkApplyAsRead(withFriendUserUUID("u1"), &pb.MarkApplyAsReadRequest{ApplyIds: []int64{1, 2}}))
		assert.True(t, markAllCalled)
		assert.True(t, markSomeCalled)
	})

	t.Run("get_friend_list_and_sync_friend_list", func(t *testing.T) {
		now := time.Unix(1700000000, 0)
		deletedAt := gorm.DeletedAt{Time: now.Add(2 * time.Minute), Valid: true}
		svc := NewFriendService(&fakeFriendRepoForService{
			getFriendListFn: func(_ context.Context, userUUID, groupTag string, page, pageSize int) ([]*model.UserRelation, int64, int64, error) {
				assert.Equal(t, "u1", userUUID)
				assert.Equal(t, 1, page)
				assert.Equal(t, 20, pageSize)
				return []*model.UserRelation{
					nil,
					{PeerUuid: "u2", Remark: "r2", GroupTag: "g1", Source: "search", CreatedAt: now},
				}, 1, 10, nil
			},
			syncFriendListFn: func(_ context.Context, userUUID string, version int64, limit int) ([]*model.UserRelation, int64, bool, error) {
				assert.Equal(t, "u1", userUUID)
				assert.Equal(t, int64(0), version)
				assert.Equal(t, 100, limit)
				return []*model.UserRelation{
					{PeerUuid: "u2", Remark: "r2", GroupTag: "g1", Source: "s1", CreatedAt: now.Add(1 * time.Minute), UpdatedAt: now.Add(1 * time.Minute)},
					{PeerUuid: "u3", Remark: "r3", GroupTag: "g2", Source: "s2", UpdatedAt: now.Add(2 * time.Minute), DeletedAt: deletedAt},
				}, now.Add(3 * time.Minute).UnixMilli(), true, nil
			},
		}, &fakeApplyRepoForService{}, &fakeBlacklistRepoForService{})

		friendResp, friendErr := svc.GetFriendList(withFriendUserUUID("u1"), &pb.GetFriendListRequest{})
		require.NoError(t, friendErr)
		require.Len(t, friendResp.Items, 1)
		assert.Equal(t, "u2", friendResp.Items[0].Uuid)
		assert.Equal(t, int32(1), friendResp.Pagination.Page)
		assert.Equal(t, int64(10), friendResp.Version)

		syncResp, syncErr := svc.SyncFriendList(withFriendUserUUID("u1"), &pb.SyncFriendListRequest{})
		require.NoError(t, syncErr)
		require.Len(t, syncResp.Changes, 2)
		assert.Equal(t, "add", syncResp.Changes[0].ChangeType)
		assert.Equal(t, "delete", syncResp.Changes[1].ChangeType)
		assert.True(t, syncResp.HasMore)
		assert.Equal(t, syncResp.Changes[1].ChangedAt, syncResp.LatestVersion)
	})
}

func TestUserFriendServiceMutationsAndRelations(t *testing.T) {
	initUserFriendTestLogger()

	t.Run("delete_remark_tag_check", func(t *testing.T) {
		friendRepo := &fakeFriendRepoForService{
			deleteRelationFn: func(_ context.Context, _, _ string) error {
				return repository.ErrRecordNotFound
			},
			setRemarkFn: func(_ context.Context, _, _, _ string) error {
				return errors.New("db failed")
			},
			setTagFn: func(_ context.Context, _, _, _ string) error {
				return nil
			},
			checkIsFriendFn: func(_ context.Context, _, _ string) (bool, error) {
				return true, nil
			},
			batchCheckIsFriendFn: func(_ context.Context, _ string, peers []string) (map[string]bool, error) {
				return map[string]bool{
					"u2": true,
				}, nil
			},
		}
		svc := NewFriendService(friendRepo, &fakeApplyRepoForService{}, &fakeBlacklistRepoForService{})

		err := svc.DeleteFriend(withFriendUserUUID("u1"), &pb.DeleteFriendRequest{UserUuid: "u2"})
		requireFriendStatusCode(t, err, codes.NotFound, consts.CodeNotFriend)

		err = svc.SetFriendRemark(withFriendUserUUID("u1"), &pb.SetFriendRemarkRequest{UserUuid: "u2", Remark: "r"})
		requireFriendStatusCode(t, err, codes.Internal, consts.CodeInternalError)

		err = svc.SetFriendTag(withFriendUserUUID("u1"), &pb.SetFriendTagRequest{UserUuid: "u2", GroupTag: "work"})
		require.NoError(t, err)

		checkResp, checkErr := svc.CheckIsFriend(context.Background(), &pb.CheckIsFriendRequest{UserUuid: "u1", PeerUuid: "u2"})
		require.NoError(t, checkErr)
		assert.True(t, checkResp.IsFriend)

		batchResp, batchErr := svc.BatchCheckIsFriend(context.Background(), &pb.BatchCheckIsFriendRequest{UserUuid: "u1", PeerUuids: []string{"u2", ""}})
		require.NoError(t, batchErr)
		require.Len(t, batchResp.Items, 1)
		assert.Equal(t, "u2", batchResp.Items[0].PeerUuid)
		assert.True(t, batchResp.Items[0].IsFriend)
	})

	t.Run("relation_status_branches", func(t *testing.T) {
		now := time.Unix(1700000000, 0)
		svc := NewFriendService(&fakeFriendRepoForService{
			getRelationStatusFn: func(_ context.Context, userUUID, peerUUID string) (*model.UserRelation, error) {
				switch peerUUID {
				case "nil":
					return nil, nil
				case "friend":
					return &model.UserRelation{Status: 0, Remark: "r", GroupTag: "g"}, nil
				case "black":
					return &model.UserRelation{Status: 1}, nil
				case "deleted":
					return &model.UserRelation{Status: 2, DeletedAt: gorm.DeletedAt{Valid: true, Time: now}}, nil
				default:
					return nil, errors.New("db failed")
				}
			},
		}, &fakeApplyRepoForService{}, &fakeBlacklistRepoForService{})

		nilResp, nilErr := svc.GetRelationStatus(context.Background(), &pb.GetRelationStatusRequest{UserUuid: "u1", PeerUuid: "nil"})
		require.NoError(t, nilErr)
		assert.Equal(t, "none", nilResp.Relation)

		friendResp, friendErr := svc.GetRelationStatus(context.Background(), &pb.GetRelationStatusRequest{UserUuid: "u1", PeerUuid: "friend"})
		require.NoError(t, friendErr)
		assert.Equal(t, "friend", friendResp.Relation)
		assert.True(t, friendResp.IsFriend)
		assert.Equal(t, "r", friendResp.Remark)

		blackResp, blackErr := svc.GetRelationStatus(context.Background(), &pb.GetRelationStatusRequest{UserUuid: "u1", PeerUuid: "black"})
		require.NoError(t, blackErr)
		assert.Equal(t, "blacklist", blackResp.Relation)
		assert.True(t, blackResp.IsBlacklist)

		deletedResp, deletedErr := svc.GetRelationStatus(context.Background(), &pb.GetRelationStatusRequest{UserUuid: "u1", PeerUuid: "deleted"})
		require.NoError(t, deletedErr)
		assert.Equal(t, "deleted", deletedResp.Relation)

		errResp, err := svc.GetRelationStatus(context.Background(), &pb.GetRelationStatusRequest{UserUuid: "u1", PeerUuid: "err"})
		require.Nil(t, errResp)
		requireFriendStatusCode(t, err, codes.Internal, consts.CodeInternalError)
	})

	t.Run("relation_status_invalid_params", func(t *testing.T) {
		svc := NewFriendService(&fakeFriendRepoForService{}, &fakeApplyRepoForService{}, &fakeBlacklistRepoForService{})
		resp, err := svc.GetRelationStatus(context.Background(), nil)
		require.Nil(t, resp)
		requireFriendStatusCode(t, err, codes.InvalidArgument, consts.CodeParamError)
	})
}
