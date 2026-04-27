package model

// MsgType = 消息类型代号。
// 1-99 为普通业务消息，100+ 为系统/控制消息。
type MsgType int16

const (
	MsgTypeText     MsgType = 1
	MsgTypeImage    MsgType = 2
	MsgTypeVoice    MsgType = 3
	MsgTypeVideo    MsgType = 4
	MsgTypeFile     MsgType = 5
	MsgTypeLocation MsgType = 6

	MsgTypeSystem MsgType = 100
)

// ParseMsgType 校验并转换消息类型。
func ParseMsgType(value int32) (MsgType, bool) {
	msgType := MsgType(value)
	switch msgType {
	case MsgTypeText,
		MsgTypeImage,
		MsgTypeVoice,
		MsgTypeVideo,
		MsgTypeFile,
		MsgTypeLocation,
		MsgTypeSystem:
		return msgType, true
	default:
		return 0, false
	}
}
