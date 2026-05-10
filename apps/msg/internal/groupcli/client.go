package groupcli

import (
	"context"
	"fmt"

	grouppb "github.com/013677890/LCchat-Backend/apps/group/pb"
	msgsvc "github.com/013677890/LCchat-Backend/apps/msg/internal/domain/message"
	"google.golang.org/grpc"
)

// Client 通过 group-service gRPC 查询群成员与角色，实现 message.GroupRoleQuerier。
type Client struct {
	groupClient grouppb.GroupServiceClient
}

var _ msgsvc.GroupRoleQuerier = (*Client)(nil)

// NewClient 创建群组客户端。conn 为指向 group-service 的 gRPC 连接。
func NewClient(conn *grpc.ClientConn) *Client {
	if conn == nil {
		return nil
	}
	return &Client{groupClient: grouppb.NewGroupServiceClient(conn)}
}

// QueryMemberRole 返回 userUUID 在 groupUUID 中的角色。
// 0=普通成员, 1=管理员, 2=群主, -1=不在群内。
func (c *Client) QueryMemberRole(ctx context.Context, groupUUID, userUUID string) (int8, error) {
	if c == nil || c.groupClient == nil {
		return -1, fmt.Errorf("group service client 未初始化")
	}
	resp, err := c.groupClient.CheckGroupMember(ctx, &grouppb.CheckGroupMemberRequest{
		GroupUuid: groupUUID,
		UserUuid:  userUUID,
	})
	if err != nil {
		return -1, fmt.Errorf("调用 GroupService.CheckGroupMember 失败: %w", err)
	}
	if !resp.GetIsMember() {
		return -1, nil
	}
	return int8(resp.GetRole()), nil
}

// GetGroupMemberUUIDs 获取群组所有有效成员 UUID 列表。
func (c *Client) GetGroupMemberUUIDs(ctx context.Context, groupUUID string) ([]string, error) {
	if c == nil || c.groupClient == nil {
		return nil, fmt.Errorf("group service client 未初始化")
	}
	resp, err := c.groupClient.GetGroupMemberIds(ctx, &grouppb.GetGroupMemberIdsRequest{GroupUuid: groupUUID})
	if err != nil {
		return nil, fmt.Errorf("调用 GroupService.GetGroupMemberIds 失败: %w", err)
	}
	uuids := make([]string, 0, len(resp.GetUserUuids()))
	seen := make(map[string]struct{}, len(resp.GetUserUuids()))
	for _, userUUID := range resp.GetUserUuids() {
		if userUUID == "" {
			continue
		}
		if _, exists := seen[userUUID]; exists {
			continue
		}
		seen[userUUID] = struct{}{}
		uuids = append(uuids, userUUID)
	}
	return uuids, nil
}
