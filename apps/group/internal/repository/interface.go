package repository

import (
	"context"

	"github.com/013677890/LCchat-Backend/model"
)

// IGroupRepository 定义 group-service 当前阶段需要的仓储抽象。
type IGroupRepository interface {
	// CreateGroup 创建群与初始成员关系。
	CreateGroup(ctx context.Context, group *model.GroupInfo, members []*model.GroupMember) error

	// AddMembers 向群内添加成员。
	AddMembers(ctx context.Context, groupUUID, operatorUUID string, members []*model.GroupMember) error

	// RemoveMember 移除或退出群成员。
	RemoveMember(ctx context.Context, groupUUID, operatorUUID, targetUUID string) error

	// DismissGroup 解散群。
	DismissGroup(ctx context.Context, groupUUID, operatorUUID string) error

	// UpdateGroupInfo 更新群资料。
	UpdateGroupInfo(ctx context.Context, groupUUID, operatorUUID string, name, avatar *string) error

	// GetGroupInfo 按群 UUID 获取有效群资料。
	GetGroupInfo(ctx context.Context, groupUUID string) (*model.GroupInfo, error)

	// GetGroupMembers 获取群内有效成员列表。
	GetGroupMembers(ctx context.Context, groupUUID string) ([]*model.GroupMember, error)

	// CheckGroupMember 检查指定用户是否仍是群内有效成员，并返回角色。
	CheckGroupMember(ctx context.Context, groupUUID, userUUID string) (bool, int8, error)

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
