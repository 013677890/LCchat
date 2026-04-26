package consumer

func ackRequiredForEvent(eventType string, seq int64) bool {
	return eventType == "MSG_PUSH" && seq > 0
}