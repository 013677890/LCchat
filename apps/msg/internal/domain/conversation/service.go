package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	pb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/async"
)

const (
	previewMaxRunes = 20
)

type lastMsgPreviewPayload struct {
	SenderUUID string `json:"sender_uuid"`
	Preview    string `json:"preview"`
}

// Service 会话领域服务
//
// 职责边界：
//   - ✅ Upsert 个人会话 + 群会话热数据（供 usecase 调用）
//   - ✅ 会话列表拉取（全量/增量，含群聊热数据拼装）
//   - ✅ 标记已读 / 删除会话 / 更新设置
//   - ❌ 不依赖 message 领域
//   - ❌ 不直接写 Kafka
type Service struct {
	repo    Repository
	groupQr GroupMembershipQuerier
}

// GroupMembershipQuerier 查询用户在群内的成员资格/角色。
// 返回 >=0 表示是群成员（0=普通成员/1=管理员/2=群主），<0 表示非成员。
// 由 infra 层用 group-service 实现；在会话域内自定义接口以保持领域隔离
// （groupcli.Client 结构化满足此接口，无需依赖 message 领域）。
type GroupMembershipQuerier interface {
	QueryMemberRole(ctx context.Context, groupUUID, userUUID string) (int8, error)
}

// NewService 创建会话领域服务
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// SetGroupMembershipQuerier 注入群成员权威点查实现（启动阶段调用一次）。
// Kafka 投影尚未为新成员建行时，读取链路只允许通过该接口确认身份；未注入时明确失败，
// 禁止仅凭一条不完整的本地 conversation 行放行。
func (s *Service) SetGroupMembershipQuerier(q GroupMembershipQuerier) {
	s.groupQr = q
}

// isGroupConv 判断会话是否为群聊。
// 约定：群会话 conv_id 为群 UUID（无前缀），单聊为 "p2p-{a}-{b}"。
func isGroupConv(convId string) bool {
	return !strings.HasPrefix(convId, "p2p-")
}

// ==================== UpsertForMessage (供 usecase 编排调用) ====================

// UpsertForMessage 发消息时更新/创建个人会话
//
// 参数说明：
//   - ownerUuid:  会话归属用户（发送方或接收方）
//   - msg:        刚落库的消息实体
//   - convType:   会话类型（P2P / GROUP）
//   - targetUuid: 单聊为对端 UUID，群聊为群 UUID
//   - isSender:   是否为发送方（控制未读数逻辑：发送方不加未读，接收方 DB 层面 +1）
func (s *Service) UpsertForMessage(
	ctx context.Context,
	ownerUuid string,
	msg *model.Message,
	convType pb.ConvType,
	targetUuid string,
	isSender bool,
) error {
	conv := buildMessageConversation(ownerUuid, msg, convType, targetUuid, isSender)

	// isSender 透传给 repository，控制 ON DUPLICATE KEY UPDATE 中的 unread 逻辑
	if err := s.repo.Upsert(ctx, conv, isSender); err != nil {
		return err
	}
	return nil
}

// RepairForMessage 幂等修复发消息产生的个人会话投影。
//
// 与 UpsertForMessage 的区别是：接收方未读数不做 +1，而是由仓储按 max_seq/read_seq 重算，
// 因此可以在幂等命中、异步补偿、重复重试时安全反复执行。
func (s *Service) RepairForMessage(
	ctx context.Context,
	ownerUuid string,
	msg *model.Message,
	convType pb.ConvType,
	targetUuid string,
	isSender bool,
) error {
	conv := buildMessageConversation(ownerUuid, msg, convType, targetUuid, isSender)
	if !isSender {
		conv.UnreadCount = ComputeUnreadCount(msg.Seq, 0)
	}
	if err := s.repo.RepairForMessage(ctx, conv, isSender); err != nil {
		return err
	}
	return nil
}

func buildMessageConversation(
	ownerUuid string,
	msg *model.Message,
	convType pb.ConvType,
	targetUuid string,
	isSender bool,
) *model.Conversation {
	preview := buildLastMsgPreview(msg)
	sendTime := msg.SendTime

	conv := &model.Conversation{
		ConvId:      msg.ConvId,
		Type:        int8(convType),
		OwnerUuid:   ownerUuid,
		TargetUuid:  targetUuid,
		LastMsgId:   msg.MsgId,
		LastMsgAt:   &sendTime,
		LastMsgPrev: preview,
		MaxSeq:      msg.Seq,
		Status:      0, // 新消息会重新激活已删除的会话
	}
	if convType == pb.ConvType_CONV_TYPE_GROUP {
		// 发送权限已经在 workflow Step 0 由 group-service 权威确认。
		// Kafka member_added/group_created 事件随后会用正 projection_version 覆盖 version=0；
		// 这里显式写 Active，禁止创建 membership_status=0 的非法 GROUP 行。
		conv.MembershipStatus = model.ConversationMembershipActive
	}

	if isSender {
		conv.ReadSeq = msg.Seq
		conv.UnreadCount = 0
	} else {
		// 普通发送路径 INSERT 时只代表“这条新消息未读”。
		// 修复路径会在调用方覆盖为 max_seq-read_seq 语义，避免漏写后补行时少算。
		conv.UnreadCount = 1
	}
	return conv
}

// UpsertGroupConv 发群消息时更新群会话热数据
//
// 每发一条群消息只 UPDATE 一行 max_seq + last_msg_*；仓储按 seq 整组单调推进。
func (s *Service) UpsertGroupConv(ctx context.Context, msg *model.Message) error {
	preview := buildLastMsgPreview(msg)
	sendTime := msg.SendTime

	gc := &model.GroupConversation{
		GroupUuid:   msg.ConvId,
		MaxSeq:      msg.Seq,
		LastMsgId:   msg.MsgId,
		LastMsgPrev: preview,
		LastMsgAt:   &sendTime,
	}

	if err := s.repo.UpsertGroupConv(ctx, gc); err != nil {
		return err
	}
	return nil
}

// GetByOwnerAndConvId 获取单个个人会话记录
func (s *Service) GetByOwnerAndConvId(ctx context.Context, ownerUuid, convId string) (*model.Conversation, error) {
	return s.repo.GetByOwnerAndConvId(ctx, ownerUuid, convId)
}

// ResolveReadAccess 校验用户对会话的读取权限并返回 clear_seq（拉取历史时用于过滤已删除位点）。
//
// 规则：
//   - P2P：必须存在当前用户的本地会话行。
//   - GROUP Active：同时要求成员投影有效且群共享状态为 normal。
//   - GROUP Left/群已解散：直接拒绝，conversation 行存在也不能作为权限凭证。
//   - GROUP 投影尚未建行：向 group-service 做权威单成员点查；确认有效后临时放行，
//     clear_seq=0，随后 group.cache 消费者会补齐正式成员投影。
//
// 本地投影解决正常读取的性能与退群后的长期权限收敛；缺行点查只处理 Kafka 可见性窗口。
func (s *Service) ResolveReadAccess(ctx context.Context, ownerUuid, convId string) (int64, error) {
	if isGroupConv(convId) {
		conv, err := s.resolveGroupAccess(ctx, ownerUuid, convId)
		if err != nil {
			return 0, err
		}
		if conv == nil {
			return 0, nil
		}
		return conv.ClearSeq, nil
	}

	conv, err := s.repo.GetByOwnerAndConvId(ctx, ownerUuid, convId)
	if err == nil {
		return conv.ClearSeq, nil
	}
	if !errors.Is(err, ErrConversationNotFound) {
		return 0, err
	}
	return 0, ErrConversationNotFound
}

// resolveGroupAccess 使用本地成员投影处理稳定态，仅在 conversation 尚未投影时调用
// group-service 点查。这避免正常消息补拉产生额外 RPC，同时不再永久信任退群用户的旧行。
func (s *Service) resolveGroupAccess(
	ctx context.Context,
	ownerUuid, groupUuid string,
) (*model.Conversation, error) {
	conv, err := s.repo.GetByOwnerAndConvId(ctx, ownerUuid, groupUuid)
	if err == nil {
		if conv.Type != int8(pb.ConvType_CONV_TYPE_GROUP) {
			return nil, ErrInvalidGroupMembershipState
		}
		switch conv.MembershipStatus {
		case model.ConversationMembershipLeft:
			return nil, ErrConversationNotFound
		case model.ConversationMembershipActive:
			groupConv, groupErr := s.repo.GetGroupConv(ctx, groupUuid)
			if groupErr == nil {
				if groupConv.GroupStatus != model.GroupConversationStatusNormal {
					return nil, ErrConversationNotFound
				}
				return conv, nil
			}

			if !errors.Is(groupErr, ErrConversationNotFound) {
				return nil, groupErr
			}

			// group.cache 与 msg.push 存在短暂竞态时可能缺共享群行；此时继续走下面的
			// 权威点查，不从不完整本地状态猜测权限。
		default:
			return nil, ErrInvalidGroupMembershipState
		}
	} else if !errors.Is(err, ErrConversationNotFound) {
		return nil, err
	}

	if s.groupQr == nil {
		return nil, ErrGroupMembershipQuerierUnavailable
	}
	role, err := s.groupQr.QueryMemberRole(ctx, groupUuid, ownerUuid)
	if err != nil {
		return nil, err
	}
	if role < 0 {
		return nil, ErrConversationNotFound
	}

	// 返回 nil 表示权威服务已放行，但个人会话投影仍在 Kafka 可见性窗口内。
	return nil, nil
}

// ==================== GetConversations ====================

// GetConversations 查询会话列表 (双协程并发多路归并版)
func (s *Service) GetConversations(ctx context.Context, ownerUuid string, updatedSince int64, cursor string, pageSize int) ([]*pb.ConversationItem, bool, string, error) {
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 50
	}

	// 1. 解析游标
	var cursorTimeMs int64 = 0
	var cursorId int64 = 0
	if cursor != "" {
		parts := strings.SplitN(cursor, "_", 2)
		if len(parts) == 2 {
			cursorTimeMs, _ = strconv.ParseInt(parts[0], 10, 64)
			cursorId, _ = strconv.ParseInt(parts[1], 10, 64)
		}
	}

	// 2. 使用请求内协程池并发拉取数据，继承父请求取消和超时语义。
	fetchSize := pageSize + 1
	const asyncTimeout = 5 * time.Second
	group := async.NewGroup(ctx, asyncTimeout)

	var p2pConvs []*model.Conversation
	var groupConvs []*model.Conversation

	if err := group.Go(func(taskCtx context.Context) error {
		convs, err := s.repo.ListP2P(taskCtx, ownerUuid, updatedSince, cursorTimeMs, cursorId, fetchSize)
		if err != nil {
			return fmt.Errorf("获取会话列表 P2P 查询失败: %w", err)
		}
		p2pConvs = convs
		return nil
	}); err != nil {
		return nil, false, "", err
	}

	if err := group.Go(func(taskCtx context.Context) error {
		convs, err := s.repo.ListGroup(taskCtx, ownerUuid, updatedSince, cursorTimeMs, cursorId, fetchSize)
		if err != nil {
			return fmt.Errorf("获取会话列表 Group 查询失败: %w", err)
		}
		groupConvs = convs
		return nil
	}); err != nil {
		if waitErr := group.Wait(); waitErr != nil {
			return nil, false, "", waitErr
		}
		return nil, false, "", err
	}

	if err := group.Wait(); err != nil {
		return nil, false, "", err
	}

	// 3. 内存合并数据
	merged := append(p2pConvs, groupConvs...)

	// 如果数据为空，提前返回
	if len(merged) == 0 {
		return []*pb.ConversationItem{}, false, strconv.FormatInt(updatedSince, 10), nil
	}

	// 4. 内存降序排序 (核心：由于 ListGroup 已映射真实时间到 UpdatedAt，直接通用比较)
	sort.Slice(merged, func(i, j int) bool {
		timeI := merged[i].UpdatedAt.UnixMilli()
		timeJ := merged[j].UpdatedAt.UnixMilli()
		if timeI == timeJ {
			return merged[i].Id > merged[j].Id // 时间相同，比较 ID (降序)
		}
		return timeI > timeJ // 时间不同，比较时间 (降序)
	})

	// 5. 截断控制：判断是否有下一页
	hasMore := len(merged) > pageSize
	if hasMore {
		merged = merged[:pageSize] // 严格切出前 pageSize 条
	}

	// 6. 组装返回结果
	items := make([]*pb.ConversationItem, 0, len(merged))
	var nextCursorStr string

	for _, conv := range merged {
		// 群聊未读数动态计算（此时 conv.MaxSeq 已经是真实的群 MaxSeq）
		if conv.Type == int8(pb.ConvType_CONV_TYPE_GROUP) {
			readFloor := conv.ReadSeq
			if conv.ClearSeq > readFloor {
				readFloor = conv.ClearSeq
			}
			unread := int(conv.MaxSeq - readFloor)
			if unread < 0 {
				unread = 0
			}
			conv.UnreadCount = unread
		}

		items = append(items, modelToConvItem(conv))

		// 最后一条记录构成下一次的联合游标
		nextCursorStr = fmt.Sprintf("%d_%d", conv.UpdatedAt.UnixMilli(), conv.Id)
	}

	return items, hasMore, nextCursorStr, nil
}

// ==================== MarkRead ====================

// MarkRead 标记会话已读
//
// DB 层面 read_seq = GREATEST(read_seq, readSeq)，并与已读同步事件同事务写入 outbox。
// 返回最新计算得到的 unread_count。
//
// 群会话先通过显式成员投影/权威点查完成权限裁决，再统一按需 upsert 已读位点；
// 不允许先更新一条可能属于已退群用户的旧 conversation 行。
func (s *Service) MarkRead(ctx context.Context, ownerUuid, convId string, readSeq int64) (int32, error) {
	events, err := buildMarkReadOutboxEvents(ctx, ownerUuid, convId, readSeq)
	if err != nil {
		return 0, err
	}

	if isGroupConv(convId) {
		if _, err := s.resolveGroupAccess(ctx, ownerUuid, convId); err != nil {
			return 0, err
		}
		conv, err := s.repo.UpsertGroupReadSeqWithOutbox(ctx, ownerUuid, convId, readSeq, events)
		if err != nil {
			return 0, err
		}
		return int32(conv.UnreadCount), nil
	}

	conv, err := s.repo.UpdateReadSeqWithOutbox(ctx, ownerUuid, convId, readSeq, events)
	if err != nil {
		return 0, err
	}
	return int32(conv.UnreadCount), nil
}

// ==================== DeleteConversation ====================

// DeleteConversation 逻辑删除会话
//
// status=1 + clear_seq=权威 max_seq + read_seq=权威 max_seq + unread=0。
// P2P 收到新消息时由 Upsert 重新激活；GROUP 通过共享 max_seq>clear_seq 派生重新可见，
// 不给全体成员写扩散。
func (s *Service) DeleteConversation(ctx context.Context, ownerUuid, convId string) error {
	if err := s.repo.Delete(ctx, ownerUuid, convId); err != nil {
		return err
	}
	return nil
}

// ==================== UpdateSettings ====================

// UpdateSettings 更新会话设置（免打扰/置顶）
func (s *Service) UpdateSettings(ctx context.Context, ownerUuid, convId string, mute *bool, pin *bool) error {
	if err := s.repo.UpdateSettings(ctx, ownerUuid, convId, mute, pin); err != nil {
		return err
	}
	return nil
}

// ==================== 辅助方法 ====================

// truncatePreview 截取消息预览（超过 maxLen 截断 + "..."）。
func truncatePreview(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen]) + "..."
}

func buildLastMsgPreview(msg *model.Message) string {
	payload := lastMsgPreviewPayload{
		SenderUUID: msg.FromUuid,
		Preview:    buildPreviewText(msg),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return `{"sender_uuid":"","preview":""}`
	}
	return string(data)
}

func buildPreviewText(msg *model.Message) string {
	switch model.MsgType(msg.MsgType) {
	case model.MsgTypeImage:
		return "[图片]"
	case model.MsgTypeVoice:
		return "[语音]"
	case model.MsgTypeVideo:
		return "[视频]"
	case model.MsgTypeFile:
		return "[文件]"
	case model.MsgTypeLocation:
		return "[位置]"
	case model.MsgTypeSystem:
		return "[系统消息]"
	}

	type textContent struct {
		Text string `json:"text"`
	}
	var content textContent
	if err := json.Unmarshal([]byte(msg.Content), &content); err == nil && content.Text != "" {
		return truncatePreview(content.Text, previewMaxRunes)
	}
	return truncatePreview(msg.Content, previewMaxRunes)
}

// modelToConvItem 将 model.Conversation 转换为 pb.ConversationItem
func modelToConvItem(conv *model.Conversation) *pb.ConversationItem {
	item := &pb.ConversationItem{
		ConvId:      conv.ConvId,
		ConvType:    pb.ConvType(conv.Type),
		TargetUuid:  conv.TargetUuid,
		UnreadCount: int32(conv.UnreadCount),
		Mute:        conv.Mute,
		Pin:         conv.Pin,
		UpdatedAt:   conv.UpdatedAt.UnixMilli(),
		Status:      int32(conv.Status),
	}

	if conv.LastMsgId != "" {
		var sendTimeMs int64
		if conv.LastMsgAt != nil {
			sendTimeMs = conv.LastMsgAt.UnixMilli()
		}
		item.LastMsg = &pb.LastMsgPreview{
			MsgId:       conv.LastMsgId,
			PreviewJson: conv.LastMsgPrev,
			SendTime:    sendTimeMs,
		}
	}

	return item
}

// ComputeUnreadCount 计算未读数 (导出供 usecase 复用)
func ComputeUnreadCount(maxSeq, readSeq int64) int {
	count := int(maxSeq - readSeq)
	if count < 0 {
		return 0
	}
	return count
}
