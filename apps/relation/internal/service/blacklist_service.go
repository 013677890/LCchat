package service

import (
	"context"
	"errors"

	"github.com/013677890/LCchat-Backend/apps/relation/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/util"

	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type blacklistServiceImpl struct {
	db            *gorm.DB
	redisClient   *goredis.Client
	blacklistRepo repository.IBlacklistRepository
	friendRepo    repository.IFriendRepository
}

// NewBlacklistService 创建黑名单服务实例。
//
// 当前最小实现只依赖 relation 域自己的黑名单仓储即可闭环；friendRepo 先保留在结构体中，
// 便于后续如果需要联动好友关系规则时直接扩展，而不用再次调整依赖注入图。
func NewBlacklistService(
	db *gorm.DB,
	redisClient *goredis.Client,
	blacklistRepo repository.IBlacklistRepository,
	friendRepo repository.IFriendRepository,
) IBlacklistService {
	return &blacklistServiceImpl{
		db:            db,
		redisClient:   redisClient,
		blacklistRepo: blacklistRepo,
		friendRepo:    friendRepo,
	}
}

// AddBlacklist 拉黑用户。
//
// 该接口严格复用单体阶段的业务语义：
//  1. 当前用户必须已登录；
//  2. 不能拉黑自己；
//  3. 若已在黑名单中则直接返回业务错误；
//  4. 否则调用 repository 写入黑名单关系。
func (s *blacklistServiceImpl) AddBlacklist(ctx context.Context, req *pb.AddBlacklistRequest) error {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	if req == nil || req.TargetUuid == "" {
		return apperr.New(consts.CodeParamError)
	}
	if req.TargetUuid == currentUserUUID {
		return apperr.New(consts.CodeCannotBlacklistSelf)
	}

	isBlocked, err := s.blacklistRepo.IsBlocked(ctx, currentUserUUID, req.TargetUuid)
	if err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "检查黑名单失败")
	}
	if isBlocked {
		return apperr.New(consts.CodeAlreadyInBlacklist)
	}

	if err := s.blacklistRepo.AddBlacklist(ctx, currentUserUUID, req.TargetUuid); err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "拉黑用户失败")
	}
	return nil
}

// RemoveBlacklist 取消拉黑。
//
// 为了保持业务反馈一致，service 层会先判定当前是否真的处于黑名单中，
// 不在黑名单时直接返回 CodeNotInBlacklist，而不是让下层返回通用数据库错误。
func (s *blacklistServiceImpl) RemoveBlacklist(ctx context.Context, req *pb.RemoveBlacklistRequest) error {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	if req == nil || req.UserUuid == "" {
		return apperr.New(consts.CodeParamError)
	}

	isBlocked, err := s.blacklistRepo.IsBlocked(ctx, currentUserUUID, req.UserUuid)
	if err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "检查黑名单失败")
	}
	if !isBlocked {
		return apperr.New(consts.CodeNotInBlacklist)
	}

	if err := s.blacklistRepo.RemoveBlacklist(ctx, currentUserUUID, req.UserUuid); err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return apperr.New(consts.CodeNotInBlacklist)
		}
		return apperr.Wrap(err, consts.CodeInternalError, "取消拉黑失败")
	}
	return nil
}

// GetBlacklistList 获取黑名单列表。
//
// 当前仅返回 relation 域持有的字段：uuid 与 blacklisted_at；昵称、头像后续通过 profile
// 内部接口批量补齐，避免 relation-service 跨表越权查询。
func (s *blacklistServiceImpl) GetBlacklistList(ctx context.Context, req *pb.GetBlacklistListRequest) (*pb.GetBlacklistListResponse, error) {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	page := int32(1)
	pageSize := int32(20)
	if req != nil {
		if req.Page > 0 {
			page = req.Page
		}
		if req.PageSize > 0 {
			pageSize = req.PageSize
		}
	}

	relations, total, err := s.blacklistRepo.GetBlacklistList(ctx, currentUserUUID, int(page), int(pageSize))
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取黑名单列表失败")
	}

	if len(relations) == 0 {
		return &pb.GetBlacklistListResponse{
			Items:      []*pb.BlacklistItem{},
			Pagination: buildPagination(page, pageSize, total),
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
		Items:      items,
		Pagination: buildPagination(page, pageSize, total),
	}, nil
}

// CheckIsBlacklist 判断是否存在拉黑关系。
//
// 该接口不依赖登录态上下文，而是直接以请求中的 user_uuid / target_uuid 为判断输入，
// 便于被网关、消息服务或其他内部调用链复用。
func (s *blacklistServiceImpl) CheckIsBlacklist(ctx context.Context, req *pb.CheckIsBlacklistRequest) (*pb.CheckIsBlacklistResponse, error) {
	if req == nil || req.UserUuid == "" || req.TargetUuid == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	isBlocked, err := s.blacklistRepo.IsBlocked(ctx, req.UserUuid, req.TargetUuid)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "判断是否拉黑失败")
	}

	return &pb.CheckIsBlacklistResponse{IsBlacklist: isBlocked}, nil
}
