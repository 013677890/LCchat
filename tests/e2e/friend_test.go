//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func testFriendEdges(t *testing.T, f *Fixture, ctx context.Context) {
	t.Helper()
	a, b, c, d := f.Users["A"], f.Users["B"], f.Users["C"], f.Users["D"]
	a1 := a.Devices["A1"]
	c1 := c.Devices["C1"]
	d1 := d.Devices["D1"]

	t.Run("建立 A-B 好友关系", func(t *testing.T) {
		ensureFriend(t, f, ctx, a, b)
	})
	t.Run("自己申请和不存在用户", func(t *testing.T) {
		self, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/friend/apply", a1.AccessToken, "A1", map[string]any{"targetUuid": a.UUID})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "自己申请自己", self)
		missing, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/friend/apply", a1.AccessToken, "A1", map[string]any{"targetUuid": "missing-user"})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "申请不存在用户", missing)
	})
	t.Run("好友申请重复、拒绝和重复审核", func(t *testing.T) {
		response, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/friend/apply", c1.AccessToken, "C1", map[string]any{
			"targetUuid": d.UUID, "reason": "reject-edge",
		})
		if err != nil {
			t.Fatal(err)
		}
		var apply map[string]any
		requireJSONData(t, "创建好友申请", response, &apply)
		applyID := int64Value(apply, "applyId")
		duplicate, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/friend/apply", c1.AccessToken, "C1", map[string]any{
			"targetUuid": d.UUID, "reason": "reject-edge",
		})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "重复好友申请", duplicate)
		handlePath := "/api/v1/auth/friend/apply/handle"
		rejected, err := f.api.DoJSON(ctx, "POST", handlePath, d1.AccessToken, "D1", map[string]any{"applyId": applyID, "action": 2, "remark": "rejected"})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "拒绝好友申请", rejected)
		repeated, err := f.api.DoJSON(ctx, "POST", handlePath, d1.AccessToken, "D1", map[string]any{"applyId": applyID, "action": 2})
		if err != nil {
			t.Fatal(err)
		}
		if repeated.Status == 200 && repeated.Body.Code == 0 {
			t.Logf("观察：重复处理已拒绝好友申请按幂等成功处理")
		} else {
			t.Logf("重复处理好友申请返回错误：%s", repeated.Summary())
		}
	})
	t.Run("好友备注和标签清空", func(t *testing.T) {
		remark, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/friend/remark", a1.AccessToken, "A1", map[string]any{"userUuid": b.UUID, "remark": "临时备注"})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "设置好友备注", remark)
		tag, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/friend/tag", a1.AccessToken, "A1", map[string]any{"userUuid": b.UUID, "groupTag": "临时标签"})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "设置好友标签", tag)
		clearRemark, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/friend/remark", a1.AccessToken, "A1", map[string]any{"userUuid": b.UUID, "remark": ""})
		if err != nil {
			t.Fatal(err)
		}
		if clearRemark.Status != 200 || clearRemark.Body.Code != 0 {
			t.Errorf("备注清空请求未成功，当前接口可能无法清空：%s", clearRemark.Summary())
		}
		clearTag, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/friend/tag", a1.AccessToken, "A1", map[string]any{"userUuid": b.UUID, "groupTag": ""})
		if err != nil {
			t.Fatal(err)
		}
		if clearTag.Status != 200 || clearTag.Body.Code != 0 {
			t.Errorf("标签清空请求未成功，当前接口可能无法清空：%s", clearTag.Summary())
		}
	})
	t.Run("黑名单阻断申请、消息和关系查询", func(t *testing.T) {
		add, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/blacklist", a1.AccessToken, "A1", map[string]any{"targetUuid": c.UUID})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "加入黑名单", add)
		apply, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/friend/apply", a1.AccessToken, "A1", map[string]any{"targetUuid": c.UUID})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "黑名单后申请好友", apply)
		message, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/messages/send", a1.AccessToken, "A1", map[string]any{
			"clientMsgId": fmt.Sprintf("blacklist-%s", f.suffix), "convType": 1, "targetUuid": c.UUID, "msgType": 1, "content": `{"text":"blocked"}`,
		})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "黑名单后发送消息", message)
		relation, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/friend/relation", a1.AccessToken, "A1", map[string]any{"userUuid": a.UUID, "peerUuid": c.UUID})
		if err != nil {
			t.Fatal(err)
		}
		if relation.Status != 200 || relation.Body.Code != 0 {
			t.Fatalf("黑名单关系查询失败：%s", relation.Summary())
		}
		var data map[string]any
		requireJSONData(t, "黑名单关系查询", relation, &data)
		if blacklisted, _ := data["isBlacklist"].(bool); !blacklisted {
			t.Errorf("关系查询未反映黑名单状态：%#v", data)
		}
		remove, err := f.api.DoJSON(ctx, "DELETE", "/api/v1/auth/blacklist/"+c.UUID, a1.AccessToken, "A1", nil)
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "移除黑名单", remove)
	})
}

func ensureFriend(t *testing.T, f *Fixture, ctx context.Context, from, to *User) {
	t.Helper()
	device := from.Devices["A1"]
	if from.Label != "A" {
		for _, candidate := range from.Devices {
			device = candidate
			break
		}
	}
	relation, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/friend/relation", device.AccessToken, device.ID, map[string]any{"userUuid": from.UUID, "peerUuid": to.UUID})
	if err == nil && relation.Status == 200 && relation.Body.Code == 0 {
		var data map[string]any
		if relation.DecodeData(&data) == nil {
			if isFriend, _ := data["isFriend"].(bool); isFriend {
				return
			}
		}
	}
	apply, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/friend/apply", device.AccessToken, device.ID, map[string]any{"targetUuid": to.UUID, "reason": "e2e-setup"})
	if err != nil {
		t.Fatalf("建立好友关系请求失败: %v", err)
	}
	var applyData map[string]any
	requireJSONData(t, "建立好友申请", apply, &applyData)
	applyID := int64Value(applyData, "applyId")
	acceptDevice := to.Devices["B1"]
	if acceptDevice.AccessToken == "" {
		for _, candidate := range to.Devices {
			acceptDevice = candidate
			break
		}
	}
	handle, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/friend/apply/handle", acceptDevice.AccessToken, acceptDevice.ID, map[string]any{"applyId": applyID, "action": 1})
	if err != nil {
		t.Fatalf("处理好友申请请求失败: %v", err)
	}
	requireHTTPSuccess(t, "接受好友申请", handle)
}

// decodeRawJSON 仅用于调试后端返回的任意 JSON，测试断言仍使用 APIResponse 的 code。
func decodeRawJSON(response HTTPResponse) any {
	var value any
	_ = json.Unmarshal(response.Body.Data, &value)
	return value
}
