package repository

import (
	"context"

	"time"

	"github.com/013677890/LCchat-Backend/model"

	"github.com/013677890/LCchat-Backend/pkg/groupevent"
)

// GroupInfoUpdates 描述群资料更新意图。

//

// 这里仅承载仍由 UpdateGroupInfo 负责的正式资料字段：

//  1. name / avatar 允许管理员及以上更新；

//  2. add_mode 仅允许群主更新；

//  3. notice 已拆到独立接口，避免不同权限矩阵继续混在同一写入口。

type GroupInfoUpdates struct {
	Name *string

	Avatar *string

	AddMode *int8
}

// ApplyJoinGroupResult 描述用户申请入群后的最终结果。

//

// 当 joined_directly=true 时表示 add_mode=0 已直接入群；

// 当 joined_directly=false 且 apply_id>0 时表示已创建待审批申请记录。

type ApplyJoinGroupResult struct {
	ApplyID int64

	JoinedDirectly bool
}

// CheckGroupSendPermissionResult 描述群消息发送权限检查结果。

//

// 该结果把“是否允许发送”和“为什么不允许”一起返回，方便 msg-service

// 在拒绝发送时给客户端明确提示，而不是只能得到一个模糊的权限失败。

type CheckGroupSendPermissionResult struct {
	CanSend bool

	Role int8

	Reason string

	MuteUntil int64

	MuteAll bool
}

// IsEmpty 判断当前更新意图是否为空。

func (u GroupInfoUpdates) IsEmpty() bool {

	return u.Name == nil && u.Avatar == nil && u.AddMode == nil

}

// IGroupRepository 定义 group-service 当前阶段需要的仓储抽象。

type IGroupRepository interface {

	// CreateGroup 创建群与初始成员关系。

	CreateGroup(ctx context.Context, group *model.GroupInfo, members []*model.GroupMember) error

	// AddMembers 向群内添加成员。

	AddMembers(ctx context.Context, groupUUID, operatorUUID string, members []*model.GroupMember) error

	// RemoveMember 移除或退出群成员。

	RemoveMember(ctx context.Context, groupUUID, operatorUUID, targetUUID string) error

	// LeaveGroup 当前用户主动退出群聊。

	LeaveGroup(ctx context.Context, groupUUID, operatorUUID string) error

	// DismissGroup 解散群。

	DismissGroup(ctx context.Context, groupUUID, operatorUUID string) error

	// UpdateGroupInfo 更新群资料。

	UpdateGroupInfo(ctx context.Context, groupUUID, operatorUUID string, updates GroupInfoUpdates) error

	// UpdateGroupNotice 独立更新群公告。

	UpdateGroupNotice(ctx context.Context, groupUUID, operatorUUID, notice string) error

	// TransferGroupOwner 转让群主。

	TransferGroupOwner(ctx context.Context, groupUUID, operatorUUID, targetUserUUID string) error

	// UpdateMemberRole 更新群成员角色。

	UpdateMemberRole(ctx context.Context, groupUUID, operatorUUID, targetUserUUID string, role int8) error

	// SearchGroupMembers 搜索群成员并返回分页结果。

	SearchGroupMembers(ctx context.Context, groupUUID, operatorUUID, keyword string, page, pageSize int) ([]*model.GroupMember, int64, error)

	// UpdateMyGroupNickname 更新当前用户自己的群名片。

	UpdateMyGroupNickname(ctx context.Context, groupUUID, userUUID, nickname string) error

	// MuteGroupMember 设置或取消指定成员单人禁言。

	MuteGroupMember(ctx context.Context, groupUUID, operatorUUID, targetUserUUID string, muteUntil *time.Time) error

	// UpdateGroupMuteSetting 更新全员禁言开关。

	UpdateGroupMuteSetting(ctx context.Context, groupUUID, operatorUUID string, muteAll bool) error

	// ApplyJoinGroup 按当前群 add_mode 执行直加入群或创建待审批申请。

	ApplyJoinGroup(ctx context.Context, groupUUID, applicantUUID, reason string) (ApplyJoinGroupResult, error)

	// CancelJoinGroupApplication 撤销当前用户自己发起的待审批入群申请。

	CancelJoinGroupApplication(ctx context.Context, groupUUID, applicantUUID string) error

	// GetMyJoinGroupApplication 获取当前用户在指定群的最新申请状态。

	GetMyJoinGroupApplication(ctx context.Context, groupUUID, applicantUUID string) (*model.GroupJoinRequest, error)

	// ListMyJoinGroupApplications 获取当前用户发起的入群申请列表。

	ListMyJoinGroupApplications(ctx context.Context, applicantUUID string, status *int8, page, pageSize int) ([]*model.GroupJoinRequest, int64, error)

	// ReviewJoinGroup 审批入群申请。

	ReviewJoinGroup(ctx context.Context, groupUUID, operatorUUID string, applyID int64, action int8, remark string) error

	// ListJoinRequests 获取群待审批入群申请列表。

	ListJoinRequests(ctx context.Context, groupUUID, operatorUUID string, page, pageSize int) ([]*model.GroupJoinRequest, int64, error)

	// ListReviewedJoinRequests 获取群已审批入群申请列表。

	ListReviewedJoinRequests(ctx context.Context, groupUUID, operatorUUID string, status *int8, page, pageSize int) ([]*model.GroupJoinRequest, int64, error)

	// GetJoinRequestPendingCount 获取群待审批入群申请数量。

	GetJoinRequestPendingCount(ctx context.Context, groupUUID, operatorUUID string) (int64, error)

	// GetGroupInfo 按群 UUID 获取有效群资料。

	GetGroupInfo(ctx context.Context, groupUUID string) (*model.GroupInfo, error)

	// GetGroupsByUUIDs 按群 UUID 批量查询群资料，用于申请列表补齐展示字段。

	GetGroupsByUUIDs(ctx context.Context, groupUUIDs []string) (map[string]*model.GroupInfo, error)

	// GetGroupMembers 获取群内有效成员列表。

	GetGroupMembers(ctx context.Context, groupUUID string) ([]*model.GroupMember, error)

	// CheckGroupMember 检查指定用户是否仍是群内有效成员，并返回角色。

	CheckGroupMember(ctx context.Context, groupUUID, userUUID string) (bool, int8, error)

	// CheckGroupSendPermission 检查用户是否允许在群内发送消息。

	CheckGroupSendPermission(ctx context.Context, groupUUID, userUUID string) (CheckGroupSendPermissionResult, error)

	// ListUserGroups 获取当前用户所属的有效群列表。

	ListUserGroups(ctx context.Context, userUUID string) ([]*model.GroupInfo, error)

	// GetUserProfiles 按用户 UUID 批量查询资料，用于补齐昵称和头像。

	GetUserProfiles(ctx context.Context, userUUIDs []string) (map[string]*model.UserProfile, error)
}

// GroupRepository 是 IGroupRepository 的语义化别名。

type GroupRepository = IGroupRepository

// IGroupCacheProjectorRepository 定义 group.cache 投影链路需要的最小仓储能力。

//

// 这里单独拆一个接口，而不是把投影能力塞进 IGroupRepository，原因有两点：

//  1. service 层只依赖业务读写能力，不需要知道 Kafka 投影细节；

//  2. consumer 只关心“如何把事件同步到 Redis”，避免把两类职责耦在同一抽象上。

type IGroupCacheProjectorRepository interface {

	// ApplyGroupCacheEvent 根据 outbox 事件 payload 同步 Redis 投影。

	//

	// 约束：

	//  1. 该方法只负责缓存投影，不做业务权限判断；

	//  2. 遇到 Redis 可重试错误时直接返回 error，由 Kafka 手动提交模式负责重试；

	//  3. payload 非法时返回明确错误，交给上层决定是否跳过。

	ApplyGroupCacheEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error
}

// GroupCacheProjectorRepository 是 IGroupCacheProjectorRepository 的语义化别名。

type GroupCacheProjectorRepository = IGroupCacheProjectorRepository
