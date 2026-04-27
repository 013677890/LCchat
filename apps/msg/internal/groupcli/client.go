package groupcli

import (
	"context"
	"fmt"

	msgsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/message"
	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"google.golang.org/grpc"
)

// Client 通过 user-service gRPC 查询群成员角色，实现 message.GroupRoleQuerier。
type Client struct {
	groupClient userpb.GroupServiceClient
}

var _ msgsvc.GroupRoleQuerier = (*Client)(nil)

// NewClient 创建群组客户端。conn 为指向 user-service 的 gRPC 连接。
func NewClient(conn *grpc.ClientConn) *Client {
	if conn == nil {
		return nil
	}
	return &Client{groupClient: userpb.NewGroupServiceClient(conn)}
}

// QueryMemberRole 返回 userUUID 在 groupUUID 中的角色。
// 0=普通成员, 1=管理员, 2=群主, -1=不在群内。
func (c *Client) QueryMemberRole(ctx context.Context, groupUUID, userUUID string) (int8, error) {
	if c == nil || c.groupClient == nil {
		return -1, fmt.Errorf("group service client 未初始化")
	}
	resp, err := c.groupClient.GetGroupMembers(ctx, &userpb.GetGroupMembersRequest{GroupUuid: groupUUID})
	if err != nil {
		return -1, fmt.Errorf("调用 GroupService.GetGroupMembers 失败: %w", err)
	}
	for _, m := range resp.GetMembers() {
		if m != nil && m.UserUuid == userUUID {
			return int8(m.Role), nil
		}
	}
	return -1, nil
}

// GetGroupMemberUUIDs 获取群组所有有效成员 UUID 列表。
func (c *Client) GetGroupMemberUUIDs(ctx context.Context, groupUUID string) ([]string, error) {
	if c == nil || c.groupClient == nil {
		return nil, fmt.Errorf("group service client 未初始化")
	}
	resp, err := c.groupClient.GetGroupMembers(ctx, &userpb.GetGroupMembersRequest{GroupUuid: groupUUID})
	if err != nil {
		return nil, fmt.Errorf("调用 GroupService.GetGroupMembers 失败: %w", err)
	}
	uuids := make([]string, 0, len(resp.GetMembers()))
	for _, m := range resp.GetMembers() {
		if m != nil && m.UserUuid != "" {
			uuids = append(uuids, m.UserUuid)
		}
	}
	return uuids, nil
}
