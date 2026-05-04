package service

import (
	"time"

	"github.com/013677890/LCchat-Backend/apps/auth/internal/repository"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/model"
)

// buildLoginUserInfo 将账号模型转换为 auth 登录返回的最小展示信息。
func buildLoginUserInfo(user *model.UserAccount) *authpb.LoginUserInfo {
	if user == nil {
		return nil
	}
	return &authpb.LoginUserInfo{
		Uuid:     user.UserUuid,
		Nickname: user.LoginNickname,
		Avatar:   user.LoginAvatar,
	}
}

// buildRegisterResponse 将注册后的账号模型转换为注册响应。
func buildRegisterResponse(user *model.UserAccount) *authpb.RegisterResponse {
	if user == nil {
		return nil
	}
	return &authpb.RegisterResponse{
		UserUuid:  user.UserUuid,
		Nickname:  user.LoginNickname,
		Email:     user.Email,
		Telephone: user.Telephone,
	}
}

// buildLoginResponse 组装密码登录响应。
func buildLoginResponse(accessToken, refreshToken string, expiresIn int64, user *model.UserAccount) *authpb.LoginResponse {
	return &authpb.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		UserInfo:     buildLoginUserInfo(user),
	}
}

// buildLoginByCodeResponse 组装验证码登录响应。
func buildLoginByCodeResponse(accessToken, refreshToken string, expiresIn int64, user *model.UserAccount) *authpb.LoginByCodeResponse {
	return &authpb.LoginByCodeResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		UserInfo:     buildLoginUserInfo(user),
	}
}

// buildFindAccountByEmailResponse 将账号模型转换为内部邮箱查询响应。
func buildFindAccountByEmailResponse(user *model.UserAccount) *authpb.FindAccountByEmailResponse {
	if user == nil {
		return &authpb.FindAccountByEmailResponse{Found: false}
	}
	return &authpb.FindAccountByEmailResponse{
		UserUuid: user.UserUuid,
		Status:   int32(user.Status),
		Found:    true,
	}
}

// buildAccountStatusItemProto 将账号状态仓储结果转换为内部批量查询响应项。
func buildAccountStatusItemProto(item *repository.AccountStatusItem) *authpb.AccountStatusItem {
	if item == nil {
		return nil
	}
	return &authpb.AccountStatusItem{
		UserUuid: item.UserUUID,
		Status:   item.Status,
		Exists:   item.Exists,
	}
}

// buildChangeEmailResponse 组装换绑邮箱响应。
func buildChangeEmailResponse(email string) *authpb.ChangeEmailResponse {
	return &authpb.ChangeEmailResponse{Email: email}
}

// buildDeleteAccountResponse 组装注销账号响应。
func buildDeleteAccountResponse(deleteAt, recoverDeadline time.Time) *authpb.DeleteAccountResponse {
	return &authpb.DeleteAccountResponse{
		DeleteAt:        deleteAt.Format(time.RFC3339),
		RecoverDeadline: recoverDeadline.Format(time.RFC3339),
	}
}

// buildDeviceItemProto 将设备会话模型转换为设备列表项。
func buildDeviceItemProto(session *model.DeviceSession, currentDeviceID string, lastSeenSec int64) *authpb.DeviceItem {
	if session == nil {
		return nil
	}
	return &authpb.DeviceItem{
		DeviceId:        session.DeviceId,
		DeviceName:      session.DeviceName,
		Platform:        session.Platform,
		AppVersion:      session.AppVersion,
		IsCurrentDevice: currentDeviceID != "" && session.DeviceId == currentDeviceID,
		Status:          int32(session.Status),
		LastSeenAt:      lastSeenSec * 1000,
	}
}

// buildOnlineStatusProto 将在线状态结果转换为单用户在线状态 proto。
func buildOnlineStatusProto(userUUID string, isOnline bool, lastSeenSec int64, platforms []string) *authpb.OnlineStatus {
	return &authpb.OnlineStatus{
		UserUuid:        userUUID,
		IsOnline:        isOnline,
		LastSeenAt:      lastSeenSec * 1000,
		OnlinePlatforms: platforms,
	}
}

// buildOnlineStatusItemProto 将在线状态结果转换为批量在线状态项。
func buildOnlineStatusItemProto(userUUID string, isOnline bool, lastSeenSec int64) *authpb.OnlineStatusItem {
	return &authpb.OnlineStatusItem{
		UserUuid:   userUUID,
		IsOnline:   isOnline,
		LastSeenAt: lastSeenSec * 1000,
	}
}
