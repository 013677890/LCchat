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
