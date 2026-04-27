package conversation

import "errors"

// 领域错误定义
var (
	// ErrConversationNotFound 会话不存在
	ErrConversationNotFound = errors.New("conversation: not found")

	// ErrInvalidCursor 分页游标格式错误
	ErrInvalidCursor = errors.New("conversation: invalid cursor")
)
