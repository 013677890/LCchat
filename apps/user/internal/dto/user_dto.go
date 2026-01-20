package dto

import (
	"ChatServer/apps/user/pb"
	"ChatServer/model"
	"time"
)

// ==================== 用户信息 DTO ====================

// UserInfo 用户信息DTO
type UserInfo struct {
	UUID      string    // 用户UUID
	Nickname  string    // 昵称
	Telephone string    // 手机号
	Email     string    // 邮箱
	Avatar    string    // 头像URL
	Gender    int       // 性别(0:男 1:女 2:未知)
	Signature string    // 个性签名
	Birthday  string    // 生日(YYYY-MM-DD)
	Status    int       // 状态(0:正常 1:禁用)
	CreatedAt time.Time // 创建时间
	UpdatedAt time.Time // 更新时间
}

// GetProfileResponse 获取个人信息响应DTO
type GetProfileResponse struct {
	UserInfo *UserInfo // 用户信息
}

// GetOtherProfileRequest 获取他人信息请求DTO
type GetOtherProfileRequest struct {
	UserUUID string // 用户UUID
}

// GetOtherProfileResponse 获取他人信息响应DTO
type GetOtherProfileResponse struct {
	UserInfo *UserInfo // 用户信息
	IsFriend bool      // 是否好友
}

// UpdateProfileRequest 更新基本信息请求DTO
type UpdateProfileRequest struct {
	Nickname  string // 昵称
	Gender    int32  // 性别(0:男 1:女 2:未知)
	Birthday  string // 生日(YYYY-MM-DD)
	Signature string // 个性签名
}

// UpdateProfileResponse 更新基本信息响应DTO
type UpdateProfileResponse struct {
	UserInfo *UserInfo // 更新后的用户信息
}

// UploadAvatarRequest 上传头像请求DTO
type UploadAvatarRequest struct {
	AvatarData []byte // 头像数据
}

// UploadAvatarResponse 上传头像响应DTO
type UploadAvatarResponse struct {
	AvatarURL string // 头像URL
}

// ChangePasswordRequest 修改密码请求DTO
type ChangePasswordRequest struct {
	OldPassword string // 旧密码
	NewPassword string // 新密码
}

// ChangeEmailRequest 换绑邮箱请求DTO
type ChangeEmailRequest struct {
	NewEmail   string // 新邮箱
	VerifyCode string // 验证码
}

// ChangeEmailResponse 换绑邮箱响应DTO
type ChangeEmailResponse struct {
	Email string // 邮箱
}

// ChangeTelephoneRequest 换绑手机请求DTO
type ChangeTelephoneRequest struct {
	NewTelephone string // 新手机号
	VerifyCode   string // 验证码
}

// ChangeTelephoneResponse 换绑手机响应DTO
type ChangeTelephoneResponse struct {
	Telephone string // 手机号
}

// GetQRCodeRequest 获取用户二维码请求DTO
type GetQRCodeRequest struct{}

// GetQRCodeResponse 获取用户二维码响应DTO
type GetQRCodeResponse struct {
	QRCode      string // 二维码内容
	QRCodeImage string // 二维码图片(base64)
	ExpireAt    string // 过期时间
}

// ParseQRCodeRequest 解析二维码请求DTO
type ParseQRCodeRequest struct {
	QRCode string // 二维码内容
}

// ParseQRCodeResponse 解析二维码响应DTO
type ParseQRCodeResponse struct {
	UserInfo *UserInfo // 用户信息
	IsFriend bool      // 是否好友
}

// DeleteAccountRequest 注销账号请求DTO
type DeleteAccountRequest struct {
	Password string // 密码
	Reason   string // 注销原因
}

// DeleteAccountResponse 注销账号响应DTO
type DeleteAccountResponse struct {
	DeleteAt        string // 注销时间
	RecoverDeadline string // 恢复截止时间
}

// BatchGetProfileRequest 批量获取用户信息请求DTO
type BatchGetProfileRequest struct {
	UserUUIDs []string // 用户UUID列表
}

// BatchGetProfileResponse 批量获取用户信息响应DTO
type BatchGetProfileResponse struct {
	Users []*SimpleUserInfo // 用户信息列表
}

// SimpleUserInfo 简化用户信息DTO
type SimpleUserInfo struct {
	UUID     string // 用户UUID
	Nickname string // 昵称
	Avatar   string // 头像URL
}

// ==================== 转换函数 ====================

// ConvertSimpleUserModelsToSimpleUserInfoList 批量将数据库模型转换为简化用户DTO
func ConvertSimpleUserModelsToSimpleUserInfoList(users []*model.UserInfo) []*SimpleUserInfo {
	if users == nil {
		return []*SimpleUserInfo{}
	}

	result := make([]*SimpleUserInfo, 0, len(users))
	for _, user := range users {
		result = append(result, &SimpleUserInfo{
			UUID:     user.Uuid,
			Nickname: user.Nickname,
			Avatar:   user.Avatar,
		})
	}
	return result
}

// ConvertSimpleUserModelsToProto 批量将数据库模型转换为Protobuf消息
func ConvertSimpleUserModelsToProto(users []*model.UserInfo) []*pb.SimpleUserInfo {
	if users == nil {
		return []*pb.SimpleUserInfo{}
	}

	result := make([]*pb.SimpleUserInfo, 0, len(users))
	for _, user := range users {
		result = append(result, &pb.SimpleUserInfo{
			Uuid:     user.Uuid,
			Nickname: user.Nickname,
			Avatar:   user.Avatar,
		})
	}
	return result
}

// ConvertSimpleUserInfoDTOToProto 将DTO转换为Protobuf消息
func ConvertSimpleUserInfoDTOToProto(user *SimpleUserInfo) *pb.SimpleUserInfo {
	if user == nil {
		return nil
	}
	return &pb.SimpleUserInfo{
		Uuid:     user.UUID,
		Nickname: user.Nickname,
		Avatar:   user.Avatar,
	}
}

// ConvertSimpleUserInfoListToProto 批量将DTO转换为Protobuf消息
func ConvertSimpleUserInfoListToProto(users []*SimpleUserInfo) []*pb.SimpleUserInfo {
	if users == nil {
		return []*pb.SimpleUserInfo{}
	}

	result := make([]*pb.SimpleUserInfo, 0, len(users))
	for _, user := range users {
		result = append(result, ConvertSimpleUserInfoDTOToProto(user))
	}
	return result
}
