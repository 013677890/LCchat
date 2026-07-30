package service

import (
	"context"
	"strings"

	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/util"
)

// buildDeviceUserAgent 生成精简版设备 UserAgent 文本。
//
// auth-service 当前只需要保留最小设备展示信息，因此这里把 platform / appVersion /
// osVersion 组合成紧凑字符串，供设备会话落库和设备列表展示复用。
func buildDeviceUserAgent(deviceInfo *authpb.DeviceInfo) string {
	// 没有设备信息时直接返回空串，避免后续字符串拼接产生无意义分隔符。
	if deviceInfo == nil {
		return ""
	}

	platform := deviceInfo.GetPlatform()
	appVersion := deviceInfo.GetAppVersion()
	osVersion := deviceInfo.GetOsVersion()

	result := ""

	// 第一段优先放 platform，作为设备展示文案的主标签。
	if platform != "" {
		result = platform
	}

	// 第二段拼 appVersion；只有前面已有内容时才补分隔符。
	if appVersion != "" {
		if result != "" {
			result += "/" + appVersion
		} else {
			result = appVersion
		}
	}

	// 最后一段把 osVersion 放进括号，保持设备列表展示更紧凑。
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
//
// 登录、刷新 Token、验证码登录等链路都依赖 device_id 做设备级登录态隔离，因此这里统一
// 做 trim + 空值校验，避免每个 service 方法重复写相同保护逻辑。
func getRequiredDeviceID(ctx context.Context) (string, error) {
	// 统一先做 trim，避免把只包含空白符的 header 误当成合法 device_id。
	deviceID := strings.TrimSpace(util.GetDeviceIDFromContext(ctx))
	if deviceID == "" {
		return "", apperr.New(consts.CodeParamError)
	}
	return deviceID, nil
}
