package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/apps/relation/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/status"
)

var relationServiceLoggerOnce sync.Once

func initRelationServiceTestLogger() {
	relationServiceLoggerOnce.Do(func() {
		logger.ReplaceGlobal(zap.NewNop())
	})
}

func withRelationUserUUID(userUUID string) context.Context {
	return context.WithValue(context.Background(), "user_uuid", userUUID)
}

func requireRelationBizCode(t *testing.T, err error, wantBizCode int) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, wantBizCode, apperr.Code(err))
	stErr := apperr.ToStatus(err)
	st, ok := status.FromError(stErr)
	require.True(t, ok)
	require.NotEmpty(t, st.Message())
	require.Equal(t, wantBizCode, apperr.Code(apperr.FromStatus(stErr)))
}

type fakeFriendRepoForService struct {
	getFriendListFn      func(context.Context, string, string, int, int) ([]*model.UserRelation, int64, int64, error)
	getFriendRelationFn  func(context.Context, string, string) (*model.UserRelation, error)
	createRelationFn     func(context.Context, string, string) error
	deleteRelationFn     func(context.Context, string, string) error
	cleanupRelationFn    func(context.Context, string) error
	setRemarkFn          func(context.Context, string, string, string) error
	setTagFn             func(context.Context, string, string, string) error
	isFriendFn           func(context.Context, string, string) (bool, error)
	checkIsFriendFn      func(context.Context, string, string) (bool, error)
	batchCheckIsFriendFn func(context.Context, string, []string) (map[string]bool, error)
	getRelationStatusFn  func(context.Context, string, string) (*model.UserRelation, error)
	syncFriendListFn     func(context.Context, string, repository.FriendSyncCursor, int) ([]*model.UserRelation, repository.FriendSyncCursor, bool, error)
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

func (f *fakeFriendRepoForService) CleanupAccountRelations(ctx context.Context, userUUID string) error {
	if f.cleanupRelationFn == nil {
		return nil
	}
	return f.cleanupRelationFn(ctx, userUUID)
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

func (f *fakeFriendRepoForService) SyncFriendList(ctx context.Context, userUUID string, cursor repository.FriendSyncCursor, limit int) ([]*model.UserRelation, repository.FriendSyncCursor, bool, error) {
	if f.syncFriendListFn == nil {
		return nil, repository.FriendSyncCursor{}, false, nil
	}
	return f.syncFriendListFn(ctx, userUUID, cursor, limit)
}

type fakeApplyRepoForService struct {
	createFn           func(context.Context, *model.ApplyRequest) (*model.ApplyRequest, error)
	getByIDFn          func(context.Context, int64) (*model.ApplyRequest, error)
	getPendingListFn   func(context.Context, string, int, int, int) ([]*model.ApplyRequest, int64, error)
	getSentListFn      func(context.Context, string, int, int, int) ([]*model.ApplyRequest, int64, error)
	cleanupAppliesFn   func(context.Context, string) error
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

func (f *fakeApplyRepoForService) CleanupAccountApplies(ctx context.Context, userUUID string) error {
	if f.cleanupAppliesFn == nil {
		return nil
	}
	return f.cleanupAppliesFn(ctx, userUUID)
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

func TestRelationFriendServiceSendFriendApply(t *testing.T) {
	initRelationServiceTestLogger()

	repoErr := errors.New("repo failed")

	tests := []struct {
		name            string
		ctx             context.Context
		req             *pb.SendFriendApplyRequest
		isFriend        bool
		isFriendErr     error
		existsPending   bool
		existsErr       error
		blockedByTarget bool
		blockedBySelf   bool
		blacklistErr    error
		createErr       error
		wantErr         int
		wantApplyID     int64
	}{
		{name: "unauthenticated", ctx: context.Background(), req: &pb.SendFriendApplyRequest{TargetUuid: "u2"}, wantErr: consts.CodeUnauthorized},
		{name: "cannot_add_self", ctx: withRelationUserUUID("u1"), req: &pb.SendFriendApplyRequest{TargetUuid: "u1"}, wantErr: consts.CodeCannotAddSelf},
		{name: "already_friend", ctx: withRelationUserUUID("u1"), req: &pb.SendFriendApplyRequest{TargetUuid: "u2"}, isFriend: true, wantErr: consts.CodeAlreadyFriend},
		{name: "pending_exists", ctx: withRelationUserUUID("u1"), req: &pb.SendFriendApplyRequest{TargetUuid: "u2"}, existsPending: true, wantErr: consts.CodeFriendRequestSent},
		{name: "blocked_by_target", ctx: withRelationUserUUID("u1"), req: &pb.SendFriendApplyRequest{TargetUuid: "u2"}, blockedByTarget: true, wantErr: consts.CodePeerBlacklistYou},
		{name: "blocked_by_self", ctx: withRelationUserUUID("u1"), req: &pb.SendFriendApplyRequest{TargetUuid: "u2"}, blockedBySelf: true, wantErr: consts.CodeYouBlacklistPeer},
		{name: "create_error", ctx: withRelationUserUUID("u1"), req: &pb.SendFriendApplyRequest{TargetUuid: "u2"}, createErr: repoErr, wantErr: consts.CodeInternalError},
		{name: "success", ctx: withRelationUserUUID("u1"), req: &pb.SendFriendApplyRequest{TargetUuid: "u2", Reason: "hi", Source: "search"}, wantApplyID: 101},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			friendRepo := &fakeFriendRepoForService{
				isFriendFn: func(_ context.Context, userUUID, friendUUID string) (bool, error) {
					assert.Equal(t, "u1", userUUID)
					assert.Equal(t, "u2", friendUUID)
					return tt.isFriend, tt.isFriendErr
				},
			}
			applyRepo := &fakeApplyRepoForService{
				existsPendingReqFn: func(_ context.Context, applicantUUID, targetUUID string) (bool, error) {
					assert.Equal(t, "u1", applicantUUID)
					assert.Equal(t, "u2", targetUUID)
					return tt.existsPending, tt.existsErr
				},
				createFn: func(_ context.Context, apply *model.ApplyRequest) (*model.ApplyRequest, error) {
					apply.Id = tt.wantApplyID
					return apply, tt.createErr
				},
			}
			blacklistRepo := &fakeBlacklistRepoForService{
				isBlockedFn: func(_ context.Context, userUUID, targetUUID string) (bool, error) {
					switch {
					case userUUID == "u2" && targetUUID == "u1":
						return tt.blockedByTarget, tt.blacklistErr
					case userUUID == "u1" && targetUUID == "u2":
						return tt.blockedBySelf, tt.blacklistErr
					default:
						return false, nil
					}
				},
			}

			svc := NewFriendService(nil, nil, friendRepo, applyRepo, blacklistRepo)
			resp, err := svc.SendFriendApply(tt.ctx, tt.req)

			if tt.wantErr != 0 {
				require.Nil(t, resp)
				requireRelationBizCode(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantApplyID, resp.ApplyId)
		})
	}
}

func TestRelationFriendServiceHandleFriendApply(t *testing.T) {
	initRelationServiceTestLogger()

	repoErr := errors.New("repo failed")

	t.Run("apply_not_found", func(t *testing.T) {
		svc := NewFriendService(nil, nil, &fakeFriendRepoForService{}, &fakeApplyRepoForService{}, &fakeBlacklistRepoForService{})
		err := svc.HandleFriendApply(withRelationUserUUID("u1"), &pb.HandleFriendApplyRequest{ApplyId: 1, Action: 1})
		requireRelationBizCode(t, err, consts.CodeApplyNotFoundOrHandle)
	})

	t.Run("no_permission", func(t *testing.T) {
		applyRepo := &fakeApplyRepoForService{
			getByIDFn: func(_ context.Context, _ int64) (*model.ApplyRequest, error) {
				return &model.ApplyRequest{Id: 1, ApplicantUuid: "u2", TargetUuid: "u3"}, nil
			},
		}
		svc := NewFriendService(nil, nil, &fakeFriendRepoForService{}, applyRepo, &fakeBlacklistRepoForService{})
		err := svc.HandleFriendApply(withRelationUserUUID("u1"), &pb.HandleFriendApplyRequest{ApplyId: 1, Action: 1})
		requireRelationBizCode(t, err, consts.CodeNoPermission)
	})

	t.Run("invalid_action", func(t *testing.T) {
		getByIDCalled := false
		applyRepo := &fakeApplyRepoForService{
			getByIDFn: func(_ context.Context, _ int64) (*model.ApplyRequest, error) {
				getByIDCalled = true
				return &model.ApplyRequest{Id: 1, ApplicantUuid: "u2", TargetUuid: "u1"}, nil
			},
		}
		svc := NewFriendService(nil, nil, &fakeFriendRepoForService{}, applyRepo, &fakeBlacklistRepoForService{})
		err := svc.HandleFriendApply(withRelationUserUUID("u1"), &pb.HandleFriendApplyRequest{ApplyId: 1, Action: 99})
		requireRelationBizCode(t, err, consts.CodeParamError)
		assert.False(t, getByIDCalled)
	})

	t.Run("accept_success", func(t *testing.T) {
		called := false
		applyRepo := &fakeApplyRepoForService{
			getByIDFn: func(_ context.Context, _ int64) (*model.ApplyRequest, error) {
				return &model.ApplyRequest{Id: 1, ApplicantUuid: "u2", TargetUuid: "u1"}, nil
			},
			acceptApplyFn: func(_ context.Context, applyID int64, userUUID, friendUUID, remark string) (bool, error) {
				called = true
				assert.Equal(t, int64(1), applyID)
				assert.Equal(t, "u1", userUUID)
				assert.Equal(t, "u2", friendUUID)
				assert.Equal(t, "buddy", remark)
				return false, nil
			},
		}
		svc := NewFriendService(nil, nil, &fakeFriendRepoForService{}, applyRepo, &fakeBlacklistRepoForService{})
		err := svc.HandleFriendApply(withRelationUserUUID("u1"), &pb.HandleFriendApplyRequest{ApplyId: 1, Action: 1, Remark: "buddy"})
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("reject_idempotent", func(t *testing.T) {
		applyRepo := &fakeApplyRepoForService{
			getByIDFn: func(_ context.Context, _ int64) (*model.ApplyRequest, error) {
				return &model.ApplyRequest{Id: 1, ApplicantUuid: "u2", TargetUuid: "u1"}, nil
			},
			updateStatusFn: func(_ context.Context, _ int64, _ int, _ string) error {
				return repository.ErrApplyNotFound
			},
		}
		svc := NewFriendService(nil, nil, &fakeFriendRepoForService{}, applyRepo, &fakeBlacklistRepoForService{})
		err := svc.HandleFriendApply(withRelationUserUUID("u1"), &pb.HandleFriendApplyRequest{ApplyId: 1, Action: 2})
		require.NoError(t, err)
	})

	t.Run("reject_error", func(t *testing.T) {
		applyRepo := &fakeApplyRepoForService{
			getByIDFn: func(_ context.Context, _ int64) (*model.ApplyRequest, error) {
				return &model.ApplyRequest{Id: 1, ApplicantUuid: "u2", TargetUuid: "u1"}, nil
			},
			updateStatusFn: func(_ context.Context, _ int64, _ int, _ string) error {
				return repoErr
			},
		}
		svc := NewFriendService(nil, nil, &fakeFriendRepoForService{}, applyRepo, &fakeBlacklistRepoForService{})
		err := svc.HandleFriendApply(withRelationUserUUID("u1"), &pb.HandleFriendApplyRequest{ApplyId: 1, Action: 2})
		requireRelationBizCode(t, err, consts.CodeInternalError)
	})
}

func TestRelationFriendServiceSyncFriendListCursor(t *testing.T) {
	initRelationServiceTestLogger()

	t.Run("uses_exact_cursor_and_returns_next_cursor", func(t *testing.T) {
		updatedAt := time.UnixMilli(1710000000123)
		inputCursor := repository.FriendSyncCursor{UpdatedAtUnixMilli: 1710000000000, LastID: 7, Exact: true}
		var gotCursor repository.FriendSyncCursor
		friendRepo := &fakeFriendRepoForService{
			syncFriendListFn: func(_ context.Context, userUUID string, cursor repository.FriendSyncCursor, limit int) ([]*model.UserRelation, repository.FriendSyncCursor, bool, error) {
				assert.Equal(t, "u1", userUUID)
				assert.Equal(t, 1, limit)
				gotCursor = cursor
				nextCursor := repository.FriendSyncCursor{UpdatedAtUnixMilli: updatedAt.UnixMilli(), LastID: 8, Exact: true}
				return []*model.UserRelation{{
					Id:        8,
					PeerUuid:  "u2",
					UpdatedAt: updatedAt,
				}}, nextCursor, true, nil
			},
		}
		svc := NewFriendService(nil, nil, friendRepo, &fakeApplyRepoForService{}, &fakeBlacklistRepoForService{})
		resp, err := svc.SyncFriendList(withRelationUserUUID("u1"), &pb.SyncFriendListRequest{
			Version: 999,
			Limit:   1,
			Cursor:  repository.EncodeFriendSyncCursor(inputCursor),
		})
		require.NoError(t, err)
		assert.Equal(t, inputCursor, gotCursor)
		assert.True(t, resp.HasMore)
		assert.Equal(t, updatedAt.UnixMilli(), resp.LatestVersion)
		assert.Equal(t, "v1:1710000000123:8", resp.NextCursor)
	})

	t.Run("rejects_invalid_cursor", func(t *testing.T) {
		called := false
		friendRepo := &fakeFriendRepoForService{
			syncFriendListFn: func(context.Context, string, repository.FriendSyncCursor, int) ([]*model.UserRelation, repository.FriendSyncCursor, bool, error) {
				called = true
				return nil, repository.FriendSyncCursor{}, false, nil
			},
		}
		svc := NewFriendService(nil, nil, friendRepo, &fakeApplyRepoForService{}, &fakeBlacklistRepoForService{})
		resp, err := svc.SyncFriendList(withRelationUserUUID("u1"), &pb.SyncFriendListRequest{Cursor: "bad-cursor"})
		require.Nil(t, resp)
		requireRelationBizCode(t, err, consts.CodeParamError)
		assert.False(t, called)
	})
}

func TestRelationFriendServiceGetUnreadApplyCountDegrade(t *testing.T) {
	initRelationServiceTestLogger()

	applyRepo := &fakeApplyRepoForService{
		getUnreadCountFn: func(_ context.Context, _ string) (int64, error) {
			return 0, errors.New("redis failed")
		},
	}
	svc := NewFriendService(nil, nil, &fakeFriendRepoForService{}, applyRepo, &fakeBlacklistRepoForService{})
	resp, err := svc.GetUnreadApplyCount(withRelationUserUUID("u1"), &pb.GetUnreadApplyCountRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int32(0), resp.UnreadCount)
}
