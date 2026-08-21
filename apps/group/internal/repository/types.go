package repository

import "time"

// GroupInfoUpdates 描述群资料更新意图。
//
// 这里仅承载仍由 UpdateGroupInfo 负责的正式资料字段：
//  1. name / avatar 允许管理员及以上更新；
//  2. add_mode 仅允许群主更新；
//  3. notice 已拆到独立接口，避免不同权限矩阵继续混在同一写入口。
type GroupInfoUpdates struct {
	Name    *string
	Avatar  *string
	AddMode *int8
}

// IsEmpty 判断当前更新意图是否为空。
func (u GroupInfoUpdates) IsEmpty() bool {
	return u.Name == nil && u.Avatar == nil && u.AddMode == nil
}

// ApplyJoinGroupResult 描述用户申请入群后的最终结果。
//
// 当 JoinedDirectly=true 时表示 add_mode=0 已直接入群；
// 当 JoinedDirectly=false 且 ApplyID>0 时表示已创建待审批申请记录。
type ApplyJoinGroupResult struct {
	ApplyID        int64
	JoinedDirectly bool
}

// CheckGroupSendPermissionResult 描述群消息发送权限检查结果。
//
// 该结果把“是否允许发送”和“为什么不允许”一起返回，方便 msg-service
// 在拒绝发送时给客户端明确提示，而不是只能得到一个模糊的权限失败。
//
// 数据来源是最终一致的 Redis 投影：成员 field 点查 + 群资料单值读取。
// 踢人、禁言提交后允许存在 CDC 投影窗口，不在这里回源 MySQL 做强一致裁决。
type CheckGroupSendPermissionResult struct {
	CanSend   bool
	Role      int8
	Reason    string
	MuteUntil int64
	MuteAll   bool
}

// GroupCacheReconcileTarget 是周期对账扫描使用的轻量游标记录。
//
// 使用数据库自增 ID 做 keyset 游标，避免 OFFSET 在大表扫描时反复跳过前缀行；
// UUID 才是实际聚合标识，ID 只服务于本轮稳定分页。
type GroupCacheReconcileTarget struct {
	ID        int64  `gorm:"column:id"`
	GroupUUID string `gorm:"column:uuid"`
}

// GroupSendDenyReason 是发送权限拒绝原因的稳定字符串。
//
// 当前仍通过字符串跨服务传递；后续若改为 Proto Enum，应先改这里的唯一定义，
// 禁止 group 与 msg 各自维护一份魔法值。
const (
	GroupSendDenyNotMember   = "not_group_member"
	GroupSendDenyGroupMuted  = "group_muted"
	GroupSendDenyMemberMuted = "member_muted"
)

// MuteUntilEqual 判断两份禁言截止时间是否代表同一个业务状态。
func MuteUntilEqual(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

// CloneTimePtr 复制时间指针，避免调用方后续修改同一地址影响快照或缓存编码。
func CloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
