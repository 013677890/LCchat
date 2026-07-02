package usecase

import (
	"context"
	"testing"
	"time"

	convsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/conversation"
	msgsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/message"
	pb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type workflowMessageRepo struct {
	cached *model.Message
}

func (r *workflowMessageRepo) AllocSeq(context.Context, string) (int64, error) { return 1, nil }
func (r *workflowMessageRepo) RepairSeq(context.Context, string) error         { return nil }
func (r *workflowMessageRepo) TryAcquireIdempotent(context.Context, string, string, string) (*msgsvc.IdempotentAcquireResult, error) {
	return &msgsvc.IdempotentAcquireResult{CachedMsg: r.cached}, nil
}
func (r *workflowMessageRepo) CompleteIdempotentResult(context.Context, string, string, string, string, *model.Message) error {
	return nil
}
func (r *workflowMessageRepo) ReleaseIdempotentProcessing(context.Context, string, string, string, string) error {
	return nil
}
func (r *workflowMessageRepo) SetIdempotentResult(context.Context, string, string, string, *model.Message) error {
	return nil
}
func (r *workflowMessageRepo) Create(context.Context, *model.Message) error { return nil }
func (r *workflowMessageRepo) CreateWithOutbox(context.Context, *model.Message, msgsvc.OutboxEvent) error {
	return nil
}
func (r *workflowMessageRepo) GetByDuplicateKey(context.Context, string, string, string) (*model.Message, error) {
	return nil, msgsvc.ErrMessageNotFound
}
func (r *workflowMessageRepo) GetBySeqRange(context.Context, string, int64, int, int, int64) ([]*model.Message, error) {
	return nil, nil
}
func (r *workflowMessageRepo) GetByIds(context.Context, string, []string) ([]*model.Message, error) {
	return nil, nil
}
func (r *workflowMessageRepo) GetById(context.Context, string, string) (*model.Message, error) {
	return nil, msgsvc.ErrMessageNotFound
}
func (r *workflowMessageRepo) GetMaxSeq(context.Context, string) (int64, error) { return 0, nil }
func (r *workflowMessageRepo) UpdateStatus(context.Context, string, string, int8, string) error {
	return nil
}
func (r *workflowMessageRepo) UpdateStatusWithOutbox(context.Context, string, string, int8, string, msgsvc.OutboxEvent) error {
	return nil
}

type workflowRepairCall struct {
	owner    string
	target   string
	isSender bool
	conv     *model.Conversation
}

type workflowConversationRepo struct {
	upsertCalls int
	repairs     []workflowRepairCall
}

func (r *workflowConversationRepo) GetByOwnerAndConvId(context.Context, string, string) (*model.Conversation, error) {
	return nil, convsvc.ErrConversationNotFound
}
func (r *workflowConversationRepo) ListP2P(context.Context, string, int64, int64, int64, int) ([]*model.Conversation, error) {
	return nil, nil
}
func (r *workflowConversationRepo) ListGroup(context.Context, string, int64, int64, int64, int) ([]*model.Conversation, error) {
	return nil, nil
}
func (r *workflowConversationRepo) Upsert(context.Context, *model.Conversation, bool) error {
	r.upsertCalls++
	return nil
}
func (r *workflowConversationRepo) RepairForMessage(_ context.Context, conv *model.Conversation, isSender bool) error {
	r.repairs = append(r.repairs, workflowRepairCall{
		owner:    conv.OwnerUuid,
		target:   conv.TargetUuid,
		isSender: isSender,
		conv:     conv,
	})
	return nil
}
func (r *workflowConversationRepo) UpdateReadSeq(context.Context, string, string, int64) error {
	return nil
}
func (r *workflowConversationRepo) UpdateReadSeqWithOutbox(context.Context, string, string, int64, []convsvc.OutboxEvent) (*model.Conversation, error) {
	return &model.Conversation{}, nil
}
func (r *workflowConversationRepo) UpsertGroupReadSeqWithOutbox(context.Context, string, string, int64, []convsvc.OutboxEvent) (*model.Conversation, error) {
	return &model.Conversation{}, nil
}
func (r *workflowConversationRepo) Delete(context.Context, string, string) error { return nil }
func (r *workflowConversationRepo) UpdateSettings(context.Context, string, string, *bool, *bool) error {
	return nil
}
func (r *workflowConversationRepo) BatchInitGroupMemberConv(context.Context, []string, string) error {
	return nil
}
func (r *workflowConversationRepo) UpsertGroupConv(context.Context, *model.GroupConversation) error {
	return nil
}
func (r *workflowConversationRepo) GetGroupConv(context.Context, string) (*model.GroupConversation, error) {
	return nil, convsvc.ErrConversationNotFound
}

func TestSendMessageWorkflowIdempotentHitRepairsP2PConversationProjection(t *testing.T) {
	now := time.Now()
	cached := &model.Message{
		ConvId:      "p2p-a-b",
		Seq:         9,
		MsgId:       "msg-9",
		ClientMsgId: "client-9",
		FromUuid:    "a",
		DeviceId:    "dev-1",
		MsgType:     int16(model.MsgTypeText),
		Content:     `{"text":"hello"}`,
		SendTime:    now,
	}
	msgService := msgsvc.NewService(&workflowMessageRepo{cached: cached})
	convRepo := &workflowConversationRepo{}
	convService := convsvc.NewService(convRepo)
	workflow := NewSendMessageWorkflow(msgService, convService, nil, nil)

	resp, err := workflow.Execute(context.Background(), "a", "dev-1", &pb.SendMessageRequest{
		TargetUuid:  "b",
		ConvType:    pb.ConvType_CONV_TYPE_P2P,
		ClientMsgId: "client-9",
		MsgType:     int32(model.MsgTypeText),
		Content:     `{"text":"hello"}`,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "msg-9", resp.GetMsgId())
	assert.Equal(t, int64(9), resp.GetSeq())
	assert.Equal(t, 0, convRepo.upsertCalls)
	require.Len(t, convRepo.repairs, 2)

	assert.Equal(t, "a", convRepo.repairs[0].owner)
	assert.Equal(t, "b", convRepo.repairs[0].target)
	assert.True(t, convRepo.repairs[0].isSender)
	assert.Equal(t, int64(9), convRepo.repairs[0].conv.ReadSeq)
	assert.Equal(t, 0, convRepo.repairs[0].conv.UnreadCount)

	assert.Equal(t, "b", convRepo.repairs[1].owner)
	assert.Equal(t, "a", convRepo.repairs[1].target)
	assert.False(t, convRepo.repairs[1].isSender)
	assert.Equal(t, int64(9), convRepo.repairs[1].conv.MaxSeq)
	assert.Equal(t, 9, convRepo.repairs[1].conv.UnreadCount)
}
