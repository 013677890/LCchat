package dto

import (
	userpb "ChatServer/apps/user/pb"
)

// ==================== 通用 DTO 定义 ====================

// UserInfo 用户信息 DTO
type UserInfo struct {
	UUID      string `json:"uuid"`      // 用户UUID
	Nickname  string `json:"nickname"`  // 昵称
	Telephone string `json:"telephone"` // 手机号
	Email     string `json:"email"`     // 邮箱
	Avatar    string `json:"avatar"`    // 头像
	Gender    int8   `json:"gender"`    // 性别(0:男 1:女 2:未知)
	Signature string `json:"signature"` // 个性签名
	Birthday  string `json:"birthday"`  // 生日(YYYY-MM-DD)
	Status    int8   `json:"status"`    // 状态(0:正常 1:禁用)
}

// SimpleUserInfo 简化用户信息 DTO
type SimpleUserInfo struct {
	UUID     string `json:"uuid"`     // 用户UUID
	Nickname string `json:"nickname"` // 昵称
	Avatar   string `json:"avatar"`   // 头像URL
}

// DeviceInfo 设备信息 DTO（通用类型）
type DeviceInfo struct {
	DeviceName string `json:"deviceName"` // 设备名称
	Platform   string `json:"platform"`   // 平台(iOS/Android/Web)
	OSVersion  string `json:"osVersion"`  // 系统版本
	AppVersion string `json:"appVersion"` // 应用版本
}

// PaginationInfo 分页信息 DTO
type PaginationInfo struct {
	Page     int32 `json:"page"`     // 当前页码
	PageSize int32 `json:"pageSize"` // 每页大小
	Total    int64 `json:"total"`    // 总记录数
}

// ==================== 通用 DTO 转换函数 ====================

// ConvertUserInfoFromProto 将 Protobuf 用户信息转换为 DTO
func ConvertUserInfoFromProto(pb *userpb.UserInfo) *UserInfo {
	if pb == nil {
		return nil
	}
	return &UserInfo{
		UUID:      pb.Uuid,
		Nickname:  pb.Nickname,
		Telephone: pb.Telephone,
		Email:     pb.Email,
		Avatar:    pb.Avatar,
		Gender:    int8(pb.Gender),
		Signature: pb.Signature,
		Birthday:  pb.Birthday,
		Status:    int8(pb.Status),
	}
}

// ConvertSimpleUserInfoFromProto 将 Protobuf 简化用户信息转换为 DTO
func ConvertSimpleUserInfoFromProto(pb *userpb.SimpleUserInfo) *SimpleUserInfo {
	if pb == nil {
		return nil
	}
	return &SimpleUserInfo{
		UUID:     pb.Uuid,
		Nickname: pb.Nickname,
		Avatar:   pb.Avatar,
	}
}

// ConvertSimpleUserItemsFromProto 批量将 Protobuf 简化用户信息转换为 DTO
func ConvertSimpleUserItemsFromProto(pbs []*userpb.SimpleUserInfo) []*SimpleUserInfo {
	if pbs == nil {
		return []*SimpleUserInfo{}
	}

	result := make([]*SimpleUserInfo, 0, len(pbs))
	for _, pb := range pbs {
		result = append(result, ConvertSimpleUserInfoFromProto(pb))
	}
	return result
}

// ConvertDeviceInfoFromProto 将 Protobuf 设备信息转换为 DTO
func ConvertDeviceInfoFromProto(pb *userpb.DeviceInfo) *DeviceInfo {
	if pb == nil {
		return nil
	}
	return &DeviceInfo{
		DeviceName: pb.DeviceName,
		Platform:   pb.Platform,
		OSVersion:  pb.OsVersion,
		AppVersion: pb.AppVersion,
	}
}

// ConvertPaginationInfoFromProto 将 Protobuf 分页信息转换为 DTO
func ConvertPaginationInfoFromProto(pb *userpb.PaginationInfo) *PaginationInfo {
	if pb == nil {
		return nil
	}
	return &PaginationInfo{
		Page:     pb.Page,
		PageSize: pb.PageSize,
		Total:    pb.Total,
	}
}
