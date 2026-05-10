package service

import (
	"context"

	pb "github.com/013677890/LCchat-Backend/apps/group/pb"
)

// IGroupService 定义 group-service 对外暴露的业务接口。
//
// 当前 proto 已经把群组服务的公共方法面定义出来了，
// 因此这里先完整镜像一份 service 接口，确保：
//  1. handler 只依赖抽象，不依赖具体实现；
//  2. 后续补充业务逻辑时不需要再改 handler / wire 的依赖方向；
//  3. 测试时可以方便地为每个方法注入 mock。
//
// 注意：当前阶段这些方法只是“骨架能力声明”，具体实现会统一返回“暂未实现”，
// 目的是先把服务边界搭起来，而不是提前塞入未经确认的业务规则。
type IGroupService interface {
	// CreateGroup 创建群。
	CreateGroup(ctx context.Context, req *pb.CreateGroupRequest) (*pb.CreateGroupResponse, error)

	// DismissGroup 解散群。
	DismissGroup(ctx context.Context, req *pb.DismissGroupRequest) error

	// GetGroupInfo 获取群资料。
	GetGroupInfo(ctx context.Context, req *pb.GetGroupInfoRequest) (*pb.GetGroupInfoResponse, error)

	// UpdateGroupInfo 更新群资料。
	UpdateGroupInfo(ctx context.Context, req *pb.UpdateGroupInfoRequest) error

	// AddMember 添加群成员。
	AddMember(ctx context.Context, req *pb.AddMemberRequest) error

	// RemoveMember 移除群成员。
	RemoveMember(ctx context.Context, req *pb.RemoveMemberRequest) error

	// GetMemberList 获取群成员列表。
	GetMemberList(ctx context.Context, req *pb.GetMemberListRequest) (*pb.GetMemberListResponse, error)

	// GetGroupList 获取当前用户的群列表。
	GetGroupList(ctx context.Context, req *pb.GetGroupListRequest) (*pb.GetGroupListResponse, error)

	// GetGroupMemberIds 获取群成员 UUID 列表。
	GetGroupMemberIds(ctx context.Context, req *pb.GetGroupMemberIdsRequest) (*pb.GetGroupMemberIdsResponse, error)

	// CheckGroupMember 检查指定用户是否为群成员并返回角色。
	CheckGroupMember(ctx context.Context, req *pb.CheckGroupMemberRequest) (*pb.CheckGroupMemberResponse, error)
}

// GroupService 是 IGroupService 的语义化别名。
type GroupService = IGroupService
