package dto

import (
	"ChatServer/model"
	pb "ChatServer/apps/user/pb"
)

// ==================== 黑名单服务 DTO ====================

// AddBlacklistRequest 拉黑用户请求DTO
type AddBlacklistRequest struct {
	TargetUUID string // 目标用户UUID
}

// AddBlacklistResponse 拉黑用户响应DTO
type AddBlacklistResponse struct{}

// RemoveBlacklistRequest 取消拉黑请求DTO
type RemoveBlacklistRequest struct {
	UserUUID string // 用户UUID
}

// RemoveBlacklistResponse 取消拉黑响应DTO
type RemoveBlacklistResponse struct{}

// GetBlacklistListRequest 获取黑名单列表请求DTO
type GetBlacklistListRequest struct {
	Page     int32 // 页码
	PageSize int32 // 每页大小
}

// BlacklistItem 黑名单项DTO
type BlacklistItem struct {
	UUID          string // 用户UUID
	Nickname      string // 昵称
	Avatar        string // 头像
	BlacklistedAt int64  // 拉黑时间（毫秒时间戳）
}

// GetBlacklistListResponse 获取黑名单列表响应DTO
type GetBlacklistListResponse struct {
	Items      []*BlacklistItem   // 黑名单列表
	Pagination *pb.PaginationInfo // 分页信息
}

// CheckIsBlacklistRequest 判断是否拉黑请求DTO
type CheckIsBlacklistRequest struct {
	UserUUID   string // 当前用户UUID
	TargetUUID string // 目标用户UUID
}

// CheckIsBlacklistResponse 判断是否拉黑响应DTO
type CheckIsBlacklistResponse struct {
	IsBlacklist bool // 是否拉黑
}

// ==================== 黑名单转换函数 ====================

// ConvertBlacklistModelToDTO 将数据库模型转换为黑名单DTO
func ConvertBlacklistModelToDTO(relation *model.UserRelation, user *model.UserInfo) *BlacklistItem {
	if relation == nil {
		return nil
	}

	item := &BlacklistItem{
		BlacklistedAt: relation.UpdatedAt.Unix() * 1000,
	}

	if user != nil {
		item.UUID = user.Uuid
		item.Nickname = user.Nickname
		item.Avatar = user.Avatar
	}

	return item
}

// ConvertBlacklistModelsToDTO 批量转换
func ConvertBlacklistModelsToDTO(relations []*model.UserRelation, users []*model.UserInfo) []*BlacklistItem {
	if relations == nil {
		return []*BlacklistItem{}
	}

	// 创建用户映射
	userMap := make(map[string]*model.UserInfo)
	for _, user := range users {
		userMap[user.Uuid] = user
	}

	result := make([]*BlacklistItem, 0, len(relations))
	for _, relation := range relations {
		user := userMap[relation.PeerUuid]
		result = append(result, ConvertBlacklistModelToDTO(relation, user))
	}
	return result
}

// ConvertBlacklistItemToProto 将黑名单DTO转换为Protobuf消息
func ConvertBlacklistItemToProto(item *BlacklistItem) *pb.BlacklistItem {
	if item == nil {
		return nil
	}
	return &pb.BlacklistItem{
		Uuid:          item.UUID,
		Nickname:      item.Nickname,
		Avatar:        item.Avatar,
		BlacklistedAt: item.BlacklistedAt,
	}
}

// ConvertBlacklistItemListToProto 批量转换
func ConvertBlacklistItemListToProto(items []*BlacklistItem) []*pb.BlacklistItem {
	if items == nil {
		return []*pb.BlacklistItem{}
	}

	result := make([]*pb.BlacklistItem, 0, len(items))
	for _, item := range items {
		result = append(result, ConvertBlacklistItemToProto(item))
	}
	return result
}
