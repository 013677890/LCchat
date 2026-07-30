package message

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	pb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/id"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/msgevent"
	"google.golang.org/protobuf/proto"
)

// ==================== 配置 ====================

const createMessageSeqRetryLimit = 2

// Config 消息领域服务配置（通过 main.go 注入，支持环境变量覆盖）
type Config struct {
	// RecallWindow 撤回窗口时长，默认 2 分钟
	RecallWindow time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{RecallWindow: 2 * time.Minute}
}

// ==================== 群角色查询（外部适配） ====================

// GroupRoleQuerier 查询用户在群内的角色。
// 0=普通成员, 1=管理员, 2=群主, -1=非群成员。
// 由 infra 层通过 group-service gRPC 实现，msg 领域仅依赖此接口。
type GroupRoleQuerier interface {
	QueryMemberRole(ctx context.Context, groupUUID, userUUID string) (int8, error)
}

// ==================== Service 定义 ====================

// Service 消息领域服务
//
// 职责边界：
//   - ✅ 消息创建（幂等检查 + ULID 生成 + conv_id 计算 + seq 分配 + DB/outbox 落库）
//   - ✅ 消息拉取（按 seq 范围、按 ID 批量）
//   - ✅ 消息撤回（权限校验 + 时间窗口 + DB 状态更新）
//   - ❌ 不涉及会话 Upsert（由 usecase 层调用 conversation.Service 完成）
//   - ❌ 不直接投递 Kafka（通过 outbox_events + CDC 异步路由）
//   - ❌ 不依赖 conversation 领域包（保持领域隔离）
type Service struct {
	repo        Repository
	config      Config
	groupRoleQr GroupRoleQuerier // 可选；nil 时群管撤回退化为仅允许发送者本人
}

// NewService 创建消息领域服务
func NewService(repo Repository, cfg ...Config) *Service {
	c := DefaultConfig()
	if len(cfg) > 0 {
		c = cfg[0]
	}
	return &Service{repo: repo, config: c}
}

// SetGroupRoleQuerier 注入群角色查询实现（可选依赖，启动阶段调用一次）。
func (s *Service) SetGroupRoleQuerier(q GroupRoleQuerier) {
	s.groupRoleQr = q
}

// ==================== CreateMessage ====================

// CreateResult 创建消息的返回结果
type CreateResult struct {
	// Msg 消息实体（无论是新建还是幂等命中，都包含 msg_id / seq / conv_id / send_time）
	Msg *model.Message

	// IsIdempotent 标记此次调用是否命中了幂等缓存
	//   true  → 消息之前已成功创建过，usecase 层应跳过 Upsert 会话和 Kafka 投递
	//   false → 消息是本次新创建的，usecase 层继续后续步骤
	IsIdempotent bool
}

// CreateMessage 创建一条消息
//
// 完整流程：
//
//	┌──────────────────────────────────────────────────────────────┐
//	│ ① Redis SETNX "PROCESSING:{token}" (10s TTL)               │
//	│    ├─ 获取锁成功 → 继续执行                                    │
//	│    ├─ 值为 "PROCESSING:{token}" → 并发请求，返回 ErrIdempotentProcessing │
//	│    ├─ 值为 JSON → 幂等命中，直接返回缓存结果                      │
//	│    └─ Redis 异常 → 降级继续（DB 唯一索引兜底）                    │
//	│                                                              │
//	│ ② 计算 conv_id                                               │
//	│    ├─ 单聊: p2p-{sorted(uuid_a, uuid_b)}                     │
//	│    └─ 群聊: target_uuid (即群 UUID)                           │
//	│                                                              │
//	│ ③ 生成 msg_id (ULID，时间有序，无 B+ 树页分裂)                   │
//	│                                                              │
//	│ ④ Redis INCR msg:seq:{conv_id} → 分配会话内严格递增 seq          │
//	│                                                              │
//	│ ⑤ DB INSERT message 表                                       │
//	│    └─ 如触发 uidx_sender_client 唯一索引冲突                    │
//	│       → 查出已有记录，返回 IsIdempotent=true                    │
//	│                                                              │
//	│ ⑥ Redis SET 幂等结果缓存 (覆盖 "PROCESSING"，10min TTL)        │
//	│                                                              │
//	│ ⑦ 返回 CreateResult{Msg, IsIdempotent: false}                │
//	└──────────────────────────────────────────────────────────────┘
//
// fromUuid / deviceId 是鉴权主体，由 handler 从 gRPC metadata（ctxmeta）解析后显式传入。
func (s *Service) CreateMessage(ctx context.Context, fromUuid, deviceId string, req *pb.SendMessageRequest) (*CreateResult, error) {
	// ---- Step 1: 幂等检查（Redis SETNX） ----
	// 三态返回：
	//   - ({LeaseToken}, nil):  锁获取成功，继续创建
	//   - ({CachedMsg}, nil):   幂等命中，直接返回缓存
	//   - (nil, ErrProcessing): 并发请求正在处理
	//   - (nil, otherErr):      Redis 异常，降级继续（DB 唯一索引兜底）
	acquireResult, err := s.repo.TryAcquireIdempotent(ctx, fromUuid, deviceId, req.ClientMsgId)
	if err != nil {
		if errors.Is(err, ErrIdempotentProcessing) {
			return nil, ErrIdempotentProcessing
		}

		// Redis 异常 → 降级：不拦截，靠 DB 唯一索引兜底
	}

	if acquireResult != nil && acquireResult.CachedMsg != nil {
		return &CreateResult{Msg: acquireResult.CachedMsg, IsIdempotent: true}, nil
	}

	leaseToken := ""
	if acquireResult != nil {
		leaseToken = acquireResult.LeaseToken
	}
	leaseResolved := leaseToken == ""
	defer func() {
		if leaseResolved {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		if err := s.repo.ReleaseIdempotentProcessing(cleanupCtx, fromUuid, deviceId, req.ClientMsgId, leaseToken); err != nil {
			logger.Warn(cleanupCtx, "创建消息：释放幂等 PROCESSING 标记失败",
				logger.String("client_msg_id", req.ClientMsgId),
				logger.ErrorField("error", err),
			)
		}
	}()

	cacheIdempotentResult := func(resultMsg *model.Message) {
		var cacheErr error
		if leaseToken != "" {
			cacheErr = s.repo.CompleteIdempotentResult(ctx, fromUuid, deviceId, req.ClientMsgId, leaseToken, resultMsg)
			if cacheErr == nil {
				leaseResolved = true
			}
		} else {
			cacheErr = s.repo.SetIdempotentResult(ctx, fromUuid, deviceId, req.ClientMsgId, resultMsg)
		}
		if cacheErr != nil {
			logger.Warn(ctx, "创建消息：回写幂等缓存失败",
				logger.String("msg_id", resultMsg.MsgId),
				logger.ErrorField("error", cacheErr),
			)
		}
	}

	// ---- Step 2: 校验消息类型 ----
	msgType, ok := model.ParseMsgType(req.MsgType)
	if !ok {
		return nil, ErrUnsupportedMsgType
	}

	// ---- Step 3: 计算 conv_id ----
	convId := computeConvId(req.ConvType, fromUuid, req.TargetUuid)

	// ---- Step 4: 生成 ULID msg_id ----
	// ulid.Make() 内部使用 sync.Pool 并发安全熵池，无需手动创建随机源
	msgId := id.GenerateULID()

	// ---- Step 5: 准备 Message 公共字段 ----
	now := time.Now()

	// at_users: proto []string → JSON string 存 DB
	atUsersJSON := "[]"
	if len(req.AtUsers) > 0 {
		if data, err := json.Marshal(req.AtUsers); err == nil {
			atUsersJSON = string(data)
		}
	}

	var msg *model.Message
	created := false
	for attempt := 0; attempt <= createMessageSeqRetryLimit; attempt++ {
		// 每次重试都重新分配 seq：上一次失败的 seq 已被 DB 唯一索引证明不可用，
		// 继续复用只会再次冲突，必须先 RepairSeq 再拿新的会话位点。
		// Redis 计数器负责常规分配；若计数器丢失，仓储会从 DB 最大 seq 恢复。
		seq, allocErr := s.repo.AllocSeq(ctx, convId)
		if allocErr != nil {
			return nil, fmt.Errorf("CreateMessage: alloc seq failed: %w", allocErr)
		}

		msg = &model.Message{
			ConvId:       convId,
			Seq:          seq,
			MsgId:        msgId,
			ClientMsgId:  req.ClientMsgId,
			FromUuid:     fromUuid,
			DeviceId:     deviceId,
			MsgType:      int16(msgType),
			Content:      req.Content,
			Status:       0, // 0=正常
			ReplyToMsgId: req.ReplyToMsgId,
			AtUsers:      atUsersJSON,
			SendTime:     now,
		}

		payload, buildErr := buildMsgPushOutboxPayload(ctx, req, msg)
		if buildErr != nil {
			return nil, fmt.Errorf("CreateMessage: build outbox event failed: %w", buildErr)
		}
		createErr := s.repo.CreateWithOutbox(ctx, msg, OutboxEvent{
			EventType: msgevent.EventTypeMsgPush,
			EntityID:  msg.ConvId,
			Payload:   payload,
		})

		if createErr == nil {
			created = true
			break
		}
		if errors.Is(createErr, ErrDuplicateMessage) {
			// DB 唯一索引兜底：SETNX 降级时可能走到这里。
			existMsg, queryErr := s.repo.GetByDuplicateKey(ctx, fromUuid, deviceId, req.ClientMsgId)
			if queryErr != nil {
				return nil, fmt.Errorf("CreateMessage: query duplicate failed: %w", queryErr)
			}
			cacheIdempotentResult(existMsg)
			return &CreateResult{Msg: existMsg, IsIdempotent: true}, nil
		}

		if errors.Is(createErr, ErrDuplicateMessageSeq) {
			// 只有 DB 的 uidx_conv_seq 会触发该错误，说明 Redis 计数器已经落后于事实表。
			// 先把 Redis 修到 DB 最大 seq，再进入下一轮重新 AllocSeq，最多重试有限次数避免死循环。
			if repairErr := s.repo.RepairSeq(ctx, convId); repairErr != nil {
				return nil, fmt.Errorf("CreateMessage: repair seq failed: %w", repairErr)
			}
			continue
		}

		return nil, fmt.Errorf("CreateMessage: db insert failed: %w", createErr)
	}

	if !created {
		return nil, fmt.Errorf("CreateMessage: seq retry exhausted: %w", ErrDuplicateMessageSeq)
	}

	// ---- Step 6: 回写幂等缓存 ----
	// 覆盖 "PROCESSING:{token}" → 实际结果 JSON，TTL 延长到 10 分钟。
	// 若回写失败，defer 会尽力释放 PROCESSING，让下一次重试走 DB 唯一索引兜底。
	cacheIdempotentResult(msg)

	return &CreateResult{Msg: msg, IsIdempotent: false}, nil
}

func buildMsgPushOutboxPayload(ctx context.Context, req *pb.SendMessageRequest, msg *model.Message) (string, error) {
	msgItemData, err := proto.Marshal(ModelToMsgItem(msg))
	if err != nil {
		return "", fmt.Errorf("marshal MsgItem failed: %w", err)
	}

	return msgevent.EncodeMsgPush(&pb.MsgPushEvent{
		EventId:      id.GenerateULID(),
		ReceiverUuid: req.TargetUuid,
		DeviceId:     msg.DeviceId,
		Type:         "MSG_PUSH",
		ConvType:     req.ConvType,
		Data:         msgItemData,
		FromUuid:     msg.FromUuid,
		TraceId:      ctxmeta.TraceID(ctx),
		ServerTs:     msg.SendTime.UnixMilli(),
		Seq:          msg.Seq,
	})
}

// ==================== PullMessages ====================

// PullMessages 按会话增量拉取历史消息
//
// 拉取策略：
//   - FORWARD  (direction=1): seq > anchor_seq，拉取新消息（增量同步）
//   - BACKWARD (direction=2): seq < anchor_seq，拉取更早的历史（上滑加载）
//
// clear_seq 过滤：
//
//	用户删除会话后，只返回 seq > clear_seq 的消息，删除前的历史不可见
//
// hasMore 判断 (N+1 Trick)：
//
//	实际查 limit+1 条，结果数 > limit 说明还有更多，截断返回
func (s *Service) PullMessages(ctx context.Context, convId string, anchorSeq int64, direction int, limit int, clearSeq int64) ([]*pb.MsgItem, bool, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	msgs, err := s.repo.GetBySeqRange(ctx, convId, anchorSeq, direction, limit+1, clearSeq)
	if err != nil {
		return nil, false, fmt.Errorf("拉取消息失败: %w", err)
	}

	hasMore := len(msgs) > limit
	if hasMore {
		msgs = msgs[:limit]
	}
	if direction == DirectionBackward {
		reverseMessages(msgs)
	}

	items := make([]*pb.MsgItem, 0, len(msgs))
	for _, msg := range msgs {
		items = append(items, ModelToMsgItem(msg))
	}

	return items, hasMore, nil
}

// GetMaxSeq 查询会话当前 seq 上界。
//
// 短期语义：该值可能来自 Redis 已分配序号上界，而不是严格“已落库最大 seq”。
// 当 Redis INCR 成功但 DB 落库失败时会产生 seq 空洞，客户端做 gap 补拉时需要容忍空洞。
func (s *Service) GetMaxSeq(ctx context.Context, convId string) (int64, error) {
	maxSeq, err := s.repo.GetMaxSeq(ctx, convId)
	if err != nil {
		return 0, fmt.Errorf("查询会话最大序号失败: %w", err)
	}
	return maxSeq, nil
}

func reverseMessages(msgs []*model.Message) {
	for left, right := 0, len(msgs)-1; left < right; left, right = left+1, right-1 {
		msgs[left], msgs[right] = msgs[right], msgs[left]
	}
}

// ==================== GetMessagesByIds ====================

// GetMessagesByIds 批量按 ID 查询消息
//
// 使用场景：@ 消息跳转、引用/回复预览、seq gap 补拉
// 找不到的 msg_id 静默跳过（proto 约定）
func (s *Service) GetMessagesByIds(ctx context.Context, convId string, msgIds []string) ([]*pb.MsgItem, error) {
	msgs, err := s.repo.GetByIds(ctx, convId, msgIds)
	if err != nil {
		return nil, fmt.Errorf("批量查询消息失败: %w", err)
	}

	items := make([]*pb.MsgItem, 0, len(msgs))
	for _, msg := range msgs {
		items = append(items, ModelToMsgItem(msg))
	}

	return items, nil
}

// ==================== RecallMessage ====================

// RecallMessage 撤回一条消息，并在同一事务中写入 msg.push Outbox 通知。
//
// 流程：查消息 → 校验已撤回 → 校验权限 → 校验时间窗口 → 更新 DB + Outbox。
// 返回被撤回的原始消息，便于调用方保持原响应语义。
func (s *Service) RecallMessage(ctx context.Context, convId, msgId, operatorUuid string) (*model.Message, error) {
	// 1. 查消息
	msg, err := s.repo.GetById(ctx, convId, msgId)
	if err != nil {
		return nil, err
	}

	// 2. 重复撤回检查
	if msg.Status == 1 {
		return nil, ErrMessageAlreadyRecalled
	}

	// 3. 权限校验
	isGroupConv := !strings.HasPrefix(convId, "p2p-")
	isSelf := msg.FromUuid == operatorUuid
	isAdmin := false

	if !isSelf {
		if !isGroupConv {
			return nil, ErrRecallNoPermission
		}
		if s.groupRoleQr == nil {
			return nil, ErrRecallNoPermission
		}
		role, err := s.groupRoleQr.QueryMemberRole(ctx, convId, operatorUuid)
		if err != nil {
			return nil, fmt.Errorf("查询群角色失败: %w", err)
		}
		if role < 1 {
			return nil, ErrRecallNoPermission
		}
		isAdmin = true
	}

	// 4. 时间窗口校验（群管理员/群主不受时间限制）
	if !isAdmin && time.Since(msg.SendTime) > s.config.RecallWindow {
		return nil, ErrRecallTimeout
	}

	// 5. 构造撤回提示与 CDC Outbox 事件，保证业务事实与下行通知原子提交。
	recallContent, _ := json.Marshal(map[string]string{
		"text":     "撤回了一条消息",
		"operator": operatorUuid,
	})
	serverTs := time.Now().UnixMilli()
	payload, err := buildRecallOutboxPayload(ctx, convId, msgId, operatorUuid, serverTs)
	if err != nil {
		return nil, fmt.Errorf("构造撤回 outbox 事件失败: %w", err)
	}

	if err := s.repo.UpdateStatusWithOutbox(ctx, convId, msgId, 1, string(recallContent), OutboxEvent{
		EventType: msgevent.EventTypeMsgPush,
		EntityID:  convId,
		Payload:   payload,
	}); err != nil {
		return nil, fmt.Errorf("撤回消息更新状态失败: %w", err)
	}

	return msg, nil
}

// buildRecallOutboxPayload 构造严格 protojson 的 MSG_RECALL 下行事件。
func buildRecallOutboxPayload(ctx context.Context, convId, msgId, operatorUuid string, serverTs int64) (string, error) {
	noticeData, err := proto.Marshal(&pb.RecallNotice{
		ConvId:     convId,
		MsgId:      msgId,
		Operator:   operatorUuid,
		RecallTime: serverTs,
	})
	if err != nil {
		return "", fmt.Errorf("marshal RecallNotice failed: %w", err)
	}

	receiverUuid, convType := resolveRecallReceiver(convId, operatorUuid)
	return msgevent.EncodeMsgPush(&pb.MsgPushEvent{
		EventId:      id.GenerateULID(),
		ReceiverUuid: receiverUuid,
		DeviceId:     ctxmeta.DeviceID(ctx),
		Type:         "MSG_RECALL",
		ConvType:     convType,
		Data:         noticeData,
		TraceId:      ctxmeta.TraceID(ctx),
		FromUuid:     operatorUuid,
		ServerTs:     serverTs,
		Seq:          0,
	})
}

// resolveRecallReceiver 根据会话类型计算撤回通知的下行接收目标。
func resolveRecallReceiver(convId, operatorUuid string) (string, pb.ConvType) {
	if strings.HasPrefix(convId, "p2p-") {
		return extractPeerUUIDFromP2PConvID(convId, operatorUuid), pb.ConvType_CONV_TYPE_P2P
	}
	return convId, pb.ConvType_CONV_TYPE_GROUP
}

// extractPeerUUIDFromP2PConvID 从 P2P 会话 ID 中解析操作者的对端用户 UUID。
func extractPeerUUIDFromP2PConvID(convId, selfUuid string) string {
	body := strings.TrimPrefix(convId, "p2p-")
	parts := strings.SplitN(body, "-", 2)
	if len(parts) != 2 {
		return ""
	}
	if parts[0] == selfUuid {
		return parts[1]
	}
	return parts[0]
}

// ==================== 辅助方法 ====================

// computeConvId 计算会话 ID
//   - 单聊 (P2P):  "p2p-{sorted(uuid1, uuid2)}"  → 双方看到同一个 conv_id
//   - 群聊 (GROUP): 直接使用 target_uuid（群 UUID）
func computeConvId(convType pb.ConvType, fromUuid, targetUuid string) string {
	if convType == pb.ConvType_CONV_TYPE_GROUP {
		return targetUuid
	}
	uuids := []string{fromUuid, targetUuid}
	sort.Strings(uuids)
	return "p2p-" + strings.Join(uuids, "-")
}

// ModelToMsgItem 将 model.Message 转换为 pb.MsgItem（导出供 handler/usecase 复用）
//
// 转换规则：
//   - send_time: time.Time → unix 毫秒
//   - at_users:  JSON string → []string
//   - msg_type/status: int 宽度转换
func ModelToMsgItem(msg *model.Message) *pb.MsgItem {
	item := &pb.MsgItem{
		MsgId:        msg.MsgId,
		ClientMsgId:  msg.ClientMsgId,
		ConvId:       msg.ConvId,
		Seq:          msg.Seq,
		FromUuid:     msg.FromUuid,
		MsgType:      int32(msg.MsgType),
		Content:      msg.Content,
		Status:       int32(msg.Status),
		SendTime:     msg.SendTime.UnixMilli(),
		ReplyToMsgId: msg.ReplyToMsgId,
	}

	// at_users: DB 存 JSON '["uuid1","uuid2"]' → proto repeated string
	if msg.AtUsers != "" && msg.AtUsers != "[]" {
		var atUsers []string
		if err := json.Unmarshal([]byte(msg.AtUsers), &atUsers); err == nil {
			item.AtUsers = atUsers
		}
	}

	return item
}
