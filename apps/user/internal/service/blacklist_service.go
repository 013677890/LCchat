package service

import (
	"context"
	"errors"

	"github.com/013677890/LCchat-Backend/apps/user/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/util"
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
func (s *blacklistServiceImpl) AddBlacklist(ctx context.Context, req *pb.AddBlacklistRequest) error {
	// 1. 从context中获取当前用户UUID
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}

	// 2. 参数校验
	if req == nil || req.TargetUuid == "" {
		return apperr.New(consts.CodeParamError)
	}

	// 3. 不能拉黑自己
	if req.TargetUuid == currentUserUUID {
		return apperr.New(consts.CodeCannotBlacklistSelf)
	}

	// 4. 判断是否已在黑名单中
	isBlocked, err := s.blacklistRepo.IsBlocked(ctx, currentUserUUID, req.TargetUuid)
	if err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "检查黑名单失败")
	}
	if isBlocked {
		return apperr.New(consts.CodeAlreadyInBlacklist)
	}

	// 5. 拉黑用户
	if err := s.blacklistRepo.AddBlacklist(ctx, currentUserUUID, req.TargetUuid); err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "拉黑用户失败")
	}

	return nil
}

// RemoveBlacklist 取消拉黑
func (s *blacklistServiceImpl) RemoveBlacklist(ctx context.Context, req *pb.RemoveBlacklistRequest) error {
	// 1. 从context中获取当前用户UUID
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}

	// 2. 参数校验
	if req == nil || req.UserUuid == "" {
		return apperr.New(consts.CodeParamError)
	}

	// 3. 判断是否已在黑名单中
	isBlocked, err := s.blacklistRepo.IsBlocked(ctx, currentUserUUID, req.UserUuid)
	if err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "检查黑名单失败")
	}
	if !isBlocked {
		return apperr.New(consts.CodeNotInBlacklist)
	}

	// 4. 取消拉黑
	if err := s.blacklistRepo.RemoveBlacklist(ctx, currentUserUUID, req.UserUuid); err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return apperr.New(consts.CodeNotInBlacklist)
		}
		return apperr.Wrap(err, consts.CodeInternalError, "取消拉黑失败")
	}

	return nil
}

// GetBlacklistList 获取黑名单列表
func (s *blacklistServiceImpl) GetBlacklistList(ctx context.Context, req *pb.GetBlacklistListRequest) (*pb.GetBlacklistListResponse, error) {
	// 1. 从context中获取当前用户UUID
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 2. 兜底分页参数
	page := req.Page
	pageSize := req.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	// 3. 获取黑名单列表
	relations, total, err := s.blacklistRepo.GetBlacklistList(ctx, currentUserUUID, int(page), int(pageSize))
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取黑名单列表失败")
	}

	if len(relations) == 0 {
		return &pb.GetBlacklistListResponse{
			Items: []*pb.BlacklistItem{},
			Pagination: &pb.PaginationInfo{
				Page:       page,
				PageSize:   pageSize,
				Total:      total,
				TotalPages: int32((total + int64(pageSize) - 1) / int64(pageSize)),
			},
		}, nil
	}

	items := make([]*pb.BlacklistItem, 0, len(relations))
	for _, relation := range relations {
		if relation == nil {
			continue
		}
		blacklistedAt := relation.UpdatedAt
		if relation.BlacklistedAt != nil {
			blacklistedAt = *relation.BlacklistedAt
		}
		items = append(items, &pb.BlacklistItem{
			Uuid:          relation.PeerUuid,
			BlacklistedAt: blacklistedAt.UnixMilli(),
		})
	}

	return &pb.GetBlacklistListResponse{
		Items: items,
		Pagination: &pb.PaginationInfo{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: int32((total + int64(pageSize) - 1) / int64(pageSize)),
		},
	}, nil
}

// CheckIsBlacklist 判断是否拉黑
func (s *blacklistServiceImpl) CheckIsBlacklist(ctx context.Context, req *pb.CheckIsBlacklistRequest) (*pb.CheckIsBlacklistResponse, error) {
	if req == nil || req.UserUuid == "" || req.TargetUuid == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	isBlocked, err := s.blacklistRepo.IsBlocked(ctx, req.UserUuid, req.TargetUuid)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "判断是否拉黑失败")
	}

	return &pb.CheckIsBlacklistResponse{
		IsBlacklist: isBlocked,
	}, nil
}
