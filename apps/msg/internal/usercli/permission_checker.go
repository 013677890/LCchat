package usercli

import (
	"context"
	"fmt"

	grouppb "github.com/013677890/LCchat-Backend/apps/group/pb"
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	relationpb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"google.golang.org/grpc"
)

// PermissionChecker 通过 relation/group 服务校验消息发送权限。
//
// msg-service 只持久化已授权的消息；好友、黑名单、群成员等跨领域规则
// 通过 gRPC 查询 relation-service 与 group-service，避免 message 领域直接依赖社交/群组表。
type PermissionChecker struct {
	friendClient    relationpb.FriendServiceClient
	blacklistClient relationpb.BlacklistServiceClient
	groupClient     grouppb.GroupServiceClient
}

// NewPermissionChecker 创建消息发送权限校验器。
func NewPermissionChecker(relationConn, groupConn *grpc.ClientConn) *PermissionChecker {
	if relationConn == nil && groupConn == nil {
		return nil
	}
	checker := &PermissionChecker{}
	if relationConn != nil {
		checker.friendClient = relationpb.NewFriendServiceClient(relationConn)
		checker.blacklistClient = relationpb.NewBlacklistServiceClient(relationConn)
	}
	if groupConn != nil {
		checker.groupClient = grouppb.NewGroupServiceClient(groupConn)
	}
	return checker
}

// CheckCanSend 校验发送者是否允许向目标会话发送消息。
func (c *PermissionChecker) CheckCanSend(ctx context.Context, req *msgpb.SendMessageRequest) error {
	if req == nil || req.FromUuid == "" || req.TargetUuid == "" {
		return apperr.New(consts.CodeParamError)
	}

	switch req.ConvType {
	case msgpb.ConvType_CONV_TYPE_P2P:
		return c.checkP2P(ctx, req.FromUuid, req.TargetUuid)
	case msgpb.ConvType_CONV_TYPE_GROUP:
		return c.checkGroup(ctx, req.FromUuid, req.TargetUuid)
	default:
		return apperr.New(consts.CodeParamError)
	}
}

func (c *PermissionChecker) checkP2P(ctx context.Context, fromUUID, targetUUID string) error {
	if fromUUID == targetUUID {
		return apperr.New(consts.CodeParamError)
	}
	if c == nil || c.friendClient == nil || c.blacklistClient == nil {
		return apperr.NewWithMessage(consts.CodeInternalError, "消息权限校验器未初始化")
	}

	blockedBySelf, err := c.blacklistClient.CheckIsBlacklist(ctx, &relationpb.CheckIsBlacklistRequest{
		UserUuid:   fromUUID,
		TargetUuid: targetUUID,
	})
	if err != nil {
		return normalizeRemoteError(err, "检查本人黑名单关系失败")
	}
	if blockedBySelf.GetIsBlacklist() {
		return apperr.New(consts.CodeYouBlacklistPeer)
	}

	blockedByPeer, err := c.blacklistClient.CheckIsBlacklist(ctx, &relationpb.CheckIsBlacklistRequest{
		UserUuid:   targetUUID,
		TargetUuid: fromUUID,
	})
	if err != nil {
		return normalizeRemoteError(err, "检查对方黑名单关系失败")
	}
	if blockedByPeer.GetIsBlacklist() {
		return apperr.New(consts.CodePeerBlacklistYou)
	}

	friendResp, err := c.friendClient.CheckIsFriend(ctx, &relationpb.CheckIsFriendRequest{
		UserUuid: fromUUID,
		PeerUuid: targetUUID,
	})
	if err != nil {
		return normalizeRemoteError(err, "检查好友关系失败")
	}
	if !friendResp.GetIsFriend() {
		return apperr.New(consts.CodeNotFriend)
	}

	return nil
}

func (c *PermissionChecker) checkGroup(ctx context.Context, fromUUID, groupUUID string) error {
	if c == nil || c.groupClient == nil {
		return apperr.NewWithMessage(consts.CodeInternalError, "群组权限校验器未初始化")
	}

	resp, err := c.groupClient.CheckGroupMember(ctx, &grouppb.CheckGroupMemberRequest{
		GroupUuid: groupUUID,
		UserUuid:  fromUUID,
	})
	if err != nil {
		return normalizeRemoteError(err, "检查群成员关系失败")
	}
	if resp.GetIsMember() {
		return nil
	}
	return apperr.New(consts.CodeNotGroupMember)
}

func normalizeRemoteError(err error, fallbackMsg string) error {
	if err == nil {
		return nil
	}
	appErr := apperr.FromStatus(err)
	if code := apperr.Code(appErr); code != consts.CodeInternalError {
		return appErr
	}
	return apperr.Wrap(fmt.Errorf("%s: %w", fallbackMsg, appErr), consts.CodeInternalError, fallbackMsg)
}
