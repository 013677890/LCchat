package service

import (
	"context"
	"strings"

	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/util"
)

// buildDeviceUserAgent 生成精简版设备 UserAgent 文本。
func buildDeviceUserAgent(deviceInfo *authpb.DeviceInfo) string {
	if deviceInfo == nil {
		return ""
	}

	platform := deviceInfo.GetPlatform()
	appVersion := deviceInfo.GetAppVersion()
	osVersion := deviceInfo.GetOsVersion()

	result := ""
	if platform != "" {
		result = platform
	}
	if appVersion != "" {
		if result != "" {
			result += "/" + appVersion
		} else {
			result = appVersion
		}
	}
	if osVersion != "" {
		if result != "" {
			result += " (" + osVersion + ")"
		} else {
			result = osVersion
		}
	}

	return result
}

// getRequiredDeviceID 从上下文中提取必填设备 ID。
func getRequiredDeviceID(ctx context.Context) (string, error) {
	deviceID := strings.TrimSpace(util.GetDeviceIDFromContext(ctx))
	if deviceID == "" {
		return "", apperr.New(consts.CodeParamError)
	}
	return deviceID, nil
}

// buildLoginUserInfo 将统一用户模型转换为 auth 登录返回的最小展示信息。
func buildLoginUserInfo(user *model.UserInfo) *authpb.LoginUserInfo {
	if user == nil {
		return nil
	}
	return &authpb.LoginUserInfo{
		Uuid:     user.Uuid,
		Nickname: user.Nickname,
		Avatar:   user.Avatar,
	}
}
