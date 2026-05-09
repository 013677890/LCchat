package repository

import (
	"context"

	"github.com/013677890/LCchat-Backend/model"
)

// IGroupRepository 定义 group-service 当前阶段需要的只读仓储抽象。
//
// 本轮先补齐“可查询”的最小闭环，因此接口只暴露三类读能力：
//  1. 群本身的基础资料；
//  2. 群成员关系；
//  3. 组装成员展示信息所需的用户资料快照。
//
// 写操作仍然故意留空，避免在群管理规则尚未确认前过早固化仓储契约。
type IGroupRepository interface {
	// GetGroupInfo 按群 UUID 获取有效群资料。
	GetGroupInfo(ctx context.Context, groupUUID string) (*model.GroupInfo, error)

	// GetGroupMembers 获取群内有效成员列表。
	GetGroupMembers(ctx context.Context, groupUUID string) ([]*model.GroupMember, error)

	// ListUserGroups 获取当前用户所属的有效群列表。
	ListUserGroups(ctx context.Context, userUUID string) ([]*model.GroupInfo, error)

	// GetUserProfiles 按用户 UUID 批量查询资料，用于补齐昵称和头像。
	GetUserProfiles(ctx context.Context, userUUIDs []string) (map[string]*model.UserProfile, error)
}

// GroupRepository 是 IGroupRepository 的语义化别名。
//
// 保留这个别名是为了和其他服务的命名风格保持一致，
// 后续如果需要在构造函数、测试桩、注入点中表达“这是 group 仓储依赖”，可直接使用该别名。
type GroupRepository = IGroupRepository
