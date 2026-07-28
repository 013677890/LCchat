package conversation

import "errors"

// 领域错误定义
var (
	// ErrConversationNotFound 会话不存在
	ErrConversationNotFound = errors.New("conversation: not found")

	// ErrInvalidCursor 分页游标格式错误
	ErrInvalidCursor = errors.New("conversation: invalid cursor")

	// ErrInvalidGroupMembershipState 表示 GROUP conversation 没有显式的成员投影状态。
	// 出现该错误说明写路径违反“GROUP 只能是 Active/Left”的模型契约。
	ErrInvalidGroupMembershipState = errors.New("conversation: invalid group membership state")

	// ErrGroupMembershipQuerierUnavailable 表示成员投影尚未建行时，无法向 group-service
	// 做权威单成员点查。此时必须失败，禁止仅凭 conv_id 或旧 conversation 行放行。
	ErrGroupMembershipQuerierUnavailable = errors.New("conversation: group membership querier unavailable")
)
