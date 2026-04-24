package conversation

import (
	"context"
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
	upsertFn             func(ctx context.Context, conv *model.Conversation, isSender bool) error
	upsertGroupConvFn    func(ctx context.Context, gc *model.GroupConversation) error
	batchInitFn          func(ctx context.Context, members []string, groupUUID string) error
	getByOwnerAndConvFn  func(ctx context.Context, owner, convId string) (*model.Conversation, error)
	updateReadSeqFn      func(ctx context.Context, owner, convId string, readSeq int64) error
	deleteFn             func(ctx context.Context, owner, convId string) error
	updateSettingsFn     func(ctx context.Context, owner, convId string, mute *bool, pin *bool) error
	listP2PFn            func(ctx context.Context, owner string, since, cursorMs, cursorId int64, size int) ([]*model.Conversation, error)
	listGroupFn          func(ctx context.Context, owner string, since, cursorMs, cursorId int64, size int) ([]*model.Conversation, error)
	getGroupConvFn       func(ctx context.Context, groupUuid string) (*model.GroupConversation, error)
}

func (m *mockRepo) Upsert(ctx context.Context, conv *model.Conversation, isSender bool) error {
	if m.upsertFn != nil {
		return m.upsertFn(ctx, conv, isSender)
	}
	return nil
}
func (m *mockRepo) UpsertGroupConv(ctx context.Context, gc *model.GroupConversation) error {
	if m.upsertGroupConvFn != nil {
		return m.upsertGroupConvFn(ctx, gc)
	}
	return nil
}
func (m *mockRepo) BatchInitGroupMemberConv(ctx context.Context, members []string, groupUUID string) error {
	if m.batchInitFn != nil {
		return m.batchInitFn(ctx, members, groupUUID)
	}
	return nil
}
func (m *mockRepo) GetByOwnerAndConvId(ctx context.Context, owner, convId string) (*model.Conversation, error) {
	if m.getByOwnerAndConvFn != nil {
		return m.getByOwnerAndConvFn(ctx, owner, convId)
	}
	return nil, ErrConversationNotFound
}
func (m *mockRepo) UpdateReadSeq(ctx context.Context, owner, convId string, readSeq int64) error {
	if m.updateReadSeqFn != nil {
		return m.updateReadSeqFn(ctx, owner, convId, readSeq)
	}
	return nil
}
func (m *mockRepo) Delete(ctx context.Context, owner, convId string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, owner, convId)
	}
	return nil
}
func (m *mockRepo) UpdateSettings(ctx context.Context, owner, convId string, mute *bool, pin *bool) error {
	if m.updateSettingsFn != nil {
		return m.updateSettingsFn(ctx, owner, convId, mute, pin)
	}
	return nil
}
func (m *mockRepo) ListP2P(ctx context.Context, owner string, since, cursorMs, cursorId int64, size int) ([]*model.Conversation, error) {
	if m.listP2PFn != nil {
		return m.listP2PFn(ctx, owner, since, cursorMs, cursorId, size)
	}
	return nil, nil
}
func (m *mockRepo) ListGroup(ctx context.Context, owner string, since, cursorMs, cursorId int64, size int) ([]*model.Conversation, error) {
	if m.listGroupFn != nil {
		return m.listGroupFn(ctx, owner, since, cursorMs, cursorId, size)
	}
	return nil, nil
}
func (m *mockRepo) GetGroupConv(ctx context.Context, groupUuid string) (*model.GroupConversation, error) {
	if m.getGroupConvFn != nil {
		return m.getGroupConvFn(ctx, groupUuid)
	}
	return nil, ErrConversationNotFound
}

// ==================== UpsertForMessage ====================

func TestUpsertForMessage_Sender(t *testing.T) {
	var captured *model.Conversation
	var capturedIsSender bool

	repo := &mockRepo{
		upsertFn: func(_ context.Context, conv *model.Conversation, isSender bool) error {
			captured = conv
			capturedIsSender = isSender
			return nil
		},
	}
	svc := NewService(repo)

	now := time.Now()
	msg := &model.Message{
		ConvId: "p2p-a-b", MsgId: "m1", Seq: 5, FromUuid: "a",
		Content: `{"text":"hi"}`, MsgType: 1, SendTime: now,
	}
	err := svc.UpsertForMessage(context.Background(), "a", msg, pb.ConvType_CONV_TYPE_P2P, "b", true)
	require.NoError(t, err)

	assert.True(t, capturedIsSender)
	assert.Equal(t, int64(5), captured.MaxSeq)
	assert.Equal(t, int64(5), captured.ReadSeq)
	assert.Equal(t, 0, captured.UnreadCount)
}

func TestUpsertForMessage_Receiver(t *testing.T) {
	var captured *model.Conversation
	var capturedIsSender bool

	repo := &mockRepo{
		upsertFn: func(_ context.Context, conv *model.Conversation, isSender bool) error {
			captured = conv
			capturedIsSender = isSender
			return nil
		},
	}
	svc := NewService(repo)

	now := time.Now()
	msg := &model.Message{
		ConvId: "p2p-a-b", MsgId: "m1", Seq: 5, FromUuid: "a",
		Content: `{"text":"hi"}`, MsgType: 1, SendTime: now,
	}
	err := svc.UpsertForMessage(context.Background(), "b", msg, pb.ConvType_CONV_TYPE_P2P, "a", false)
	require.NoError(t, err)

	assert.False(t, capturedIsSender)
	assert.Equal(t, 1, captured.UnreadCount)
	assert.Equal(t, int64(0), captured.ReadSeq)
}

// ==================== UpsertGroupConv ====================

func TestUpsertGroupConv(t *testing.T) {
	var captured *model.GroupConversation
	repo := &mockRepo{
		upsertGroupConvFn: func(_ context.Context, gc *model.GroupConversation) error {
			captured = gc
			return nil
		},
	}
	svc := NewService(repo)

	now := time.Now()
	msg := &model.Message{
		ConvId: "group-xyz", MsgId: "m2", Seq: 10, FromUuid: "sender",
		Content: `{"text":"hello group"}`, MsgType: 1, SendTime: now,
	}
	err := svc.UpsertGroupConv(context.Background(), msg)
	require.NoError(t, err)

	assert.Equal(t, "group-xyz", captured.GroupUuid)
	assert.Equal(t, int64(10), captured.MaxSeq)
	assert.Equal(t, "m2", captured.LastMsgId)
}

// ==================== EnsureGroupMembersConv ====================

func TestEnsureGroupMembersConv(t *testing.T) {
	var capturedMembers []string
	var capturedGroup string

	repo := &mockRepo{
		batchInitFn: func(_ context.Context, members []string, groupUUID string) error {
			capturedMembers = members
			capturedGroup = groupUUID
			return nil
		},
	}
	svc := NewService(repo)

	err := svc.EnsureGroupMembersConv(context.Background(), []string{"u1", "u2", "u3"}, "g-100")
	require.NoError(t, err)
	assert.Equal(t, []string{"u1", "u2", "u3"}, capturedMembers)
	assert.Equal(t, "g-100", capturedGroup)
}

// ==================== MarkRead ====================

func TestMarkRead_ReturnsUnread(t *testing.T) {
	repo := &mockRepo{
		updateReadSeqFn: func(_ context.Context, _, _ string, _ int64) error { return nil },
		getByOwnerAndConvFn: func(_ context.Context, _, _ string) (*model.Conversation, error) {
			return &model.Conversation{UnreadCount: 3}, nil
		},
	}
	svc := NewService(repo)

	unread, err := svc.MarkRead(context.Background(), "user1", "conv1", 50)
	require.NoError(t, err)
	assert.Equal(t, int32(3), unread)
}

// ==================== buildPreviewText ====================

func TestBuildPreviewText_Image(t *testing.T) {
	msg := &model.Message{MsgType: int16(model.MsgTypeImage)}
	assert.Equal(t, "[图片]", buildPreviewText(msg))
}

func TestBuildPreviewText_Text(t *testing.T) {
	msg := &model.Message{MsgType: int16(model.MsgTypeText), Content: `{"text":"hello world"}`}
	assert.Equal(t, "hello world", buildPreviewText(msg))
}

func TestBuildPreviewText_System(t *testing.T) {
	msg := &model.Message{MsgType: int16(model.MsgTypeSystem)}
	assert.Equal(t, "[系统消息]", buildPreviewText(msg))
}

func TestBuildPreviewText_LongTextTruncated(t *testing.T) {
	long := `{"text":"这是一段超过二十个字符的长文本内容用来测试截断功能是否正确工作"}`
	msg := &model.Message{MsgType: int16(model.MsgTypeText), Content: long}
	preview := buildPreviewText(msg)
	runes := []rune(preview)
	assert.LessOrEqual(t, len(runes), previewMaxRunes+3) // +3 for "..."
}
