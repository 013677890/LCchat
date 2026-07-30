package service

import (
	"context"
	"errors"

	"github.com/013677890/LCchat-Backend/apps/relation/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/realtimepush"
	"github.com/013677890/LCchat-Backend/pkg/util"
)

type blacklistServiceImpl struct {
	blacklistRepo     repository.IBlacklistRepository
	realtimePublisher realtimepush.Publisher
}

// NewBlacklistService 创建黑名单服务实例。
//
// 当前黑名单流程只通过 IBlacklistRepository 读写关系事实，并在成功后发布实时通知。
// 构造器不预留尚未使用的数据库、Redis 或好友仓储依赖；后续若出现真实规则再按用途显式引入。
func NewBlacklistService(
	blacklistRepo repository.IBlacklistRepository,
	realtimePublisher realtimepush.Publisher,
) IBlacklistService {
	return &blacklistServiceImpl{
		blacklistRepo:     blacklistRepo,
		realtimePublisher: realtimePublisher,
	}
}

// AddBlacklist 拉黑用户。
//
// 该接口严格复用单体阶段的业务语义：
//  1. 当前用户必须已登录；
//  2. 不能拉黑自己；
//  3. 若已在黑名单中则直接返回业务错误；
//  4. 否则调用 repository 写入黑名单关系。
//
// 错误码映射：
//   - codes.Unauthenticated: 未登录
//   - codes.InvalidArgument: 参数错误或拉黑自己
//   - codes.AlreadyExists: 已在黑名单中
//   - codes.Internal: 系统内部错误
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
	s.publishBlacklistRelationChanged(ctx, currentUserUUID, req.TargetUuid, friendChangeBlacklistAdded)
	return nil
}

// RemoveBlacklist 取消拉黑。
//
// 为了保持业务反馈一致，service 层会先判定当前是否真的处于黑名单中，
// 不在黑名单时直接返回 CodeNotInBlacklist，而不是让下层返回通用数据库错误。
//
// 错误码映射：
//   - codes.Unauthenticated: 未登录
//   - codes.InvalidArgument: 参数错误
//   - codes.NotFound: 当前不在黑名单中
//   - codes.Internal: 系统内部错误
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

	s.publishBlacklistRelationChanged(ctx, currentUserUUID, req.UserUuid, friendChangeBlacklistRemoved)
	return nil
}

// GetBlacklistList 获取黑名单列表。
//
// 当前仅返回 relation 域持有的字段：uuid 与 blacklisted_at；昵称、头像后续通过 profile
// 内部接口批量补齐，避免 relation-service 跨表越权查询。
//
// 错误码映射：
//   - codes.Unauthenticated: 未登录
//   - codes.Internal: 系统内部错误
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
		return buildBlacklistListResponse(nil, page, pageSize, total), nil
	}

	items := make([]*pb.BlacklistItem, 0, len(relations))
	for _, relation := range relations {
		if item := buildBlacklistItemProto(relation); item != nil {
			items = append(items, item)
		}
	}

	return buildBlacklistListResponse(items, page, pageSize, total), nil
}

// CheckIsBlacklist 判断是否存在拉黑关系。
//
// 该接口不依赖登录态上下文，而是直接以请求中的 user_uuid / target_uuid 为判断输入，
// 便于被网关、消息服务或其他内部调用链复用。
//
// 错误码映射：
//   - codes.InvalidArgument: 参数错误
//   - codes.Internal: 系统内部错误
func (s *blacklistServiceImpl) CheckIsBlacklist(ctx context.Context, req *pb.CheckIsBlacklistRequest) (*pb.CheckIsBlacklistResponse, error) {
	if req == nil || req.UserUuid == "" || req.TargetUuid == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	isBlocked, err := s.blacklistRepo.IsBlocked(ctx, req.UserUuid, req.TargetUuid)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "判断是否拉黑失败")
	}

	return buildCheckIsBlacklistResponse(isBlocked), nil
}
