package pb

import (
	"context"

	relationpb "github.com/013677890/LCchat-Backend/apps/relation/pb"
)

// 这一组方法统一落在 relation-service 相关 client 上，
// 把好友与黑名单 facade 从认证/资料接口中拆开，方便后续继续演进。

// SendFriendApply 发送好友申请。
func (c *userServiceClientImpl) SendFriendApply(ctx context.Context, req *relationpb.SendFriendApplyRequest) (*relationpb.SendFriendApplyResponse, error) {
	return c.friendClient.SendFriendApply(ctx, req)
}

// GetFriendApplyList 获取好友申请列表。
func (c *userServiceClientImpl) GetFriendApplyList(ctx context.Context, req *relationpb.GetFriendApplyListRequest) (*relationpb.GetFriendApplyListResponse, error) {
	return c.friendClient.GetFriendApplyList(ctx, req)
}

// GetSentApplyList 获取发出的申请列表。
func (c *userServiceClientImpl) GetSentApplyList(ctx context.Context, req *relationpb.GetSentApplyListRequest) (*relationpb.GetSentApplyListResponse, error) {
	return c.friendClient.GetSentApplyList(ctx, req)
}

// HandleFriendApply 处理好友申请。
func (c *userServiceClientImpl) HandleFriendApply(ctx context.Context, req *relationpb.HandleFriendApplyRequest) (*relationpb.HandleFriendApplyResponse, error) {
	return c.friendClient.HandleFriendApply(ctx, req)
}

// GetUnreadApplyCount 获取未读申请数量。
func (c *userServiceClientImpl) GetUnreadApplyCount(ctx context.Context, req *relationpb.GetUnreadApplyCountRequest) (*relationpb.GetUnreadApplyCountResponse, error) {
	return c.friendClient.GetUnreadApplyCount(ctx, req)
}

// MarkApplyAsRead 标记申请已读。
func (c *userServiceClientImpl) MarkApplyAsRead(ctx context.Context, req *relationpb.MarkApplyAsReadRequest) (*relationpb.MarkApplyAsReadResponse, error) {
	return c.friendClient.MarkApplyAsRead(ctx, req)
}

// GetFriendList 获取好友列表。
func (c *userServiceClientImpl) GetFriendList(ctx context.Context, req *relationpb.GetFriendListRequest) (*relationpb.GetFriendListResponse, error) {
	return c.friendClient.GetFriendList(ctx, req)
}

// SyncFriendList 好友增量同步。
func (c *userServiceClientImpl) SyncFriendList(ctx context.Context, req *relationpb.SyncFriendListRequest) (*relationpb.SyncFriendListResponse, error) {
	return c.friendClient.SyncFriendList(ctx, req)
}

// DeleteFriend 删除好友。
func (c *userServiceClientImpl) DeleteFriend(ctx context.Context, req *relationpb.DeleteFriendRequest) (*relationpb.DeleteFriendResponse, error) {
	return c.friendClient.DeleteFriend(ctx, req)
}

// SetFriendRemark 设置好友备注。
func (c *userServiceClientImpl) SetFriendRemark(ctx context.Context, req *relationpb.SetFriendRemarkRequest) (*relationpb.SetFriendRemarkResponse, error) {
	return c.friendClient.SetFriendRemark(ctx, req)
}

// SetFriendTag 设置好友标签。
func (c *userServiceClientImpl) SetFriendTag(ctx context.Context, req *relationpb.SetFriendTagRequest) (*relationpb.SetFriendTagResponse, error) {
	return c.friendClient.SetFriendTag(ctx, req)
}

// CheckIsFriend 判断是否好友。
func (c *userServiceClientImpl) CheckIsFriend(ctx context.Context, req *relationpb.CheckIsFriendRequest) (*relationpb.CheckIsFriendResponse, error) {
	return c.friendClient.CheckIsFriend(ctx, req)
}

// BatchCheckIsFriend 批量判断是否好友。
func (c *userServiceClientImpl) BatchCheckIsFriend(ctx context.Context, req *relationpb.BatchCheckIsFriendRequest) (*relationpb.BatchCheckIsFriendResponse, error) {
	return c.friendClient.BatchCheckIsFriend(ctx, req)
}

// GetRelationStatus 获取关系状态。
func (c *userServiceClientImpl) GetRelationStatus(ctx context.Context, req *relationpb.GetRelationStatusRequest) (*relationpb.GetRelationStatusResponse, error) {
	return c.friendClient.GetRelationStatus(ctx, req)
}

// AddBlacklist 拉黑用户。
func (c *userServiceClientImpl) AddBlacklist(ctx context.Context, req *relationpb.AddBlacklistRequest) (*relationpb.AddBlacklistResponse, error) {
	return c.blacklistClient.AddBlacklist(ctx, req)
}

// RemoveBlacklist 取消拉黑。
func (c *userServiceClientImpl) RemoveBlacklist(ctx context.Context, req *relationpb.RemoveBlacklistRequest) (*relationpb.RemoveBlacklistResponse, error) {
	return c.blacklistClient.RemoveBlacklist(ctx, req)
}

// GetBlacklistList 获取黑名单列表。
func (c *userServiceClientImpl) GetBlacklistList(ctx context.Context, req *relationpb.GetBlacklistListRequest) (*relationpb.GetBlacklistListResponse, error) {
	return c.blacklistClient.GetBlacklistList(ctx, req)
}

// CheckIsBlacklist 判断是否拉黑。
func (c *userServiceClientImpl) CheckIsBlacklist(ctx context.Context, req *relationpb.CheckIsBlacklistRequest) (*relationpb.CheckIsBlacklistResponse, error) {
	return c.blacklistClient.CheckIsBlacklist(ctx, req)
}
