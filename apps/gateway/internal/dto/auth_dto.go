package dto

import (
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
)

// ==================== 认证服务相关 DTO ====================

// RegisterRequest 注册请求 DTO
type RegisterRequest struct {
	Email      string `json:"email" binding:"required,email"`            // 邮箱（必填）
	Password   string `json:"password" binding:"required,min=6,max=20"`  // 密码（必填）
	VerifyCode string `json:"verifyCode" binding:"required,len=6"`       // 验证码（必填）
	Nickname   string `json:"nickname" binding:"omitempty,min=2,max=20"` // 昵称（可选）
	Telephone  string `json:"telephone" binding:"omitempty,len=11"`      // 手机号（可选）
}

// RegisterResponse 注册响应 DTO
type RegisterResponse struct {
	UserUUID  string `json:"userUuid"`  // 用户UUID
	Email     string `json:"email"`     // 邮箱
	Telephone string `json:"telephone"` // 手机号
	Nickname  string `json:"nickname"`  // 昵称
}

// LoginRequest 登录请求 DTO（密码）
type LoginRequest struct {
	Account    string      `json:"account" binding:"required,min=1"`         // 账号（手机号或邮箱）
	Password   string      `json:"password" binding:"required,min=6,max=20"` // 密码
	DeviceInfo *DeviceInfo `json:"deviceInfo"`                               // 设备信息
}

// LoginResponse 登录响应 DTO
type LoginResponse struct {
	AccessToken  string        `json:"accessToken"`  // 访问令牌
	RefreshToken string        `json:"refreshToken"` // 刷新令牌
	TokenType    string        `json:"tokenType"`    // 令牌类型
	ExpiresIn    int64         `json:"expiresIn"`    // 过期时间(秒)
	UserInfo     *LoginProfile `json:"userInfo"`     // 用户登录资料
}

// LoginByCodeRequest 验证码登录请求 DTO
type LoginByCodeRequest struct {
	Email      string      `json:"email" binding:"required,email"`      // 邮箱
	VerifyCode string      `json:"verifyCode" binding:"required,len=6"` // 验证码
	DeviceInfo *DeviceInfo `json:"deviceInfo"`                          // 设备信息
}

// LoginByCodeResponse 验证码登录响应 DTO（同LoginResponse）
type LoginByCodeResponse struct {
	AccessToken  string        `json:"accessToken"`  // 访问令牌
	RefreshToken string        `json:"refreshToken"` // 刷新令牌
	TokenType    string        `json:"tokenType"`    // 令牌类型
	ExpiresIn    int64         `json:"expiresIn"`    // 过期时间(秒)
	UserInfo     *LoginProfile `json:"userInfo"`     // 用户登录资料
}

// SendVerifyCodeRequest 发送验证码请求 DTO
type SendVerifyCodeRequest struct {
	Email string `json:"email" binding:"required,email"`        // 邮箱
	Type  int32  `json:"type" binding:"required,oneof=1 2 3 4"` // 1:注册 2:登录 3:重置密码 4:换绑邮箱
}

// SendVerifyCodeResponse 发送验证码响应 DTO
type SendVerifyCodeResponse struct {
	ExpireSeconds int64 `json:"expireSeconds"` // 过期时间(秒)
}

// VerifyCodeRequest 校验验证码请求 DTO
type VerifyCodeRequest struct {
	Email      string `json:"email" binding:"required,email"`        // 邮箱
	VerifyCode string `json:"verifyCode" binding:"required,len=6"`   // 验证码
	Type       int32  `json:"type" binding:"required,oneof=1 2 3 4"` // 1:注册 2:登录 3:重置密码 4:换绑邮箱
}

// VerifyCodeResponse 校验验证码响应 DTO
type VerifyCodeResponse struct {
	Valid bool `json:"valid"` // 是否有效
}

// RefreshTokenRequest 刷新Token请求 DTO
type RefreshTokenRequest struct {
	UserUUID     string `json:"uuid" binding:"required,min=1"`         // 用户UUID
	DeviceID     string `json:"device_id" binding:"required,min=1"`    // 设备ID
	RefreshToken string `json:"refreshToken" binding:"required,min=1"` // 刷新令牌
}

// RefreshTokenResponse 刷新Token响应 DTO
type RefreshTokenResponse struct {
	AccessToken string `json:"accessToken"` // 访问令牌
	TokenType   string `json:"tokenType"`   // 令牌类型
	ExpiresIn   int64  `json:"expiresIn"`   // 过期时间(秒)
}

// LogoutRequest 登出请求 DTO
type LogoutRequest struct {
	DeviceID string `json:"deviceId" binding:"required,min=1"` // 设备ID
}

// LogoutResponse 登出响应 DTO
type LogoutResponse struct{}

// ResetPasswordRequest 重置密码请求 DTO
type ResetPasswordRequest struct {
	Email       string `json:"email" binding:"required,email"`              // 邮箱
	VerifyCode  string `json:"verifyCode" binding:"required,len=6"`         // 验证码
	NewPassword string `json:"newPassword" binding:"required,min=6,max=20"` // 新密码
}

// ResetPasswordResponse 重置密码响应 DTO
type ResetPasswordResponse struct{}

// ==================== 认证服务 DTO 转换函数 ====================

// ConvertToProtoRegisterRequest 将 DTO 注册请求转换为 Protobuf 请求
func ConvertToProtoRegisterRequest(dto *RegisterRequest) *authpb.RegisterRequest {
	if dto == nil {
		return nil
	}
	return &authpb.RegisterRequest{
		Email:      dto.Email,
		Password:   dto.Password,
		VerifyCode: dto.VerifyCode,
		Nickname:   dto.Nickname,
		Telephone:  dto.Telephone,
	}
}

// ConvertToProtoLoginRequest 将 DTO 登录请求转换为 Protobuf 请求
func ConvertToProtoLoginRequest(dto *LoginRequest) *authpb.LoginRequest {
	if dto == nil {
		return nil
	}

	var deviceInfo *authpb.DeviceInfo
	if dto.DeviceInfo != nil {
		deviceInfo = &authpb.DeviceInfo{
			DeviceName: dto.DeviceInfo.DeviceName,
			Platform:   dto.DeviceInfo.Platform,
			OsVersion:  dto.DeviceInfo.OSVersion,
			AppVersion: dto.DeviceInfo.AppVersion,
		}
	}

	return &authpb.LoginRequest{
		Account:    dto.Account,
		Password:   dto.Password,
		DeviceInfo: deviceInfo,
	}
}

// ConvertToProtoLoginByCodeRequest 将 DTO 验证码登录请求转换为 Protobuf 请求
func ConvertToProtoLoginByCodeRequest(dto *LoginByCodeRequest) *authpb.LoginByCodeRequest {
	if dto == nil {
		return nil
	}

	var deviceInfo *authpb.DeviceInfo
	if dto.DeviceInfo != nil {
		deviceInfo = &authpb.DeviceInfo{
			DeviceName: dto.DeviceInfo.DeviceName,
			Platform:   dto.DeviceInfo.Platform,
			OsVersion:  dto.DeviceInfo.OSVersion,
			AppVersion: dto.DeviceInfo.AppVersion,
		}
	}

	return &authpb.LoginByCodeRequest{
		Email:      dto.Email,
		VerifyCode: dto.VerifyCode,
		DeviceInfo: deviceInfo,
	}
}

// ConvertToProtoSendVerifyCodeRequest 将 DTO 发送验证码请求转换为 Protobuf 请求
func ConvertToProtoSendVerifyCodeRequest(dto *SendVerifyCodeRequest) *authpb.SendVerifyCodeRequest {
	if dto == nil {
		return nil
	}
	return &authpb.SendVerifyCodeRequest{
		Email: dto.Email,
		Type:  dto.Type,
	}
}

// ConvertToProtoVerifyCodeRequest 将 DTO 校验验证码请求转换为 Protobuf 请求
func ConvertToProtoVerifyCodeRequest(dto *VerifyCodeRequest) *authpb.VerifyCodeRequest {
	if dto == nil {
		return nil
	}
	return &authpb.VerifyCodeRequest{
		Email:      dto.Email,
		VerifyCode: dto.VerifyCode,
		Type:       dto.Type,
	}
}

// ConvertToProtoRefreshTokenRequest 将 DTO 刷新Token请求转换为 Protobuf 请求
func ConvertToProtoRefreshTokenRequest(dto *RefreshTokenRequest) *authpb.RefreshTokenRequest {
	if dto == nil {
		return nil
	}
	return &authpb.RefreshTokenRequest{
		RefreshToken: dto.RefreshToken,
	}
}

// ConvertToProtoLogoutRequest 将 DTO 登出请求转换为 Protobuf 请求
func ConvertToProtoLogoutRequest(dto *LogoutRequest) *authpb.LogoutRequest {
	if dto == nil {
		return nil
	}
	return &authpb.LogoutRequest{
		DeviceId: dto.DeviceID,
	}
}

// ConvertToProtoResetPasswordRequest 将 DTO 重置密码请求转换为 Protobuf 请求
func ConvertToProtoResetPasswordRequest(dto *ResetPasswordRequest) *authpb.ResetPasswordRequest {
	if dto == nil {
		return nil
	}
	return &authpb.ResetPasswordRequest{
		Email:       dto.Email,
		VerifyCode:  dto.VerifyCode,
		NewPassword: dto.NewPassword,
	}
}

// ==================== 认证服务 gRPC响应到DTO转换函数 ====================

// ConvertRegisterResponseFromProto 将 Protobuf 注册响应转换为 DTO
func ConvertRegisterResponseFromProto(pb *authpb.RegisterResponse) *RegisterResponse {
	if pb == nil {
		return nil
	}
	return &RegisterResponse{
		UserUUID:  pb.UserUuid,
		Email:     pb.Email,
		Telephone: pb.Telephone,
		Nickname:  pb.Nickname,
	}
}

// ConvertLoginResponseFromProto 将 Protobuf 登录响应转换为 DTO
func ConvertLoginResponseFromProto(pb *authpb.LoginResponse) *LoginResponse {
	if pb == nil {
		return nil
	}
	return &LoginResponse{
		AccessToken:  pb.AccessToken,
		RefreshToken: pb.RefreshToken,
		TokenType:    pb.TokenType,
		ExpiresIn:    pb.ExpiresIn,
		UserInfo:     ConvertLoginProfileFromProto(pb.UserInfo),
	}
}

// ConvertLoginByCodeResponseFromProto 将 Protobuf 验证码登录响应转换为 DTO
func ConvertLoginByCodeResponseFromProto(pb *authpb.LoginByCodeResponse) *LoginByCodeResponse {
	if pb == nil {
		return nil
	}
	return &LoginByCodeResponse{
		AccessToken:  pb.AccessToken,
		RefreshToken: pb.RefreshToken,
		TokenType:    pb.TokenType,
		ExpiresIn:    pb.ExpiresIn,
		UserInfo:     ConvertLoginProfileFromProto(pb.UserInfo),
	}
}

// ConvertSendVerifyCodeResponseFromProto 将 Protobuf 发送验证码响应转换为 DTO
func ConvertSendVerifyCodeResponseFromProto(pb *authpb.SendVerifyCodeResponse) *SendVerifyCodeResponse {
	if pb == nil {
		return nil
	}
	return &SendVerifyCodeResponse{
		ExpireSeconds: pb.ExpireSeconds,
	}
}

// ConvertVerifyCodeResponseFromProto 将 Protobuf 校验验证码响应转换为 DTO
func ConvertVerifyCodeResponseFromProto(pb *authpb.VerifyCodeResponse) *VerifyCodeResponse {
	if pb == nil {
		return nil
	}
	return &VerifyCodeResponse{
		Valid: pb.Valid,
	}
}

// ConvertRefreshTokenResponseFromProto 将 Protobuf 刷新Token响应转换为 DTO
func ConvertRefreshTokenResponseFromProto(pb *authpb.RefreshTokenResponse) *RefreshTokenResponse {
	if pb == nil {
		return nil
	}
	return &RefreshTokenResponse{
		AccessToken: pb.AccessToken,
		TokenType:   pb.TokenType,
		ExpiresIn:   pb.ExpiresIn,
	}
}

// ConvertLogoutResponseFromProto 将 Protobuf 登出响应转换为 DTO
func ConvertLogoutResponseFromProto(pb *authpb.LogoutResponse) *LogoutResponse {
	if pb == nil {
		return &LogoutResponse{}
	}
	return &LogoutResponse{}
}

// ConvertResetPasswordResponseFromProto 将 Protobuf 重置密码响应转换为 DTO
func ConvertResetPasswordResponseFromProto(pb *authpb.ResetPasswordResponse) *ResetPasswordResponse {
	if pb == nil {
		return &ResetPasswordResponse{}
	}
	return &ResetPasswordResponse{}
}
