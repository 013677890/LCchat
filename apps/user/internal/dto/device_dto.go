package dto

import (
	pb "ChatServer/apps/user/pb"
	"ChatServer/model"
)

// ==================== 设备服务 DTO ====================

// GetDeviceListRequest 获取设备列表请求DTO
type GetDeviceListRequest struct{}

// DeviceItem 设备项DTO
type DeviceItem struct {
	DeviceID        string // 设备ID
	DeviceName      string // 设备名称
	Platform        string // 平台
	AppVersion      string // 应用版本
	IsCurrentDevice bool   // 是否当前设备
	Status          int32  // 状态
	LastSeenAt      int64  // 最后活跃时间（毫秒时间戳）
}

// GetDeviceListResponse 获取设备列表响应DTO
type GetDeviceListResponse struct {
	Devices []*DeviceItem // 设备列表
}

// KickDeviceRequestNew 踢出设备请求DTO
type KickDeviceRequestNew struct {
	DeviceID string // 设备ID
}

// KickDeviceResponseNew 踢出设备响应DTO
type KickDeviceResponseNew struct{}

// GetOnlineStatusRequest 获取在线状态请求DTO
type GetOnlineStatusRequest struct {
	UserUUID string // 用户UUID
}

// OnlineStatus 在线状态DTO（用于单个用户）
type OnlineStatus struct {
	UserUUID        string   // 用户UUID
	IsOnline        bool     // 是否在线
	LastSeenAt      int64    // 最后活跃时间（毫秒时间戳）
	OnlinePlatforms []string // 在线的平台列表
}

// GetOnlineStatusResponse 获取在线状态响应DTO
type GetOnlineStatusResponse struct {
	Status *OnlineStatus // 在线状态
}

// BatchGetOnlineStatusRequest 批量获取在线状态请求DTO
type BatchGetOnlineStatusRequest struct {
	UserUUIDs []string // 用户UUID列表
}

// OnlineStatusItem 在线状态项DTO（用于批量）
type OnlineStatusItem struct {
	UserUUID   string // 用户UUID
	IsOnline   bool   // 是否在线
	LastSeenAt int64  // 最后活跃时间（毫秒时间戳）
}

// BatchGetOnlineStatusResponse 批量获取在线状态响应DTO
type BatchGetOnlineStatusResponse struct {
	Users []*OnlineStatusItem // 在线状态列表
}

// ==================== 转换函数 ====================

// ConvertDeviceSessionModelToDTO 将数据库模型转换为设备DTO
func ConvertDeviceSessionModelToDTO(session *model.DeviceSession, currentDeviceID string) *DeviceItem {
	if session == nil {
		return nil
	}

	item := &DeviceItem{
		DeviceID:        session.DeviceId,
		DeviceName:      session.DeviceName,
		Platform:        session.Platform,
		AppVersion:      session.AppVersion,
		Status:          int32(session.Status),
		IsCurrentDevice: session.DeviceId == currentDeviceID,
	}

	if session.LastSeenAt != nil {
		item.LastSeenAt = session.LastSeenAt.Unix() * 1000
	}

	return item
}

// ConvertDeviceSessionModelsToDTO 批量转换
func ConvertDeviceSessionModelsToDTO(sessions []*model.DeviceSession, currentDeviceID string) []*DeviceItem {
	if sessions == nil {
		return []*DeviceItem{}
	}

	result := make([]*DeviceItem, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, ConvertDeviceSessionModelToDTO(session, currentDeviceID))
	}
	return result
}

// ConvertDeviceItemToProto 将设备DTO转换为Protobuf消息
func ConvertDeviceItemToProto(item *DeviceItem) *pb.DeviceItem {
	if item == nil {
		return nil
	}
	return &pb.DeviceItem{
		DeviceId:        item.DeviceID,
		DeviceName:      item.DeviceName,
		Platform:        item.Platform,
		AppVersion:      item.AppVersion,
		IsCurrentDevice: item.IsCurrentDevice,
		Status:          item.Status,
		LastSeenAt:      item.LastSeenAt,
	}
}

// ConvertDeviceItemListToProto 批量转换
func ConvertDeviceItemListToProto(items []*DeviceItem) []*pb.DeviceItem {
	if items == nil {
		return []*pb.DeviceItem{}
	}

	result := make([]*pb.DeviceItem, 0, len(items))
	for _, item := range items {
		result = append(result, ConvertDeviceItemToProto(item))
	}
	return result
}

// ConvertOnlineStatusToProto 将在线状态DTO转换为Protobuf消息
func ConvertOnlineStatusToProto(status *OnlineStatus) *pb.OnlineStatus {
	if status == nil {
		return nil
	}
	return &pb.OnlineStatus{
		UserUuid:        status.UserUUID,
		IsOnline:        status.IsOnline,
		LastSeenAt:      status.LastSeenAt,
		OnlinePlatforms: status.OnlinePlatforms,
	}
}

// ConvertOnlineStatusItemToProto 将在线状态项DTO转换为Protobuf消息
func ConvertOnlineStatusItemToProto(item *OnlineStatusItem) *pb.OnlineStatusItem {
	if item == nil {
		return nil
	}
	return &pb.OnlineStatusItem{
		UserUuid:   item.UserUUID,
		IsOnline:   item.IsOnline,
		LastSeenAt: item.LastSeenAt,
	}
}

// ConvertOnlineStatusItemListToProto 批量转换
func ConvertOnlineStatusItemListToProto(items []*OnlineStatusItem) []*pb.OnlineStatusItem {
	if items == nil {
		return []*pb.OnlineStatusItem{}
	}

	result := make([]*pb.OnlineStatusItem, 0, len(items))
	for _, item := range items {
		result = append(result, ConvertOnlineStatusItemToProto(item))
	}
	return result
}
