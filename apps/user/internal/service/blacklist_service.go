package service

import (
	"ChatServer/apps/user/internal/dto"
	"ChatServer/apps/user/internal/repository"
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// blacklistServiceImpl 黑名单服务实现
type blacklistServiceImpl struct {
	blacklistRepo repository.IBlacklistRepository
}

// NewBlacklistService 创建黑名单服务实例
func NewBlacklistService(blacklistRepo repository.IBlacklistRepository) BlacklistService {
	return &blacklistServiceImpl{
		blacklistRepo: blacklistRepo,
	}
}

// AddBlacklist 拉黑用户
func (s *blacklistServiceImpl) AddBlacklist(ctx context.Context, req *dto.AddBlacklistRequest) error {
	return status.Error(codes.Unimplemented, "拉黑用户功能暂未实现")
}

// RemoveBlacklist 取消拉黑
func (s *blacklistServiceImpl) RemoveBlacklist(ctx context.Context, req *dto.RemoveBlacklistRequest) error {
	return status.Error(codes.Unimplemented, "取消拉黑功能暂未实现")
}

// GetBlacklistList 获取黑名单列表
func (s *blacklistServiceImpl) GetBlacklistList(ctx context.Context, req *dto.GetBlacklistListRequest) (*dto.GetBlacklistListResponse, error) {
	return nil, status.Error(codes.Unimplemented, "获取黑名单列表功能暂未实现")
}

// CheckIsBlacklist 判断是否拉黑
func (s *blacklistServiceImpl) CheckIsBlacklist(ctx context.Context, req *dto.CheckIsBlacklistRequest) (*dto.CheckIsBlacklistResponse, error) {
	return nil, status.Error(codes.Unimplemented, "判断是否拉黑功能暂未实现")
}
