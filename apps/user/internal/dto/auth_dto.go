package dto

import (
	pb "ChatServer/apps/user/pb"
	"ChatServer/model"
)

// ==================== 认证相关 DTO ====================

// LoginRequest 登录请求DTO
type LoginRequest struct {
	Telephone  string         // 手机号
	Password   string         // 密码
	DeviceInfo *pb.DeviceInfo // 设备信息
}

// LoginResponse 登录响应DTO
type LoginResponse struct {
	UserInfo *UserInfo // 用户信息
}

// LoginByCodeRequest 验证码登录请求DTO
type LoginByCodeRequest struct {
	Email      string         // 邮箱
	VerifyCode string         // 验证码
	DeviceInfo *pb.DeviceInfo // 设备信息
}

// LoginByCodeResponse 验证码登录响应DTO
type LoginByCodeResponse struct {
	UserInfo *UserInfo // 用户信息
}

// RegisterRequest 注册请求DTO
type RegisterRequest struct {
	Telephone  string         // 手机号
	Password   string         // 密码
	VerifyCode string         // 验证码
	Nickname   string         // 昵称（可选）
	DeviceInfo *pb.DeviceInfo // 设备信息
}

// RegisterResponse 注册响应DTO
type RegisterResponse struct {
	UserUUID  string // 用户UUID
	Telephone string // 手机号
	Nickname  string // 昵称
}

// SendVerifyCodeRequest 发送验证码请求DTO
type SendVerifyCodeRequest struct {
	Email string // 邮箱
	Type  int32  // 验证码类型(1:注册 2:登录 3:重置密码 4:换绑邮箱)
}

// SendVerifyCodeResponse 发送验证码响应DTO
type SendVerifyCodeResponse struct {
	ExpireSeconds int64 // 过期时间（秒）
}

// VerifyCodeRequest 校验验证码请求DTO
type VerifyCodeRequest struct {
	Email      string // 邮箱
	VerifyCode string // 验证码
	Type       int32  // 验证码类型
}

// VerifyCodeResponse 校验验证码响应DTO
type VerifyCodeResponse struct {
	Valid bool // 是否有效
}

// RefreshTokenRequest 刷新Token请求DTO
type RefreshTokenRequest struct {
	RefreshToken string // 刷新Token
}

// RefreshTokenResponse 刷新Token响应DTO
type RefreshTokenResponse struct {
	AccessToken string // 访问Token
	TokenType   string // Token类型
	ExpiresIn   int64  // 过期时间（秒）
}

// LogoutRequest 登出请求DTO
type LogoutRequest struct {
	DeviceID string // 设备ID
}

// ResetPasswordRequest 重置密码请求DTO
type ResetPasswordRequest struct {
	Email       string // 邮箱
	VerifyCode  string // 验证码
	NewPassword string // 新密码
}

// ResetPasswordResponse 重置密码响应DTO
type ResetPasswordResponse struct{}

// ==================== 转换函数 ====================

// ConvertModelToUserInfo 将数据库模型转换为DTO
func ConvertModelToUserInfo(user *model.UserInfo) *UserInfo {
	if user == nil {
		return nil
	}
	return &UserInfo{
		UUID:      user.Uuid,
		Nickname:  user.Nickname,
		Telephone: user.Telephone,
		Email:     user.Email,
		Avatar:    user.Avatar,
		Gender:    int(user.Gender),
		Signature: user.Signature,
		Birthday:  user.Birthday,
		Status:    int(user.Status),
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}

// ConvertToProtoUserInfo 将DTO转换为Protobuf消息
func ConvertToProtoUserInfo(user *UserInfo) *pb.UserInfo {
	if user == nil {
		return nil
	}
	return &pb.UserInfo{
		Uuid:      user.UUID,
		Nickname:  user.Nickname,
		Telephone: user.Telephone,
		Email:     user.Email,
		Avatar:    user.Avatar,
		Gender:    int32(user.Gender),
		Signature: user.Signature,
		Birthday:  user.Birthday,
		Status:    int32(user.Status),
	}
}

// ConvertModelToProtoUserInfo 将数据库模型直接转换为Protobuf消息
func ConvertModelToProtoUserInfo(user *model.UserInfo) *pb.UserInfo {
	if user == nil {
		return nil
	}
	return &pb.UserInfo{
		Uuid:      user.Uuid,
		Nickname:  user.Nickname,
		Telephone: user.Telephone,
		Email:     user.Email,
		Avatar:    user.Avatar,
		Gender:    int32(user.Gender),
		Signature: user.Signature,
		Birthday:  user.Birthday,
		Status:    int32(user.Status),
	}
}

// ConvertModelsToProtoUserInfoList 批量将数据库模型转换为Protobuf消息列表
func ConvertModelsToProtoUserInfoList(users []*model.UserInfo) []*pb.UserInfo {
	if users == nil {
		return []*pb.UserInfo{}
	}

	result := make([]*pb.UserInfo, 0, len(users))
	for _, user := range users {
		result = append(result, ConvertModelToProtoUserInfo(user))
	}
	return result
}
