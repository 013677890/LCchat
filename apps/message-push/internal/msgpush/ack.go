package msgpush

// ackRequiredForEvent 判断该事件是否要求客户端回 ACK。
//
// 仅 MSG_PUSH 且 seq>0 需要 ACK：客户端用 seq 做会话有序确认；
// 撤回/已读类通知是派生状态，允许丢失后由拉取自愈，因此不强制 ACK。
// seq==0 视为合约不完整，无法形成有意义的确认位点，同样不要求 ACK。
func ackRequiredForEvent(eventType string, seq int64) bool {
	return eventType == "MSG_PUSH" && seq > 0
}
