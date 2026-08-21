package repository

import (
	"context"
	"time"

	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/groupevent"
)

// IGroupRepository 定义 group-service 当前阶段需要的仓储抽象。
//
// service 只依赖本接口。运行时实现是 repository/compose.Facade：
// 写命令转发 store，展示与权限读取转发 cache。
// 父包只保留协议，不再作为运行时仓储实现，避免再 import cache 造成循环依赖。
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
	// UpdateGroupMemberNickname 管理员或群主修改指定成员群名片。
	UpdateGroupMemberNickname(ctx context.Context, groupUUID, operatorUUID, targetUserUUID, nickname string) error
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
	// GetJoinRequestApplicant 获取指定申请对应的申请人 UUID。
	GetJoinRequestApplicant(ctx context.Context, groupUUID string, applyID int64) (string, error)
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
	// SearchGroups 按群名或完整群号搜索正常群。
	SearchGroups(ctx context.Context, keyword string, page, pageSize int) ([]*model.GroupInfo, int64, error)
	// CheckGroupMember 检查指定用户是否仍是群内有效成员，并返回角色。
	CheckGroupMember(ctx context.Context, groupUUID, userUUID string) (bool, int8, error)
	// CheckGroupSendPermission 检查用户是否允许在群内发送消息。
	CheckGroupSendPermission(ctx context.Context, groupUUID, userUUID string) (CheckGroupSendPermissionResult, error)
	// ListUserGroups 获取当前用户所属的有效群列表。
	ListUserGroups(ctx context.Context, userUUID string) ([]*model.GroupInfo, error)
	// GetUserProfiles 按用户 UUID 批量查询资料，用于补齐昵称和头像。
	GetUserProfiles(ctx context.Context, userUUIDs []string) (map[string]*model.UserProfile, error)
}

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
	//  3. payload 非法时返回 ErrInvalidProjectorPayload，上层必须标记永久错误并立即落死信。
	ApplyGroupCacheEvent(ctx context.Context, payload groupevent.GroupCacheEventPayload) error
	// ReconcileGroupCache 从 MySQL 权威快照重建指定群的资料、成员、待审批申请
	// 以及历史成员对应的用户群反向索引；所有写入仍受 cache_version 栅栏保护。
	ReconcileGroupCache(ctx context.Context, groupUUID string) error
	// ListGroupCacheReconcileTargets 使用 ID 游标分页扫描群聚合，供周期对账任务调用。
	ListGroupCacheReconcileTargets(ctx context.Context, afterID int64, limit int) ([]GroupCacheReconcileTarget, error)
}
