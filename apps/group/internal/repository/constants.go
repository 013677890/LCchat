package repository

// 群、成员、入群申请的权威状态与角色枚举。
//
// 这些值与 MySQL 列、Redis 投影 JSON、group.cache 快照共用同一套数字约定。
// 子包必须引用这里的常量，禁止在 store / cache / projection 里再写一份魔法数字。
const (
	// GroupStatusNormal 表示群可被查询、发言和写入。
	GroupStatusNormal int8 = 0
	// GroupStatusDisabled 表示群被停用，对外按不存在处理。
	GroupStatusDisabled int8 = 1
	// GroupStatusDismissed 表示群已解散。
	GroupStatusDismissed int8 = 2

	// MemberRoleMember 是普通成员。
	MemberRoleMember int8 = 0
	// MemberRoleAdmin 是管理员。
	MemberRoleAdmin int8 = 1
	// MemberRoleOwner 是群主。
	MemberRoleOwner int8 = 2

	// MemberStatusNormal 表示成员关系仍然有效。
	MemberStatusNormal int8 = 0
	// MemberStatusQuit 表示主动退群。
	MemberStatusQuit int8 = 1
	// MemberStatusKicked 表示被移出群。
	MemberStatusKicked int8 = 2

	// JoinRequestStatusPending 表示申请待审批。
	JoinRequestStatusPending int8 = 0
	// JoinRequestStatusApproved 表示申请已通过。
	JoinRequestStatusApproved int8 = 1
	// JoinRequestStatusRejected 表示申请已拒绝。
	JoinRequestStatusRejected int8 = 2
	// JoinRequestStatusCanceled 表示申请人主动撤销。
	JoinRequestStatusCanceled int8 = 3

	// MaxGroupAdminCount 是单个群允许的管理员席位上限，不包含群主。
	MaxGroupAdminCount int64 = 10
)
