package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	pb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/logger"
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
	repo Repository
}

// NewService 创建会话领域服务
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
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
		Status:      0, // 重新激活已删除的会话
	}

	// 发送方初始化时 read_seq 追平
	if isSender {
		conv.ReadSeq = msg.Seq
		conv.UnreadCount = 0
	} else {
		// INSERT 场景（首次创建会话）时 unread_count=1
		// UPDATE 场景会被 repo.Upsert 里的 gorm.Expr("unread_count + 1") 覆盖
		conv.UnreadCount = 1
	}

	// isSender 透传给 repository，控制 ON DUPLICATE KEY UPDATE 中的 unread 逻辑
	if err := s.repo.Upsert(ctx, conv, isSender); err != nil {
		return err
	}
	return nil
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

// GetByOwnerAndConvId 获取单个个人会话记录
func (s *Service) GetByOwnerAndConvId(ctx context.Context, ownerUuid, convId string) (*model.Conversation, error) {
	return s.repo.GetByOwnerAndConvId(ctx, ownerUuid, convId)
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

	// 2. 使用 RunSafe + channel 并发拉取数据 (各拉 pageSize + 1 条用于判断 hasMore)
	//
	// RunSafe 优势：Context 传播 + panic recover + 独立超时控制
	// 协调策略：channel 通知完成，context.WithTimeout 兜底防止协程池提交失败导致永久阻塞
	type convResult struct {
		convs []*model.Conversation
		err   error
	}

	fetchSize := pageSize + 1
	p2pCh := make(chan convResult, 1)
	groupCh := make(chan convResult, 1)

	const asyncTimeout = 5 * time.Second

	async.RunSafe(ctx, func(taskCtx context.Context) {
		convs, err := s.repo.ListP2P(taskCtx, ownerUuid, updatedSince, cursorTimeMs, cursorId, fetchSize)
		p2pCh <- convResult{convs, err}
	}, asyncTimeout)

	async.RunSafe(ctx, func(taskCtx context.Context) {
		convs, err := s.repo.ListGroup(taskCtx, ownerUuid, updatedSince, cursorTimeMs, cursorId, fetchSize)
		groupCh <- convResult{convs, err}
	}, asyncTimeout)

	// 收集结果：使用「优先级 Select」模式避免 Go select 伪随机问题。
	//
	// ⚠️ 普通 select 的陷阱：当 waitCtx 已超时且 dataCh 也有数据时，两个 case
	// 同时就绪，Go 会随机选择，有 50% 概率丢弃已有数据并触发多余的同步回退。
	//
	// 修复策略：外层 select 先阻塞等待（数据 OR 超时），超时后再用内层非阻塞
	// select（default 分支）抢读一次 channel，确保「有数据一定优先取数据」。
	waitCtx, waitCancel := context.WithTimeout(ctx, asyncTimeout)
	defer waitCancel()

	var p2pConvs []*model.Conversation
	var groupConvs []*model.Conversation
	var err1, err2 error

	// 优先级 Select — P2P
	select {
	case r := <-p2pCh:
		p2pConvs, err1 = r.convs, r.err
	case <-waitCtx.Done():
		// 超时触发：但先尝试非阻塞读，防止数据在超时瞬间同时到达被随机丢弃
		select {
		case r := <-p2pCh:
			p2pConvs, err1 = r.convs, r.err
		default:
			logger.Warn(ctx, "获取会话列表：P2P 异步超时")
			p2pConvs, err1 = s.repo.ListP2P(ctx, ownerUuid, updatedSince, cursorTimeMs, cursorId, fetchSize)
		}
	}

	// 优先级 Select — Group
	select {
	case r := <-groupCh:
		groupConvs, err2 = r.convs, r.err
	case <-waitCtx.Done():
		select {
		case r := <-groupCh:
			groupConvs, err2 = r.convs, r.err
		default:
			logger.Warn(ctx, "获取会话列表：Group 异步超时")
			groupConvs, err2 = s.repo.ListGroup(ctx, ownerUuid, updatedSince, cursorTimeMs, cursorId, fetchSize)
		}
	}

	if err1 != nil {
		return nil, false, "", fmt.Errorf("获取会话列表 P2P 查询失败: %w", err1)
	}
	if err2 != nil {
		return nil, false, "", fmt.Errorf("获取会话列表 Group 查询失败: %w", err2)
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
// DB 层面 read_seq = GREATEST(read_seq, readSeq)，同步计算 unread_count
// 返回最新计算得到的 unread_count
func (s *Service) MarkRead(ctx context.Context, ownerUuid, convId string, readSeq int64) (int32, error) {
	err := s.repo.UpdateReadSeq(ctx, ownerUuid, convId, readSeq)
	if err != nil {
		return 0, err
	}
	// 查询最新的会话状态获取 unread_count
	conv, err := s.repo.GetByOwnerAndConvId(ctx, ownerUuid, convId)
	if err != nil {
		return 0, err
	}
	return int32(conv.UnreadCount), nil
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
