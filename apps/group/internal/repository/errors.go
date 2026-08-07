package repository

import "errors"

// Group repository 层领域错误定义。
var (
	// ErrGroupDismissed 表示群已解散。
	ErrGroupDismissed = errors.New("group dismissed")
	// ErrNoPermission 表示当前操作者没有执行群管理操作的权限。
	ErrNoPermission = errors.New("no permission")
	// ErrCannotKickOwner 表示不能移除群主。
	ErrCannotKickOwner = errors.New("cannot kick owner")
	// ErrCannotQuitAsOwner 表示群主不能主动退群。
	ErrCannotQuitAsOwner = errors.New("cannot quit as owner")
	// ErrGroupMemberNotFound 表示目标成员不存在或已不在群内。
	ErrGroupMemberNotFound = errors.New("group member not found")
	// ErrGroupApplyNotFound 表示入群申请不存在或已处理。
	ErrGroupApplyNotFound = errors.New("group join request not found")
	// ErrGroupApplyAlreadyExists 表示当前用户已存在待审批入群申请。
	ErrGroupApplyAlreadyExists = errors.New("group join request already exists")
	// ErrAlreadyGroupMember 表示目标用户已经是群内有效成员。
	ErrAlreadyGroupMember = errors.New("already group member")
	// ErrAdminLimitExceeded 表示管理员数量已达到上限。
	ErrAdminLimitExceeded = errors.New("admin limit exceeded")
	// ErrInvalidProjectorPayload 表示 group.cache 事件内容不满足投影所需最小约束。
	//
	// 这类错误通常不是基础设施抖动，而是事件内容本身不完整；
	// 上层 consumer 可以据此决定记录告警后跳过，而不是无限重试同一坏消息。
	ErrInvalidProjectorPayload = errors.New("invalid projector payload")
)
