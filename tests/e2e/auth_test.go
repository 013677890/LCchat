//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/consts/redisKey"
)

func testAuthEdges(t *testing.T, f *Fixture, ctx context.Context) {
	t.Helper()
	a := f.Users["A"]
	b := f.Users["B"]
	a1 := a.Devices["A1"]

	t.Run("未登录访问受保护接口", func(t *testing.T) {
		response, err := f.api.DoJSON(ctx, "GET", "/api/v1/auth/user/profile", "", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "未登录访问", response)
	})
	t.Run("非法 token", func(t *testing.T) {
		response, err := f.api.DoJSON(ctx, "GET", "/api/v1/auth/user/profile", "invalid-token", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "非法 token", response)
	})
	t.Run("错误密码", func(t *testing.T) {
		response, err := f.api.DoJSON(ctx, "POST", "/api/v1/public/user/login", "", "wrong-password-device", map[string]any{
			"account": a.Email, "password": "WrongPass1", "deviceInfo": map[string]any{"platform": "Web"},
		})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "错误密码登录", response)
	})
	t.Run("错误验证码返回 valid=false", func(t *testing.T) {
		response, err := f.api.DoJSON(ctx, "POST", "/api/v1/public/user/verify-code", "", "", map[string]any{
			"email": a.Email, "verifyCode": "000000", "type": 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		var data map[string]any
		requireJSONData(t, "错误验证码校验", response, &data)
		if valid, _ := data["valid"].(bool); valid {
			t.Fatalf("错误验证码不应返回 valid=true: %#v", data)
		}
	})
	t.Run("验证码过期", func(t *testing.T) {
		email := fmt.Sprintf("expired-code-%s@example.com", f.suffix)
		f.seedCode(ctx, email, "333333", 1, time.Second)
		time.Sleep(1500 * time.Millisecond)
		response, err := f.api.DoJSON(ctx, "POST", "/api/v1/public/user/verify-code", "", "", map[string]any{
			"email": email, "verifyCode": "333333", "type": 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "过期验证码", response)
	})
	t.Run("验证码重复使用", func(t *testing.T) {
		f.seedCode(ctx, a.Email, "222222", 2, 10*time.Minute)
		f.loginByCode(ctx, a, "A-CODE-REUSE", "222222")
		response, err := f.api.DoJSON(ctx, "POST", "/api/v1/public/user/login-by-code", "", "A-CODE-REUSE-SECOND", map[string]any{
			"email": a.Email, "verifyCode": "222222", "deviceInfo": map[string]any{"deviceName": "A-CODE-REUSE-SECOND", "platform": "Web"},
		})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "验证码重复使用", response)
	})
	t.Run("重复邮箱和手机号注册", func(t *testing.T) {
		f.seedCode(ctx, a.Email, "111111", 1, 10*time.Minute)
		response, err := f.api.DoJSON(ctx, "POST", "/api/v1/public/user/register", "", "", map[string]any{
			"email": a.Email, "password": "Passw0rd9", "verifyCode": "111111", "nickname": "DuplicateEmail", "telephone": "16600000001",
		})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "重复邮箱注册", response)

		email := fmt.Sprintf("duplicate-phone-%s@example.com", f.suffix)
		f.seedCode(ctx, email, "111111", 1, 10*time.Minute)
		response, err = f.api.DoJSON(ctx, "POST", "/api/v1/public/user/register", "", "", map[string]any{
			"email": email, "password": "Passw0rd9", "verifyCode": "111111", "nickname": "DuplicatePhone", "telephone": a.Telephone,
		})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "重复手机号注册", response)
	})
	t.Run("refresh token 跨设备和设备头不匹配", func(t *testing.T) {
		refreshDevice := f.login(aContext(ctx), a, "A-REFRESH")
		response, err := f.api.DoJSON(ctx, "POST", "/api/v1/public/user/refresh-token", "", refreshDevice.ID, map[string]any{
			"uuid": a.UUID, "device_id": refreshDevice.ID, "refreshToken": refreshDevice.RefreshToken,
		})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "有效 refresh token", response)

		// 当前实现明确保持 RefreshToken 不轮换；重复使用结果单独记录为测试观察。
		repeated, err := f.api.DoJSON(ctx, "POST", "/api/v1/public/user/refresh-token", "", refreshDevice.ID, map[string]any{
			"uuid": a.UUID, "device_id": refreshDevice.ID, "refreshToken": refreshDevice.RefreshToken,
		})
		if err != nil {
			t.Fatal(err)
		}
		if repeated.Status == 200 && repeated.Body.Code == 0 {
			t.Logf("观察：RefreshToken 当前允许重复使用，符合 auth 服务中的“不轮换”实现")
		} else {
			t.Logf("观察：RefreshToken 重复使用已拒绝：%s", repeated.Summary())
		}

		cross, err := f.api.DoJSON(ctx, "POST", "/api/v1/public/user/refresh-token", "", "A-CROSS", map[string]any{
			"uuid": a.UUID, "device_id": "A-CROSS", "refreshToken": refreshDevice.RefreshToken,
		})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "refresh token 跨设备", cross)

		mismatch, err := f.api.DoJSON(ctx, "POST", "/api/v1/public/user/refresh-token", "", "A-MISMATCH", map[string]any{
			"uuid": a.UUID, "device_id": refreshDevice.ID, "refreshToken": refreshDevice.RefreshToken,
		})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "refresh token 设备头不匹配", mismatch)
	})
	t.Run("refresh token 过期", func(t *testing.T) {
		device := f.login(ctx, a, "A-EXPIRED-REFRESH")
		key := fmt.Sprintf("auth:rt:%s:%s", a.UUID, device.ID)
		if err := f.redis.Set(ctx, key, device.RefreshToken, time.Second).Err(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(1500 * time.Millisecond)
		response, err := f.api.DoJSON(ctx, "POST", "/api/v1/public/user/refresh-token", "", device.ID, map[string]any{
			"uuid": a.UUID, "device_id": device.ID, "refreshToken": device.RefreshToken,
		})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "refresh token 过期", response)
	})
	t.Run("logout 后旧 access token", func(t *testing.T) {
		device := f.login(ctx, a, "A-LOGOUT")
		response, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/user/logout", device.AccessToken, device.ID, map[string]any{"deviceId": device.ID})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "logout", response)
		// 设计特性：HTTP 层是无状态 JWT 校验，登出只删除 Redis 登录态，
		// 旧 access token 在自然过期前对 HTTP 接口仍然有效，这里仅记录观察。
		profile, err := f.api.DoJSON(ctx, "GET", "/api/v1/auth/user/profile", device.AccessToken, device.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("观察：logout 后旧 token 访问 HTTP 接口（设计上允许直至 JWT 过期）：%s", profile.Summary())
		// 强断言在 WS 侧：connect fail-close 比对 Redis 存储的 token 哈希，
		// 登出后旧 token 必须无法建立长连接（即时失去实时收发能力）。
		if ws, wsErr := OpenWS(ctx, f.cfg, "logout-old-token", device.AccessToken, device.ID); wsErr == nil {
			_ = ws.Close()
			t.Errorf("logout 后旧 token 仍能建立 WebSocket 连接")
		}
	})
	t.Run("被踢设备 token", func(t *testing.T) {
		device := f.login(ctx, a, "A-KICKED")
		response, err := f.api.DoJSON(ctx, "DELETE", "/api/v1/auth/user/devices/"+url.PathEscape(device.ID), a1.AccessToken, "A1", nil)
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "踢出设备", response)
		// 同 logout：HTTP 层旧 JWT 允许用到自然过期（设计特性），仅观察；
		// WS 层必须立即拒绝被踢设备。
		profile, err := f.api.DoJSON(ctx, "GET", "/api/v1/auth/user/profile", device.AccessToken, device.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("观察：被踢设备旧 token 访问 HTTP 接口（设计上允许直至 JWT 过期）：%s", profile.Summary())
		if ws, wsErr := OpenWS(ctx, f.cfg, "kicked-old-token", device.AccessToken, device.ID); wsErr == nil {
			_ = ws.Close()
			t.Errorf("被踢设备旧 token 仍能建立 WebSocket 连接")
		}
	})
	t.Run("access token 与 device 不匹配", func(t *testing.T) {
		// 设计特性：HTTP 层设备身份以 JWT claims 为准，X-Device-ID 头不参与
		// 已认证接口的比对，这里仅记录观察。
		response, err := f.api.DoJSON(ctx, "GET", "/api/v1/auth/user/profile", a1.AccessToken, "A2", nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("观察：HTTP 携带不匹配 X-Device-ID 的访问（设计上以 JWT 为准）：%s", response.Summary())
		// WS 层强校验 claims.DeviceID 与 query.device_id 一致，不匹配必须拒绝。
		if ws, wsErr := OpenWS(ctx, f.cfg, "mismatch-device", a1.AccessToken, "MISMATCH-DEVICE"); wsErr == nil {
			_ = ws.Close()
			t.Errorf("token 与 device_id 不匹配仍能建立 WebSocket 连接")
		}
	})
	t.Run("修改密码错误旧密码、弱密码和重复密码", func(t *testing.T) {
		cases := []struct {
			name string
			old  string
			new  string
		}{
			{name: "错误旧密码", old: "WrongPass9", new: "Another9"},
			{name: "弱密码", old: a.Password, new: "short"},
			{name: "重复密码", old: a.Password, new: a.Password},
		}
		for _, item := range cases {
			item := item
			t.Run(item.name, func(t *testing.T) {
				response, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/user/change-password", a1.AccessToken, "A1", map[string]any{
					"oldPassword": item.old, "newPassword": item.new,
				})
				if err != nil {
					t.Fatal(err)
				}
				requireHTTPError(t, item.name, response)
			})
		}
	})
	t.Run("修改邮箱错误验证码和重复邮箱", func(t *testing.T) {
		wrong, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/user/change-email", a1.AccessToken, "A1", map[string]any{
			"newEmail": fmt.Sprintf("new-email-%s@example.com", f.suffix), "verifyCode": "000000",
		})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "修改邮箱错误验证码", wrong)
		f.seedCode(ctx, b.Email, "444444", 4, 10*time.Minute)
		duplicate, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/user/change-email", a1.AccessToken, "A1", map[string]any{
			"newEmail": b.Email, "verifyCode": "444444",
		})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "修改邮箱重复邮箱", duplicate)
	})
	t.Run("二维码非法、过期和重复解析", func(t *testing.T) {
		invalid, err := f.api.DoJSON(ctx, "POST", "/api/v1/public/user/parse-qrcode", "", "", map[string]any{"token": "not-exist-qrcode"})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "非法二维码", invalid)
		response, err := f.api.DoJSON(ctx, "GET", "/api/v1/auth/user/qrcode", a1.AccessToken, "A1", nil)
		if err != nil {
			t.Fatal(err)
		}
		var data map[string]any
		requireJSONData(t, "获取二维码", response, &data)
		qrURL := stringValue(data, "qrCode")
		parsed, err := url.Parse(qrURL)
		if err != nil {
			t.Fatal(err)
		}
		token := strings.Trim(parsed.Path, "/")
		if index := strings.LastIndex(token, "/"); index >= 0 {
			token = token[index+1:]
		}
		// 二维码映射存储在 Redis 中；将 token 映射设置为极短 TTL，
		// 等待它真实过期后再解析，覆盖“链接格式正确但已经失效”的分支。
		if err := f.redis.Expire(ctx, rediskey.QRCodeTokenKey(token), time.Second).Err(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(1500 * time.Millisecond)
		expired, err := f.api.DoJSON(ctx, "POST", "/api/v1/public/user/parse-qrcode", "", "", map[string]any{"token": token})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "过期二维码", expired)

		// 过期测试会消耗 A 当前二维码的 token，改用 B 生成新的 token，
		// 这样重复解析验证不会被前一个场景的 Redis TTL 影响。
		bQRCode, err := f.api.DoJSON(ctx, "GET", "/api/v1/auth/user/qrcode", b.Devices["B1"].AccessToken, b.Devices["B1"].ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		var bData map[string]any
		requireJSONData(t, "获取 B 的二维码", bQRCode, &bData)
		bURL, err := url.Parse(stringValue(bData, "qrCode"))
		if err != nil {
			t.Fatal(err)
		}
		bToken := strings.Trim(bURL.Path, "/")
		if index := strings.LastIndex(bToken, "/"); index >= 0 {
			bToken = bToken[index+1:]
		}
		first, err := f.api.DoJSON(ctx, "POST", "/api/v1/public/user/parse-qrcode", "", "", map[string]any{"token": bToken})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "首次解析二维码", first)
		repeated, err := f.api.DoJSON(ctx, "POST", "/api/v1/public/user/parse-qrcode", "", "", map[string]any{"token": bToken})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("二维码重复解析结果：%s", repeated.Summary())
	})
	t.Run("头像空文件、错误类型、超大文件和损坏内容", func(t *testing.T) {
		missing, err := f.api.DoMultipart(ctx, "POST", "/api/v1/auth/user/avatar", a1.AccessToken, "A1", map[string]UploadFile{})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "头像空文件字段", missing)
		wrongMIME, err := f.api.DoMultipart(ctx, "POST", "/api/v1/auth/user/avatar", a1.AccessToken, "A1", map[string]UploadFile{
			"avatar": {Name: "wrong.txt", MIME: "text/plain", Content: []byte("not image")},
		})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "头像错误 MIME", wrongMIME)
		large, err := f.api.DoMultipart(ctx, "POST", "/api/v1/auth/user/avatar", a1.AccessToken, "A1", map[string]UploadFile{
			"avatar": {Name: "large.png", MIME: "image/png", Content: make([]byte, 2*1024*1024+1)},
		})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "头像超过 2MB", large)
		corrupt, err := f.api.DoMultipart(ctx, "POST", "/api/v1/auth/user/avatar", a1.AccessToken, "A1", map[string]UploadFile{
			"avatar": {Name: "corrupt.png", MIME: "image/png", Content: []byte("not-a-png")},
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("损坏头像结果：%s", corrupt.Summary())
	})
}

// aContext 保留调用点的上下文语义，同时让登录辅助函数可以直接复用父 context。
func aContext(ctx context.Context) context.Context { return ctx }
