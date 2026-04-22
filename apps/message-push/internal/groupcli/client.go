package groupcli

import (
	"context"
	"fmt"

	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"google.golang.org/grpc"
)

// Client 封装群组成员查询 gRPC 调用。
type Client struct {
	groupClient userpb.GroupServiceClient
}

// NewClient 创建群组客户端。
func NewClient(conn *grpc.ClientConn) *Client {
	if conn == nil {
		return nil
	}
	return &Client{groupClient: userpb.NewGroupServiceClient(conn)}
}

// GetGroupMembers 获取群组成员 UUID 列表。
func (c *Client) GetGroupMembers(ctx context.Context, groupUUID string) ([]string, error) {
	if c == nil || c.groupClient == nil {
		return nil, fmt.Errorf("group service client 未初始化")
	}
	resp, err := c.groupClient.GetGroupMembers(ctx, &userpb.GetGroupMembersRequest{GroupUuid: groupUUID})
	if err != nil {
		return nil, fmt.Errorf("调用 user GroupService.GetGroupMembers 失败: %w", err)
	}
	if resp == nil || len(resp.Members) == 0 {
		return []string{}, nil
	}
	userUUIDs := make([]string, 0, len(resp.Members))
	seen := make(map[string]struct{}, len(resp.Members))
	for _, member := range resp.Members {
		if member == nil || member.UserUuid == "" {
			continue
		}
		if _, exists := seen[member.UserUuid]; exists {
			continue
		}
		seen[member.UserUuid] = struct{}{}
		userUUIDs = append(userUUIDs, member.UserUuid)
	}
	return userUUIDs, nil
}
