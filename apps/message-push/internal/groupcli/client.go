package groupcli

import (
	"context"
	"fmt"
	"strings"

	grouppb "github.com/013677890/LCchat-Backend/apps/group/pb"
	"google.golang.org/grpc"
)

// groupRoleAdmin 与 group-service 成员角色约定保持一致：0=成员、1=管理员、2=群主。
const groupRoleAdmin int32 = 1

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
	return uniqueUserUUIDs(resp.GetUserUuids()), nil
}

// GetGroupAdmins 获取群主和管理员 UUID 列表。
func (c *Client) GetGroupAdmins(ctx context.Context, groupUUID string) ([]string, error) {
	if c == nil || c.groupClient == nil {
		return nil, fmt.Errorf("group service client 未初始化")
	}
	resp, err := c.groupClient.GetMemberList(ctx, &grouppb.GetMemberListRequest{GroupUuid: groupUUID})
	if err != nil {
		return nil, fmt.Errorf("调用 group GroupService.GetMemberList 失败: %w", err)
	}
	if resp == nil || len(resp.GetMembers()) == 0 {
		return []string{}, nil
	}

	adminUUIDs := make([]string, 0, len(resp.GetMembers()))
	for _, member := range resp.GetMembers() {
		if member.GetRole() < groupRoleAdmin {
			continue
		}
		adminUUIDs = append(adminUUIDs, member.GetUserUuid())
	}
	return uniqueUserUUIDs(adminUUIDs), nil
}

// uniqueUserUUIDs 清理用户 UUID 列表并保持首次出现顺序去重。
func uniqueUserUUIDs(userUUIDs []string) []string {
	if len(userUUIDs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(userUUIDs))
	uniqueUUIDs := make([]string, 0, len(userUUIDs))
	for _, userUUID := range userUUIDs {
		userUUID = strings.TrimSpace(userUUID)
		if userUUID == "" {
			continue
		}
		if _, exists := seen[userUUID]; exists {
			continue
		}
		seen[userUUID] = struct{}{}
		uniqueUUIDs = append(uniqueUUIDs, userUUID)
	}
	return uniqueUUIDs
}
