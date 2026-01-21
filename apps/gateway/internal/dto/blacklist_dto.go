package dto

import (
	userpb "ChatServer/apps/user/pb"
)

// ==================== 黑名单服务相关 DTO ====================

// AddBlacklistRequest 拉黑用户请求 DTO
type AddBlacklistRequest struct {
	TargetUUID string `json:"targetUuid" binding:"required"` // 目标用户UUID
}

// AddBlacklistResponse 拉黑用户响应 DTO
type AddBlacklistResponse struct{}

// RemoveBlacklistRequest 取消拉黑请求 DTO
type RemoveBlacklistRequest struct {
	UserUUID string `json:"userUuid" binding:"required"` // 用户UUID
}

// RemoveBlacklistResponse 取消拉黑响应 DTO
type RemoveBlacklistResponse struct{}

// GetBlacklistListRequest 获取黑名单列表请求 DTO
type GetBlacklistListRequest struct {
	Page     int32 `json:"page" binding:"min=1"`             // 页码
	PageSize int32 `json:"pageSize" binding:"min=1,max=100"` // 每页大小
}

// GetBlacklistListResponse 获取黑名单列表响应 DTO
type GetBlacklistListResponse struct {
	Items      []*BlacklistItem `json:"items"`      // 黑名单列表
	Pagination *PaginationInfo  `json:"pagination"` // 分页信息
}

// BlacklistItem 黑名单项 DTO
type BlacklistItem struct {
	UUID          string `json:"uuid"`          // 用户UUID
	Nickname      string `json:"nickname"`      // 昵称
	Avatar        string `json:"avatar"`        // 头像
	BlacklistedAt int64  `json:"blacklistedAt"` // 拉黑时间（毫秒时间戳）
}

// CheckIsBlacklistRequest 判断是否拉黑请求 DTO
type CheckIsBlacklistRequest struct {
	UserUUID   string `json:"userUuid" binding:"required"`   // 当前用户UUID
	TargetUUID string `json:"targetUuid" binding:"required"` // 目标用户UUID
}

// CheckIsBlacklistResponse 判断是否拉黑响应 DTO
type CheckIsBlacklistResponse struct {
	IsBlacklist bool `json:"isBlacklist"` // 是否拉黑
}

// ==================== 黑名单服务 DTO 转换函数 ====================

// ConvertToProtoAddBlacklistRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoAddBlacklistRequest(dto *AddBlacklistRequest) *userpb.AddBlacklistRequest {
	if dto == nil {
		return nil
	}
	return &userpb.AddBlacklistRequest{
		TargetUuid: dto.TargetUUID,
	}
}

// ConvertToProtoRemoveBlacklistRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoRemoveBlacklistRequest(dto *RemoveBlacklistRequest) *userpb.RemoveBlacklistRequest {
	if dto == nil {
		return nil
	}
	return &userpb.RemoveBlacklistRequest{
		UserUuid: dto.UserUUID,
	}
}

// ConvertToProtoCheckIsBlacklistRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoCheckIsBlacklistRequest(dto *CheckIsBlacklistRequest) *userpb.CheckIsBlacklistRequest {
	if dto == nil {
		return nil
	}
	return &userpb.CheckIsBlacklistRequest{
		UserUuid:   dto.UserUUID,
		TargetUuid: dto.TargetUUID,
	}
}

// ConvertBlacklistItemFromProto 将 Protobuf 黑名单项转换为 DTO
func ConvertBlacklistItemFromProto(pb *userpb.BlacklistItem) *BlacklistItem {
	if pb == nil {
		return nil
	}
	return &BlacklistItem{
		UUID:          pb.Uuid,
		Nickname:      pb.Nickname,
		Avatar:        pb.Avatar,
		BlacklistedAt: pb.BlacklistedAt,
	}
}

// ConvertBlacklistItemsFromProto 批量将 Protobuf 黑名单项转换为 DTO
func ConvertBlacklistItemsFromProto(pbs []*userpb.BlacklistItem) []*BlacklistItem {
	if pbs == nil {
		return []*BlacklistItem{}
	}

	result := make([]*BlacklistItem, 0, len(pbs))
	for _, pb := range pbs {
		result = append(result, ConvertBlacklistItemFromProto(pb))
	}
	return result
}

// ==================== 黑名单服务 gRPC响应到DTO转换函数 ====================

// ConvertAddBlacklistResponseFromProto 将 Protobuf 拉黑用户响应转换为 DTO
func ConvertAddBlacklistResponseFromProto(pb *userpb.AddBlacklistResponse) *AddBlacklistResponse {
	if pb == nil {
		return nil
	}
	return &AddBlacklistResponse{}
}

// ConvertRemoveBlacklistResponseFromProto 将 Protobuf 取消拉黑响应转换为 DTO
func ConvertRemoveBlacklistResponseFromProto(pb *userpb.RemoveBlacklistResponse) *RemoveBlacklistResponse {
	if pb == nil {
		return nil
	}
	return &RemoveBlacklistResponse{}
}

// ConvertGetBlacklistListResponseFromProto 将 Protobuf 获取黑名单列表响应转换为 DTO
func ConvertGetBlacklistListResponseFromProto(pb *userpb.GetBlacklistListResponse) *GetBlacklistListResponse {
	if pb == nil {
		return nil
	}

	items := ConvertBlacklistItemsFromProto(pb.Items)

	return &GetBlacklistListResponse{
		Items:      items,
		Pagination: ConvertPaginationInfoFromProto(pb.Pagination),
	}
}

// ConvertCheckIsBlacklistResponseFromProto 将 Protobuf 判断是否拉黑响应转换为 DTO
func ConvertCheckIsBlacklistResponseFromProto(pb *userpb.CheckIsBlacklistResponse) *CheckIsBlacklistResponse {
	if pb == nil {
		return nil
	}
	return &CheckIsBlacklistResponse{
		IsBlacklist: pb.IsBlacklist,
	}
}
