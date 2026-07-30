package svc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	connectpb "github.com/013677890/LCchat-Backend/apps/connect/pb"
	rediskey "github.com/013677890/LCchat-Backend/consts/redisKey"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
)

const (
	// EnvelopeTypeMessageAck 是客户端收到需回执下行消息后的上行 ACK 类型。
	EnvelopeTypeMessageAck = "MSG_ACK"
	// EnvelopeTypeMessageAckLower 兼容小写 WebSocket 上行风格。
	EnvelopeTypeMessageAckLower = "msg_ack"
	// EnvelopeTypeMessageAckAck 是服务端确认 ACK 已处理的下行帧类型。
	EnvelopeTypeMessageAckAck = "MSG_ACK_ACK"
	// EnvelopeTypeError 是 WebSocket 协议层错误帧类型。
	EnvelopeTypeError = "error"
)

var errAckStoreUnavailable = errors.New("message ack store unavailable")

var storeMessageAckScript = redis.NewScript(`
local current = redis.call("GET", KEYS[1])
local next_seq = tonumber(ARGV[1])
local ttl_sec = tonumber(ARGV[2])

if not next_seq or next_seq <= 0 then
  return redis.error_reply("invalid ack seq")
end

if (not current) or ((tonumber(current) or 0) < next_seq) then
  redis.call("SET", KEYS[1], ARGV[1], "EX", ARGV[2])
  return next_seq
end

redis.call("EXPIRE", KEYS[1], ARGV[2])
return tonumber(current) or 0
`)

// MessageAckData 定义客户端消息 ACK 上行 data 结构。
// 二进制 wire format 对应 proto/connect/ws_control.proto 中的 MessageAck。
type MessageAckData struct {
	ConvID string
	Seq    int64
	MsgID  string
}

// MessageAckResponseData 定义服务端 ACK 确认帧 data 结构。
// 二进制 wire format 对应 proto/connect/ws_control.proto 中的 MessageAckAck。
type MessageAckResponseData struct {
	ConvID string
	Seq    int64
}

// ParseProtoEnvelope 解析 WebSocket Binary 帧中的 connect.MessageEnvelope。
func (s *ConnectService) ParseProtoEnvelope(raw []byte) (*connectpb.MessageEnvelope, error) {
	var envelope connectpb.MessageEnvelope
	if err := proto.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	envelope.Type = strings.TrimSpace(envelope.Type)
	if envelope.Type == "" {
		return nil, errors.New("type is required")
	}
	return &envelope, nil
}

// MarshalProtoEnvelope 组装 WebSocket Binary 帧中的 connect.MessageEnvelope。
func (s *ConnectService) MarshalProtoEnvelope(msgType string, data []byte, seq int64) ([]byte, error) {
	return proto.Marshal(&connectpb.MessageEnvelope{
		Type:     msgType,
		Data:     data,
		Seq:      seq,
		ServerTs: time.Now().UnixMilli(),
	})
}

// ParseMessageAck 解析并校验客户端 ACK protobuf data。
func (s *ConnectService) ParseMessageAck(data []byte) (*MessageAckData, error) {
	if len(data) == 0 {
		return nil, errors.New("ack data is required")
	}

	var ack MessageAckData
	idx := 0
	for idx < len(data) {
		key, err := readProtoVarint(data, &idx)
		if err != nil {
			return nil, err
		}
		fieldNum := int(key >> 3)
		wireType := key & 0x7

		switch fieldNum {
		case 1: // conv_id
			if wireType != 2 {
				return nil, errors.New("invalid conv_id wire type")
			}
			value, readErr := readProtoBytes(data, &idx)
			if readErr != nil {
				return nil, readErr
			}
			ack.ConvID = strings.TrimSpace(string(value))

		case 2: // seq
			if wireType != 0 {
				return nil, errors.New("invalid seq wire type")
			}
			value, readErr := readProtoVarint(data, &idx)
			if readErr != nil {
				return nil, readErr
			}
			ack.Seq = int64(value)

		case 3: // msg_id
			if wireType != 2 {
				return nil, errors.New("invalid msg_id wire type")
			}
			value, readErr := readProtoBytes(data, &idx)
			if readErr != nil {
				return nil, readErr
			}
			ack.MsgID = strings.TrimSpace(string(value))

		default:
			if err := skipProtoField(data, &idx, wireType); err != nil {
				return nil, err
			}
		}
	}

	if ack.ConvID == "" {
		return nil, errors.New("conv_id is required")
	}
	if ack.Seq <= 0 {
		return nil, errors.New("seq must be positive")
	}

	return &ack, nil
}

// MarshalMessageAckAck 序列化服务端 ACK 确认 payload。
func (s *ConnectService) MarshalMessageAckAck(data MessageAckResponseData) []byte {
	var out []byte
	out = appendProtoStringField(out, 1, data.ConvID)
	out = appendProtoInt64Field(out, 2, data.Seq)
	return out
}

// MarshalErrorFrame 序列化 WebSocket 协议层错误 payload。
func (s *ConnectService) MarshalErrorFrame(code int, message string) []byte {
	var out []byte
	out = appendProtoInt64Field(out, 1, int64(code))
	out = appendProtoStringField(out, 2, message)
	return out
}

// StoreMessageAck 记录 user+device+conv 维度的最大客户端 ACK seq。
// Redis 脚本保证并发情况下只会前进位点，不会被旧 ACK 覆盖回退。
func (s *ConnectService) StoreMessageAck(ctx context.Context, session *Session, ack *MessageAckData) (int64, error) {
	if s == nil || s.redisClient == nil {
		return 0, errAckStoreUnavailable
	}
	if session == nil || strings.TrimSpace(session.UserUUID) == "" || strings.TrimSpace(session.DeviceID) == "" {
		return 0, errors.New("session identity is required")
	}
	if ack == nil || strings.TrimSpace(ack.ConvID) == "" || ack.Seq <= 0 {
		return 0, errors.New("invalid message ack")
	}

	key := rediskey.MsgDeviceAckKey(session.UserUUID, session.DeviceID, ack.ConvID)
	stored, err := storeMessageAckScript.Run(ctx, s.redisClient, []string{key}, ack.Seq, int(rediskey.MsgDeviceAckTTL.Seconds())).Int64()
	if err != nil {
		return 0, fmt.Errorf("store message ack failed: %w", err)
	}
	return stored, nil
}

// IsMessageAckEnvelopeType 判断是否为客户端 ACK 上行类型。
func IsMessageAckEnvelopeType(msgType string) bool {
	return strings.EqualFold(strings.TrimSpace(msgType), EnvelopeTypeMessageAck) || strings.EqualFold(strings.TrimSpace(msgType), EnvelopeTypeMessageAckLower)
}

func appendProtoStringField(out []byte, fieldNum int, value string) []byte {
	if value == "" {
		return out
	}
	out = appendProtoVarint(out, uint64(fieldNum<<3|2))
	out = appendProtoVarint(out, uint64(len(value)))
	return append(out, value...)
}

func appendProtoInt64Field(out []byte, fieldNum int, value int64) []byte {
	if value == 0 {
		return out
	}
	out = appendProtoVarint(out, uint64(fieldNum<<3|0))
	return appendProtoVarint(out, uint64(value))
}

func appendProtoVarint(out []byte, value uint64) []byte {
	for value >= 0x80 {
		out = append(out, byte(value)|0x80)
		value >>= 7
	}
	return append(out, byte(value))
}

func readProtoVarint(data []byte, idx *int) (uint64, error) {
	var value uint64
	for shift := uint(0); shift < 64; shift += 7 {
		if *idx >= len(data) {
			return 0, errors.New("unexpected eof while reading varint")
		}
		b := data[*idx]
		*idx += 1
		value |= uint64(b&0x7f) << shift
		if b < 0x80 {
			return value, nil
		}
	}
	return 0, errors.New("varint overflow")
}

func readProtoBytes(data []byte, idx *int) ([]byte, error) {
	length, err := readProtoVarint(data, idx)
	if err != nil {
		return nil, err
	}
	if length > uint64(len(data)-*idx) {
		return nil, errors.New("unexpected eof while reading bytes")
	}
	start := *idx
	*idx += int(length)
	return data[start:*idx], nil
}

func skipProtoField(data []byte, idx *int, wireType uint64) error {
	switch wireType {
	case 0:
		_, err := readProtoVarint(data, idx)
		return err
	case 1:
		if len(data)-*idx < 8 {
			return errors.New("unexpected eof while skipping fixed64")
		}
		*idx += 8
		return nil
	case 2:
		_, err := readProtoBytes(data, idx)
		return err
	case 5:
		if len(data)-*idx < 4 {
			return errors.New("unexpected eof while skipping fixed32")
		}
		*idx += 4
		return nil
	default:
		return fmt.Errorf("unsupported wire type %d", wireType)
	}
}
