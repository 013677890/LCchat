package pb

import (
	"context"

	grouppb "github.com/013677890/LCchat-Backend/apps/group/pb"
	"google.golang.org/grpc"
)

// groupServiceClientImpl 群组服务 gRPC 客户端实现。
type groupServiceClientImpl struct {
	groupClient grouppb.GroupServiceClient
}

// NewGroupServiceClient 创建群组服务 gRPC 客户端实例。
func NewGroupServiceClient(conn *grpc.ClientConn) GroupServiceClient {
	return &groupServiceClientImpl{groupClient: grouppb.NewGroupServiceClient(conn)}
}

func (c *groupServiceClientImpl) CreateGroup(ctx context.Context, req *grouppb.CreateGroupRequest) (*grouppb.CreateGroupResponse, error) {
	return c.groupClient.CreateGroup(ctx, req)
}

func (c *groupServiceClientImpl) DismissGroup(ctx context.Context, req *grouppb.DismissGroupRequest) (*grouppb.DismissGroupResponse, error) {
	return c.groupClient.DismissGroup(ctx, req)
}

func (c *groupServiceClientImpl) GetGroupInfo(ctx context.Context, req *grouppb.GetGroupInfoRequest) (*grouppb.GetGroupInfoResponse, error) {
	return c.groupClient.GetGroupInfo(ctx, req)
}

func (c *groupServiceClientImpl) UpdateGroupInfo(ctx context.Context, req *grouppb.UpdateGroupInfoRequest) (*grouppb.UpdateGroupInfoResponse, error) {
	return c.groupClient.UpdateGroupInfo(ctx, req)
}

func (c *groupServiceClientImpl) AddMember(ctx context.Context, req *grouppb.AddMemberRequest) (*grouppb.AddMemberResponse, error) {
	return c.groupClient.AddMember(ctx, req)
}

func (c *groupServiceClientImpl) RemoveMember(ctx context.Context, req *grouppb.RemoveMemberRequest) (*grouppb.RemoveMemberResponse, error) {
	return c.groupClient.RemoveMember(ctx, req)
}

func (c *groupServiceClientImpl) GetMemberList(ctx context.Context, req *grouppb.GetMemberListRequest) (*grouppb.GetMemberListResponse, error) {
	return c.groupClient.GetMemberList(ctx, req)
}

func (c *groupServiceClientImpl) GetGroupList(ctx context.Context, req *grouppb.GetGroupListRequest) (*grouppb.GetGroupListResponse, error) {
	return c.groupClient.GetGroupList(ctx, req)
}

func (c *groupServiceClientImpl) GetGroupMemberIds(ctx context.Context, req *grouppb.GetGroupMemberIdsRequest) (*grouppb.GetGroupMemberIdsResponse, error) {
	return c.groupClient.GetGroupMemberIds(ctx, req)
}

func (c *groupServiceClientImpl) CheckGroupMember(ctx context.Context, req *grouppb.CheckGroupMemberRequest) (*grouppb.CheckGroupMemberResponse, error) {
	return c.groupClient.CheckGroupMember(ctx, req)
}
