//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func testGroupEdges(t *testing.T, f *Fixture, ctx context.Context) {
	t.Helper()
	a, b, c := f.Users["A"], f.Users["B"], f.Users["C"]
	a1 := a.Devices["A1"]
	b1 := b.Devices["B1"]
	c1 := c.Devices["C1"]

	createGroup := func(name string, addMode int) string {
		response, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/groups", a1.AccessToken, a1.ID, map[string]any{
			"name": name, "addMode": addMode,
		})
		if err != nil {
			t.Fatalf("创建群失败: %v", err)
		}
		var data map[string]any
		requireJSONData(t, "创建群", response, &data)
		groupUUID := stringValue(data, "groupUuid")
		if groupUUID == "" {
			t.Fatalf("创建群未返回 groupUuid: %#v", data)
		}
		// CreateGroup 当前默认 addMode=0（直接入群），创建接口本身没有暴露
		// addMode 字段；需要审批的用例必须再通过群资料更新接口切换为 1。
		if addMode != 0 {
			update, err := f.api.DoJSON(ctx, "PATCH", "/api/v1/auth/groups/"+groupUUID, a1.AccessToken, a1.ID, map[string]any{"addMode": addMode})
			if err != nil {
				t.Fatalf("设置群加群方式失败: %v", err)
			}
			requireHTTPSuccess(t, "设置群加群方式", update)
		}
		return groupUUID
	}

	groupUUID := createGroup("go-edge-group-"+f.suffix, 0)
	t.Run("无效群 UUID、非成员访问和群主离群", func(t *testing.T) {
		invalid, err := f.api.DoJSON(ctx, "GET", "/api/v1/auth/groups/not-exist", a1.AccessToken, a1.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "无效群 UUID", invalid)
		nonMember, err := f.api.DoJSON(ctx, "GET", "/api/v1/auth/groups/"+groupUUID, c1.AccessToken, c1.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		if nonMember.Status != 200 || nonMember.Body.Code != 0 {
			t.Errorf("非成员读取群资料未返回公开资料：%s", nonMember.Summary())
		} else {
			t.Logf("观察：当前接口允许非成员读取群资料：%s", nonMember.Summary())
		}
		nonMemberUpdate, err := f.api.DoJSON(ctx, "PATCH", "/api/v1/auth/groups/"+groupUUID, c1.AccessToken, c1.ID, map[string]any{"name": "not-allowed"})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "非成员修改群信息", nonMemberUpdate)
		ownerLeave, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/groups/"+groupUUID+"/leave", a1.AccessToken, a1.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "群主离群", ownerLeave)
	})
	t.Run("重复加成员、普通成员越权和管理员权限", func(t *testing.T) {
		add, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/groups/"+groupUUID+"/members", a1.AccessToken, a1.ID, map[string]any{"userUuids": []string{b.UUID}})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "添加群成员", add)
		duplicate, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/groups/"+groupUUID+"/members", a1.AccessToken, a1.ID, map[string]any{"userUuids": []string{b.UUID}})
		if err != nil {
			t.Fatal(err)
		}
		if duplicate.Status == 200 && duplicate.Body.Code == 0 {
			t.Logf("观察：重复添加有效群成员按幂等成功处理")
		} else {
			t.Logf("重复添加群成员返回错误：%s", duplicate.Summary())
		}
		memberUpdate, err := f.api.DoJSON(ctx, "PATCH", "/api/v1/auth/groups/"+groupUUID, b1.AccessToken, b1.ID, map[string]any{"name": "member-not-allowed"})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "普通成员修改群信息", memberUpdate)
		memberRole, err := f.api.DoJSON(ctx, "PATCH", "/api/v1/auth/groups/"+groupUUID+"/members/"+a.UUID+"/role", b1.AccessToken, b1.ID, map[string]any{"role": 1})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "普通成员修改角色", memberRole)
		memberMute, err := f.api.DoJSON(ctx, "PATCH", "/api/v1/auth/groups/"+groupUUID+"/members/"+a.UUID+"/mute", b1.AccessToken, b1.ID, map[string]any{"muteUntil": time.Now().Add(time.Minute).UnixMilli()})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "普通成员禁言他人", memberMute)

		promote, err := f.api.DoJSON(ctx, "PATCH", "/api/v1/auth/groups/"+groupUUID+"/members/"+b.UUID+"/role", a1.AccessToken, a1.ID, map[string]any{"role": 1})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "群主设置管理员", promote)
		adminNotice, err := f.api.DoJSON(ctx, "PUT", "/api/v1/auth/groups/"+groupUUID+"/notice", b1.AccessToken, b1.ID, map[string]any{"notice": "admin-can-update"})
		if err != nil {
			t.Fatal(err)
		}
		if adminNotice.Status != 200 || adminNotice.Body.Code != 0 {
			t.Logf("观察：管理员修改群公告未成功：%s", adminNotice.Summary())
		}
		demote, err := f.api.DoJSON(ctx, "PATCH", "/api/v1/auth/groups/"+groupUUID+"/members/"+b.UUID+"/role", a1.AccessToken, a1.ID, map[string]any{"role": 0})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "群主撤销管理员", demote)
	})
	t.Run("成员禁言、取消禁言、全员禁言和取消", func(t *testing.T) {
		mute, err := f.api.DoJSON(ctx, "PATCH", "/api/v1/auth/groups/"+groupUUID+"/members/"+b.UUID+"/mute", a1.AccessToken, a1.ID, map[string]any{"muteUntil": time.Now().Add(time.Minute).UnixMilli()})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "禁言成员", mute)
		// 禁言状态由群服务写入并由读模型异步传播，留出一小段传播窗口后再验证发送权限。
		time.Sleep(500 * time.Millisecond)
		mutedMessage, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/messages/send", b1.AccessToken, b1.ID, map[string]any{
			"clientMsgId": "muted-" + f.suffix, "convType": 2, "targetUuid": groupUUID, "msgType": 1, "content": `{"text":"muted"}`,
		})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "被禁言成员发群消息", mutedMessage)
		unmute, err := f.api.DoJSON(ctx, "PATCH", "/api/v1/auth/groups/"+groupUUID+"/members/"+b.UUID+"/mute", a1.AccessToken, a1.ID, map[string]any{"muteUntil": 0})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "取消成员禁言", unmute)
		time.Sleep(500 * time.Millisecond)
		allMute, err := f.api.DoJSON(ctx, "PATCH", "/api/v1/auth/groups/"+groupUUID+"/mute-setting", a1.AccessToken, a1.ID, map[string]any{"muteAll": true})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "开启全员禁言", allMute)
		waitForGroupMuteAll(t, f, ctx, groupUUID, a1.AccessToken, a1.ID, true, 3*time.Second)
		allMutedMessage, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/messages/send", b1.AccessToken, b1.ID, map[string]any{
			"clientMsgId": "all-muted-" + f.suffix, "convType": 2, "targetUuid": groupUUID, "msgType": 1, "content": `{"text":"all-muted"}`,
		})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "全员禁言成员发群消息", allMutedMessage)
		allUnmute, err := f.api.DoJSON(ctx, "PATCH", "/api/v1/auth/groups/"+groupUUID+"/mute-setting", a1.AccessToken, a1.ID, map[string]any{"muteAll": false})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "取消全员禁言", allUnmute)
		waitForGroupMuteAll(t, f, ctx, groupUUID, a1.AccessToken, a1.ID, false, 3*time.Second)
	})
	t.Run("群申请重复、拒绝、重复审核和撤回后再次申请", func(t *testing.T) {
		reviewGroup := createGroup("go-review-group-"+f.suffix, 1)
		apply, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/groups/"+reviewGroup+"/apply", c1.AccessToken, c1.ID, map[string]any{"reason": "review-edge"})
		if err != nil {
			t.Fatal(err)
		}
		var applyData map[string]any
		requireJSONData(t, "提交入群申请", apply, &applyData)
		applyID := int64Value(applyData, "applyId")
		if applyID <= 0 {
			t.Fatalf("审批型群申请未返回有效 applyId：data=%#v", applyData)
		}
		duplicate, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/groups/"+reviewGroup+"/apply", c1.AccessToken, c1.ID, map[string]any{"reason": "review-edge"})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "重复入群申请", duplicate)
		reject, err := f.api.DoJSON(ctx, "POST", fmt.Sprintf("/api/v1/auth/groups/%s/join-requests/%d/review", reviewGroup, applyID), a1.AccessToken, a1.ID, map[string]any{"action": 2, "remark": "reject"})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "拒绝入群申请", reject)
		repeated, err := f.api.DoJSON(ctx, "POST", fmt.Sprintf("/api/v1/auth/groups/%s/join-requests/%d/review", reviewGroup, applyID), a1.AccessToken, a1.ID, map[string]any{"action": 2})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "重复审核入群申请", repeated)
		reapply, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/groups/"+reviewGroup+"/apply", c1.AccessToken, c1.ID, map[string]any{"reason": "reapply"})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("拒绝后再次申请结果：%s", reapply.Summary())
		if reapply.Status == 200 && reapply.Body.Code == 0 {
			var reapplyData map[string]any
			if err := reapply.DecodeData(&reapplyData); err == nil {
				reapplyID := int64Value(reapplyData, "applyId")
				cancel, err := f.api.DoJSON(ctx, "DELETE", "/api/v1/auth/groups/"+reviewGroup+"/apply", c1.AccessToken, c1.ID, nil)
				if err != nil {
					t.Fatal(err)
				}
				requireHTTPSuccess(t, "撤回再次申请", cancel)
				t.Logf("撤回申请 applyId=%d", reapplyID)
			}
		}
	})

	t.Run("直接入群", func(t *testing.T) {
		directGroup := createGroup("go-direct-group-"+f.suffix, 0)
		response, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/groups/"+directGroup+"/apply", c1.AccessToken, c1.ID, map[string]any{"reason": "direct"})
		if err != nil {
			t.Fatal(err)
		}
		var data map[string]any
		requireJSONData(t, "直接入群申请", response, &data)
		if joined, _ := data["joinedDirectly"].(bool); !joined {
			t.Errorf("直接入群没有返回 joinedDirectly=true：%#v", data)
		}
	})
	t.Run("解散群后继续访问和发消息", func(t *testing.T) {
		dismiss, err := f.api.DoJSON(ctx, "DELETE", "/api/v1/auth/groups/"+groupUUID, a1.AccessToken, a1.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "解散群", dismiss)
		waitForGroupInfoError(t, f, ctx, groupUUID, b1.AccessToken, b1.ID, 3*time.Second)
		message, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/messages/send", b1.AccessToken, b1.ID, map[string]any{
			"clientMsgId": "dismissed-" + f.suffix, "convType": 2, "targetUuid": groupUUID, "msgType": 1, "content": `{"text":"dismissed"}`,
		})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "向已解散群发消息", message)
	})
}

// waitForGroupMuteAll 等待群资料中的全员禁言开关进入期望状态。
func waitForGroupMuteAll(t *testing.T, f *Fixture, ctx context.Context, groupUUID, token, deviceID string, muted bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	lastSummary := ""
	for time.Now().Before(deadline) {
		response, err := f.api.DoJSON(ctx, "GET", "/api/v1/auth/groups/"+groupUUID, token, deviceID, nil)
		if err != nil {
			t.Fatalf("查询群全员禁言状态失败: %v", err)
		}
		lastSummary = response.Summary()
		if response.Status == 200 && response.Body.Code == 0 {
			var data map[string]any
			if err := response.DecodeData(&data); err == nil {
				if current, ok := data["muteAll"].(bool); ok && current == muted {
					return
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("等待群全员禁言 muted=%t 超时，最后响应：%s", muted, lastSummary)
}

// waitForGroupInfoError 等待已解散群的资料接口最终返回错误。
// 解散操作会先写数据库，再由缓存投影异步失效；因此验证最终不可读，
// 而不是把刚完成 DELETE 后的极短缓存传播窗口直接当成最终结论。
func waitForGroupInfoError(t *testing.T, f *Fixture, ctx context.Context, groupUUID, token, deviceID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last HTTPResponse
	for time.Now().Before(deadline) {
		response, err := f.api.DoJSON(ctx, "GET", "/api/v1/auth/groups/"+groupUUID, token, deviceID, nil)
		if err != nil {
			t.Fatalf("查询已解散群失败: %v", err)
		}
		last = response
		if response.Status != 200 || response.Body.Code != 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	requireHTTPError(t, "访问已解散群", last)
}
