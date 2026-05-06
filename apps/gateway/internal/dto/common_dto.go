package dto

import (
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	relationpb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
	commonpb "github.com/013677890/LCchat-Backend/pkg/commonpb"
)

// ==================== 通用 DTO 定义 ====================

// UserProfile 用户资料 DTO。
type UserProfile struct {
	UUID      string `json:"uuid"`      // 用户UUID
	Nickname  string `json:"nickname"`  // 昵称
	Avatar    string `json:"avatar"`    // 头像
	Gender    int8   `json:"gender"`    // 性别(1:男 2:女 3:未知)
	Signature string `json:"signature"` // 个性签名
	Birthday  string `json:"birthday"`  // 生日(YYYY-MM-DD)
}

// LoginProfile 登录态最小资料 DTO。
type LoginProfile struct {
	UUID     string `json:"uuid"`     // 用户UUID
	Nickname string `json:"nickname"` // 昵称
	Avatar   string `json:"avatar"`   // 头像
}

// SimpleUserInfo 简化用户信息 DTO
type SimpleUserInfo struct {
	UUID      string `json:"uuid"`      // 用户UUID
	Nickname  string `json:"nickname"`  // 昵称
	Avatar    string `json:"avatar"`    // 头像URL
	Gender    int32  `json:"gender"`    // 性别
	Signature string `json:"signature"` // 个性签名
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
	Page       int32 `json:"page"`       // 当前页码
	PageSize   int32 `json:"pageSize"`   // 每页大小
	Total      int64 `json:"total"`      // 总记录数
	TotalPages int32 `json:"totalPages"` // 总页数
}

// ==================== 通用 DTO 转换函数 ====================

// ConvertUserProfileFromProto 将资料域 Protobuf 用户资料转换为 DTO。
func ConvertUserProfileFromProto(pb *userpb.UserInfo) *UserProfile {
	if pb == nil {
		return nil
	}
	return &UserProfile{
		UUID:      pb.Uuid,
		Nickname:  pb.Nickname,
		Avatar:    pb.Avatar,
		Gender:    int8(pb.Gender),
		Signature: pb.Signature,
		Birthday:  pb.Birthday,
	}
}

// ConvertLoginProfileFromProto 将 auth 登录返回的最小资料转换为 DTO。
func ConvertLoginProfileFromProto(pb *authpb.LoginUserInfo) *LoginProfile {
	if pb == nil {
		return nil
	}
	return &LoginProfile{
		UUID:     pb.Uuid,
		Nickname: pb.Nickname,
		Avatar:   pb.Avatar,
	}
}

// ConvertSimpleUserInfoFromProto 将 Protobuf 简化用户信息转换为 DTO
func ConvertSimpleUserInfoFromProto(pb *userpb.SimpleUserInfo) *SimpleUserInfo {
	if pb == nil {
		return nil
	}
	return &SimpleUserInfo{
		UUID:      pb.Uuid,
		Nickname:  pb.Nickname,
		Avatar:    pb.Avatar,
		Gender:    pb.Gender,
		Signature: pb.Signature,
	}
}

// ConvertRelationSimpleUserInfoFromProto 将 relation 域的简化用户信息转换为 DTO。
func ConvertRelationSimpleUserInfoFromProto(pb *relationpb.SimpleUserInfo) *SimpleUserInfo {
	if pb == nil {
		return nil
	}
	return &SimpleUserInfo{
		UUID:      pb.Uuid,
		Nickname:  pb.Nickname,
		Avatar:    pb.Avatar,
		Gender:    pb.Gender,
		Signature: pb.Signature,
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

// ConvertPaginationInfoFromProto 将 Protobuf 分页信息转换为 DTO
func ConvertPaginationInfoFromProto(pb *userpb.PaginationInfo) *PaginationInfo {
	if pb == nil {
		return nil
	}
	return &PaginationInfo{
		Page:       pb.Page,
		PageSize:   pb.PageSize,
		Total:      pb.Total,
		TotalPages: pb.TotalPages,
	}
}

// ConvertCommonPaginationInfoFromProto 将跨服务共享分页信息转换为 DTO。
func ConvertCommonPaginationInfoFromProto(pb *commonpb.PaginationInfo) *PaginationInfo {
	if pb == nil {
		return nil
	}
	return &PaginationInfo{
		Page:       pb.Page,
		PageSize:   pb.PageSize,
		Total:      pb.Total,
		TotalPages: pb.TotalPages,
	}
}
