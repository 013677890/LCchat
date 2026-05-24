package conversation

import (
	"context"
	"testing"
	"time"

	pb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/msgevent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func init() { logger.ReplaceGlobal(zap.NewNop()) }

// ==================== mock Repository ====================

type mockRepo struct {
	upsertFn            func(ctx context.Context, conv *model.Conversation, isSender bool) error
	upsertGroupConvFn   func(ctx context.Context, gc *model.GroupConversation) error
	batchInitFn         func(ctx context.Context, members []string, groupUUID string) error
	getByOwnerAndConvFn func(ctx context.Context, owner, convId string) (*model.Conversation, error)
	updateReadSeqFn     func(ctx context.Context, owner, convId string, readSeq int64) error
	updateReadOutboxFn  func(ctx context.Context, owner, convId string, readSeq int64, events []OutboxEvent) (*model.Conversation, error)
	deleteFn            func(ctx context.Context, owner, convId string) error
	updateSettingsFn    func(ctx context.Context, owner, convId string, mute *bool, pin *bool) error
	listP2PFn           func(ctx context.Context, owner string, since, cursorMs, cursorId int64, size int) ([]*model.Conversation, error)
	listGroupFn         func(ctx context.Context, owner string, since, cursorMs, cursorId int64, size int) ([]*model.Conversation, error)
	getGroupConvFn      func(ctx context.Context, groupUuid string) (*model.GroupConversation, error)
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
func (m *mockRepo) UpdateReadSeqWithOutbox(ctx context.Context, owner, convId string, readSeq int64, events []OutboxEvent) (*model.Conversation, error) {
	if m.updateReadOutboxFn != nil {
		return m.updateReadOutboxFn(ctx, owner, convId, readSeq, events)
	}
	return &model.Conversation{}, nil
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
	var gotEvents []OutboxEvent
	repo := &mockRepo{
		updateReadOutboxFn: func(_ context.Context, owner, convId string, readSeq int64, events []OutboxEvent) (*model.Conversation, error) {
			assert.Equal(t, "user1", owner)
			assert.Equal(t, "group-1", convId)
			assert.Equal(t, int64(50), readSeq)
			gotEvents = events
			return &model.Conversation{UnreadCount: 3}, nil
		},
	}
	svc := NewService(repo)
	ctx := ctxmeta.WithDeviceID(context.Background(), "dev-1")

	unread, err := svc.MarkRead(ctx, "user1", "group-1", 50)
	require.NoError(t, err)
	assert.Equal(t, int32(3), unread)
	require.Len(t, gotEvents, 1)
	assertMarkReadEvent(t, gotEvents[0], "group-1", "MSG_MARK_READ", "user1", "dev-1", pb.ConvType_CONV_TYPE_GROUP, 50)
}

func TestMarkRead_P2P_WritesReadReceiptOutbox(t *testing.T) {
	var gotEvents []OutboxEvent
	repo := &mockRepo{
		updateReadOutboxFn: func(_ context.Context, owner, convId string, readSeq int64, events []OutboxEvent) (*model.Conversation, error) {
			assert.Equal(t, "reader", owner)
			assert.Equal(t, "p2p-peer-reader", convId)
			assert.Equal(t, int64(88), readSeq)
			gotEvents = events
			return &model.Conversation{UnreadCount: 0}, nil
		},
	}
	svc := NewService(repo)

	unread, err := svc.MarkRead(context.Background(), "reader", "p2p-peer-reader", 88)
	require.NoError(t, err)
	assert.Equal(t, int32(0), unread)
	require.Len(t, gotEvents, 2)
	assertMarkReadEvent(t, gotEvents[0], "p2p-peer-reader", "MSG_MARK_READ", "reader", "", pb.ConvType_CONV_TYPE_P2P, 88)
	assertMarkReadEvent(t, gotEvents[1], "p2p-peer-reader", "MSG_READ_RECEIPT", "peer", "", pb.ConvType_CONV_TYPE_P2P, 88)
}

func assertMarkReadEvent(t *testing.T, event OutboxEvent, convId, pushType, receiverUUID, deviceID string, convType pb.ConvType, readSeq int64) {
	t.Helper()
	assert.Equal(t, msgevent.EventTypeMsgPush, event.EventType)
	assert.Equal(t, convId, event.EntityID)

	pushEvent, err := msgevent.DecodeMsgPush([]byte(event.Payload))
	require.NoError(t, err)
	assert.NotEmpty(t, pushEvent.GetEventId())
	assert.Equal(t, pushType, pushEvent.GetType())
	assert.Equal(t, receiverUUID, pushEvent.GetReceiverUuid())
	assert.Equal(t, deviceID, pushEvent.GetDeviceId())
	assert.Equal(t, convType, pushEvent.GetConvType())

	var notice pb.MarkReadNotice
	require.NoError(t, proto.Unmarshal(pushEvent.GetData(), &notice))
	assert.Equal(t, convId, notice.GetConvId())
	assert.Equal(t, readSeq, notice.GetReadSeq())
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
