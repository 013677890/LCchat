package usecase

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	convsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/conversation"
	msgsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/message"
	pb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/logger"

	"google.golang.org/protobuf/proto"
)

const (
	// batchSyncDefaultLimit 控制客户端未显式指定 limit 时，每个会话首轮补拉的大小。
	// 登录恢复通常只有少量离线消息，默认 10 能避免 50 个会话同时返回大量历史数据。
	batchSyncDefaultLimit = 10
	// batchSyncMaxLimit 与 Proto/HTTP DTO 的单会话上限保持一致。
	batchSyncMaxLimit = 50
	// batchSyncMaxConversations 限制一次请求中的会话数量，避免单请求无限读扩散。
	batchSyncMaxConversations = 50
	// batchSyncMaxTotalLimit 限制所有会话有效 limit 之和，从响应体大小和数据库读量两端兜底。
	batchSyncMaxTotalLimit = 500
	// batchSyncConcurrency 是 msg-service 内部并发查询上限。批量接口消除了客户端 N 次
	// HTTP/gRPC 往返，但不能把 N 个会话无界地同时压到 MySQL 和 Redis。
	batchSyncConcurrency = 8
	// batchSyncMaxMessagePayloadBytes 为返回消息本体预留 3 MiB 预算。Gateway 的 gRPC
	// 接收上限为 4 MiB，额外约 1 MiB 留给结果元数据、字段 tag 和长度前缀。
	batchSyncMaxMessagePayloadBytes = 3 * 1024 * 1024
)

// MessageReadWorkflow 统一编排需要同时访问 conversation 与 message 领域的只读用例。
//
// 读取消息前必须先由 conversation.Service 裁决当前用户是否可读，并取得该用户视角的
// clear_seq；通过后才允许 message.Service 查询消息事实。把这条边界集中在 Workflow，
// 可确保单会话拉取、批量同步和消息 ID 反查不会出现不同的越权或删除可见性语义。
type MessageReadWorkflow struct {
	msgService  *msgsvc.Service
	convService *convsvc.Service
}

// NewMessageReadWorkflow 创建消息读取协调用例。
func NewMessageReadWorkflow(
	msgService *msgsvc.Service,
	convService *convsvc.Service,
) *MessageReadWorkflow {
	return &MessageReadWorkflow{
		msgService:  msgService,
		convService: convService,
	}
}

// PullMessages 执行一个会话的权限裁决、清理位点过滤和消息拉取。
// ownerUUID 是 Handler 从可信 gRPC metadata 中解析出的登录主体，不从请求体接受。
func (w *MessageReadWorkflow) PullMessages(
	ctx context.Context,
	ownerUUID string,
	req *pb.PullMessagesRequest,
) (*pb.PullMessagesResponse, error) {
	if req == nil {
		return nil, apperr.New(consts.CodeParamError)
	}

	clearSeq, err := w.requireConversationAccess(ctx, ownerUUID, req.ConvId)
	if err != nil {
		return nil, err
	}

	direction := msgsvc.DirectionForward
	if req.Direction == pb.PullDirection_PULL_DIRECTION_BACKWARD {
		direction = msgsvc.DirectionBackward
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}

	maxSeq, err := w.msgService.GetMaxSeq(ctx, req.ConvId)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, consts.GetMessage(consts.CodeInternalError))
	}

	messages, hasMore, err := w.msgService.PullMessages(
		ctx,
		req.ConvId,
		req.AnchorSeq,
		direction,
		limit,
		clearSeq,
	)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, consts.GetMessage(consts.CodeInternalError))
	}

	return &pb.PullMessagesResponse{
		Messages: messages,
		HasMore:  hasMore,
		MaxSeq:   maxSeq,
	}, nil
}

// GetMessagesByIDs 在完成与拉取接口相同的读取权限裁决后，按消息 ID 批量反查。
func (w *MessageReadWorkflow) GetMessagesByIDs(
	ctx context.Context,
	ownerUUID string,
	req *pb.GetMessagesByIdsRequest,
) (*pb.GetMessagesByIdsResponse, error) {
	if req == nil {
		return nil, apperr.New(consts.CodeParamError)
	}
	if _, err := w.requireConversationAccess(ctx, ownerUUID, req.ConvId); err != nil {
		return nil, err
	}

	messages, err := w.msgService.GetMessagesByIds(ctx, req.ConvId, req.MsgIds)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, consts.GetMessage(consts.CodeInternalError))
	}
	return &pb.GetMessagesByIdsResponse{Messages: messages}, nil
}

// BatchSyncMessages 按客户端为每个会话保存的独立 seq 位点批量补拉消息。
//
// 失败边界分成两层：
//   - 请求级错误（参数非法、重复 conv_id、调用被取消）整体返回 gRPC 错误；
//   - 会话级错误（无读取权限、会话不存在、单次读取失败）写入对应 result.error_code，
//     其他会话继续执行。
//
// 这种语义允许客户端先提交所有本地位点，再只针对失败项或 has_more 项重试，不会因为
// 一个已退出的群聊阻断其余几十个正常会话的登录恢复。
func (w *MessageReadWorkflow) BatchSyncMessages(
	ctx context.Context,
	ownerUUID string,
	req *pb.BatchSyncMessagesRequest,
) (*pb.BatchSyncMessagesResponse, error) {
	return executeBatchSyncMessages(ctx, req, func(
		itemCtx context.Context,
		convID string,
		afterSeq int64,
		limit int,
	) (*pb.PullMessagesResponse, error) {
		resp, pullErr := w.PullMessages(itemCtx, ownerUUID, &pb.PullMessagesRequest{
			ConvId:    convID,
			AnchorSeq: afterSeq,
			Limit:     int32(limit),
			Direction: pb.PullDirection_PULL_DIRECTION_FORWARD,
		})
		// “会话不存在/无权读取”是客户端会话列表过期时的正常分支；内部读取失败才需要
		// 在服务端留下原始错误，返回给客户端的 result 只暴露稳定业务码。
		if pullErr != nil &&
			itemCtx.Err() == nil &&
			apperr.Code(pullErr) == consts.CodeInternalError {
			logger.Warn(itemCtx, "批量同步消息：单会话读取失败",
				logger.String("conv_id", convID),
				logger.ErrorField("error", pullErr),
			)
		}
		return resp, pullErr
	})
}

func (w *MessageReadWorkflow) requireConversationAccess(
	ctx context.Context,
	ownerUUID string,
	convID string,
) (int64, error) {
	clearSeq, err := w.convService.ResolveReadAccess(ctx, ownerUUID, convID)
	if err != nil {
		if errors.Is(err, convsvc.ErrConversationNotFound) {
			// 对不存在与无读取资格统一返回“会话不存在”，避免泄露其他用户的会话存在性。
			return 0, apperr.New(consts.CodeConversationNotFound)
		}
		return 0, apperr.Wrap(err, consts.CodeInternalError, consts.GetMessage(consts.CodeInternalError))
	}
	return clearSeq, nil
}

// batchConversationPuller 是单个会话的读取函数。生产代码会把它绑定到 PullMessages；
// 独立类型让批量编排可以在不启动 gRPC/MySQL 的情况下做确定性单元测试。
type batchConversationPuller func(
	ctx context.Context,
	convID string,
	afterSeq int64,
	limit int,
) (*pb.PullMessagesResponse, error)

type preparedConversationSync struct {
	convID   string
	afterSeq int64
	limit    int
}

// executeBatchSyncMessages 负责严格校验、并发编排和结果归位。
// results 预先按请求长度分配，每个 goroutine 只写自己的固定下标，因此不需要锁，
// 同时无论各会话查询完成顺序如何，响应顺序始终与请求一致。
func executeBatchSyncMessages(
	ctx context.Context,
	req *pb.BatchSyncMessagesRequest,
	puller batchConversationPuller,
) (*pb.BatchSyncMessagesResponse, error) {
	prepared, err := prepareBatchSyncRequest(req)
	if err != nil {
		return nil, err
	}
	if puller == nil {
		return nil, apperr.New(consts.CodeInternalError)
	}

	results := make([]*pb.ConversationSyncResult, len(prepared))
	group := async.NewGroup(ctx, 0)

	// 并发组负责请求取消、等待收敛和 panic 恢复；信号量只限制真正进入 Redis/MySQL
	// 读取区间的任务数。即使一次提交 50 个会话，也最多有 8 个同时访问存储。
	readSlots := make(chan struct{}, batchSyncConcurrency)

	for index, item := range prepared {
		index := index
		item := item

		if err := group.Go(func(groupCtx context.Context) error {
			select {
			case readSlots <- struct{}{}:
				defer func() { <-readSlots }()
			case <-groupCtx.Done():
				return groupCtx.Err()
			}

			resp, pullErr := puller(groupCtx, item.convID, item.afterSeq, item.limit)
			if pullErr != nil {
				// 父请求取消/超时属于整批调用的传输语义，不能伪装成一个普通的部分失败。
				// 其余业务/读取错误则保持在当前会话结果中，允许同批其他项成功。
				if ctxErr := groupCtx.Err(); ctxErr != nil {
					return ctxErr
				}
				results[index] = &pb.ConversationSyncResult{
					ConvId:    item.convID,
					NextSeq:   item.afterSeq,
					ErrorCode: int32(apperr.Code(pullErr)),
				}

				return nil
			}

			if resp == nil {
				// 防御下游违反 “response 与 error 不可同时为 nil” 的函数契约。
				results[index] = &pb.ConversationSyncResult{
					ConvId:    item.convID,
					NextSeq:   item.afterSeq,
					ErrorCode: int32(consts.CodeInternalError),
				}

				return nil
			}

			nextSeq := item.afterSeq

			// PullMessages 保证 messages 按 seq 升序。这里仍从尾部跳过潜在 nil 项，
			// 避免某个异常元素把可安全推进的 next_seq 重置为 0。
			for messageIndex := len(resp.Messages) - 1; messageIndex >= 0; messageIndex-- {
				if resp.Messages[messageIndex] != nil {
					nextSeq = resp.Messages[messageIndex].Seq
					break
				}
			}

			results[index] = &pb.ConversationSyncResult{
				ConvId:    item.convID,
				Messages:  resp.Messages,
				HasMore:   resp.HasMore,
				MaxSeq:    resp.MaxSeq,
				NextSeq:   nextSeq,
				ErrorCode: int32(consts.CodeSuccess),
			}

			return nil
		}); err != nil {
			// Go 可能在父请求恰好取消时拒绝新任务；返回前仍需等待此前已提交的任务
			// 收敛，避免它们在函数返回后继续写 results。
			_ = group.Wait()
			return nil, err
		}
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}
	enforceBatchSyncMessageBudget(prepared, results, batchSyncMaxMessagePayloadBytes)
	return &pb.BatchSyncMessagesResponse{Results: results}, nil
}

// enforceBatchSyncMessageBudget 对整批消息本体施加确定性的响应字节预算。
//
// 计数上限只能限制“消息条数”，不能限制 content 的实际字节数；若直接把最多 500 条
// 大消息交给 gRPC，可能超过 Gateway 的 4 MiB 接收上限，导致整批成功查询最终在传输层
// 失败。本函数按请求顺序保留每个会话的消息前缀，超出预算时截断并强制 has_more=true，
// next_seq 只推进到真正返回给客户端的最后一条消息。
//
// 预算按 proto.Size(message)+16 估算。16 字节覆盖 repeated message 的字段 tag、长度前缀
// 及外层结果长度变化；另有约 1 MiB 总安全余量，因此不会贴着 gRPC 4 MiB 上限发送。
func enforceBatchSyncMessageBudget(
	prepared []preparedConversationSync,
	results []*pb.ConversationSyncResult,
	maxMessageBytes int,
) {
	if maxMessageBytes <= 0 {
		return
	}

	remainingBytes := maxMessageBytes
	for index, result := range results {
		if result == nil || result.ErrorCode != int32(consts.CodeSuccess) || len(result.Messages) == 0 {
			continue
		}

		keepCount := len(result.Messages)
		for messageIndex, message := range result.Messages {
			messageBytes := 16
			if message != nil {
				messageBytes += proto.Size(message)
			}
			if messageBytes > remainingBytes {
				// 单个会话必须返回一个连续前缀；当前消息放不下时不能跳过它去返回
				// 同会话更大的 seq，否则客户端推进 next_seq 后会永久漏掉该消息。
				keepCount = messageIndex
				break
			}
			remainingBytes -= messageBytes
		}

		if keepCount == len(result.Messages) {
			continue
		}

		result.Messages = result.Messages[:keepCount]
		result.HasMore = true
		result.NextSeq = prepared[index].afterSeq
		for messageIndex := keepCount - 1; messageIndex >= 0; messageIndex-- {
			if result.Messages[messageIndex] != nil {
				result.NextSeq = result.Messages[messageIndex].Seq
				break
			}
		}
	}
}

// prepareBatchSyncRequest 在启动任何查询前完成全量校验，确保非法请求不会产生部分读负载。
// Proto 校验拦截器和 Gateway binding 是第一道防线；这里是 msg-service 的用例边界防线，
// 也覆盖内部调用方绕过 Gateway、或测试直接调用 Workflow 的场景。
func prepareBatchSyncRequest(req *pb.BatchSyncMessagesRequest) ([]preparedConversationSync, error) {
	if req == nil ||
		len(req.Conversations) == 0 ||
		len(req.Conversations) > batchSyncMaxConversations {
		return nil, apperr.New(consts.CodeParamError)
	}

	prepared := make([]preparedConversationSync, 0, len(req.Conversations))
	seenConversationIDs := make(map[string]struct{}, len(req.Conversations))
	totalLimit := 0

	for _, item := range req.Conversations {
		if item == nil ||
			item.ConvId == "" ||
			strings.TrimSpace(item.ConvId) != item.ConvId ||
			utf8.RuneCountInString(item.ConvId) > 128 ||
			item.AfterSeq < 0 ||
			item.Limit < 0 ||
			item.Limit > batchSyncMaxLimit {
			return nil, apperr.New(consts.CodeParamError)
		}

		if _, duplicated := seenConversationIDs[item.ConvId]; duplicated {
			// 重复会话会让客户端无法确定应采用哪一个 next_seq，严格拒绝而不是静默合并。
			return nil, apperr.New(consts.CodeParamError)
		}
		seenConversationIDs[item.ConvId] = struct{}{}

		effectiveLimit := int(item.Limit)
		if effectiveLimit == 0 {
			effectiveLimit = batchSyncDefaultLimit
		}
		totalLimit += effectiveLimit
		if totalLimit > batchSyncMaxTotalLimit {
			return nil, apperr.New(consts.CodeParamError)
		}

		prepared = append(prepared, preparedConversationSync{
			convID:   item.ConvId,
			afterSeq: item.AfterSeq,
			limit:    effectiveLimit,
		})
	}

	return prepared, nil
}
