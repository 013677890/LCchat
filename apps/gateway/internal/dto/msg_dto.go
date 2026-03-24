package dto

import (
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
)

// ==================== 消息发送 DTO ====================

// SendMessageRequest 发送消息请求
type SendMessageRequest struct {
	ClientMsgID  string   `json:"clientMsgId" binding:"required"`
	ConvType     int32    `json:"convType" binding:"required"`
	TargetUUID   string   `json:"targetUuid" binding:"required"`
	MsgType      int32    `json:"msgType" binding:"required"`
	Content      string   `json:"content" binding:"required"`
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

// ConvertToProtoSendMessageRequest 将发送消息 DTO 转换为 Protobuf 请求
// fromUUID 和 deviceID 由 Gateway 从 JWT context 注入
func ConvertToProtoSendMessageRequest(dto *SendMessageRequest, fromUUID, deviceID string) *msgpb.SendMessageRequest {
	if dto == nil {
		return nil
	}
	return &msgpb.SendMessageRequest{
		FromUuid:     fromUUID,
		DeviceId:     deviceID,
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
	Limit     int32  `form:"limit"`
	Direction int32  `form:"direction"`
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

// ==================== 消息反查 DTO ====================

// GetMessagesByIdsRequest 批量获取消息请求
type GetMessagesByIdsRequest struct {
	ConvID string   `json:"convId" binding:"required"`
	MsgIDs []string `json:"msgIds" binding:"required"`
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
	ConvID string `json:"convId" binding:"required"`
	MsgID  string `json:"msgId" binding:"required"`
}

// ConvertToProtoRecallMessageRequest 将撤回消息 DTO 转换为 Protobuf 请求
// operatorUUID 由 Gateway 从 JWT context 注入
func ConvertToProtoRecallMessageRequest(dto *RecallMessageRequest, operatorUUID string) *msgpb.RecallMessageRequest {
	if dto == nil {
		return nil
	}
	return &msgpb.RecallMessageRequest{
		ConvId:       dto.ConvID,
		MsgId:        dto.MsgID,
		OperatorUuid: operatorUUID,
	}
}

// ==================== 会话列表 DTO ====================

// GetConversationsRequest 获取会话列表请求（Query 绑定）
type GetConversationsRequest struct {
	UpdatedSince int64  `form:"updatedSince"`
	PageSize     int32  `form:"pageSize"`
	Cursor       string `form:"cursor"`
}

// GetConversationsResponse 获取会话列表响应
type GetConversationsResponse struct {
	Conversations []*ConversationItemDTO `json:"conversations"`
	HasMore       bool                   `json:"hasMore"`
	NextCursor    string                 `json:"nextCursor"`
}

// ConvertToProtoGetConversationsRequest 将获取会话列表 DTO 转换为 Protobuf 请求
// ownerUUID 由 Gateway 从 JWT context 注入
func ConvertToProtoGetConversationsRequest(dto *GetConversationsRequest, ownerUUID string) *msgpb.GetConversationsRequest {
	if dto == nil {
		return nil
	}
	return &msgpb.GetConversationsRequest{
		OwnerUuid:    ownerUUID,
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
	ReadSeq int64  `json:"readSeq" binding:"required"`
}

// MarkReadResponse 标记已读响应
type MarkReadResponse struct {
	UnreadCount int32 `json:"unreadCount"`
}

// ConvertToProtoMarkReadRequest 将标记已读 DTO 转换为 Protobuf 请求
// ownerUUID 由 Gateway 从 JWT context 注入
func ConvertToProtoMarkReadRequest(dto *MarkReadRequest, ownerUUID string) *msgpb.MarkReadRequest {
	if dto == nil {
		return nil
	}
	return &msgpb.MarkReadRequest{
		ConvId:    dto.ConvID,
		OwnerUuid: ownerUUID,
		ReadSeq:   dto.ReadSeq,
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

// ConvertToProtoDeleteConversationRequest 将删除会话 DTO 转换为 Protobuf 请求
// ownerUUID 由 Gateway 从 JWT context 注入
func ConvertToProtoDeleteConversationRequest(dto *DeleteConversationRequest, ownerUUID string) *msgpb.DeleteConversationRequest {
	if dto == nil {
		return nil
	}
	return &msgpb.DeleteConversationRequest{
		ConvId:    dto.ConvID,
		OwnerUuid: ownerUUID,
	}
}

// ==================== 更新会话设置 DTO ====================

// UpdateConvSettingsRequest 更新会话设置请求
type UpdateConvSettingsRequest struct {
	ConvID string `json:"convId" binding:"required"`
	Mute   *bool  `json:"mute"`
	Pin    *bool  `json:"pin"`
}

// ConvertToProtoUpdateConvSettingsRequest 将更新会话设置 DTO 转换为 Protobuf 请求
// ownerUUID 由 Gateway 从 JWT context 注入
func ConvertToProtoUpdateConvSettingsRequest(dto *UpdateConvSettingsRequest, ownerUUID string) *msgpb.UpdateConvSettingsRequest {
	if dto == nil {
		return nil
	}
	req := &msgpb.UpdateConvSettingsRequest{
		ConvId:    dto.ConvID,
		OwnerUuid: ownerUUID,
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
