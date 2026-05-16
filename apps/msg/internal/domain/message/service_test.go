package message

import (
	"context"
	"errors"
	"testing"
	"time"

	pb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func init() { logger.ReplaceGlobal(zap.NewNop()) }

// ==================== mock Repository ====================

type mockRepo struct {
	tryAcquireFn        func(ctx context.Context, from, dev, cid string) (*model.Message, error)
	setIdempotentFn     func(ctx context.Context, from, dev, cid string, msg *model.Message) error
	allocSeqFn          func(ctx context.Context, convId string) (int64, error)
	createFn            func(ctx context.Context, msg *model.Message) error
	getByDuplicateKeyFn func(ctx context.Context, from, dev, cid string) (*model.Message, error)
	getBySeqRangeFn     func(ctx context.Context, convId string, anchor int64, dir int, limit int, clearSeq int64) ([]*model.Message, error)
	getByIdsFn          func(ctx context.Context, convId string, ids []string) ([]*model.Message, error)
	getByIdFn           func(ctx context.Context, convId, msgId string) (*model.Message, error)
	getMaxSeqFn         func(ctx context.Context, convId string) (int64, error)
	updateStatusFn      func(ctx context.Context, convId, msgId string, status int8, content string) error
}

func (m *mockRepo) TryAcquireIdempotent(ctx context.Context, from, dev, cid string) (*model.Message, error) {
	if m.tryAcquireFn != nil {
		return m.tryAcquireFn(ctx, from, dev, cid)
	}
	return nil, nil
}
func (m *mockRepo) SetIdempotentResult(ctx context.Context, from, dev, cid string, msg *model.Message) error {
	if m.setIdempotentFn != nil {
		return m.setIdempotentFn(ctx, from, dev, cid, msg)
	}
	return nil
}
func (m *mockRepo) AllocSeq(ctx context.Context, convId string) (int64, error) {
	if m.allocSeqFn != nil {
		return m.allocSeqFn(ctx, convId)
	}
	return 1, nil
}
func (m *mockRepo) Create(ctx context.Context, msg *model.Message) error {
	if m.createFn != nil {
		return m.createFn(ctx, msg)
	}
	return nil
}
func (m *mockRepo) GetByDuplicateKey(ctx context.Context, from, dev, cid string) (*model.Message, error) {
	if m.getByDuplicateKeyFn != nil {
		return m.getByDuplicateKeyFn(ctx, from, dev, cid)
	}
	return nil, ErrMessageNotFound
}
func (m *mockRepo) GetBySeqRange(ctx context.Context, convId string, anchor int64, dir int, limit int, clearSeq int64) ([]*model.Message, error) {
	if m.getBySeqRangeFn != nil {
		return m.getBySeqRangeFn(ctx, convId, anchor, dir, limit, clearSeq)
	}
	return nil, nil
}
func (m *mockRepo) GetByIds(ctx context.Context, convId string, ids []string) ([]*model.Message, error) {
	if m.getByIdsFn != nil {
		return m.getByIdsFn(ctx, convId, ids)
	}
	return nil, nil
}
func (m *mockRepo) GetById(ctx context.Context, convId, msgId string) (*model.Message, error) {
	if m.getByIdFn != nil {
		return m.getByIdFn(ctx, convId, msgId)
	}
	return nil, ErrMessageNotFound
}
func (m *mockRepo) GetMaxSeq(ctx context.Context, convId string) (int64, error) {
	if m.getMaxSeqFn != nil {
		return m.getMaxSeqFn(ctx, convId)
	}
	return 0, nil
}
func (m *mockRepo) UpdateStatus(ctx context.Context, convId, msgId string, status int8, content string) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, convId, msgId, status, content)
	}
	return nil
}

// ==================== mock GroupRoleQuerier ====================

type mockRoleQuerier struct {
	roleFn func(ctx context.Context, groupUUID, userUUID string) (int8, error)
}

func (m *mockRoleQuerier) QueryMemberRole(ctx context.Context, groupUUID, userUUID string) (int8, error) {
	return m.roleFn(ctx, groupUUID, userUUID)
}

// ==================== CreateMessage ====================

func newP2PReq() *pb.SendMessageRequest {
	return &pb.SendMessageRequest{
		FromUuid:    "user_aaa",
		DeviceId:    "dev1",
		ConvType:    pb.ConvType_CONV_TYPE_P2P,
		TargetUuid:  "user_bbb",
		ClientMsgId: "cmid-1",
		MsgType:     1,
		Content:     `{"text":"hello"}`,
	}
}

func TestCreateMessage_Success(t *testing.T) {
	repo := &mockRepo{}
	svc := NewService(repo)

	result, err := svc.CreateMessage(context.Background(), newP2PReq())
	require.NoError(t, err)
	assert.False(t, result.IsIdempotent)
	assert.Equal(t, "p2p-user_aaa-user_bbb", result.Msg.ConvId)
	assert.Equal(t, int64(1), result.Msg.Seq)
	assert.Equal(t, int16(1), result.Msg.MsgType)
}

func TestCreateMessage_IdempotentHit(t *testing.T) {
	cached := &model.Message{MsgId: "cached-id", Seq: 5}
	repo := &mockRepo{
		tryAcquireFn: func(_ context.Context, _, _, _ string) (*model.Message, error) {
			return cached, nil
		},
	}
	svc := NewService(repo)

	result, err := svc.CreateMessage(context.Background(), newP2PReq())
	require.NoError(t, err)
	assert.True(t, result.IsIdempotent)
	assert.Equal(t, "cached-id", result.Msg.MsgId)
}

func TestCreateMessage_IdempotentProcessing(t *testing.T) {
	repo := &mockRepo{
		tryAcquireFn: func(_ context.Context, _, _, _ string) (*model.Message, error) {
			return nil, ErrIdempotentProcessing
		},
	}
	svc := NewService(repo)

	_, err := svc.CreateMessage(context.Background(), newP2PReq())
	assert.ErrorIs(t, err, ErrIdempotentProcessing)
}

func TestCreateMessage_UnsupportedMsgType(t *testing.T) {
	repo := &mockRepo{}
	svc := NewService(repo)
	req := newP2PReq()
	req.MsgType = 999

	_, err := svc.CreateMessage(context.Background(), req)
	assert.ErrorIs(t, err, ErrUnsupportedMsgType)
}

func TestCreateMessage_DBDuplicate_Fallback(t *testing.T) {
	existing := &model.Message{MsgId: "existing-id", Seq: 3}
	repo := &mockRepo{
		createFn: func(_ context.Context, _ *model.Message) error {
			return ErrDuplicateMessage
		},
		getByDuplicateKeyFn: func(_ context.Context, _, _, _ string) (*model.Message, error) {
			return existing, nil
		},
	}
	svc := NewService(repo)

	result, err := svc.CreateMessage(context.Background(), newP2PReq())
	require.NoError(t, err)
	assert.True(t, result.IsIdempotent)
	assert.Equal(t, "existing-id", result.Msg.MsgId)
}

func TestCreateMessage_GroupConvId(t *testing.T) {
	repo := &mockRepo{}
	svc := NewService(repo)
	req := newP2PReq()
	req.ConvType = pb.ConvType_CONV_TYPE_GROUP
	req.TargetUuid = "group-uuid-123"

	result, err := svc.CreateMessage(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "group-uuid-123", result.Msg.ConvId)
}

// ==================== RecallMessage ====================

func TestRecallMessage_SelfRecall_Success(t *testing.T) {
	now := time.Now()
	repo := &mockRepo{
		getByIdFn: func(_ context.Context, _, _ string) (*model.Message, error) {
			return &model.Message{MsgId: "m1", FromUuid: "userA", Status: 0, SendTime: now}, nil
		},
	}
	svc := NewService(repo, Config{RecallWindow: 2 * time.Minute})

	msg, err := svc.RecallMessage(context.Background(), "p2p-userA-userB", "m1", "userA")
	require.NoError(t, err)
	assert.Equal(t, "m1", msg.MsgId)
}

func TestRecallMessage_AlreadyRecalled(t *testing.T) {
	repo := &mockRepo{
		getByIdFn: func(_ context.Context, _, _ string) (*model.Message, error) {
			return &model.Message{Status: 1}, nil
		},
	}
	svc := NewService(repo)

	_, err := svc.RecallMessage(context.Background(), "p2p-a-b", "m1", "a")
	assert.ErrorIs(t, err, ErrMessageAlreadyRecalled)
}

func TestRecallMessage_P2P_OtherUserDenied(t *testing.T) {
	repo := &mockRepo{
		getByIdFn: func(_ context.Context, _, _ string) (*model.Message, error) {
			return &model.Message{FromUuid: "userA", Status: 0, SendTime: time.Now()}, nil
		},
	}
	svc := NewService(repo)

	_, err := svc.RecallMessage(context.Background(), "p2p-userA-userB", "m1", "userB")
	assert.ErrorIs(t, err, ErrRecallNoPermission)
}

func TestRecallMessage_SelfRecall_Timeout(t *testing.T) {
	old := time.Now().Add(-10 * time.Minute)
	repo := &mockRepo{
		getByIdFn: func(_ context.Context, _, _ string) (*model.Message, error) {
			return &model.Message{FromUuid: "u", Status: 0, SendTime: old}, nil
		},
	}
	svc := NewService(repo, Config{RecallWindow: 2 * time.Minute})

	_, err := svc.RecallMessage(context.Background(), "p2p-u-v", "m1", "u")
	assert.ErrorIs(t, err, ErrRecallTimeout)
}

func TestRecallMessage_GroupAdmin_NoTimeLimit(t *testing.T) {
	old := time.Now().Add(-1 * time.Hour)
	repo := &mockRepo{
		getByIdFn: func(_ context.Context, _, _ string) (*model.Message, error) {
			return &model.Message{FromUuid: "member", Status: 0, SendTime: old}, nil
		},
	}
	svc := NewService(repo, Config{RecallWindow: 2 * time.Minute})
	svc.SetGroupRoleQuerier(&mockRoleQuerier{
		roleFn: func(_ context.Context, _, _ string) (int8, error) { return 2, nil },
	})

	msg, err := svc.RecallMessage(context.Background(), "group-uuid", "m1", "admin")
	require.NoError(t, err)
	assert.Equal(t, "member", msg.FromUuid)
}

func TestRecallMessage_GroupNormalMember_Denied(t *testing.T) {
	repo := &mockRepo{
		getByIdFn: func(_ context.Context, _, _ string) (*model.Message, error) {
			return &model.Message{FromUuid: "member1", Status: 0, SendTime: time.Now()}, nil
		},
	}
	svc := NewService(repo)
	svc.SetGroupRoleQuerier(&mockRoleQuerier{
		roleFn: func(_ context.Context, _, _ string) (int8, error) { return 0, nil },
	})

	_, err := svc.RecallMessage(context.Background(), "group-uuid", "m1", "member2")
	assert.ErrorIs(t, err, ErrRecallNoPermission)
}

func TestRecallMessage_GroupRoleQueryError(t *testing.T) {
	repo := &mockRepo{
		getByIdFn: func(_ context.Context, _, _ string) (*model.Message, error) {
			return &model.Message{FromUuid: "member", Status: 0, SendTime: time.Now()}, nil
		},
	}
	svc := NewService(repo)
	svc.SetGroupRoleQuerier(&mockRoleQuerier{
		roleFn: func(_ context.Context, _, _ string) (int8, error) {
			return -1, errors.New("rpc timeout")
		},
	})

	_, err := svc.RecallMessage(context.Background(), "group-uuid", "m1", "admin")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "查询群角色失败")
}

// ==================== PullMessages ====================

func TestPullMessages_HasMore(t *testing.T) {
	msgs := make([]*model.Message, 3)
	for i := range msgs {
		msgs[i] = &model.Message{MsgId: "m", Seq: int64(i + 1), SendTime: time.Now()}
	}
	repo := &mockRepo{
		getBySeqRangeFn: func(_ context.Context, _ string, _ int64, _ int, limit int, _ int64) ([]*model.Message, error) {
			if limit >= 3 {
				return msgs, nil
			}
			return msgs[:limit], nil
		},
	}
	svc := NewService(repo)

	items, hasMore, err := svc.PullMessages(context.Background(), "conv1", 0, DirectionForward, 2, 0)
	require.NoError(t, err)
	assert.True(t, hasMore)
	assert.Len(t, items, 2)
}

func TestPullMessages_DefaultLimit(t *testing.T) {
	var capturedLimit int
	repo := &mockRepo{
		getBySeqRangeFn: func(_ context.Context, _ string, _ int64, _ int, limit int, _ int64) ([]*model.Message, error) {
			capturedLimit = limit
			return nil, nil
		},
	}
	svc := NewService(repo)

	_, _, _ = svc.PullMessages(context.Background(), "conv1", 0, DirectionForward, 0, 0)
	assert.Equal(t, 51, capturedLimit)
}

// ==================== computeConvId ====================

func TestComputeConvId_P2P_Sorted(t *testing.T) {
	id1 := computeConvId(pb.ConvType_CONV_TYPE_P2P, "zzz", "aaa")
	id2 := computeConvId(pb.ConvType_CONV_TYPE_P2P, "aaa", "zzz")
	assert.Equal(t, id1, id2)
	assert.Equal(t, "p2p-aaa-zzz", id1)
}

func TestComputeConvId_Group(t *testing.T) {
	id := computeConvId(pb.ConvType_CONV_TYPE_GROUP, "sender", "group-xyz")
	assert.Equal(t, "group-xyz", id)
}
