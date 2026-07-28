package dto

import (
	"strings"
	"unicode/utf8"

	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
)

// ==================== 消息发送 DTO ====================

// SendMessageRequest 发送消息请求
type SendMessageRequest struct {
	ClientMsgID  string   `json:"clientMsgId" binding:"required,min=1,max=64"`
	ConvType     int32    `json:"convType" binding:"required,oneof=1 2"`
	TargetUUID   string   `json:"targetUuid" binding:"required,min=1"`
	MsgType      int32    `json:"msgType" binding:"required"`
	Content      string   `json:"content" binding:"required,min=1,max=65536"`
	ReplyToMsgID string   `json:"replyToMsgId"`
	AtUsers      []string `json:"atUsers"`
}

// SendMessageResponse 发送消息响应
type SendMessageResponse struct {
	MsgID    string `json:"msgId"`
	Seq      int64  `json:"seq"`
	ConvID   string `json:"convId"`
	SendTime int64  `json:"sendTime"`
}

// ConvertToProtoSendMessageRequest 将发送消息 DTO 转换为 Protobuf 请求。
// 发送者身份（user_uuid/device_id）由 gRPC metadata 携带，不在请求体传递。
func ConvertToProtoSendMessageRequest(dto *SendMessageRequest) *msgpb.SendMessageRequest {
	if dto == nil {
		return nil
	}
	return &msgpb.SendMessageRequest{
		ConvType:     msgpb.ConvType(dto.ConvType),
		TargetUuid:   dto.TargetUUID,
		ClientMsgId:  dto.ClientMsgID,
		MsgType:      dto.MsgType,
		Content:      dto.Content,
		ReplyToMsgId: dto.ReplyToMsgID,
		AtUsers:      dto.AtUsers,
	}
}

// ConvertSendMessageResponseFromProto 将 Protobuf 响应转换为发送消息 DTO
func ConvertSendMessageResponseFromProto(pb *msgpb.SendMessageResponse) *SendMessageResponse {
	if pb == nil {
		return nil
	}
	return &SendMessageResponse{
		MsgID:    pb.MsgId,
		Seq:      pb.Seq,
		ConvID:   pb.ConvId,
		SendTime: pb.SendTime,
	}
}

// ==================== 消息拉取 DTO ====================

// PullMessagesRequest 拉取消息请求（Query 绑定）
type PullMessagesRequest struct {
	ConvID    string `form:"convId" binding:"required"`
	AnchorSeq int64  `form:"anchorSeq"`
	Limit     int32  `form:"limit" binding:"omitempty,min=0,max=200"`
	Direction int32  `form:"direction" binding:"omitempty,oneof=0 1 2"`
}

// PullMessagesResponse 拉取消息响应
type PullMessagesResponse struct {
	Messages []*MsgItemDTO `json:"messages"`
	HasMore  bool          `json:"hasMore"`
	MaxSeq   int64         `json:"maxSeq"`
}

// ConvertToProtoPullMessagesRequest 将拉取消息 DTO 转换为 Protobuf 请求
func ConvertToProtoPullMessagesRequest(dto *PullMessagesRequest) *msgpb.PullMessagesRequest {
	if dto == nil {
		return nil
	}
	return &msgpb.PullMessagesRequest{
		ConvId:    dto.ConvID,
		AnchorSeq: dto.AnchorSeq,
		Limit:     dto.Limit,
		Direction: msgpb.PullDirection(dto.Direction),
	}
}

// ConvertPullMessagesResponseFromProto 将 Protobuf 响应转换为拉取消息 DTO
func ConvertPullMessagesResponseFromProto(pb *msgpb.PullMessagesResponse) *PullMessagesResponse {
	if pb == nil {
		return nil
	}
	return &PullMessagesResponse{
		Messages: ConvertMsgItemsFromProto(pb.Messages),
		HasMore:  pb.HasMore,
		MaxSeq:   pb.MaxSeq,
	}
}

// ==================== 多会话消息同步 DTO ====================

const (
	batchSyncDefaultLimit     = 10
	batchSyncMaxLimit         = 50
	batchSyncMaxConversations = 50
	batchSyncMaxTotalLimit    = 500
)

// ConversationSyncCursorDTO 是一个会话独立的增量同步位点。
// AfterSeq 必须来自客户端对该会话已经持久化的位点，不能用其他会话的 seq 替代。
type ConversationSyncCursorDTO struct {
	ConvID   string `json:"convId" binding:"required,min=1,max=128"`
	AfterSeq int64  `json:"afterSeq" binding:"min=0"`
	Limit    int32  `json:"limit" binding:"omitempty,min=0,max=50"`
}

// BatchSyncMessagesRequest 按会话提交独立位点。Gateway binding 负责字段级约束，
// ValidateBatchSyncMessagesRequest 再补充重复 convId、空白字符和总预算等跨字段约束。
type BatchSyncMessagesRequest struct {
	Conversations []*ConversationSyncCursorDTO `json:"conversations" binding:"required,min=1,max=50,dive,required"`
}

// ConversationSyncResultDTO 是一个会话的独立同步结果。
// ErrorCode=0 时其余字段有效；非 0 时 Messages 为空且 NextSeq 保持原请求位点。
type ConversationSyncResultDTO struct {
	ConvID    string        `json:"convId"`
	Messages  []*MsgItemDTO `json:"messages"`
	HasMore   bool          `json:"hasMore"`
	MaxSeq    int64         `json:"maxSeq"`
	NextSeq   int64         `json:"nextSeq"`
	ErrorCode int32         `json:"errorCode"`
}

// BatchSyncMessagesResponse 中 Results 与请求 Conversations 一一对应且顺序相同。
type BatchSyncMessagesResponse struct {
	Results []*ConversationSyncResultDTO `json:"results"`
}

// ValidateBatchSyncMessagesRequest 校验需要观察整个批次才能确定的约束。
// 这些规则在 msg-service 还会再次校验；Gateway 提前拒绝是为了不让确定非法的请求
// 占用一次 gRPC 往返，更不能依赖它代替服务端领域边界校验。
func ValidateBatchSyncMessagesRequest(req *BatchSyncMessagesRequest) bool {
	if req == nil ||
		len(req.Conversations) == 0 ||
		len(req.Conversations) > batchSyncMaxConversations {
		return false
	}

	seenConversationIDs := make(map[string]struct{}, len(req.Conversations))
	totalLimit := 0
	for _, item := range req.Conversations {
		if item == nil ||
			item.ConvID == "" ||
			strings.TrimSpace(item.ConvID) != item.ConvID ||
			utf8.RuneCountInString(item.ConvID) > 128 ||
			item.AfterSeq < 0 ||
			item.Limit < 0 ||
			item.Limit > batchSyncMaxLimit {
			return false
		}
		if _, duplicated := seenConversationIDs[item.ConvID]; duplicated {
			return false
		}
		seenConversationIDs[item.ConvID] = struct{}{}

		effectiveLimit := int(item.Limit)
		if effectiveLimit == 0 {
			effectiveLimit = batchSyncDefaultLimit
		}
		totalLimit += effectiveLimit
		if totalLimit > batchSyncMaxTotalLimit {
			return false
		}
	}
	return true
}

// ConvertToProtoBatchSyncMessagesRequest 将 HTTP DTO 转换为 msg-service 请求，并保持输入顺序。
func ConvertToProtoBatchSyncMessagesRequest(dto *BatchSyncMessagesRequest) *msgpb.BatchSyncMessagesRequest {
	if dto == nil {
		return nil
	}

	conversations := make([]*msgpb.ConversationSyncCursor, 0, len(dto.Conversations))
	for _, item := range dto.Conversations {
		if item == nil {
			// 正常 HTTP 入口会在转换前拒绝 nil；保留 nil 可使绕过 Handler 的内部调用
			// 仍由 msg-service 严格拒绝，而不是在 Gateway 转换时 panic。
			conversations = append(conversations, nil)
			continue
		}
		conversations = append(conversations, &msgpb.ConversationSyncCursor{
			ConvId:   item.ConvID,
			AfterSeq: item.AfterSeq,
			Limit:    item.Limit,
		})
	}
	return &msgpb.BatchSyncMessagesRequest{Conversations: conversations}
}

// ConvertBatchSyncMessagesResponseFromProto 将逐会话结果转换为 HTTP JSON DTO。
func ConvertBatchSyncMessagesResponseFromProto(pb *msgpb.BatchSyncMessagesResponse) *BatchSyncMessagesResponse {
	if pb == nil {
		return nil
	}

	results := make([]*ConversationSyncResultDTO, 0, len(pb.Results))
	for _, item := range pb.Results {
		if item == nil {
			// 正常调用会由 Gateway service 在转换前严格拒绝 nil。直接使用转换函数时
			// 也选择失败关闭，不能静默跳项破坏 results 与请求的一一对应关系。
			return nil
		}
		results = append(results, &ConversationSyncResultDTO{
			ConvID:    item.ConvId,
			Messages:  ConvertMsgItemsFromProto(item.Messages),
			HasMore:   item.HasMore,
			MaxSeq:    item.MaxSeq,
			NextSeq:   item.NextSeq,
			ErrorCode: item.ErrorCode,
		})
	}
	return &BatchSyncMessagesResponse{Results: results}
}

// ==================== 消息反查 DTO ====================

// GetMessagesByIdsRequest 批量获取消息请求
type GetMessagesByIdsRequest struct {
	ConvID string   `json:"convId" binding:"required,min=1"`
	MsgIDs []string `json:"msgIds" binding:"required,min=1,max=50,dive,required"`
}

// GetMessagesByIdsResponse 批量获取消息响应
type GetMessagesByIdsResponse struct {
	Messages []*MsgItemDTO `json:"messages"`
}

// ConvertToProtoGetMessagesByIdsRequest 将批量获取消息 DTO 转换为 Protobuf 请求
func ConvertToProtoGetMessagesByIdsRequest(dto *GetMessagesByIdsRequest) *msgpb.GetMessagesByIdsRequest {
	if dto == nil {
		return nil
	}
	return &msgpb.GetMessagesByIdsRequest{
		ConvId: dto.ConvID,
		MsgIds: dto.MsgIDs,
	}
}

// ConvertGetMessagesByIdsResponseFromProto 将 Protobuf 响应转换为批量获取消息 DTO
func ConvertGetMessagesByIdsResponseFromProto(pb *msgpb.GetMessagesByIdsResponse) *GetMessagesByIdsResponse {
	if pb == nil {
		return nil
	}
	return &GetMessagesByIdsResponse{
		Messages: ConvertMsgItemsFromProto(pb.Messages),
	}
}

// ==================== 消息撤回 DTO ====================

// RecallMessageRequest 撤回消息请求
type RecallMessageRequest struct {
	ConvID string `json:"convId" binding:"required,min=1"`
	MsgID  string `json:"msgId" binding:"required,min=1"`
}

// ConvertToProtoRecallMessageRequest 将撤回消息 DTO 转换为 Protobuf 请求。
// 操作者身份由 gRPC metadata 携带，不在请求体传递。
func ConvertToProtoRecallMessageRequest(dto *RecallMessageRequest) *msgpb.RecallMessageRequest {
	if dto == nil {
		return nil
	}
	return &msgpb.RecallMessageRequest{
		ConvId: dto.ConvID,
		MsgId:  dto.MsgID,
	}
}

// ==================== 会话列表 DTO ====================

// GetConversationsRequest 获取会话列表请求（Query 绑定）
type GetConversationsRequest struct {
	UpdatedSince int64  `form:"updatedSince"`
	PageSize     int32  `form:"pageSize" binding:"omitempty,min=0,max=200"`
	Cursor       string `form:"cursor"`
}

// GetConversationsResponse 获取会话列表响应
type GetConversationsResponse struct {
	Conversations []*ConversationItemDTO `json:"conversations"`
	HasMore       bool                   `json:"hasMore"`
	NextCursor    string                 `json:"nextCursor"`
}

// ConvertToProtoGetConversationsRequest 将获取会话列表 DTO 转换为 Protobuf 请求。
// 会话归属用户身份由 gRPC metadata 携带，不在请求体传递。
func ConvertToProtoGetConversationsRequest(dto *GetConversationsRequest) *msgpb.GetConversationsRequest {
	if dto == nil {
		return nil
	}
	return &msgpb.GetConversationsRequest{
		UpdatedSince: dto.UpdatedSince,
		PageSize:     dto.PageSize,
		Cursor:       dto.Cursor,
	}
}

// ConvertGetConversationsResponseFromProto 将 Protobuf 响应转换为获取会话列表 DTO
func ConvertGetConversationsResponseFromProto(pb *msgpb.GetConversationsResponse) *GetConversationsResponse {
	if pb == nil {
		return nil
	}
	return &GetConversationsResponse{
		Conversations: ConvertConversationItemsFromProto(pb.Conversations),
		HasMore:       pb.HasMore,
		NextCursor:    pb.NextCursor,
	}
}

// ==================== 标记已读 DTO ====================

// MarkReadRequest 标记已读请求
type MarkReadRequest struct {
	ConvID  string `json:"convId" binding:"required"`
	ReadSeq int64  `json:"readSeq" binding:"required,gt=0"`
}

// MarkReadResponse 标记已读响应
type MarkReadResponse struct {
	UnreadCount int32 `json:"unreadCount"`
}

// ConvertToProtoMarkReadRequest 将标记已读 DTO 转换为 Protobuf 请求。
// 会话归属用户身份由 gRPC metadata 携带，不在请求体传递。
func ConvertToProtoMarkReadRequest(dto *MarkReadRequest) *msgpb.MarkReadRequest {
	if dto == nil {
		return nil
	}
	return &msgpb.MarkReadRequest{
		ConvId:  dto.ConvID,
		ReadSeq: dto.ReadSeq,
	}
}

// ConvertMarkReadResponseFromProto 将 Protobuf 响应转换为标记已读 DTO
func ConvertMarkReadResponseFromProto(pb *msgpb.MarkReadResponse) *MarkReadResponse {
	if pb == nil {
		return nil
	}
	return &MarkReadResponse{
		UnreadCount: pb.UnreadCount,
	}
}

// ==================== 删除会话 DTO ====================

// DeleteConversationRequest 删除会话请求
type DeleteConversationRequest struct {
	ConvID string `json:"convId"`
}

// ConvertToProtoDeleteConversationRequest 将删除会话 DTO 转换为 Protobuf 请求。
// 会话归属用户身份由 gRPC metadata 携带，不在请求体传递。
func ConvertToProtoDeleteConversationRequest(dto *DeleteConversationRequest) *msgpb.DeleteConversationRequest {
	if dto == nil {
		return nil
	}
	return &msgpb.DeleteConversationRequest{
		ConvId: dto.ConvID,
	}
}

// ==================== 更新会话设置 DTO ====================

// UpdateConvSettingsRequest 更新会话设置请求
type UpdateConvSettingsRequest struct {
	ConvID string `json:"convId" binding:"required"`
	Mute   *bool  `json:"mute"`
	Pin    *bool  `json:"pin"`
}

// ConvertToProtoUpdateConvSettingsRequest 将更新会话设置 DTO 转换为 Protobuf 请求。
// 会话归属用户身份由 gRPC metadata 携带，不在请求体传递。
func ConvertToProtoUpdateConvSettingsRequest(dto *UpdateConvSettingsRequest) *msgpb.UpdateConvSettingsRequest {
	if dto == nil {
		return nil
	}
	req := &msgpb.UpdateConvSettingsRequest{
		ConvId: dto.ConvID,
	}
	if dto.Mute != nil {
		req.Mute = dto.Mute
	}
	if dto.Pin != nil {
		req.Pin = dto.Pin
	}
	return req
}

// ==================== 嵌套 DTO ====================

// MsgItemDTO 消息项 DTO
type MsgItemDTO struct {
	MsgID        string   `json:"msgId"`
	ClientMsgID  string   `json:"clientMsgId"`
	ConvID       string   `json:"convId"`
	Seq          int64    `json:"seq"`
	FromUUID     string   `json:"fromUuid"`
	MsgType      int32    `json:"msgType"`
	Content      string   `json:"content"`
	Status       int32    `json:"status"`
	SendTime     int64    `json:"sendTime"`
	ReplyToMsgID string   `json:"replyToMsgId"`
	AtUsers      []string `json:"atUsers"`
}

// LastMsgPreviewDTO 最后消息预览 DTO
type LastMsgPreviewDTO struct {
	MsgID       string `json:"msgId"`
	PreviewJSON string `json:"previewJson"`
	SendTime    int64  `json:"sendTime"`
}

// ConversationItemDTO 会话列表项 DTO
type ConversationItemDTO struct {
	ConvID      string             `json:"convId"`
	ConvType    int32              `json:"convType"`
	TargetUUID  string             `json:"targetUuid"`
	LastMsg     *LastMsgPreviewDTO `json:"lastMsg"`
	UnreadCount int32              `json:"unreadCount"`
	Mute        bool               `json:"mute"`
	Pin         bool               `json:"pin"`
	UpdatedAt   int64              `json:"updatedAt"`
}

// ==================== 嵌套 DTO 转换函数 ====================

// ConvertMsgItemFromProto 将 Protobuf 消息项转换为 DTO
func ConvertMsgItemFromProto(pb *msgpb.MsgItem) *MsgItemDTO {
	if pb == nil {
		return nil
	}
	atUsers := pb.AtUsers
	if atUsers == nil {
		atUsers = []string{}
	}
	return &MsgItemDTO{
		MsgID:        pb.MsgId,
		ClientMsgID:  pb.ClientMsgId,
		ConvID:       pb.ConvId,
		Seq:          pb.Seq,
		FromUUID:     pb.FromUuid,
		MsgType:      pb.MsgType,
		Content:      pb.Content,
		Status:       pb.Status,
		SendTime:     pb.SendTime,
		ReplyToMsgID: pb.ReplyToMsgId,
		AtUsers:      atUsers,
	}
}

// ConvertMsgItemsFromProto 批量将 Protobuf 消息项转换为 DTO
func ConvertMsgItemsFromProto(pbs []*msgpb.MsgItem) []*MsgItemDTO {
	if pbs == nil {
		return []*MsgItemDTO{}
	}
	result := make([]*MsgItemDTO, 0, len(pbs))
	for _, pb := range pbs {
		result = append(result, ConvertMsgItemFromProto(pb))
	}
	return result
}

// ConvertLastMsgPreviewFromProto 将 Protobuf 最后消息预览转换为 DTO
func ConvertLastMsgPreviewFromProto(pb *msgpb.LastMsgPreview) *LastMsgPreviewDTO {
	if pb == nil {
		return nil
	}
	return &LastMsgPreviewDTO{
		MsgID:       pb.MsgId,
		PreviewJSON: pb.PreviewJson,
		SendTime:    pb.SendTime,
	}
}

// ConvertConversationItemFromProto 将 Protobuf 会话列表项转换为 DTO
func ConvertConversationItemFromProto(pb *msgpb.ConversationItem) *ConversationItemDTO {
	if pb == nil {
		return nil
	}
	return &ConversationItemDTO{
		ConvID:      pb.ConvId,
		ConvType:    int32(pb.ConvType),
		TargetUUID:  pb.TargetUuid,
		LastMsg:     ConvertLastMsgPreviewFromProto(pb.LastMsg),
		UnreadCount: pb.UnreadCount,
		Mute:        pb.Mute,
		Pin:         pb.Pin,
		UpdatedAt:   pb.UpdatedAt,
	}
}

// ConvertConversationItemsFromProto 批量将 Protobuf 会话列表项转换为 DTO
func ConvertConversationItemsFromProto(pbs []*msgpb.ConversationItem) []*ConversationItemDTO {
	if pbs == nil {
		return []*ConversationItemDTO{}
	}
	result := make([]*ConversationItemDTO, 0, len(pbs))
	for _, pb := range pbs {
		result = append(result, ConvertConversationItemFromProto(pb))
	}
	return result
}
