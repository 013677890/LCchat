package handler

import (
	"context"

	"github.com/013677890/LCchat-Backend/apps/user/internal/service"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
)

// GroupHandler 群组服务 Handler。
type GroupHandler struct {
	pb.UnimplementedGroupServiceServer

	groupService service.GroupService
}

// NewGroupHandler 创建群组 Handler 实例。
func NewGroupHandler(groupService service.GroupService) *GroupHandler {
	return &GroupHandler{groupService: groupService}
}

// GetGroupMembers 获取群成员列表。
func (h *GroupHandler) GetGroupMembers(ctx context.Context, req *pb.GetGroupMembersRequest) (*pb.GetGroupMembersResponse, error) {
	return h.groupService.GetGroupMembers(ctx, req)
}
