package dto

import (
	userpb "ChatServer/apps/user/pb"
)

// ==================== 设备服务相关 DTO ====================

// GetDeviceListRequest 获取设备列表请求 DTO
type GetDeviceListRequest struct{}

// GetDeviceListResponse 获取设备列表响应 DTO
type GetDeviceListResponse struct {
	Devices []*DeviceItem `json:"devices"` // 设备列表
}

// DeviceItem 设备项 DTO
type DeviceItem struct {
	DeviceID        string `json:"deviceId"`        // 设备ID
	DeviceName      string `json:"deviceName"`      // 设备名称
	Platform        string `json:"platform"`        // 平台
	AppVersion      string `json:"appVersion"`      // 应用版本
	IsCurrentDevice bool   `json:"isCurrentDevice"` // 是否当前设备
	Status          int32  `json:"status"`          // 状态(0:在线 1:下线 2:被踢)
	LastSeenAt      int64  `json:"lastSeenAt"`      // 最后活跃时间（毫秒时间戳）
}

// KickDeviceRequest 踢出设备请求 DTO
type KickDeviceRequest struct {
	DeviceID string `json:"deviceId" binding:"required"` // 设备ID
}

// KickDeviceResponse 踢出设备响应 DTO
type KickDeviceResponse struct{}

// GetOnlineStatusRequest 获取在线状态请求 DTO
type GetOnlineStatusRequest struct {
	UserUUID string `json:"userUuid" binding:"required"` // 用户UUID
}

// GetOnlineStatusResponse 获取在线状态响应 DTO
type GetOnlineStatusResponse struct {
	Status *OnlineStatus `json:"status"` // 在线状态
}

// OnlineStatus 在线状态 DTO（用于单个用户）
type OnlineStatus struct {
	UserUUID        string   `json:"userUuid"`        // 用户UUID
	IsOnline        bool     `json:"isOnline"`        // 是否在线
	LastSeenAt      int64    `json:"lastSeenAt"`      // 最后活跃时间（毫秒时间戳）
	OnlinePlatforms []string `json:"onlinePlatforms"` // 在线的平台列表
}

// BatchGetOnlineStatusRequest 批量获取在线状态请求 DTO
type BatchGetOnlineStatusRequest struct {
	UserUUIDs []string `json:"userUuids" binding:"required"` // 用户UUID列表
}

// BatchGetOnlineStatusResponse 批量获取在线状态响应 DTO
type BatchGetOnlineStatusResponse struct {
	Users []*OnlineStatusItem `json:"users"` // 在线状态列表
}

// OnlineStatusItem 在线状态项 DTO（用于批量）
type OnlineStatusItem struct {
	UserUUID   string `json:"userUuid"`   // 用户UUID
	IsOnline   bool   `json:"isOnline"`   // 是否在线
	LastSeenAt int64  `json:"lastSeenAt"` // 最后活跃时间（毫秒时间戳）
}

// ==================== 设备服务 DTO 转换函数 ====================

// ConvertToProtoGetOnlineStatusRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoGetOnlineStatusRequest(dto *GetOnlineStatusRequest) *userpb.GetOnlineStatusRequest {
	if dto == nil {
		return nil
	}
	return &userpb.GetOnlineStatusRequest{
		UserUuid: dto.UserUUID,
	}
}

// ConvertToProtoBatchGetOnlineStatusRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoBatchGetOnlineStatusRequest(dto *BatchGetOnlineStatusRequest) *userpb.BatchGetOnlineStatusRequest {
	if dto == nil {
		return nil
	}
	return &userpb.BatchGetOnlineStatusRequest{
		UserUuids: dto.UserUUIDs,
	}
}

// ConvertToProtoKickDeviceRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoKickDeviceRequest(dto *KickDeviceRequest) *userpb.KickDeviceRequest {
	if dto == nil {
		return nil
	}
	return &userpb.KickDeviceRequest{
		DeviceId: dto.DeviceID,
	}
}

// ConvertDeviceItemFromProto 将 Protobuf 设备项转换为 DTO
func ConvertDeviceItemFromProto(pb *userpb.DeviceItem) *DeviceItem {
	if pb == nil {
		return nil
	}
	return &DeviceItem{
		DeviceID:        pb.DeviceId,
		DeviceName:      pb.DeviceName,
		Platform:        pb.Platform,
		AppVersion:      pb.AppVersion,
		IsCurrentDevice: pb.IsCurrentDevice,
		Status:          pb.Status,
		LastSeenAt:      pb.LastSeenAt,
	}
}

// ConvertOnlineStatusFromProto 将 Protobuf 在线状态转换为 DTO
func ConvertOnlineStatusFromProto(pb *userpb.OnlineStatus) *OnlineStatus {
	if pb == nil {
		return nil
	}
	return &OnlineStatus{
		UserUUID:        pb.UserUuid,
		IsOnline:        pb.IsOnline,
		LastSeenAt:      pb.LastSeenAt,
		OnlinePlatforms: pb.OnlinePlatforms,
	}
}

// ConvertOnlineStatusItemFromProto 将 Protobuf 在线状态项转换为 DTO
func ConvertOnlineStatusItemFromProto(pb *userpb.OnlineStatusItem) *OnlineStatusItem {
	if pb == nil {
		return nil
	}
	return &OnlineStatusItem{
		UserUUID:   pb.UserUuid,
		IsOnline:   pb.IsOnline,
		LastSeenAt: pb.LastSeenAt,
	}
}

// ConvertOnlineStatusItemsFromProto 批量将 Protobuf 在线状态项转换为 DTO
func ConvertOnlineStatusItemsFromProto(pbs []*userpb.OnlineStatusItem) []*OnlineStatusItem {
	if pbs == nil {
		return []*OnlineStatusItem{}
	}

	result := make([]*OnlineStatusItem, 0, len(pbs))
	for _, pb := range pbs {
		result = append(result, ConvertOnlineStatusItemFromProto(pb))
	}
	return result
}
