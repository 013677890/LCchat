package rediskey

import (
	"fmt"
	"time"
)

// MsgDeviceAckTTL 设备消息 ACK 位点 TTL。
// ACK 用于短期可靠同步与排障观测，长期状态以后再落 MySQL。
const MsgDeviceAckTTL = 30 * 24 * time.Hour

// MsgDeviceAckKey 生成设备级消息 ACK 位点 Key: msg:ack:{user_uuid}:{device_id}:{conv_id}
// 数据类型: String (max acked seq)
// TTL: MsgDeviceAckTTL
func MsgDeviceAckKey(userUUID, deviceID, convID string) string {
	return fmt.Sprintf("msg:ack:%s:%s:%s", userUUID, deviceID, convID)
}
