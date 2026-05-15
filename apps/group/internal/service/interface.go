package service

import (
	"context"
	pb "github.com/013677890/LCchat-Backend/apps/group/pb"
)

// IGroupService 定义 group-service 对外暴露的业务接口。
//
// 当前 proto 已经稳定覆盖群组服务的核心读写方法面，
// 因此这里保持与 proto 一一对应，确保：
//  1. handler 只依赖抽象，不依赖具体实现；
//  2. 后续补充业务逻辑时不需要再改 handler / wire 的依赖方向；
//  3. 测试时可以方便地为每个方法注入 mock。
type IGroupService interface {
	// CreateGroup 创建群。
	CreateGroup(ctx context.Context, req *pb.CreateGroupRequest) (*pb.CreateGroupResponse, error)
	// DismissGroup 解散群。
	DismissGroup(ctx context.Context, req *pb.DismissGroupRequest) error
	// GetGroupInfo 获取群资料。
	GetGroupInfo(ctx context.Context, req *pb.GetGroupInfoRequest) (*pb.GetGroupInfoResponse, error)
	// UpdateGroupInfo 更新群资料。
	UpdateGroupInfo(ctx context.Context, req *pb.UpdateGroupInfoRequest) error
	// UpdateGroupNotice 独立更新群公告。
	UpdateGroupNotice(ctx context.Context, req *pb.UpdateGroupNoticeRequest) error
	// TransferGroupOwner 转让群主。
	TransferGroupOwner(ctx context.Context, req *pb.TransferGroupOwnerRequest) error
	// UpdateMemberRole 更新群成员角色。
	UpdateMemberRole(ctx context.Context, req *pb.UpdateMemberRoleRequest) error
	// ApplyJoinGroup 申请加入群聊，必要时直接入群。
	ApplyJoinGroup(ctx context.Context, req *pb.ApplyJoinGroupRequest) (*pb.ApplyJoinGroupResponse, error)
	// CancelJoinGroupApplication 撤销当前用户自己的待审批入群申请。
	CancelJoinGroupApplication(ctx context.Context, req *pb.CancelJoinGroupApplicationRequest) error
	// GetMyJoinGroupApplication 获取当前用户在指定群的最新申请状态。
	GetMyJoinGroupApplication(ctx context.Context, req *pb.GetMyJoinGroupApplicationRequest) (*pb.GetMyJoinGroupApplicationResponse, error)
	// ListMyJoinGroupApplications 获取当前用户发起的入群申请列表。
	ListMyJoinGroupApplications(ctx context.Context, req *pb.ListMyJoinGroupApplicationsRequest) (*pb.ListMyJoinGroupApplicationsResponse, error)
	// ReviewJoinGroup 审批入群申请。
	ReviewJoinGroup(ctx context.Context, req *pb.ReviewJoinGroupRequest) error
	// ListJoinRequests 获取群待审批申请列表。
	ListJoinRequests(ctx context.Context, req *pb.ListJoinRequestsRequest) (*pb.ListJoinRequestsResponse, error)
	// ListReviewedJoinRequests 获取群已审批申请列表。
	ListReviewedJoinRequests(ctx context.Context, req *pb.ListReviewedJoinRequestsRequest) (*pb.ListReviewedJoinRequestsResponse, error)
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
