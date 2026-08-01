//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"
)

func testMessageEdges(t *testing.T, f *Fixture, ctx context.Context) {
	t.Helper()
	a, b, c := f.Users["A"], f.Users["B"], f.Users["C"]
	a1 := a.Devices["A1"]
	b1 := b.Devices["B1"]
	ensureFriend(t, f, ctx, a, b)

	send := func(clientMsgID, target string, msgType int, content string) HTTPResponse {
		response, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/messages/send", a1.AccessToken, a1.ID, map[string]any{
			"clientMsgId": clientMsgID,
			"convType":    1,
			"targetUuid":  target,
			"msgType":     msgType,
			"content":     content,
		})
		if err != nil {
			t.Fatalf("发送消息请求失败: %v", err)
		}
		return response
	}

	var first map[string]any
	t.Run("重复 clientMsgId 幂等", func(t *testing.T) {
		clientMsgID := fmt.Sprintf("idempotent-%s", f.suffix)
		firstResponse := send(clientMsgID, b.UUID, 1, `{"text":"idempotent"}`)
		requireJSONData(t, "首次发送幂等消息", firstResponse, &first)
		secondResponse := send(clientMsgID, b.UUID, 1, `{"text":"idempotent"}`)
		var second map[string]any
		requireJSONData(t, "重复发送幂等消息", secondResponse, &second)
		if stringValue(first, "msgId") != stringValue(second, "msgId") || int64Value(first, "seq") != int64Value(second, "seq") {
			t.Fatalf("相同 clientMsgId 未返回同一消息：first=%#v second=%#v", first, second)
		}
	})
	convID := stringValue(first, "convId")
	msgID := stringValue(first, "msgId")
	seq := int64Value(first, "seq")

	t.Run("非好友、空内容、超长内容和非法会话类型", func(t *testing.T) {
		cases := []struct {
			name string
			body map[string]any
		}{
			{name: "非好友发送", body: map[string]any{"clientMsgId": "not-friend-" + f.suffix, "convType": 1, "targetUuid": c.UUID, "msgType": 1, "content": `{"text":"blocked"}`}},
			{name: "空内容", body: map[string]any{"clientMsgId": "empty-" + f.suffix, "convType": 1, "targetUuid": b.UUID, "msgType": 1, "content": ""}},
			{name: "超长内容", body: map[string]any{"clientMsgId": "large-" + f.suffix, "convType": 1, "targetUuid": b.UUID, "msgType": 1, "content": string(make([]byte, 65537))}},
			{name: "非法会话类型", body: map[string]any{"clientMsgId": "conv-type-" + f.suffix, "convType": 9, "targetUuid": b.UUID, "msgType": 1, "content": "x"}},
		}
		for _, item := range cases {
			item := item
			t.Run(item.name, func(t *testing.T) {
				response, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/messages/send", a1.AccessToken, a1.ID, item.body)
				if err != nil {
					t.Fatal(err)
				}
				requireHTTPError(t, item.name, response)
			})
		}
	})
	t.Run("图片、文件、音频和未知消息类型", func(t *testing.T) {
		for _, msgType := range []int{2, 3, 4, 99} {
			response := send(fmt.Sprintf("type-%d-%s", msgType, f.suffix), b.UUID, msgType, `{"url":"https://example.com/resource","name":"demo"}`)
			t.Logf("msgType=%d 结果：%s", msgType, response.Summary())
		}
	})
	t.Run("非法 JSON 内容", func(t *testing.T) {
		response := send("invalid-json-"+f.suffix, b.UUID, 1, "{not-json")
		t.Logf("非法 JSON 内容结果：%s", response.Summary())
	})
	t.Run("消息分页方向、第二页和边界", func(t *testing.T) {
		pull := func(params url.Values) HTTPResponse {
			response, err := f.api.DoJSON(ctx, "GET", queryPath("/api/v1/auth/messages/pull", params), b1.AccessToken, b1.ID, nil)
			if err != nil {
				t.Fatalf("拉取消息请求失败: %v", err)
			}
			return response
		}
		firstPage := pull(url.Values{"convId": {convID}, "anchorSeq": {"0"}, "limit": {"1"}, "direction": {"0"}})
		requireHTTPSuccess(t, "消息第一页", firstPage)
		secondPage := pull(url.Values{"convId": {convID}, "anchorSeq": {fmt.Sprint(seq)}, "limit": {"1"}, "direction": {"1"}})
		requireHTTPSuccess(t, "消息第二页", secondPage)
		reverse := pull(url.Values{"convId": {convID}, "anchorSeq": {fmt.Sprint(seq)}, "limit": {"1"}, "direction": {"2"}})
		requireHTTPSuccess(t, "反向拉取消息", reverse)
		for _, item := range []struct {
			name  string
			query url.Values
		}{
			{name: "limit 超过上限", query: url.Values{"convId": {convID}, "limit": {"201"}}},
			{name: "非法 direction", query: url.Values{"convId": {convID}, "direction": {"9"}}},
			{name: "负游标", query: url.Values{"convId": {convID}, "anchorSeq": {"-1"}}},
		} {
			response := pull(item.query)
			if response.Status == 200 && response.Body.Code == 0 {
				t.Logf("观察：%s 被接口接受并按默认游标处理：%s", item.name, response.Summary())
			} else {
				t.Logf("%s 返回错误：%s", item.name, response.Summary())
			}
		}
	})
	t.Run("离线发送后重新拉取", func(t *testing.T) {
		clientMsgID := "offline-" + f.suffix
		response := send(clientMsgID, b.UUID, 1, `{"text":"offline"}`)
		var message map[string]any
		requireJSONData(t, "离线发送", response, &message)
		pull, err := f.api.DoJSON(ctx, "GET", queryPath("/api/v1/auth/messages/pull", url.Values{
			"convId": {stringValue(message, "convId")}, "anchorSeq": {"0"}, "limit": {"50"}, "direction": {"0"},
		}), b1.AccessToken, b1.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		var data map[string]any
		requireJSONData(t, "离线消息拉取", pull, &data)
		if !containsMessageID(data, stringValue(message, "msgId")) {
			t.Errorf("离线拉取未找到消息 %s：%#v", stringValue(message, "msgId"), data)
		}
	})
	t.Run("sync-batch 大小、重复会话和游标", func(t *testing.T) {
		batch := func(conversations []any) HTTPResponse {
			response, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/messages/sync-batch", b1.AccessToken, b1.ID, map[string]any{"conversations": conversations})
			if err != nil {
				t.Fatalf("sync-batch 请求失败: %v", err)
			}
			return response
		}
		valid := batch([]any{map[string]any{"convId": convID, "afterSeq": 0, "limit": 50}})
		requireHTTPSuccess(t, "sync-batch 单会话", valid)
		requireHTTPError(t, "sync-batch 空会话", batch([]any{}))
		requireHTTPError(t, "sync-batch 重复会话", batch([]any{map[string]any{"convId": convID}, map[string]any{"convId": convID}}))
		many := make([]any, 0, 51)
		for i := 0; i < 51; i++ {
			many = append(many, map[string]any{"convId": fmt.Sprintf("fake-%d-%s", i, f.suffix), "limit": 1})
		}
		requireHTTPError(t, "sync-batch 超过 50 会话", batch(many))
	})
	t.Run("get-by-ids 空列表", func(t *testing.T) {
		response, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/messages/get-by-ids", a1.AccessToken, a1.ID, map[string]any{"convId": convID, "msgIds": []string{}})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "get-by-ids 空列表", response)
	})
	t.Run("非发送者撤回、重复撤回和超时撤回", func(t *testing.T) {
		notSender, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/messages/recall", b1.AccessToken, b1.ID, map[string]any{"convId": convID, "msgId": msgID})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "非发送者撤回", notSender)
		recalled, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/messages/recall", a1.AccessToken, a1.ID, map[string]any{"convId": convID, "msgId": msgID})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "发送者撤回", recalled)
		repeated, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/messages/recall", a1.AccessToken, a1.ID, map[string]any{"convId": convID, "msgId": msgID})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "重复撤回", repeated)

		oldResponse := send("timeout-"+f.suffix, b.UUID, 1, `{"text":"old"}`)
		var old map[string]any
		requireJSONData(t, "发送超时撤回消息", oldResponse, &old)
		db, err := f.database(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "UPDATE message SET send_time=DATE_SUB(NOW(3), INTERVAL 10 MINUTE) WHERE msg_id=?", stringValue(old, "msgId")); err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
		timeoutRecall, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/messages/recall", a1.AccessToken, a1.ID, map[string]any{"convId": stringValue(old, "convId"), "msgId": stringValue(old, "msgId")})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPError(t, "超时撤回", timeoutRecall)
	})
}

func containsMessageID(data map[string]any, messageID string) bool {
	items, _ := data["messages"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if stringValue(item, "msgId") == messageID || stringValue(item, "msg_id") == messageID {
			return true
		}
	}
	return false
}

func sendMessage(t *testing.T, f *Fixture, ctx context.Context, user *User, deviceID, target string, msgType int, content string) map[string]any {
	t.Helper()
	device := user.Devices[deviceID]
	response, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/messages/send", device.AccessToken, device.ID, map[string]any{
		"clientMsgId": fmt.Sprintf("helper-%s", f.suffix), "convType": 1, "targetUuid": target, "msgType": msgType, "content": content,
	})
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	requireJSONData(t, "发送消息", response, &data)
	return data
}

var _ = json.Valid
