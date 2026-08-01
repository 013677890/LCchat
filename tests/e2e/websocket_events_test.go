//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	connectpb "github.com/013677890/LCchat-Backend/apps/connect/pb"
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	realtimepb "github.com/013677890/LCchat-Backend/pkg/realtimepb"
	"google.golang.org/protobuf/proto"
)

func testWebSocketEvents(t *testing.T, f *Fixture, ctx context.Context) {
	t.Helper()
	a, b, c := f.Users["A"], f.Users["B"], f.Users["C"]
	a1Token, a2Token := a.Devices["A1"], a.Devices["A2"]
	b1Token, b2Token := b.Devices["B1"], b.Devices["B2"]
	c1Token := c.Devices["C1"]
	var realtimeApplyID int64
	// 该测试也允许单独通过 -run 执行，因此不能依赖前面的好友子测试建立 A-B 关系。
	ensureFriend(t, f, ctx, a, b)

	// E/F 专门用于好友实时事件，避免 A/B 已经是好友后无法再次制造申请事件。
	e := f.register(ctx, "E")
	ff := f.register(ctx, "F")
	e1Token := f.login(ctx, e, "E1")
	f1Token := f.login(ctx, ff, "F1")

	clients := make(map[string]*WSClient)
	open := func(name, token, device string) *WSClient {
		client, err := OpenWS(ctx, f.cfg, name, token, device)
		if err != nil {
			t.Fatalf("打开 %s WebSocket 失败: %v", name, err)
		}
		clients[name] = client
		t.Cleanup(func() { _ = client.Close() })
		return client
	}
	a1 := open("A1", a1Token.AccessToken, a1Token.ID)
	a2 := open("A2", a2Token.AccessToken, a2Token.ID)
	b1 := open("B1", b1Token.AccessToken, b1Token.ID)
	b2 := open("B2", b2Token.AccessToken, b2Token.ID)
	c1 := open("C1", c1Token.AccessToken, c1Token.ID)
	e1 := open("E1", e1Token.AccessToken, e1Token.ID)
	f1 := open("F1", f1Token.AccessToken, f1Token.ID)

	t.Run("heartbeat_ack", func(t *testing.T) {
		for _, client := range clients {
			if err := client.Heartbeat(eventContext(ctx, f.cfg.EventTimeout)); err != nil {
				t.Errorf("%s 心跳失败: %v", client.Name, err)
			}
		}
	})
	t.Run("FRIEND_APPLY_CREATED", func(t *testing.T) {
		response, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/friend/apply", e1Token.AccessToken, e1Token.ID, map[string]any{"targetUuid": ff.UUID, "reason": "realtime"})
		if err != nil {
			t.Fatal(err)
		}
		var data map[string]any
		requireJSONData(t, "创建实时好友申请", response, &data)
		applyID := int64Value(data, "applyId")
		realtimeApplyID = applyID
		var payload realtimepb.FriendApplyCreatedPayload
		expectEventPayload(t, f1, "FRIEND_APPLY_CREATED", &payload, func() bool {
			return payload.ApplyId == applyID && payload.ApplicantUuid == e.UUID && payload.TargetUuid == ff.UUID
		})
		t.Logf("好友申请实时事件 applyId=%d", applyID)
	})
	t.Run("FRIEND_APPLY_HANDLED 和 FRIEND_RELATION_CHANGED", func(t *testing.T) {
		if realtimeApplyID == 0 {
			t.Fatal("好友申请创建步骤没有记录 applyId")
		}
		response, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/friend/apply/handle", f1Token.AccessToken, f1Token.ID, map[string]any{"applyId": realtimeApplyID, "action": 1, "remark": "accepted"})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "处理实时好友申请", response)
		var handled realtimepb.FriendApplyHandledPayload
		expectEventPayload(t, e1, "FRIEND_APPLY_HANDLED", &handled, func() bool { return handled.Action == 1 && handled.ApplicantUuid == e.UUID })
		var relationE, relationF realtimepb.FriendRelationChangedPayload
		expectEventPayload(t, e1, "FRIEND_RELATION_CHANGED", &relationE, func() bool {
			return len(relationE.UserUuids) == 2 && slices.Contains(relationE.UserUuids, e.UUID) && slices.Contains(relationE.UserUuids, ff.UUID)
		})
		expectEventPayload(t, f1, "FRIEND_RELATION_CHANGED", &relationF, func() bool {
			return len(relationF.UserUuids) == 2 && slices.Contains(relationF.UserUuids, e.UUID) && slices.Contains(relationF.UserUuids, ff.UUID)
		})
	})

	// HTTP 发送消息，随后观察多设备推送、已读同步和 ACK。
	response, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/messages/send", a1Token.AccessToken, a1Token.ID, map[string]any{
		"clientMsgId": "ws-event-" + f.suffix, "convType": 1, "targetUuid": b.UUID, "msgType": 1, "content": `{"text":"ws-event"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	var message map[string]any
	requireJSONData(t, "WebSocket 事件消息发送", response, &message)
	convID := stringValue(message, "convId")
	messageID := stringValue(message, "msgId")
	messageSeq := int64Value(message, "seq")

	t.Run("MSG_PUSH", func(t *testing.T) {
		for _, client := range []*WSClient{b1, b2, a2} {
			var item msgpb.MsgItem
			expectEventPayload(t, client, "MSG_PUSH", &item, func() bool {
				return item.MsgId == messageID && item.ConvId == convID && item.Seq == messageSeq
			})
		}
	})
	t.Run("MSG_MARK_READ 和 MSG_READ_RECEIPT", func(t *testing.T) {
		mark, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/conversations/mark-read", b1Token.AccessToken, b1Token.ID, map[string]any{"convId": convID, "readSeq": messageSeq})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "标记消息已读", mark)
		var markRead msgpb.MarkReadNotice
		expectEventPayload(t, b2, "MSG_MARK_READ", &markRead, func() bool { return markRead.ConvId == convID && markRead.ReadSeq == messageSeq })
		var receipt msgpb.MarkReadNotice
		expectEventPayload(t, a1, "MSG_READ_RECEIPT", &receipt, func() bool { return receipt.ConvId == convID && receipt.ReadSeq == messageSeq })
	})
	t.Run("MSG_ACK 大小写、重复、越界、跨会话和错误 msg_id", func(t *testing.T) {
		for _, messageType := range []string{"MSG_ACK", "msg_ack", "MSG_ACK"} {
			ack, err := sendAndReadACK(eventContext(ctx, f.cfg.EventTimeout), b1, messageType, convID, messageSeq, messageID)
			if err != nil {
				t.Errorf("%s 失败: %v", messageType, err)
				continue
			}
			if ack.ConvId != convID || ack.Seq < messageSeq {
				t.Errorf("%s ACK 回执错误: %#v", messageType, ack)
			}
		}
		overflow, err := sendExpectError(eventContext(ctx, f.cfg.EventTimeout), b1, "MSG_ACK", convID, messageSeq+100, messageID)
		if err != nil {
			t.Errorf("超过已下发 seq 的 ACK 未返回 error: %v", err)
		} else if overflow.GetCode() == 0 {
			t.Errorf("超过已下发 seq 的 ACK 返回了成功错误帧: %#v", overflow)
		}
		otherConv, err := sendExpectError(eventContext(ctx, f.cfg.EventTimeout), b1, "MSG_ACK", "not-delivered-conv", messageSeq, messageID)
		if err != nil {
			t.Errorf("不同会话 ACK 未返回 error: %v", err)
		} else if otherConv.GetCode() == 0 {
			t.Errorf("不同会话 ACK 返回了成功错误帧: %#v", otherConv)
		}
		wrongID, err := sendAndReadACK(eventContext(ctx, f.cfg.EventTimeout), b1, "MSG_ACK", convID, messageSeq, "wrong-msg-id")
		if err != nil {
			t.Errorf("conv_id 正确但 msg_id 错误没有返回 ACK_ACK/error: %v", err)
		} else {
			t.Logf("观察：错误 msg_id 仍返回 ACK_ACK，回执=%#v", wrongID)
		}

		secondResponse, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/messages/send", a1Token.AccessToken, a1Token.ID, map[string]any{
			"clientMsgId": "ws-ack-next-" + f.suffix, "convType": 1, "targetUuid": b.UUID, "msgType": 1, "content": `{"text":"ack-next"}`,
		})
		if err != nil {
			t.Fatal(err)
		}
		var second map[string]any
		requireJSONData(t, "发送第二条 ACK 消息", secondResponse, &second)
		var pushed msgpb.MsgItem
		expectEventPayload(t, b1, "MSG_PUSH", &pushed, func() bool { return pushed.MsgId == stringValue(second, "msgId") })
		high, err := sendAndReadACK(eventContext(ctx, f.cfg.EventTimeout), b1, "MSG_ACK", convID, int64Value(second, "seq"), stringValue(second, "msgId"))
		if err != nil {
			t.Fatal(err)
		}
		low, err := sendAndReadACK(eventContext(ctx, f.cfg.EventTimeout), b1, "MSG_ACK", convID, messageSeq, messageID)
		if err != nil {
			t.Fatal(err)
		}
		if low.Seq != high.Seq {
			t.Errorf("ACK Redis 位点发生回退：high=%d low=%d", high.Seq, low.Seq)
		}
	})

	t.Run("GROUP_STATE_CHANGED、GROUP_MEMBER_MUTED、GROUP_MEMBER_REMOVED", func(t *testing.T) {
		// 直接把 B 放入创建请求，避免“创建后立即添加成员”的缓存传播窗口；
		// 后续用群资料更新触发第二次 GROUP_STATE_CHANGED，再验证成员广播。
		create, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/groups", a1Token.AccessToken, a1Token.ID, map[string]any{"name": "ws-state-" + f.suffix, "memberUuids": []string{b.UUID}})
		if err != nil {
			t.Fatal(err)
		}
		var groupData map[string]any
		requireJSONData(t, "创建实时事件群", create, &groupData)
		groupUUID := stringValue(groupData, "groupUuid")
		var created realtimepb.GroupStateChangedPayload
		expectEventPayload(t, a1, "GROUP_STATE_CHANGED", &created, func() bool { return created.GroupUuid == groupUUID })
		var createdB realtimepb.GroupStateChangedPayload
		expectEventPayload(t, b1, "GROUP_STATE_CHANGED", &createdB, func() bool { return createdB.GroupUuid == groupUUID })

		update, err := f.api.DoJSON(ctx, "PATCH", "/api/v1/auth/groups/"+groupUUID, a1Token.AccessToken, a1Token.ID, map[string]any{"name": "ws-state-updated-" + f.suffix})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "更新实时事件群资料", update)
		var updateState realtimepb.GroupStateChangedPayload
		expectEventPayload(t, a1, "GROUP_STATE_CHANGED", &updateState, func() bool { return updateState.GroupUuid == groupUUID })
		var updateStateB realtimepb.GroupStateChangedPayload
		expectEventPayload(t, b1, "GROUP_STATE_CHANGED", &updateStateB, func() bool { return updateStateB.GroupUuid == groupUUID })

		mute, err := f.api.DoJSON(ctx, "PATCH", "/api/v1/auth/groups/"+groupUUID+"/members/"+b.UUID+"/mute", a1Token.AccessToken, a1Token.ID, map[string]any{"muteUntil": time.Now().Add(time.Minute).UnixMilli()})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "实时群禁言成员", mute)
		var muted realtimepb.GroupMemberMutedPayload
		expectEventPayload(t, b1, "GROUP_MEMBER_MUTED", &muted, func() bool { return muted.GroupUuid == groupUUID && muted.UserUuid == b.UUID && muted.MuteUntil > 0 })
		var muteState realtimepb.GroupStateChangedPayload
		expectEventPayload(t, a1, "GROUP_STATE_CHANGED", &muteState, func() bool { return muteState.GroupUuid == groupUUID })

		remove, err := f.api.DoJSON(ctx, "DELETE", "/api/v1/auth/groups/"+groupUUID+"/members/"+b.UUID, a1Token.AccessToken, a1Token.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "实时群移除成员", remove)
		var removed realtimepb.GroupMemberRemovedPayload
		expectEventPayload(t, b1, "GROUP_MEMBER_REMOVED", &removed, func() bool { return removed.GroupUuid == groupUUID && removed.UserUuid == b.UUID })
		var removeState realtimepb.GroupStateChangedPayload
		expectEventPayload(t, a1, "GROUP_STATE_CHANGED", &removeState, func() bool { return removeState.GroupUuid == groupUUID })
	})
	t.Run("GROUP_JOIN_REQUEST_CREATED、GROUP_JOIN_REQUEST_REVIEWED、GROUP_DISMISSED", func(t *testing.T) {
		create, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/groups", a1Token.AccessToken, a1Token.ID, map[string]any{"name": "ws-review-" + f.suffix, "memberUuids": []string{b.UUID}})
		if err != nil {
			t.Fatal(err)
		}
		var groupData map[string]any
		requireJSONData(t, "创建实时审核群", create, &groupData)
		groupUUID := stringValue(groupData, "groupUuid")
		// 创建群接口默认 addMode=0；实时审批事件需要先切换为待审核模式。
		setReviewMode, err := f.api.DoJSON(ctx, "PATCH", "/api/v1/auth/groups/"+groupUUID, a1Token.AccessToken, a1Token.ID, map[string]any{"addMode": 1})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "设置实时审核群加群方式", setReviewMode)
		apply, err := f.api.DoJSON(ctx, "POST", "/api/v1/auth/groups/"+groupUUID+"/apply", c1Token.AccessToken, c1Token.ID, map[string]any{"reason": "ws-review"})
		if err != nil {
			t.Fatal(err)
		}
		var applyData map[string]any
		requireJSONData(t, "创建实时入群申请", apply, &applyData)
		applyID := int64Value(applyData, "applyId")
		if applyID <= 0 {
			t.Fatalf("实时审批群申请未返回有效 applyId：data=%#v", applyData)
		}
		var created realtimepb.GroupJoinRequestCreatedPayload
		expectEventPayload(t, a1, "GROUP_JOIN_REQUEST_CREATED", &created, func() bool { return created.GroupUuid == groupUUID && created.ApplyId == applyID })
		review, err := f.api.DoJSON(ctx, "POST", fmt.Sprintf("/api/v1/auth/groups/%s/join-requests/%d/review", groupUUID, applyID), a1Token.AccessToken, a1Token.ID, map[string]any{"action": 1})
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "审批实时入群申请", review)
		var reviewed realtimepb.GroupJoinRequestReviewedPayload
		expectEventPayload(t, c1, "GROUP_JOIN_REQUEST_REVIEWED", &reviewed, func() bool {
			return reviewed.GroupUuid == groupUUID && reviewed.ApplyId == applyID && reviewed.Action == 1
		})
		dismiss, err := f.api.DoJSON(ctx, "DELETE", "/api/v1/auth/groups/"+groupUUID, a1Token.AccessToken, a1Token.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		requireHTTPSuccess(t, "实时解散群", dismiss)
		var dismissedA, dismissedB realtimepb.GroupDismissedPayload
		expectEventPayload(t, a1, "GROUP_DISMISSED", &dismissedA, func() bool { return dismissedA.GroupUuid == groupUUID })
		expectEventPayload(t, b1, "GROUP_DISMISSED", &dismissedB, func() bool { return dismissedB.GroupUuid == groupUUID })
	})
}

func eventContext(parent context.Context, timeout time.Duration) context.Context {
	ctx, _ := context.WithTimeout(parent, timeout)
	return ctx
}

func expectEventPayload(t *testing.T, client *WSClient, eventType string, target proto.Message, valid func() bool) {
	t.Helper()
	// 同一个用户可能在前一个 HTTP 操作中产生了同类型异步事件，而 Kafka
	// 消费和 WebSocket 推送又不保证严格紧跟 HTTP 响应完成。因此不能只读
	// 第一条同类型事件就判定失败，必须持续读取到 payload 与当前操作匹配，
	// 否则会把“旧群的 GROUP_STATE_CHANGED”误报成“当前群事件缺失”。
	timeout := envDuration("LCCHAT_EVENT_TIMEOUT", 12*time.Second)
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Errorf("%s 等待 %s 匹配 payload 超时", client.Name, eventType)
			return
		}
		envelope, err := client.ReadUntil(eventContext(context.Background(), remaining), eventType)
		if err != nil {
			t.Errorf("%s 等待 %s 失败: %v", client.Name, eventType, err)
			return
		}
		if err := proto.Unmarshal(envelope.Data, target); err != nil {
			t.Errorf("%s 解析 %s payload 失败: %v", client.Name, eventType, err)
			return
		}
		if valid == nil || valid() {
			return
		}
		// 不匹配的同类型事件已经被消费掉，继续等待当前操作产生的事件。
	}
}

func sendAndReadACK(ctx context.Context, client *WSClient, messageType, convID string, seq int64, messageID string) (*connectpb.MessageAckAck, error) {
	if err := client.Send(messageType, &connectpb.MessageAck{ConvId: convID, Seq: seq, MsgId: messageID}, seq); err != nil {
		return nil, err
	}
	envelope, err := client.ReadUntil(ctx, "MSG_ACK_ACK")
	if err != nil {
		return nil, err
	}
	var ack connectpb.MessageAckAck
	if err := proto.Unmarshal(envelope.Data, &ack); err != nil {
		return nil, err
	}
	return &ack, nil
}

func sendExpectError(ctx context.Context, client *WSClient, messageType, convID string, seq int64, messageID string) (*connectpb.ErrorFrame, error) {
	if err := client.Send(messageType, &connectpb.MessageAck{ConvId: convID, Seq: seq, MsgId: messageID}, seq); err != nil {
		return nil, err
	}
	envelope, err := client.ReadUntil(ctx, "error")
	if err != nil {
		return nil, err
	}
	var frame connectpb.ErrorFrame
	if err := proto.Unmarshal(envelope.Data, &frame); err != nil {
		return nil, err
	}
	return &frame, nil
}
