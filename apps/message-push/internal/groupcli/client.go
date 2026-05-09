package groupcli

import (
	"context"
	"fmt"

	grouppb "github.com/013677890/LCchat-Backend/apps/group/pb"
	"google.golang.org/grpc"
)

// Client 封装 group-service 的群成员查询 gRPC 调用。
type Client struct {
	groupClient grouppb.GroupServiceClient
}

// NewClient 创建群组客户端。
func NewClient(conn *grpc.ClientConn) *Client {
	if conn == nil {
		return nil
	}
	return &Client{groupClient: grouppb.NewGroupServiceClient(conn)}
}

// GetGroupMembers 获取群组成员 UUID 列表。
func (c *Client) GetGroupMembers(ctx context.Context, groupUUID string) ([]string, error) {
	if c == nil || c.groupClient == nil {
		return nil, fmt.Errorf("group service client 未初始化")
	}
	resp, err := c.groupClient.GetGroupMemberIds(ctx, &grouppb.GetGroupMemberIdsRequest{GroupUuid: groupUUID})
	if err != nil {
		return nil, fmt.Errorf("调用 group GroupService.GetGroupMemberIds 失败: %w", err)
	}
	if resp == nil || len(resp.GetUserUuids()) == 0 {
		return []string{}, nil
	}
	userUUIDs := make([]string, 0, len(resp.GetUserUuids()))
	seen := make(map[string]struct{}, len(resp.GetUserUuids()))
	for _, userUUID := range resp.GetUserUuids() {
		if userUUID == "" {
			continue
		}
		if _, exists := seen[userUUID]; exists {
			continue
		}
		seen[userUUID] = struct{}{}
		userUUIDs = append(userUUIDs, userUUID)
	}
	return userUUIDs, nil
}
