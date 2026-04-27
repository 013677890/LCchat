package handler

import (
	"github.com/013677890/LCchat-Backend/apps/user/internal/service"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"context"
)

// FriendHandler 好友服务Handler
type FriendHandler struct {
	pb.UnimplementedFriendServiceServer

	friendService service.IFriendService
}

// NewFriendHandler 创建好友Handler实例
func NewFriendHandler(friendService service.IFriendService) *FriendHandler {
	return &FriendHandler{
		friendService: friendService,
	}
}

// SendFriendApply 发送好友申请
func (h *FriendHandler) SendFriendApply(ctx context.Context, req *pb.SendFriendApplyRequest) (*pb.SendFriendApplyResponse, error) {
	return h.friendService.SendFriendApply(ctx, req)
}

// GetFriendApplyList 获取好友申请列表
func (h *FriendHandler) GetFriendApplyList(ctx context.Context, req *pb.GetFriendApplyListRequest) (*pb.GetFriendApplyListResponse, error) {
	return h.friendService.GetFriendApplyList(ctx, req)
}

// GetSentApplyList 获取发出的申请列表
func (h *FriendHandler) GetSentApplyList(ctx context.Context, req *pb.GetSentApplyListRequest) (*pb.GetSentApplyListResponse, error) {
	return h.friendService.GetSentApplyList(ctx, req)
}

// HandleFriendApply 处理好友申请
func (h *FriendHandler) HandleFriendApply(ctx context.Context, req *pb.HandleFriendApplyRequest) (*pb.HandleFriendApplyResponse, error) {
	return &pb.HandleFriendApplyResponse{}, h.friendService.HandleFriendApply(ctx, req)
}

// GetUnreadApplyCount 获取未读申请数量
func (h *FriendHandler) GetUnreadApplyCount(ctx context.Context, req *pb.GetUnreadApplyCountRequest) (*pb.GetUnreadApplyCountResponse, error) {
	return h.friendService.GetUnreadApplyCount(ctx, req)
}

// MarkApplyAsRead 标记申请已读
func (h *FriendHandler) MarkApplyAsRead(ctx context.Context, req *pb.MarkApplyAsReadRequest) (*pb.MarkApplyAsReadResponse, error) {
	return &pb.MarkApplyAsReadResponse{}, h.friendService.MarkApplyAsRead(ctx, req)
}

// GetFriendList 获取好友列表
func (h *FriendHandler) GetFriendList(ctx context.Context, req *pb.GetFriendListRequest) (*pb.GetFriendListResponse, error) {
	return h.friendService.GetFriendList(ctx, req)
}

// SyncFriendList 好友增量同步
func (h *FriendHandler) SyncFriendList(ctx context.Context, req *pb.SyncFriendListRequest) (*pb.SyncFriendListResponse, error) {
	return h.friendService.SyncFriendList(ctx, req)
}

// DeleteFriend 删除好友
func (h *FriendHandler) DeleteFriend(ctx context.Context, req *pb.DeleteFriendRequest) (*pb.DeleteFriendResponse, error) {
	return &pb.DeleteFriendResponse{}, h.friendService.DeleteFriend(ctx, req)
}

// SetFriendRemark 设置好友备注
func (h *FriendHandler) SetFriendRemark(ctx context.Context, req *pb.SetFriendRemarkRequest) (*pb.SetFriendRemarkResponse, error) {
	return &pb.SetFriendRemarkResponse{}, h.friendService.SetFriendRemark(ctx, req)
}

// SetFriendTag 设置好友标签
func (h *FriendHandler) SetFriendTag(ctx context.Context, req *pb.SetFriendTagRequest) (*pb.SetFriendTagResponse, error) {
	return &pb.SetFriendTagResponse{}, h.friendService.SetFriendTag(ctx, req)
}

// GetTagList 获取标签列表
func (h *FriendHandler) GetTagList(ctx context.Context, req *pb.GetTagListRequest) (*pb.GetTagListResponse, error) {
	return h.friendService.GetTagList(ctx, req)
}

// CheckIsFriend 判断是否好友
func (h *FriendHandler) CheckIsFriend(ctx context.Context, req *pb.CheckIsFriendRequest) (*pb.CheckIsFriendResponse, error) {
	return h.friendService.CheckIsFriend(ctx, req)
}

// BatchCheckIsFriend 批量判断是否好友
func (h *FriendHandler) BatchCheckIsFriend(ctx context.Context, req *pb.BatchCheckIsFriendRequest) (*pb.BatchCheckIsFriendResponse, error) {
	return h.friendService.BatchCheckIsFriend(ctx, req)
}

// GetRelationStatus 获取关系状态
func (h *FriendHandler) GetRelationStatus(ctx context.Context, req *pb.GetRelationStatusRequest) (*pb.GetRelationStatusResponse, error) {
	return h.friendService.GetRelationStatus(ctx, req)
}
