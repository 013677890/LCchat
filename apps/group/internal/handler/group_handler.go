package handler

import (
	"context"
	"github.com/013677890/LCchat-Backend/apps/group/internal/service"
	pb "github.com/013677890/LCchat-Backend/apps/group/pb"
)

// GroupHandler 是 group-service 的 gRPC 入站适配层。
//
// 这里刻意保持“薄 Handler”风格：
//  1. 只负责承接 proto 请求与 proto 响应；
//  2. 不在这里编排群管理规则；
//  3. 统一把业务职责下沉到 service 层。
//
// 这样可以保证后续无论群资料、成员、审批还是角色逻辑如何演进，
// gRPC 边界层都保持稳定，便于测试和维护。
type GroupHandler struct {
	pb.UnimplementedGroupServiceServer
	groupService service.IGroupService
}

// NewGroupHandler 创建群组 Handler 实例。
//
// 依赖注入时只接受抽象接口，避免 Handler 与具体实现耦合，
// 后续如果需要替换为 mock / fake / 新实现，Wire 与测试代码都更容易复用。
func NewGroupHandler(groupService service.IGroupService) *GroupHandler {
	return &GroupHandler{groupService: groupService}
}

// CreateGroup 创建群。
func (h *GroupHandler) CreateGroup(ctx context.Context, req *pb.CreateGroupRequest) (*pb.CreateGroupResponse, error) {
	return h.groupService.CreateGroup(ctx, req)
}

// DismissGroup 解散群。
func (h *GroupHandler) DismissGroup(ctx context.Context, req *pb.DismissGroupRequest) (*pb.DismissGroupResponse, error) {
	return &pb.DismissGroupResponse{}, h.groupService.DismissGroup(ctx, req)
}

// GetGroupInfo 获取群资料。
func (h *GroupHandler) GetGroupInfo(ctx context.Context, req *pb.GetGroupInfoRequest) (*pb.GetGroupInfoResponse, error) {
	return h.groupService.GetGroupInfo(ctx, req)
}

// UpdateGroupInfo 更新群资料。
func (h *GroupHandler) UpdateGroupInfo(ctx context.Context, req *pb.UpdateGroupInfoRequest) (*pb.UpdateGroupInfoResponse, error) {
	return &pb.UpdateGroupInfoResponse{}, h.groupService.UpdateGroupInfo(ctx, req)
}

// UpdateGroupNotice 独立更新群公告。
func (h *GroupHandler) UpdateGroupNotice(ctx context.Context, req *pb.UpdateGroupNoticeRequest) (*pb.UpdateGroupNoticeResponse, error) {
	return &pb.UpdateGroupNoticeResponse{}, h.groupService.UpdateGroupNotice(ctx, req)
}

// TransferGroupOwner 转让群主。
func (h *GroupHandler) TransferGroupOwner(ctx context.Context, req *pb.TransferGroupOwnerRequest) (*pb.TransferGroupOwnerResponse, error) {
	return &pb.TransferGroupOwnerResponse{}, h.groupService.TransferGroupOwner(ctx, req)
}

// UpdateMemberRole 更新群成员角色。
func (h *GroupHandler) UpdateMemberRole(ctx context.Context, req *pb.UpdateMemberRoleRequest) (*pb.UpdateMemberRoleResponse, error) {
	return &pb.UpdateMemberRoleResponse{}, h.groupService.UpdateMemberRole(ctx, req)
}

// ApplyJoinGroup 申请加入群聊。
func (h *GroupHandler) ApplyJoinGroup(ctx context.Context, req *pb.ApplyJoinGroupRequest) (*pb.ApplyJoinGroupResponse, error) {
	return h.groupService.ApplyJoinGroup(ctx, req)
}

// CancelJoinGroupApplication 撤销当前用户自己的待审批入群申请。
func (h *GroupHandler) CancelJoinGroupApplication(ctx context.Context, req *pb.CancelJoinGroupApplicationRequest) (*pb.CancelJoinGroupApplicationResponse, error) {
	return &pb.CancelJoinGroupApplicationResponse{}, h.groupService.CancelJoinGroupApplication(ctx, req)
}

// GetMyJoinGroupApplication 获取当前用户在指定群的最新申请状态。
func (h *GroupHandler) GetMyJoinGroupApplication(ctx context.Context, req *pb.GetMyJoinGroupApplicationRequest) (*pb.GetMyJoinGroupApplicationResponse, error) {
	return h.groupService.GetMyJoinGroupApplication(ctx, req)
}

// ReviewJoinGroup 审批入群申请。
func (h *GroupHandler) ReviewJoinGroup(ctx context.Context, req *pb.ReviewJoinGroupRequest) (*pb.ReviewJoinGroupResponse, error) {
	return &pb.ReviewJoinGroupResponse{}, h.groupService.ReviewJoinGroup(ctx, req)
}

// ListJoinRequests 获取群待审批申请列表。
func (h *GroupHandler) ListJoinRequests(ctx context.Context, req *pb.ListJoinRequestsRequest) (*pb.ListJoinRequestsResponse, error) {
	return h.groupService.ListJoinRequests(ctx, req)
}

// AddMember 添加群成员。
func (h *GroupHandler) AddMember(ctx context.Context, req *pb.AddMemberRequest) (*pb.AddMemberResponse, error) {
	return &pb.AddMemberResponse{}, h.groupService.AddMember(ctx, req)
}

// RemoveMember 移除群成员。
func (h *GroupHandler) RemoveMember(ctx context.Context, req *pb.RemoveMemberRequest) (*pb.RemoveMemberResponse, error) {
	return &pb.RemoveMemberResponse{}, h.groupService.RemoveMember(ctx, req)
}

// GetMemberList 获取群成员列表。
func (h *GroupHandler) GetMemberList(ctx context.Context, req *pb.GetMemberListRequest) (*pb.GetMemberListResponse, error) {
	return h.groupService.GetMemberList(ctx, req)
}

// GetGroupList 获取当前用户的群列表。
func (h *GroupHandler) GetGroupList(ctx context.Context, req *pb.GetGroupListRequest) (*pb.GetGroupListResponse, error) {
	return h.groupService.GetGroupList(ctx, req)
}

// GetGroupMemberIds 获取群成员 UUID 列表。
func (h *GroupHandler) GetGroupMemberIds(ctx context.Context, req *pb.GetGroupMemberIdsRequest) (*pb.GetGroupMemberIdsResponse, error) {
	return h.groupService.GetGroupMemberIds(ctx, req)
}

// CheckGroupMember 检查指定用户是否为群成员并返回角色。
func (h *GroupHandler) CheckGroupMember(ctx context.Context, req *pb.CheckGroupMemberRequest) (*pb.CheckGroupMemberResponse, error) {
	return h.groupService.CheckGroupMember(ctx, req)
}
