package dto

import (
	userpb "ChatServer/apps/user/pb"
)

// ==================== 好友服务相关 DTO ====================

// SearchUserRequest 搜索用户请求 DTO
type SearchUserRequest struct {
	Keyword  string `json:"keyword" binding:"required,min=1,max=20"` // 搜索关键字
	Page     int32  `json:"page" binding:"min=1"`                    // 页码
	PageSize int32  `json:"pageSize" binding:"min=1,max=100"`        // 每页大小
}

// SearchUserResponse 搜索用户响应 DTO
type SearchUserResponse struct {
	Items      []*SimpleUserItem `json:"items"`      // 用户列表
	Pagination *PaginationInfo   `json:"pagination"` // 分页信息
}

// SimpleUserItem 简化用户信息 DTO（搜索结果）
type SimpleUserItem struct {
	UUID      string `json:"uuid"`      // 用户UUID
	Nickname  string `json:"nickname"`  // 昵称
	Email     string `json:"email"`     // 邮箱
	Avatar    string `json:"avatar"`    // 头像
	Signature string `json:"signature"` // 个性签名
	IsFriend  bool   `json:"isFriend"`  // 是否好友
}

// SendFriendApplyRequest 发送好友申请请求 DTO
type SendFriendApplyRequest struct {
	TargetUUID string `json:"targetUuid" binding:"required"`      // 目标用户UUID
	Reason     string `json:"reason" binding:"omitempty,max=100"` // 申请理由
	Source     string `json:"source" binding:"omitempty,max=20"`  // 来源
}

// SendFriendApplyResponse 发送好友申请响应 DTO
type SendFriendApplyResponse struct {
	ApplyID int64 `json:"applyId"` // 申请ID
}

// GetFriendApplyListRequest 获取好友申请列表请求 DTO
type GetFriendApplyListRequest struct {
	Status   int32 `json:"status" binding:"omitempty,oneof=0 1 2"` // 状态(0:待处理 1:已同意 2:已拒绝)
	Page     int32 `json:"page" binding:"min=1"`                   // 页码
	PageSize int32 `json:"pageSize" binding:"min=1,max=100"`       // 每页大小
}

// FriendApplyItem 好友申请信息 DTO
type FriendApplyItem struct {
	ApplyID           int64  `json:"applyId"`           // 申请ID
	ApplicantUUID     string `json:"applicantUuid"`     // 申请人UUID
	ApplicantNickname string `json:"applicantNickname"` // 申请人昵称
	ApplicantAvatar   string `json:"applicantAvatar"`   // 申请人头像
	Reason            string `json:"reason"`            // 申请理由
	Source            string `json:"source"`            // 来源
	Status            int32  `json:"status"`            // 状态
	IsRead            bool   `json:"isRead"`            // 是否已读
	CreatedAt         int64  `json:"createdAt"`         // 申请时间（毫秒时间戳）
}

// GetFriendApplyListResponse 获取好友申请列表响应 DTO
type GetFriendApplyListResponse struct {
	Items      []*FriendApplyItem `json:"items"`      // 好友申请列表
	Pagination *PaginationInfo    `json:"pagination"` // 分页信息
}

// GetSentApplyListRequest 获取发出的申请列表请求 DTO
type GetSentApplyListRequest struct {
	Status   int32 `json:"status" binding:"omitempty,oneof=0 1 2"` // 状态
	Page     int32 `json:"page" binding:"min=1"`                   // 页码
	PageSize int32 `json:"pageSize" binding:"min=1,max=100"`       // 每页大小
}

// GetSentApplyListResponse 获取发出的申请列表响应 DTO
type GetSentApplyListResponse struct {
	Items      []*SentApplyItem `json:"items"`      // 发出的申请列表
	Pagination *PaginationInfo  `json:"pagination"` // 分页信息
}

// SentApplyItem 发出的申请项 DTO
type SentApplyItem struct {
	ApplyID    int64           `json:"applyId"`    // 申请ID
	TargetUUID string          `json:"targetUuid"` // 目标用户UUID
	TargetInfo *SimpleUserInfo `json:"targetInfo"` // 目标用户信息
	Status     int32           `json:"status"`     // 状态
	CreatedAt  int64           `json:"createdAt"`  // 申请时间（毫秒时间戳）
}

// HandleFriendApplyRequest 处理好友申请请求 DTO
type HandleFriendApplyRequest struct {
	ApplyID int64  `json:"applyId" binding:"required,gt=0"`     // 申请ID
	Action  int32  `json:"action" binding:"required,oneof=1 2"` // 操作(1:同意 2:拒绝)
	Remark  string `json:"remark" binding:"omitempty,max=100"`  // 处理备注
}

// HandleFriendApplyResponse 处理好友申请响应 DTO
type HandleFriendApplyResponse struct{}

// GetUnreadApplyCountRequest 获取未读申请数量请求 DTO
type GetUnreadApplyCountRequest struct{}

// GetUnreadApplyCountResponse 获取未读申请数量响应 DTO
type GetUnreadApplyCountResponse struct {
	UnreadCount int32 `json:"unreadCount"` // 未读数量
}

// MarkApplyAsReadRequest 标记申请已读请求 DTO
type MarkApplyAsReadRequest struct {
	ApplyIDs []int64 `json:"applyIds" binding:"required"` // 申请ID列表
}

// MarkApplyAsReadResponse 标记申请已读响应 DTO
type MarkApplyAsReadResponse struct{}

// GetFriendListRequest 获取好友列表请求 DTO
type GetFriendListRequest struct {
	GroupTag string `json:"groupTag" binding:"omitempty"`     // 标签
	Page     int32  `json:"page" binding:"min=1"`             // 页码
	PageSize int32  `json:"pageSize" binding:"min=1,max=100"` // 每页大小
}

// FriendItem 好友信息 DTO
type FriendItem struct {
	UUID      string `json:"uuid"`      // 好友UUID
	Nickname  string `json:"nickname"`  // 昵称
	Avatar    string `json:"avatar"`    // 头像
	Gender    int32  `json:"gender"`    // 性别
	Signature string `json:"signature"` // 个性签名
	Remark    string `json:"remark"`    // 备注名
	GroupTag  string `json:"groupTag"`  // 标签
	Source    string `json:"source"`    // 来源
	CreatedAt int64  `json:"createdAt"` // 添加好友时间（毫秒时间戳）
}

// GetFriendListResponse 获取好友列表响应 DTO
type GetFriendListResponse struct {
	Items      []*FriendItem   `json:"items"`      // 好友列表
	Pagination *PaginationInfo `json:"pagination"` // 分页信息
	Version    int64           `json:"version"`    // 版本号
}

// SyncFriendListRequest 增量同步请求 DTO
type SyncFriendListRequest struct {
	Version int64 `json:"version" binding:"min=0"`       // 版本号
	Limit   int32 `json:"limit" binding:"min=1,max=100"` // 每次同步数量
}

// FriendChange 好友变更 DTO
type FriendChange struct {
	UUID       string `json:"uuid"`       // 好友UUID
	Nickname   string `json:"nickname"`   // 昵称
	Avatar     string `json:"avatar"`     // 头像
	Gender     int32  `json:"gender"`     // 性别
	Signature  string `json:"signature"`  // 个性签名
	Remark     string `json:"remark"`     // 备注名
	GroupTag   string `json:"groupTag"`   // 标签
	Source     string `json:"source"`     // 来源
	ChangeType string `json:"changeType"` // 变更类型(add/update/delete)
	ChangedAt  int64  `json:"changedAt"`  // 变更时间（毫秒时间戳）
}

// SyncFriendListResponse 增量同步响应 DTO
type SyncFriendListResponse struct {
	Changes       []*FriendChange `json:"changes"`       // 变更列表
	HasMore       bool            `json:"hasMore"`       // 是否还有更多
	LatestVersion int64           `json:"latestVersion"` // 最新版本号
}

// DeleteFriendRequest 删除好友请求 DTO
type DeleteFriendRequest struct {
	UserUUID string `json:"userUuid" binding:"required"` // 当前用户UUID
}

// DeleteFriendResponse 删除好友响应 DTO
type DeleteFriendResponse struct{}

// SetFriendRemarkRequest 设置好友备注请求 DTO
type SetFriendRemarkRequest struct {
	UserUUID string `json:"userUuid" binding:"required"`      // 用户UUID
	Remark   string `json:"remark" binding:"required,max=64"` // 备注名
}

// SetFriendRemarkResponse 设置好友备注响应 DTO
type SetFriendRemarkResponse struct{}

// SetFriendTagRequest 设置好友标签请求 DTO
type SetFriendTagRequest struct {
	UserUUID string `json:"userUuid" binding:"required"`        // 用户UUID
	GroupTag string `json:"groupTag" binding:"required,max=32"` // 标签
}

// SetFriendTagResponse 设置好友标签响应 DTO
type SetFriendTagResponse struct{}

// GetTagListRequest 获取标签列表请求 DTO
type GetTagListRequest struct{}

// TagItem 标签项 DTO
type TagItem struct {
	TagName string `json:"tagName"` // 标签名
	Count   int32  `json:"count"`   // 数量
}

// GetTagListResponse 获取标签列表响应 DTO
type GetTagListResponse struct {
	Tags []*TagItem `json:"tags"` // 标签列表
}

// CheckIsFriendRequest 判断是否好友请求 DTO
type CheckIsFriendRequest struct {
	UserUUID string `json:"userUuid" binding:"required"` // 当前用户UUID
	PeerUUID string `json:"peerUuid" binding:"required"` // 目标用户UUID
}

// CheckIsFriendResponse 判断是否好友响应 DTO
type CheckIsFriendResponse struct {
	IsFriend bool `json:"isFriend"` // 是否好友
}

// GetRelationStatusRequest 获取关系状态请求 DTO
type GetRelationStatusRequest struct {
	UserUUID string `json:"userUuid" binding:"required"` // 当前用户UUID
	PeerUUID string `json:"peerUuid" binding:"required"` // 目标用户UUID
}

// GetRelationStatusResponse 获取关系状态响应 DTO
type GetRelationStatusResponse struct {
	Relation    string `json:"relation"`    // 关系(none/friend/blacklist/deleted)
	IsFriend    bool   `json:"isFriend"`    // 是否好友
	IsBlacklist bool   `json:"isBlacklist"` // 是否拉黑
	Remark      string `json:"remark"`      // 备注名
	GroupTag    string `json:"groupTag"`    // 标签
}

// ==================== 好友服务 DTO 转换函数 ====================

// ConvertToProtoSearchUserRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoSearchUserRequest(dto *SearchUserRequest) *userpb.SearchUserRequest {
	if dto == nil {
		return nil
	}
	return &userpb.SearchUserRequest{
		Keyword:  dto.Keyword,
		Page:     dto.Page,
		PageSize: dto.PageSize,
	}
}

// ConvertToProtoSendFriendApplyRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoSendFriendApplyRequest(dto *SendFriendApplyRequest) *userpb.SendFriendApplyRequest {
	if dto == nil {
		return nil
	}
	return &userpb.SendFriendApplyRequest{
		TargetUuid: dto.TargetUUID,
		Reason:     dto.Reason,
		Source:     dto.Source,
	}
}

// ConvertToProtoHandleFriendApplyRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoHandleFriendApplyRequest(dto *HandleFriendApplyRequest) *userpb.HandleFriendApplyRequest {
	if dto == nil {
		return nil
	}
	return &userpb.HandleFriendApplyRequest{
		ApplyId: dto.ApplyID,
		Action:  dto.Action,
		Remark:  dto.Remark,
	}
}

// ConvertToProtoMarkApplyAsReadRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoMarkApplyAsReadRequest(dto *MarkApplyAsReadRequest) *userpb.MarkApplyAsReadRequest {
	if dto == nil {
		return nil
	}
	return &userpb.MarkApplyAsReadRequest{
		ApplyIds: dto.ApplyIDs,
	}
}

// ConvertToProtoSetFriendRemarkRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoSetFriendRemarkRequest(dto *SetFriendRemarkRequest) *userpb.SetFriendRemarkRequest {
	if dto == nil {
		return nil
	}
	return &userpb.SetFriendRemarkRequest{
		UserUuid: dto.UserUUID,
		Remark:   dto.Remark,
	}
}

// ConvertToProtoSetFriendTagRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoSetFriendTagRequest(dto *SetFriendTagRequest) *userpb.SetFriendTagRequest {
	if dto == nil {
		return nil
	}
	return &userpb.SetFriendTagRequest{
		UserUuid: dto.UserUUID,
		GroupTag: dto.GroupTag,
	}
}

// ConvertToProtoDeleteFriendRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoDeleteFriendRequest(dto *DeleteFriendRequest) *userpb.DeleteFriendRequest {
	if dto == nil {
		return nil
	}
	return &userpb.DeleteFriendRequest{
		UserUuid: dto.UserUUID,
	}
}

// ConvertToProtoCheckIsFriendRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoCheckIsFriendRequest(dto *CheckIsFriendRequest) *userpb.CheckIsFriendRequest {
	if dto == nil {
		return nil
	}
	return &userpb.CheckIsFriendRequest{
		UserUuid: dto.UserUUID,
		PeerUuid: dto.PeerUUID,
	}
}

// ConvertToProtoGetRelationStatusRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoGetRelationStatusRequest(dto *GetRelationStatusRequest) *userpb.GetRelationStatusRequest {
	if dto == nil {
		return nil
	}
	return &userpb.GetRelationStatusRequest{
		UserUuid: dto.UserUUID,
		PeerUuid: dto.PeerUUID,
	}
}

// ==================== 好友服务 gRPC响应到DTO转换函数 ====================

// ConvertSearchUserResponseFromProto 将 Protobuf 搜索用户响应转换为 DTO
func ConvertSearchUserResponseFromProto(pb *userpb.SearchUserResponse) *SearchUserResponse {
	if pb == nil {
		return nil
	}

	items := make([]*SimpleUserItem, 0, len(pb.Items))
	for _, item := range pb.Items {
		items = append(items, ConvertSimpleUserItemFromProto(item))
	}

	return &SearchUserResponse{
		Items:      items,
		Pagination: ConvertPaginationInfoFromProto(pb.Pagination),
	}
}

// ConvertSimpleUserItemFromProto 将 Protobuf 简化用户项转换为 DTO
func ConvertSimpleUserItemFromProto(pb *userpb.SimpleUserItem) *SimpleUserItem {
	if pb == nil {
		return nil
	}
	return &SimpleUserItem{
		UUID:      pb.Uuid,
		Nickname:  pb.Nickname,
		Email:     pb.Email,
		Avatar:    pb.Avatar,
		Signature: pb.Signature,
		IsFriend:  pb.IsFriend,
	}
}

// ConvertFriendApplyResponseFromProto 将 Protobuf 发送好友申请响应转换为 DTO
func ConvertFriendApplyResponseFromProto(pb *userpb.SendFriendApplyResponse) *SendFriendApplyResponse {
	if pb == nil {
		return nil
	}
	return &SendFriendApplyResponse{
		ApplyID: pb.ApplyId,
	}
}

// ConvertFriendApplyItemFromProto 将 Protobuf 好友申请项转换为 DTO
func ConvertFriendApplyItemFromProto(pb *userpb.FriendApplyItem) *FriendApplyItem {
	if pb == nil {
		return nil
	}

	applicantInfo := ConvertSimpleUserInfoFromProto(pb.ApplicantInfo)

	return &FriendApplyItem{
		ApplyID:           pb.ApplyId,
		ApplicantUUID:     pb.ApplicantUuid,
		ApplicantNickname: applicantInfo.Nickname,
		ApplicantAvatar:   applicantInfo.Avatar,
		Reason:            pb.Reason,
		Source:            pb.Source,
		Status:            pb.Status,
		IsRead:            pb.IsRead,
		CreatedAt:         pb.CreatedAt,
	}
}

// ConvertGetFriendApplyListResponseFromProto 将 Protobuf 获取好友申请列表响应转换为 DTO
func ConvertGetFriendApplyListResponseFromProto(pb *userpb.GetFriendApplyListResponse) *GetFriendApplyListResponse {
	if pb == nil {
		return nil
	}

	items := make([]*FriendApplyItem, 0, len(pb.Items))
	for _, item := range pb.Items {
		items = append(items, ConvertFriendApplyItemFromProto(item))
	}

	return &GetFriendApplyListResponse{
		Items:      items,
		Pagination: ConvertPaginationInfoFromProto(pb.Pagination),
	}
}

// ConvertSentApplyItemFromProto 将 Protobuf 发出的申请项转换为 DTO
func ConvertSentApplyItemFromProto(pb *userpb.SentApplyItem) *SentApplyItem {
	if pb == nil {
		return nil
	}
	return &SentApplyItem{
		ApplyID:    pb.ApplyId,
		TargetUUID:  pb.TargetUuid,
		TargetInfo: ConvertSimpleUserInfoFromProto(pb.TargetInfo),
		Status:     pb.Status,
		CreatedAt:  pb.CreatedAt,
	}
}

// ConvertGetSentApplyListResponseFromProto 将 Protobuf 获取发出的申请列表响应转换为 DTO
func ConvertGetSentApplyListResponseFromProto(pb *userpb.GetSentApplyListResponse) *GetSentApplyListResponse {
	if pb == nil {
		return nil
	}

	items := make([]*SentApplyItem, 0, len(pb.Items))
	for _, item := range pb.Items {
		items = append(items, ConvertSentApplyItemFromProto(item))
	}

	return &GetSentApplyListResponse{
		Items:      items,
		Pagination: ConvertPaginationInfoFromProto(pb.Pagination),
	}
}

// ConvertHandleFriendApplyResponseFromProto 将 Protobuf 处理好友申请响应转换为 DTO
func ConvertHandleFriendApplyResponseFromProto(pb *userpb.HandleFriendApplyResponse) *HandleFriendApplyResponse {
	if pb == nil {
		return nil
	}
	return &HandleFriendApplyResponse{}
}

// ConvertGetUnreadApplyCountResponseFromProto 将 Protobuf 获取未读申请数量响应转换为 DTO
func ConvertGetUnreadApplyCountResponseFromProto(pb *userpb.GetUnreadApplyCountResponse) *GetUnreadApplyCountResponse {
	if pb == nil {
		return nil
	}
	return &GetUnreadApplyCountResponse{
		UnreadCount: pb.UnreadCount,
	}
}

// ConvertMarkApplyAsReadResponseFromProto 将 Protobuf 标记申请已读响应转换为 DTO
func ConvertMarkApplyAsReadResponseFromProto(pb *userpb.MarkApplyAsReadResponse) *MarkApplyAsReadResponse {
	if pb == nil {
		return nil
	}
	return &MarkApplyAsReadResponse{}
}

// ConvertFriendItemFromProto 将 Protobuf 好友项转换为 DTO
func ConvertFriendItemFromProto(pb *userpb.FriendItem) *FriendItem {
	if pb == nil {
		return nil
	}
	return &FriendItem{
		UUID:      pb.Uuid,
		Nickname:  pb.Nickname,
		Avatar:    pb.Avatar,
		Gender:    pb.Gender,
		Signature: pb.Signature,
		Remark:    pb.Remark,
		GroupTag:  pb.GroupTag,
		Source:    pb.Source,
		CreatedAt: pb.CreatedAt,
	}
}

// ConvertGetFriendListResponseFromProto 将 Protobuf 获取好友列表响应转换为 DTO
func ConvertGetFriendListResponseFromProto(pb *userpb.GetFriendListResponse) *GetFriendListResponse {
	if pb == nil {
		return nil
	}

	items := make([]*FriendItem, 0, len(pb.Items))
	for _, item := range pb.Items {
		items = append(items, ConvertFriendItemFromProto(item))
	}

	return &GetFriendListResponse{
		Items:      items,
		Pagination: ConvertPaginationInfoFromProto(pb.Pagination),
		Version:    pb.Version,
	}
}

// ConvertFriendChangeFromProto 将 Protobuf 好友变更转换为 DTO
func ConvertFriendChangeFromProto(pb *userpb.FriendChange) *FriendChange {
	if pb == nil {
		return nil
	}
	return &FriendChange{
		UUID:       pb.Uuid,
		Nickname:   pb.Nickname,
		Avatar:     pb.Avatar,
		Gender:     pb.Gender,
		Signature:  pb.Signature,
		Remark:     pb.Remark,
		GroupTag:   pb.GroupTag,
		Source:     pb.Source,
		ChangeType: pb.ChangeType,
		ChangedAt:  pb.ChangedAt,
	}
}

// ConvertSyncFriendListResponseFromProto 将 Protobuf 增量同步响应转换为 DTO
func ConvertSyncFriendListResponseFromProto(pb *userpb.SyncFriendListResponse) *SyncFriendListResponse {
	if pb == nil {
		return nil
	}

	changes := make([]*FriendChange, 0, len(pb.Changes))
	for _, change := range pb.Changes {
		changes = append(changes, ConvertFriendChangeFromProto(change))
	}

	return &SyncFriendListResponse{
		Changes:       changes,
		HasMore:       pb.HasMore,
		LatestVersion: pb.LatestVersion,
	}
}

// ConvertDeleteFriendResponseFromProto 将 Protobuf 删除好友响应转换为 DTO
func ConvertDeleteFriendResponseFromProto(pb *userpb.DeleteFriendResponse) *DeleteFriendResponse {
	if pb == nil {
		return nil
	}
	return &DeleteFriendResponse{}
}

// ConvertSetFriendRemarkResponseFromProto 将 Protobuf 设置好友备注响应转换为 DTO
func ConvertSetFriendRemarkResponseFromProto(pb *userpb.SetFriendRemarkResponse) *SetFriendRemarkResponse {
	if pb == nil {
		return nil
	}
	return &SetFriendRemarkResponse{}
}

// ConvertSetFriendTagResponseFromProto 将 Protobuf 设置好友标签响应转换为 DTO
func ConvertSetFriendTagResponseFromProto(pb *userpb.SetFriendTagResponse) *SetFriendTagResponse {
	if pb == nil {
		return nil
	}
	return &SetFriendTagResponse{}
}

// ConvertTagItemFromProto 将 Protobuf 标签项转换为 DTO
func ConvertTagItemFromProto(pb *userpb.TagItem) *TagItem {
	if pb == nil {
		return nil
	}
	return &TagItem{
		TagName: pb.TagName,
		Count:   pb.Count,
	}
}

// ConvertGetTagListResponseFromProto 将 Protobuf 获取标签列表响应转换为 DTO
func ConvertGetTagListResponseFromProto(pb *userpb.GetTagListResponse) *GetTagListResponse {
	if pb == nil {
		return nil
	}

	tags := make([]*TagItem, 0, len(pb.Tags))
	for _, tag := range pb.Tags {
		tags = append(tags, ConvertTagItemFromProto(tag))
	}

	return &GetTagListResponse{
		Tags: tags,
	}
}

// ConvertCheckIsFriendResponseFromProto 将 Protobuf 判断是否好友响应转换为 DTO
func ConvertCheckIsFriendResponseFromProto(pb *userpb.CheckIsFriendResponse) *CheckIsFriendResponse {
	if pb == nil {
		return nil
	}
	return &CheckIsFriendResponse{
		IsFriend: pb.IsFriend,
	}
}

// ConvertGetRelationStatusResponseFromProto 将 Protobuf 获取关系状态响应转换为 DTO
func ConvertGetRelationStatusResponseFromProto(pb *userpb.GetRelationStatusResponse) *GetRelationStatusResponse {
	if pb == nil {
		return nil
	}
	return &GetRelationStatusResponse{
		Relation:    pb.Relation,
		IsFriend:    pb.IsFriend,
		IsBlacklist: pb.IsBlacklist,
		Remark:      pb.Remark,
		GroupTag:    pb.GroupTag,
	}
}