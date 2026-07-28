package usecase

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/async"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestExecuteBatchSyncMessagesPreservesOrderAndPartialFailures(t *testing.T) {
	var mu sync.Mutex
	observedLimits := make(map[string]int)

	resp, err := executeBatchSyncMessages(
		context.Background(),
		&pb.BatchSyncMessagesRequest{
			Conversations: []*pb.ConversationSyncCursor{
				{ConvId: "conv-ok", AfterSeq: 10},
				{ConvId: "conv-missing", AfterSeq: 20, Limit: 3},
				{ConvId: "conv-empty", AfterSeq: 30, Limit: 2},
			},
		},
		func(_ context.Context, convID string, _ int64, limit int) (*pb.PullMessagesResponse, error) {
			mu.Lock()
			observedLimits[convID] = limit
			mu.Unlock()

			switch convID {
			case "conv-ok":
				return &pb.PullMessagesResponse{
					Messages: []*pb.MsgItem{{Seq: 11}, {Seq: 12}},
					HasMore:  true,
					MaxSeq:   15,
				}, nil
			case "conv-missing":
				return nil, apperr.New(consts.CodeConversationNotFound)
			case "conv-empty":
				return &pb.PullMessagesResponse{MaxSeq: 30}, nil
			default:
				return nil, fmt.Errorf("unexpected conversation %q", convID)
			}
		},
	)

	require.NoError(t, err)
	require.Len(t, resp.Results, 3)

	// 即使各 goroutine 的完成顺序不可预测，响应仍必须和请求顺序严格对齐。
	okResult := resp.Results[0]
	assert.Equal(t, "conv-ok", okResult.ConvId)
	assert.Equal(t, int64(12), okResult.NextSeq)
	assert.Equal(t, int64(15), okResult.MaxSeq)
	assert.True(t, okResult.HasMore)
	assert.Zero(t, okResult.ErrorCode)

	missingResult := resp.Results[1]
	assert.Equal(t, "conv-missing", missingResult.ConvId)
	assert.Empty(t, missingResult.Messages)
	assert.Equal(t, int64(20), missingResult.NextSeq)
	assert.Equal(t, int32(consts.CodeConversationNotFound), missingResult.ErrorCode)

	emptyResult := resp.Results[2]
	assert.Equal(t, "conv-empty", emptyResult.ConvId)
	assert.Equal(t, int64(30), emptyResult.NextSeq)
	assert.Equal(t, int64(30), emptyResult.MaxSeq)
	assert.Zero(t, emptyResult.ErrorCode)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, batchSyncDefaultLimit, observedLimits["conv-ok"])
	assert.Equal(t, 3, observedLimits["conv-missing"])
	assert.Equal(t, 2, observedLimits["conv-empty"])
}

func TestExecuteBatchSyncMessagesRejectsInvalidBatchBeforeReading(t *testing.T) {
	tooManyRequestedMessages := make([]*pb.ConversationSyncCursor, 0, 11)
	for index := 0; index < 11; index++ {
		tooManyRequestedMessages = append(tooManyRequestedMessages, &pb.ConversationSyncCursor{
			ConvId: fmt.Sprintf("conv-%d", index),
			Limit:  batchSyncMaxLimit,
		})
	}

	tests := []struct {
		name string
		req  *pb.BatchSyncMessagesRequest
	}{
		{name: "nil_request"},
		{name: "empty_conversations", req: &pb.BatchSyncMessagesRequest{}},
		{
			name: "nil_conversation",
			req:  &pb.BatchSyncMessagesRequest{Conversations: []*pb.ConversationSyncCursor{nil}},
		},
		{
			name: "blank_conversation_id",
			req: &pb.BatchSyncMessagesRequest{Conversations: []*pb.ConversationSyncCursor{
				{ConvId: "  "},
			}},
		},
		{
			name: "negative_after_seq",
			req: &pb.BatchSyncMessagesRequest{Conversations: []*pb.ConversationSyncCursor{
				{ConvId: "conv-1", AfterSeq: -1},
			}},
		},
		{
			name: "limit_above_per_conversation_cap",
			req: &pb.BatchSyncMessagesRequest{Conversations: []*pb.ConversationSyncCursor{
				{ConvId: "conv-1", Limit: batchSyncMaxLimit + 1},
			}},
		},
		{
			name: "duplicate_conversation_id",
			req: &pb.BatchSyncMessagesRequest{Conversations: []*pb.ConversationSyncCursor{
				{ConvId: "conv-1"},
				{ConvId: "conv-1"},
			}},
		},
		{
			name: "total_limit_above_batch_cap",
			req:  &pb.BatchSyncMessagesRequest{Conversations: tooManyRequestedMessages},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called atomic.Bool
			resp, err := executeBatchSyncMessages(
				context.Background(),
				tt.req,
				func(context.Context, string, int64, int) (*pb.PullMessagesResponse, error) {
					called.Store(true)
					return &pb.PullMessagesResponse{}, nil
				},
			)

			require.Nil(t, resp)
			require.Error(t, err)
			assert.Equal(t, consts.CodeParamError, apperr.Code(err))
			assert.False(t, called.Load(), "非法批次必须在产生任何读取前被拒绝")
		})
	}
}

func TestPrepareBatchSyncRequestAcceptsMaximumDefaultBatch(t *testing.T) {
	conversations := make([]*pb.ConversationSyncCursor, 0, batchSyncMaxConversations)
	for index := 0; index < batchSyncMaxConversations; index++ {
		conversations = append(conversations, &pb.ConversationSyncCursor{
			ConvId: fmt.Sprintf("conv-%02d", index),
		})
	}

	prepared, err := prepareBatchSyncRequest(&pb.BatchSyncMessagesRequest{
		Conversations: conversations,
	})

	require.NoError(t, err)
	require.Len(t, prepared, batchSyncMaxConversations)
	for _, item := range prepared {
		assert.Equal(t, batchSyncDefaultLimit, item.limit)
	}
}

func TestExecuteBatchSyncMessagesBoundsConcurrentReads(t *testing.T) {
	const conversationCount = 20

	conversations := make([]*pb.ConversationSyncCursor, 0, conversationCount)
	for index := 0; index < conversationCount; index++ {
		conversations = append(conversations, &pb.ConversationSyncCursor{
			ConvId: fmt.Sprintf("conv-%02d", index),
			Limit:  1,
		})
	}

	started := make(chan struct{}, conversationCount)
	release := make(chan struct{})
	var active atomic.Int32
	var maximum atomic.Int32

	done := make(chan error, 1)
	go func() {
		_, err := executeBatchSyncMessages(
			context.Background(),
			&pb.BatchSyncMessagesRequest{Conversations: conversations},
			func(context.Context, string, int64, int) (*pb.PullMessagesResponse, error) {
				current := active.Add(1)
				for {
					previousMaximum := maximum.Load()
					if current <= previousMaximum || maximum.CompareAndSwap(previousMaximum, current) {
						break
					}
				}
				started <- struct{}{}
				<-release
				active.Add(-1)
				return &pb.PullMessagesResponse{}, nil
			},
		)
		done <- err
	}()

	// 前 8 个读取会占满并发槽；第 9 个必须等任意槽释放后才可启动。
	for index := 0; index < batchSyncConcurrency; index++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("等待并发读取启动超时")
		}
	}
	select {
	case <-started:
		t.Fatal("检测到超过并发上限的读取")
	case <-time.After(30 * time.Millisecond):
	}

	close(release)
	require.NoError(t, <-done)
	assert.LessOrEqual(t, maximum.Load(), int32(batchSyncConcurrency))
}

func TestExecuteBatchSyncMessagesPropagatesRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, err := executeBatchSyncMessages(
		ctx,
		&pb.BatchSyncMessagesRequest{
			Conversations: []*pb.ConversationSyncCursor{{ConvId: "conv-1"}},
		},
		func(ctx context.Context, _ string, _ int64, _ int) (*pb.PullMessagesResponse, error) {
			return nil, ctx.Err()
		},
	)

	require.Nil(t, resp)
	require.ErrorIs(t, err, context.Canceled)
}

func TestExecuteBatchSyncMessagesRecoversTaskPanic(t *testing.T) {
	resp, err := executeBatchSyncMessages(
		context.Background(),
		&pb.BatchSyncMessagesRequest{
			Conversations: []*pb.ConversationSyncCursor{{ConvId: "conv-1"}},
		},
		func(context.Context, string, int64, int) (*pb.PullMessagesResponse, error) {
			panic("unexpected repository panic")
		},
	)

	require.Nil(t, resp)
	require.Error(t, err)
	assert.ErrorIs(t, err, async.ErrTaskPanic)
}

func TestEnforceBatchSyncMessageBudgetKeepsPerConversationPrefix(t *testing.T) {
	firstMessage := &pb.MsgItem{Seq: 11, Content: strings.Repeat("a", 64)}
	secondMessage := &pb.MsgItem{Seq: 12, Content: strings.Repeat("b", 64)}
	laterConversationMessage := &pb.MsgItem{Seq: 21, Content: "small"}
	results := []*pb.ConversationSyncResult{
		{
			ConvId:   "conv-1",
			Messages: []*pb.MsgItem{firstMessage, secondMessage},
			NextSeq:  12,
		},
		{
			ConvId:   "conv-2",
			Messages: []*pb.MsgItem{laterConversationMessage},
			NextSeq:  21,
		},
	}
	prepared := []preparedConversationSync{
		{convID: "conv-1", afterSeq: 10},
		{convID: "conv-2", afterSeq: 20},
	}

	// 预算只够第一条消息：conv-1 必须截成 seq=11 的前缀并标记 hasMore；
	// conv-2 不能错误推进 nextSeq，因为它的消息没有实际返回。
	enforceBatchSyncMessageBudget(prepared, results, proto.Size(firstMessage)+16)

	require.Len(t, results[0].Messages, 1)
	assert.Equal(t, int64(11), results[0].NextSeq)
	assert.True(t, results[0].HasMore)
	assert.Empty(t, results[1].Messages)
	assert.Equal(t, int64(20), results[1].NextSeq)
	assert.True(t, results[1].HasMore)
}
