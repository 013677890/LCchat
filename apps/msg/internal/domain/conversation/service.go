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
	groupQr GroupMembershipQuerier // 可选；nil 时群会话回退到“依赖本地会话行”的旧行为
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

// SetGroupMembershipQuerier 注入群成员查询实现（可选依赖，启动阶段调用一次）。
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
// 每发一条群消息 UPDATE 一次 max_seq + last_msg_*
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

// EnsureGroupMembersConv 确保群成员在 conversation 表有行（INSERT IGNORE）。
// 首次发群消息时由 workflow 异步调用，后续消息因行已存在会被跳过。
func (s *Service) EnsureGroupMembersConv(ctx context.Context, memberUUIDs []string, groupUUID string) error {
	return s.repo.BatchInitGroupMemberConv(ctx, memberUUIDs, groupUUID)
}

// GetByOwnerAndConvId 获取单个个人会话记录
func (s *Service) GetByOwnerAndConvId(ctx context.Context, ownerUuid, convId string) (*model.Conversation, error) {
	return s.repo.GetByOwnerAndConvId(ctx, ownerUuid, convId)
}

// ResolveReadAccess 校验用户对会话的读取权限并返回 clear_seq（拉取历史时用于过滤已删除位点）。
//
// 规则：
//   - 本地有会话行：返回该行 clear_seq（尊重用户删除位点）。
//   - 群会话且本地无行：回退校验群成员资格；是成员则放行，clear_seq=0（从未删除过）。
//   - 其余（单聊无行 / 非群成员 / 未注入查询器）：返回 ErrConversationNotFound。
//
// 这样群成员在 conversation 行尚未异步建好（发送竞态、新入群成员）时仍可拉取群历史，
// 而不是被本地行的缺失误判为无权访问。
func (s *Service) ResolveReadAccess(ctx context.Context, ownerUuid, convId string) (int64, error) {
	conv, err := s.repo.GetByOwnerAndConvId(ctx, ownerUuid, convId)
	if err == nil {
		return conv.ClearSeq, nil
	}
	if !errors.Is(err, ErrConversationNotFound) {
		return 0, err
	}
	if isGroupConv(convId) && s.groupQr != nil {
		role, qErr := s.groupQr.QueryMemberRole(ctx, convId, ownerUuid)
		if qErr != nil {
			return 0, qErr
		}
		if role >= 0 {
			return 0, nil // 是群成员，从未删除过本地会话 → clear_seq=0
		}
	}
	return 0, ErrConversationNotFound
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
			unread := int(conv.MaxSeq - conv.ReadSeq)
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
// 群会话特殊处理：成员的 conversation 行由发送链路异步补建，可能尚不存在。
// 此时若已注入群成员查询器，则校验成员资格后按需 upsert 建行写入已读位点，
// 避免群成员在行建好前标记已读直接失败。
func (s *Service) MarkRead(ctx context.Context, ownerUuid, convId string, readSeq int64) (int32, error) {
	events, err := buildMarkReadOutboxEvents(ctx, ownerUuid, convId, readSeq)
	if err != nil {
		return 0, err
	}
	conv, err := s.repo.UpdateReadSeqWithOutbox(ctx, ownerUuid, convId, readSeq, events)
	if err == nil {
		return int32(conv.UnreadCount), nil
	}

	// 群会话本地无行：校验成员资格后按需建行写入已读位点。
	if errors.Is(err, ErrConversationNotFound) && isGroupConv(convId) && s.groupQr != nil {
		role, qErr := s.groupQr.QueryMemberRole(ctx, convId, ownerUuid)
		if qErr != nil {
			return 0, qErr
		}
		if role < 0 {
			return 0, ErrConversationNotFound // 非群成员，不建行
		}
		conv, err = s.repo.UpsertGroupReadSeqWithOutbox(ctx, ownerUuid, convId, readSeq, events)
		if err != nil {
			return 0, err
		}
		return int32(conv.UnreadCount), nil
	}
	return 0, err
}

// ==================== DeleteConversation ====================

// DeleteConversation 逻辑删除会话
//
// status=1 + clear_seq=max_seq + read_seq=max_seq + unread=0
// 收到新消息时 Upsert 自动 status=0 重新激活
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

// NowPtr 返回当前时间的指针
func NowPtr() *time.Time {
	now := time.Now()
	return &now
}
