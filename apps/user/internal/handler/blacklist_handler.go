package handler

import (
	"ChatServer/apps/user/internal/service"
	pb "ChatServer/apps/user/pb"
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
	return nil, nil
}

// RemoveBlacklist 取消拉黑
func (h *BlacklistHandler) RemoveBlacklist(ctx context.Context, req *pb.RemoveBlacklistRequest) (*pb.RemoveBlacklistResponse, error) {
	return nil, nil
}

// GetBlacklistList 获取黑名单列表
func (h *BlacklistHandler) GetBlacklistList(ctx context.Context, req *pb.GetBlacklistListRequest) (*pb.GetBlacklistListResponse, error) {
	return nil, nil
}

// CheckIsBlacklist 判断是否拉黑
func (h *BlacklistHandler) CheckIsBlacklist(ctx context.Context, req *pb.CheckIsBlacklistRequest) (*pb.CheckIsBlacklistResponse, error) {
	return nil, nil
}
