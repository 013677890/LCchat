package pb

import (
	"context"
	grouppb "github.com/013677890/LCchat-Backend/apps/group/pb"
	"google.golang.org/grpc"
)

// groupServiceClientImpl 是 gateway 侧的 group gRPC 薄客户端。
//
// 这里不叠加任何业务判断，只负责把上层 service 的调用稳定转发到
// group-service，便于统一复用 grpcx 中间件链上的超时、重试与日志能力。
type groupServiceClientImpl struct {
	groupClient grouppb.GroupServiceClient
}

// NewGroupServiceClient 创建群组服务 gRPC 客户端实例。
func NewGroupServiceClient(conn *grpc.ClientConn) GroupServiceClient {
	return &groupServiceClientImpl{groupClient: grouppb.NewGroupServiceClient(conn)}
}

// CreateGroup 转发创建群请求。
func (c *groupServiceClientImpl) CreateGroup(ctx context.Context, req *grouppb.CreateGroupRequest) (*grouppb.CreateGroupResponse, error) {
	return c.groupClient.CreateGroup(ctx, req)
}

// DismissGroup 转发解散群请求。
func (c *groupServiceClientImpl) DismissGroup(ctx context.Context, req *grouppb.DismissGroupRequest) (*grouppb.DismissGroupResponse, error) {
	return c.groupClient.DismissGroup(ctx, req)
}

// GetGroupInfo 转发获取群资料请求。
func (c *groupServiceClientImpl) GetGroupInfo(ctx context.Context, req *grouppb.GetGroupInfoRequest) (*grouppb.GetGroupInfoResponse, error) {
	return c.groupClient.GetGroupInfo(ctx, req)
}

// UpdateGroupInfo 转发更新群资料请求。
func (c *groupServiceClientImpl) UpdateGroupInfo(ctx context.Context, req *grouppb.UpdateGroupInfoRequest) (*grouppb.UpdateGroupInfoResponse, error) {
	return c.groupClient.UpdateGroupInfo(ctx, req)
}

// UpdateGroupNotice 转发独立更新群公告请求。
func (c *groupServiceClientImpl) UpdateGroupNotice(ctx context.Context, req *grouppb.UpdateGroupNoticeRequest) (*grouppb.UpdateGroupNoticeResponse, error) {
	return c.groupClient.UpdateGroupNotice(ctx, req)
}

// TransferGroupOwner 转发群主转让请求。
func (c *groupServiceClientImpl) TransferGroupOwner(ctx context.Context, req *grouppb.TransferGroupOwnerRequest) (*grouppb.TransferGroupOwnerResponse, error) {
	return c.groupClient.TransferGroupOwner(ctx, req)
}

// UpdateMemberRole 转发成员角色更新请求。
func (c *groupServiceClientImpl) UpdateMemberRole(ctx context.Context, req *grouppb.UpdateMemberRoleRequest) (*grouppb.UpdateMemberRoleResponse, error) {
	return c.groupClient.UpdateMemberRole(ctx, req)
}

// ApplyJoinGroup 转发申请加入群聊请求。
func (c *groupServiceClientImpl) ApplyJoinGroup(ctx context.Context, req *grouppb.ApplyJoinGroupRequest) (*grouppb.ApplyJoinGroupResponse, error) {
	return c.groupClient.ApplyJoinGroup(ctx, req)
}

// CancelJoinGroupApplication 转发撤销当前用户待审批入群申请请求。
func (c *groupServiceClientImpl) CancelJoinGroupApplication(ctx context.Context, req *grouppb.CancelJoinGroupApplicationRequest) (*grouppb.CancelJoinGroupApplicationResponse, error) {
	return c.groupClient.CancelJoinGroupApplication(ctx, req)
}

// GetMyJoinGroupApplication 转发当前用户在指定群的最新申请状态查询请求。
func (c *groupServiceClientImpl) GetMyJoinGroupApplication(ctx context.Context, req *grouppb.GetMyJoinGroupApplicationRequest) (*grouppb.GetMyJoinGroupApplicationResponse, error) {
	return c.groupClient.GetMyJoinGroupApplication(ctx, req)
}

// ListMyJoinGroupApplications 转发当前用户发起的入群申请列表查询请求。
func (c *groupServiceClientImpl) ListMyJoinGroupApplications(ctx context.Context, req *grouppb.ListMyJoinGroupApplicationsRequest) (*grouppb.ListMyJoinGroupApplicationsResponse, error) {
	return c.groupClient.ListMyJoinGroupApplications(ctx, req)
}

// ReviewJoinGroup 转发审批入群申请请求。
func (c *groupServiceClientImpl) ReviewJoinGroup(ctx context.Context, req *grouppb.ReviewJoinGroupRequest) (*grouppb.ReviewJoinGroupResponse, error) {
	return c.groupClient.ReviewJoinGroup(ctx, req)
}

// ListJoinRequests 转发待审批入群申请列表请求。
func (c *groupServiceClientImpl) ListJoinRequests(ctx context.Context, req *grouppb.ListJoinRequestsRequest) (*grouppb.ListJoinRequestsResponse, error) {
	return c.groupClient.ListJoinRequests(ctx, req)
}

// ListReviewedJoinRequests 转发群已审批申请列表请求。
func (c *groupServiceClientImpl) ListReviewedJoinRequests(ctx context.Context, req *grouppb.ListReviewedJoinRequestsRequest) (*grouppb.ListReviewedJoinRequestsResponse, error) {
	return c.groupClient.ListReviewedJoinRequests(ctx, req)
}

// AddMember 转发添加群成员请求。
func (c *groupServiceClientImpl) AddMember(ctx context.Context, req *grouppb.AddMemberRequest) (*grouppb.AddMemberResponse, error) {
	return c.groupClient.AddMember(ctx, req)
}

// LeaveGroup 转发当前用户主动退群请求。
func (c *groupServiceClientImpl) LeaveGroup(ctx context.Context, req *grouppb.LeaveGroupRequest) (*grouppb.LeaveGroupResponse, error) {
	return c.groupClient.LeaveGroup(ctx, req)
}

// RemoveMember 转发移除群成员请求。
func (c *groupServiceClientImpl) RemoveMember(ctx context.Context, req *grouppb.RemoveMemberRequest) (*grouppb.RemoveMemberResponse, error) {
	return c.groupClient.RemoveMember(ctx, req)
}

// GetMemberList 转发群成员列表请求。
func (c *groupServiceClientImpl) GetMemberList(ctx context.Context, req *grouppb.GetMemberListRequest) (*grouppb.GetMemberListResponse, error) {
	return c.groupClient.GetMemberList(ctx, req)
}

// SearchGroupMembers 转发群成员搜索请求。
func (c *groupServiceClientImpl) SearchGroupMembers(ctx context.Context, req *grouppb.SearchGroupMembersRequest) (*grouppb.SearchGroupMembersResponse, error) {
	return c.groupClient.SearchGroupMembers(ctx, req)
}

// UpdateMyGroupNickname 转发当前用户群名片更新请求。
func (c *groupServiceClientImpl) UpdateMyGroupNickname(ctx context.Context, req *grouppb.UpdateMyGroupNicknameRequest) (*grouppb.UpdateMyGroupNicknameResponse, error) {
	return c.groupClient.UpdateMyGroupNickname(ctx, req)
}

// UpdateGroupMemberNickname 转发管理员代改成员群名片请求。
func (c *groupServiceClientImpl) UpdateGroupMemberNickname(ctx context.Context, req *grouppb.UpdateGroupMemberNicknameRequest) (*grouppb.UpdateGroupMemberNicknameResponse, error) {
	return c.groupClient.UpdateGroupMemberNickname(ctx, req)
}

// MuteGroupMember 转发成员单人禁言设置请求。
func (c *groupServiceClientImpl) MuteGroupMember(ctx context.Context, req *grouppb.MuteGroupMemberRequest) (*grouppb.MuteGroupMemberResponse, error) {
	return c.groupClient.MuteGroupMember(ctx, req)
}

// UpdateGroupMuteSetting 转发全员禁言开关更新请求。
func (c *groupServiceClientImpl) UpdateGroupMuteSetting(ctx context.Context, req *grouppb.UpdateGroupMuteSettingRequest) (*grouppb.UpdateGroupMuteSettingResponse, error) {
	return c.groupClient.UpdateGroupMuteSetting(ctx, req)
}

// GetGroupList 转发当前用户群列表请求。
func (c *groupServiceClientImpl) GetGroupList(ctx context.Context, req *grouppb.GetGroupListRequest) (*grouppb.GetGroupListResponse, error) {
	return c.groupClient.GetGroupList(ctx, req)
}

// SearchGroups 转发群搜索请求。
func (c *groupServiceClientImpl) SearchGroups(ctx context.Context, req *grouppb.SearchGroupsRequest) (*grouppb.SearchGroupsResponse, error) {
	return c.groupClient.SearchGroups(ctx, req)
}

// GetGroupMemberIds 转发群成员 ID 列表请求。
func (c *groupServiceClientImpl) GetGroupMemberIds(ctx context.Context, req *grouppb.GetGroupMemberIdsRequest) (*grouppb.GetGroupMemberIdsResponse, error) {
	return c.groupClient.GetGroupMemberIds(ctx, req)
}

// CheckGroupMember 转发群成员关系检查请求。
func (c *groupServiceClientImpl) CheckGroupMember(ctx context.Context, req *grouppb.CheckGroupMemberRequest) (*grouppb.CheckGroupMemberResponse, error) {
	return c.groupClient.CheckGroupMember(ctx, req)
}

// CheckGroupSendPermission 转发群消息发送权限检查请求。
func (c *groupServiceClientImpl) CheckGroupSendPermission(ctx context.Context, req *grouppb.CheckGroupSendPermissionRequest) (*grouppb.CheckGroupSendPermissionResponse, error) {
	return c.groupClient.CheckGroupSendPermission(ctx, req)
}

// GetJoinRequestPendingCount 转发待审批入群申请数量查询请求。
func (c *groupServiceClientImpl) GetJoinRequestPendingCount(ctx context.Context, req *grouppb.GetJoinRequestPendingCountRequest) (*grouppb.GetJoinRequestPendingCountResponse, error) {
	return c.groupClient.GetJoinRequestPendingCount(ctx, req)
}
