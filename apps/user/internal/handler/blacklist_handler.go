package handler

import (
	"github.com/013677890/LCchat-Backend/apps/user/internal/service"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"context"
)

// BlacklistHandler 黑名单服务Handler
type BlacklistHandler struct {
	pb.UnimplementedBlacklistServiceServer

	blacklistService service.IBlacklistService
}

// NewBlacklistHandler 创建黑名单Handler实例
func NewBlacklistHandler(blacklistService service.IBlacklistService) *BlacklistHandler {
	return &BlacklistHandler{
		blacklistService: blacklistService,
	}
}

// AddBlacklist 拉黑用户
func (h *BlacklistHandler) AddBlacklist(ctx context.Context, req *pb.AddBlacklistRequest) (*pb.AddBlacklistResponse, error) {
	return &pb.AddBlacklistResponse{}, h.blacklistService.AddBlacklist(ctx, req)
}

// RemoveBlacklist 取消拉黑
func (h *BlacklistHandler) RemoveBlacklist(ctx context.Context, req *pb.RemoveBlacklistRequest) (*pb.RemoveBlacklistResponse, error) {
	return &pb.RemoveBlacklistResponse{}, h.blacklistService.RemoveBlacklist(ctx, req)
}

// GetBlacklistList 获取黑名单列表
func (h *BlacklistHandler) GetBlacklistList(ctx context.Context, req *pb.GetBlacklistListRequest) (*pb.GetBlacklistListResponse, error) {
	return h.blacklistService.GetBlacklistList(ctx, req)
}

// CheckIsBlacklist 判断是否拉黑
func (h *BlacklistHandler) CheckIsBlacklist(ctx context.Context, req *pb.CheckIsBlacklistRequest) (*pb.CheckIsBlacklistResponse, error) {
	return h.blacklistService.CheckIsBlacklist(ctx, req)
}
