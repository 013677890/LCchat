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

// TransferGroupOwner 转发群主转让请求。
func (c *groupServiceClientImpl) TransferGroupOwner(ctx context.Context, req *grouppb.TransferGroupOwnerRequest) (*grouppb.TransferGroupOwnerResponse, error) {
	return c.groupClient.TransferGroupOwner(ctx, req)
}

// UpdateMemberRole 转发成员角色更新请求。
func (c *groupServiceClientImpl) UpdateMemberRole(ctx context.Context, req *grouppb.UpdateMemberRoleRequest) (*grouppb.UpdateMemberRoleResponse, error) {
	return c.groupClient.UpdateMemberRole(ctx, req)
}

// AddMember 转发添加群成员请求。
func (c *groupServiceClientImpl) AddMember(ctx context.Context, req *grouppb.AddMemberRequest) (*grouppb.AddMemberResponse, error) {
	return c.groupClient.AddMember(ctx, req)
}

// RemoveMember 转发移除群成员请求。
func (c *groupServiceClientImpl) RemoveMember(ctx context.Context, req *grouppb.RemoveMemberRequest) (*grouppb.RemoveMemberResponse, error) {
	return c.groupClient.RemoveMember(ctx, req)
}

// GetMemberList 转发群成员列表请求。
func (c *groupServiceClientImpl) GetMemberList(ctx context.Context, req *grouppb.GetMemberListRequest) (*grouppb.GetMemberListResponse, error) {
	return c.groupClient.GetMemberList(ctx, req)
}

// GetGroupList 转发当前用户群列表请求。
func (c *groupServiceClientImpl) GetGroupList(ctx context.Context, req *grouppb.GetGroupListRequest) (*grouppb.GetGroupListResponse, error) {
	return c.groupClient.GetGroupList(ctx, req)
}

// GetGroupMemberIds 转发群成员 ID 列表请求。
func (c *groupServiceClientImpl) GetGroupMemberIds(ctx context.Context, req *grouppb.GetGroupMemberIdsRequest) (*grouppb.GetGroupMemberIdsResponse, error) {
	return c.groupClient.GetGroupMemberIds(ctx, req)
}

// CheckGroupMember 转发群成员关系检查请求。
func (c *groupServiceClientImpl) CheckGroupMember(ctx context.Context, req *grouppb.CheckGroupMemberRequest) (*grouppb.CheckGroupMemberResponse, error) {
	return c.groupClient.CheckGroupMember(ctx, req)
}
