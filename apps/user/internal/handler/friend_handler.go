package handler

import (
	"ChatServer/apps/user/internal/service"
	pb "ChatServer/apps/user/pb"
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

// SearchUser 搜索用户
func (h *FriendHandler) SearchUser(ctx context.Context, req *pb.SearchUserRequest) (*pb.SearchUserResponse, error) {
	return nil, nil
}

// SendFriendApply 发送好友申请
func (h *FriendHandler) SendFriendApply(ctx context.Context, req *pb.SendFriendApplyRequest) (*pb.SendFriendApplyResponse, error) {
	return nil, nil
}

// GetFriendApplyList 获取好友申请列表
func (h *FriendHandler) GetFriendApplyList(ctx context.Context, req *pb.GetFriendApplyListRequest) (*pb.GetFriendApplyListResponse, error) {
	return nil, nil
}

// GetSentApplyList 获取发出的申请列表
func (h *FriendHandler) GetSentApplyList(ctx context.Context, req *pb.GetSentApplyListRequest) (*pb.GetSentApplyListResponse, error) {
	return nil, nil
}

// HandleFriendApply 处理好友申请
func (h *FriendHandler) HandleFriendApply(ctx context.Context, req *pb.HandleFriendApplyRequest) (*pb.HandleFriendApplyResponse, error) {
	return nil, nil
}

// GetUnreadApplyCount 获取未读申请数量
func (h *FriendHandler) GetUnreadApplyCount(ctx context.Context, req *pb.GetUnreadApplyCountRequest) (*pb.GetUnreadApplyCountResponse, error) {
	return nil, nil
}

// MarkApplyAsRead 标记申请已读
func (h *FriendHandler) MarkApplyAsRead(ctx context.Context, req *pb.MarkApplyAsReadRequest) (*pb.MarkApplyAsReadResponse, error) {
	return nil, nil
}

// GetFriendList 获取好友列表
func (h *FriendHandler) GetFriendList(ctx context.Context, req *pb.GetFriendListRequest) (*pb.GetFriendListResponse, error) {
	return nil, nil
}

// SyncFriendList 好友增量同步
func (h *FriendHandler) SyncFriendList(ctx context.Context, req *pb.SyncFriendListRequest) (*pb.SyncFriendListResponse, error) {
	return nil, nil
}

// DeleteFriend 删除好友
func (h *FriendHandler) DeleteFriend(ctx context.Context, req *pb.DeleteFriendRequest) (*pb.DeleteFriendResponse, error) {
	return nil, nil
}

// SetFriendRemark 设置好友备注
func (h *FriendHandler) SetFriendRemark(ctx context.Context, req *pb.SetFriendRemarkRequest) (*pb.SetFriendRemarkResponse, error) {
	return nil, nil
}

// SetFriendTag 设置好友标签
func (h *FriendHandler) SetFriendTag(ctx context.Context, req *pb.SetFriendTagRequest) (*pb.SetFriendTagResponse, error) {
	return nil, nil
}

// GetTagList 获取标签列表
func (h *FriendHandler) GetTagList(ctx context.Context, req *pb.GetTagListRequest) (*pb.GetTagListResponse, error) {
	return nil, nil
}

// CheckIsFriend 判断是否好友
func (h *FriendHandler) CheckIsFriend(ctx context.Context, req *pb.CheckIsFriendRequest) (*pb.CheckIsFriendResponse, error) {
	return nil, nil
}

// GetRelationStatus 获取关系状态
func (h *FriendHandler) GetRelationStatus(ctx context.Context, req *pb.GetRelationStatusRequest) (*pb.GetRelationStatusResponse, error) {
	return nil, nil
}
