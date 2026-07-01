package message

import "errors"

// 领域错误定义
var (
	// ErrIdempotentProcessing 另一个相同请求正在处理中（SETNX 返回 false 且值为 "PROCESSING:{token}"）
	ErrIdempotentProcessing = errors.New("message: idempotent request is processing")

	// ErrDuplicateMessage 消息重复（DB 唯一索引冲突）
	ErrDuplicateMessage = errors.New("message: duplicate message")

	// ErrDuplicateMessageSeq 会话内 seq 重复，通常表示 Redis seq 计数器丢失或回退。
	ErrDuplicateMessageSeq = errors.New("message: duplicate message seq")

	// ErrDuplicateMessageID 全局消息 ID 重复，理论上极少发生，不能按客户端幂等处理。
	ErrDuplicateMessageID = errors.New("message: duplicate message id")

	// ErrUnsupportedMsgType 消息类型不支持
	ErrUnsupportedMsgType = errors.New("message: unsupported msg type")

	// ErrMessageNotFound 消息不存在
	ErrMessageNotFound = errors.New("message: not found")

	// ErrRecallTimeout 撤回超时（超过撤回窗口）
	ErrRecallTimeout = errors.New("message: recall timeout")

	// ErrRecallNoPermission 无权限撤回（非发送者且非群管理员）
	ErrRecallNoPermission = errors.New("message: recall no permission")

	// ErrMessageAlreadyRecalled 消息已被撤回
	ErrMessageAlreadyRecalled = errors.New("message: already recalled")
)

// 幂等 Redis 值标记
const (
	idempotentProcessingPrefix = "PROCESSING:"
	idempotentLeaseTokenBytes  = 16
	idempotentLockTTLSec       = 10 // SETNX 初始 TTL（秒），防止处理超时后锁永远不释放
)

// 拉取消息方向
const (
	DirectionForward  = 1 // seq > anchor（拉新消息）
	DirectionBackward = 2 // seq < anchor（拉历史）
)
