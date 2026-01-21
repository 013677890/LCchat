package dto

import (
	userpb "ChatServer/apps/user/pb"
)

// ==================== 用户信息服务 DTO ====================

// GetProfileRequest 获取个人信息请求 DTO
type GetProfileRequest struct{}

// GetProfileResponse 获取个人信息响应 DTO
type GetProfileResponse struct {
	UserInfo *UserInfo `json:"userInfo"` // 用户信息
}

// GetOtherProfileRequest 获取他人信息请求 DTO
type GetOtherProfileRequest struct {
	UserUUID string `json:"userUuid" binding:"required"` // 用户UUID
}

// GetOtherProfileResponse 获取他人信息响应 DTO
type GetOtherProfileResponse struct {
	UserInfo *UserInfo `json:"userInfo"` // 用户信息
	IsFriend bool      `json:"isFriend"` // 是否好友
}

// UpdateProfileRequest 更新基本信息请求 DTO
type UpdateProfileRequest struct {
	Nickname  string `json:"nickname" binding:"required,max=20"`    // 昵称
	Gender    int32  `json:"gender" binding:"oneof=0 1 2"`          // 性别(0:男 1:女 2:未知)
	Birthday  string `json:"birthday" binding:"omitempty"`          // 生日(YYYY-MM-DD)
	Signature string `json:"signature" binding:"omitempty,max=100"` // 个性签名
}

// UpdateProfileResponse 更新基本信息响应 DTO
type UpdateProfileResponse struct {
	UserInfo *UserInfo `json:"userInfo"` // 更新后的用户信息
}

// UploadAvatarRequest 上传头像请求 DTO
type UploadAvatarRequest struct {
	Avatar string `json:"avatar" binding:"required"` // 头像URL
}

// UploadAvatarResponse 上传头像响应 DTO
type UploadAvatarResponse struct {
	AvatarURL string `json:"avatarUrl"` // 头像URL
}

// ChangePasswordRequest 修改密码请求 DTO
type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"required,min=8,max=16"` // 旧密码
	NewPassword string `json:"newPassword" binding:"required,min=8,max=16"` // 新密码
}

// ChangePasswordResponse 修改密码响应 DTO
type ChangePasswordResponse struct{}

// ChangeEmailRequest 换绑邮箱请求 DTO
type ChangeEmailRequest struct {
	NewEmail   string `json:"newEmail" binding:"required,email"`   // 新邮箱
	VerifyCode string `json:"verifyCode" binding:"required,len=6"` // 验证码
}

// ChangeEmailResponse 换绑邮箱响应 DTO
type ChangeEmailResponse struct {
	Email string `json:"email"` // 邮箱
}

// ChangeTelephoneRequest 换绑手机请求 DTO
type ChangeTelephoneRequest struct {
	NewTelephone string `json:"newTelephone" binding:"required,len=11"` // 新手机号
	VerifyCode   string `json:"verifyCode" binding:"required,len=6"`    // 验证码
}

// ChangeTelephoneResponse 换绑手机响应 DTO
type ChangeTelephoneResponse struct {
	Telephone string `json:"telephone"` // 手机号
}

// GetQRCodeRequest 获取用户二维码请求 DTO
type GetQRCodeRequest struct{}

// GetQRCodeResponse 获取用户二维码响应 DTO
type GetQRCodeResponse struct {
	QRCode      string `json:"qrCode"`      // 二维码内容
	QRCodeImage string `json:"qrCodeImage"` // 二维码图片(base64)
	ExpireAt    string `json:"expireAt"`    // 过期时间
}

// ParseQRCodeRequest 解析二维码请求 DTO
type ParseQRCodeRequest struct {
	QRCode string `json:"qrCode" binding:"required"` // 二维码内容
}

// ParseQRCodeResponse 解析二维码响应 DTO
type ParseQRCodeResponse struct {
	UserInfo *UserInfo `json:"userInfo"` // 用户信息
	IsFriend bool      `json:"isFriend"` // 是否好友
}

// DeleteAccountRequest 注销账号请求 DTO
type DeleteAccountRequest struct {
	Password string `json:"password" binding:"required,min=8,max=16"` // 密码
	Reason   string `json:"reason" binding:"omitempty,max=200"`       // 注销原因
}

// DeleteAccountResponse 注销账号响应 DTO
type DeleteAccountResponse struct {
	DeleteAt        string `json:"deleteAt"`        // 注销时间
	RecoverDeadline string `json:"recoverDeadline"` // 恢复截止时间
}

// BatchGetProfileRequest 批量获取用户信息请求 DTO
type BatchGetProfileRequest struct {
	UserUUIDs []string `json:"userUuids" binding:"required"` // 用户UUID列表
}

// BatchGetProfileResponse 批量获取用户信息响应 DTO
type BatchGetProfileResponse struct {
	Users []*SimpleUserInfo `json:"users"` // 用户信息列表
}

// ==================== 用户信息 DTO 转换函数 ====================

// ConvertToProtoGetOtherProfileRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoGetOtherProfileRequest(dto *GetOtherProfileRequest) *userpb.GetOtherProfileRequest {
	if dto == nil {
		return nil
	}
	return &userpb.GetOtherProfileRequest{
		UserUuid: dto.UserUUID,
	}
}

// ConvertToProtoUpdateProfileRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoUpdateProfileRequest(dto *UpdateProfileRequest) *userpb.UpdateProfileRequest {
	if dto == nil {
		return nil
	}
	return &userpb.UpdateProfileRequest{
		Nickname:  dto.Nickname,
		Gender:    dto.Gender,
		Birthday:  dto.Birthday,
		Signature: dto.Signature,
	}
}

// ConvertToProtoChangePasswordRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoChangePasswordRequest(dto *ChangePasswordRequest) *userpb.ChangePasswordRequest {
	if dto == nil {
		return nil
	}
	return &userpb.ChangePasswordRequest{
		OldPassword: dto.OldPassword,
		NewPassword: dto.NewPassword,
	}
}

// ConvertToProtoChangeEmailRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoChangeEmailRequest(dto *ChangeEmailRequest) *userpb.ChangeEmailRequest {
	if dto == nil {
		return nil
	}
	return &userpb.ChangeEmailRequest{
		NewEmail:   dto.NewEmail,
		VerifyCode: dto.VerifyCode,
	}
}

// ConvertToProtoChangeTelephoneRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoChangeTelephoneRequest(dto *ChangeTelephoneRequest) *userpb.ChangeTelephoneRequest {
	if dto == nil {
		return nil
	}
	return &userpb.ChangeTelephoneRequest{
		NewTelephone: dto.NewTelephone,
		VerifyCode:   dto.VerifyCode,
	}
}

// ConvertToProtoParseQRCodeRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoParseQRCodeRequest(dto *ParseQRCodeRequest) *userpb.ParseQRCodeRequest {
	if dto == nil {
		return nil
	}
	return &userpb.ParseQRCodeRequest{
		Qrcode: dto.QRCode,
	}
}

// ConvertToProtoDeleteAccountRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoDeleteAccountRequest(dto *DeleteAccountRequest) *userpb.DeleteAccountRequest {
	if dto == nil {
		return nil
	}
	return &userpb.DeleteAccountRequest{
		Password: dto.Password,
		Reason:   dto.Reason,
	}
}

// ConvertToProtoBatchGetProfileRequest 将 DTO 转换为 Protobuf 请求
func ConvertToProtoBatchGetProfileRequest(dto *BatchGetProfileRequest) *userpb.BatchGetProfileRequest {
	if dto == nil {
		return nil
	}
	return &userpb.BatchGetProfileRequest{
		UserUuids: dto.UserUUIDs,
	}
}

// ==================== 用户信息 gRPC响应到DTO转换函数 ====================

// ConvertGetProfileResponseFromProto 将 Protobuf 获取个人信息响应转换为 DTO
func ConvertGetProfileResponseFromProto(pb *userpb.GetProfileResponse) *GetProfileResponse {
	if pb == nil {
		return nil
	}
	return &GetProfileResponse{
		UserInfo: ConvertUserInfoFromProto(pb.UserInfo),
	}
}

// ConvertGetOtherProfileResponseFromProto 将 Protobuf 获取他人信息响应转换为 DTO
func ConvertGetOtherProfileResponseFromProto(pb *userpb.GetOtherProfileResponse) *GetOtherProfileResponse {
	if pb == nil {
		return nil
	}
	return &GetOtherProfileResponse{
		UserInfo: ConvertUserInfoFromProto(pb.UserInfo),
		IsFriend: pb.IsFriend,
	}
}

// ConvertUpdateProfileResponseFromProto 将 Protobuf 更新基本信息响应转换为 DTO
func ConvertUpdateProfileResponseFromProto(pb *userpb.UpdateProfileResponse) *UpdateProfileResponse {
	if pb == nil {
		return nil
	}
	return &UpdateProfileResponse{
		UserInfo: ConvertUserInfoFromProto(pb.UserInfo),
	}
}

// ConvertUploadAvatarResponseFromProto 将 Protobuf 上传头像响应转换为 DTO
func ConvertUploadAvatarResponseFromProto(pb *userpb.UploadAvatarResponse) *UploadAvatarResponse {
	if pb == nil {
		return nil
	}
	return &UploadAvatarResponse{
		AvatarURL: pb.AvatarUrl,
	}
}

// ConvertChangePasswordResponseFromProto 将 Protobuf 修改密码响应转换为 DTO
func ConvertChangePasswordResponseFromProto(pb *userpb.ChangePasswordResponse) *ChangePasswordResponse {
	if pb == nil {
		return nil
	}
	return &ChangePasswordResponse{}
}

// ConvertChangeEmailResponseFromProto 将 Protobuf 换绑邮箱响应转换为 DTO
func ConvertChangeEmailResponseFromProto(pb *userpb.ChangeEmailResponse) *ChangeEmailResponse {
	if pb == nil {
		return nil
	}
	return &ChangeEmailResponse{
		Email: pb.Email,
	}
}

// ConvertChangeTelephoneResponseFromProto 将 Protobuf 换绑手机响应转换为 DTO
func ConvertChangeTelephoneResponseFromProto(pb *userpb.ChangeTelephoneResponse) *ChangeTelephoneResponse {
	if pb == nil {
		return nil
	}
	return &ChangeTelephoneResponse{
		Telephone: pb.Telephone,
	}
}

// ConvertGetQRCodeResponseFromProto 将 Protobuf 获取二维码响应转换为 DTO
func ConvertGetQRCodeResponseFromProto(pb *userpb.GetQRCodeResponse) *GetQRCodeResponse {
	if pb == nil {
		return nil
	}
	return &GetQRCodeResponse{
		QRCode:      pb.Qrcode,
		QRCodeImage: pb.QrcodeImage,
		ExpireAt:    pb.ExpireAt,
	}
}

// ConvertParseQRCodeResponseFromProto 将 Protobuf 解析二维码响应转换为 DTO
func ConvertParseQRCodeResponseFromProto(pb *userpb.ParseQRCodeResponse) *ParseQRCodeResponse {
	if pb == nil {
		return nil
	}
	return &ParseQRCodeResponse{
		UserInfo: ConvertUserInfoFromProto(pb.UserInfo),
		IsFriend: pb.IsFriend,
	}
}

// ConvertDeleteAccountResponseFromProto 将 Protobuf 注销账号响应转换为 DTO
func ConvertDeleteAccountResponseFromProto(pb *userpb.DeleteAccountResponse) *DeleteAccountResponse {
	if pb == nil {
		return nil
	}
	return &DeleteAccountResponse{
		DeleteAt:        pb.DeleteAt,
		RecoverDeadline: pb.RecoverDeadline,
	}
}

// ConvertBatchGetProfileResponseFromProto 将 Protobuf 批量获取用户信息响应转换为 DTO
func ConvertBatchGetProfileResponseFromProto(pb *userpb.BatchGetProfileResponse) *BatchGetProfileResponse {
	if pb == nil {
		return nil
	}
	return &BatchGetProfileResponse{
		Users: ConvertSimpleUserItemsFromProto(pb.Users),
	}
}
