package service

import (
	"context"
	"errors"
	"time"

	"github.com/013677890/LCchat-Backend/apps/relation/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/util"

	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type friendServiceImpl struct {
	db            *gorm.DB
	redisClient   *goredis.Client
	friendRepo    repository.IFriendRepository
	applyRepo     repository.IApplyRepository
	blacklistRepo repository.IBlacklistRepository
}

// NewFriendService 创建好友服务实例。
//
// relation-service 当前仍处于拆分中的最小可运行阶段，因此 service 层主要承担：
//  1. 参数与权限校验；
//  2. 仓储调用编排；
//  3. 业务错误到统一错误码的映射；
//  4. relation proto 响应对象的组装。
func NewFriendService(
	db *gorm.DB,
	redisClient *goredis.Client,
	friendRepo repository.IFriendRepository,
	applyRepo repository.IApplyRepository,
	blacklistRepo repository.IBlacklistRepository,
) IFriendService {
	return &friendServiceImpl{
		db:            db,
		redisClient:   redisClient,
		friendRepo:    friendRepo,
		applyRepo:     applyRepo,
		blacklistRepo: blacklistRepo,
	}
}

// SendFriendApply 发送好友申请。
//
// 该方法按当前单体阶段既有语义完成校验：
//  1. 必须从上下文中拿到当前登录用户；
//  2. 不允许给自己发好友申请；
//  3. 已经是好友、已有待处理申请、任一方向存在拉黑关系时都应拒绝；
//  4. 通过校验后才真正落库创建申请记录。
func (s *friendServiceImpl) SendFriendApply(ctx context.Context, req *pb.SendFriendApplyRequest) (*pb.SendFriendApplyResponse, error) {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}
	if req == nil || req.TargetUuid == "" {
		return nil, apperr.New(consts.CodeParamError)
	}
	if currentUserUUID == req.TargetUuid {
		return nil, apperr.New(consts.CodeCannotAddSelf)
	}

	isFriend, err := s.friendRepo.IsFriend(ctx, currentUserUUID, req.TargetUuid)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "检查是否好友失败")
	}
	if isFriend {
		return nil, apperr.New(consts.CodeAlreadyFriend)
	}

	exists, err := s.applyRepo.ExistsPendingRequest(ctx, currentUserUUID, req.TargetUuid)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "检查待处理申请失败")
	}
	if exists {
		return nil, apperr.New(consts.CodeFriendRequestSent)
	}

	// 先检查对方是否拉黑当前用户，再检查当前用户是否拉黑对方，保证错误码语义明确。
	isBlockedByTarget, err := s.blacklistRepo.IsBlocked(ctx, req.TargetUuid, currentUserUUID)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "检查是否被拉黑失败")
	}
	if isBlockedByTarget {
		return nil, apperr.New(consts.CodePeerBlacklistYou)
	}

	isBlocked, err := s.blacklistRepo.IsBlocked(ctx, currentUserUUID, req.TargetUuid)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "检查拉黑状态失败")
	}
	if isBlocked {
		return nil, apperr.New(consts.CodeYouBlacklistPeer)
	}

	apply := &model.ApplyRequest{
		ApplyType:     0,
		ApplicantUuid: currentUserUUID,
		TargetUuid:    req.TargetUuid,
		Status:        0,
		IsRead:        false,
		Reason:        req.Reason,
		Source:        req.Source,
	}

	createdApply, err := s.applyRepo.Create(ctx, apply)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "创建好友申请失败")
	}

	return &pb.SendFriendApplyResponse{ApplyId: createdApply.Id}, nil
}

// GetFriendApplyList 获取收到的好友申请列表。
//
// 当前最小闭环先返回 relation 域自身拥有的信息：申请记录以及申请人 UUID；
// 用户昵称头像后续再通过 InternalProfileService 批量聚合补齐。
func (s *friendServiceImpl) GetFriendApplyList(ctx context.Context, req *pb.GetFriendApplyListRequest) (*pb.GetFriendApplyListResponse, error) {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	page := int32(1)
	pageSize := int32(20)
	status := int32(-1)
	if req != nil {
		status = req.Status
		if req.Page > 0 {
			page = req.Page
		}
		if req.PageSize > 0 {
			pageSize = req.PageSize
		}
	}

	applies, total, err := s.applyRepo.GetPendingList(ctx, currentUserUUID, int(status), int(page), int(pageSize))
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取好友申请列表失败")
	}

	if len(applies) == 0 {
		if err := s.applyRepo.ClearUnreadCount(ctx, currentUserUUID); err != nil {
			logger.Warn(ctx, "清除好友申请未读数量失败",
				logger.String("user_uuid", currentUserUUID),
				logger.ErrorField("error", err),
			)
		}
		return &pb.GetFriendApplyListResponse{
			Items:      []*pb.FriendApplyItem{},
			Pagination: buildPagination(page, pageSize, total),
		}, nil
	}

	items := make([]*pb.FriendApplyItem, 0, len(applies))
	unreadIDs := make([]int64, 0, len(applies))
	for _, apply := range applies {
		if apply == nil {
			continue
		}
		if !apply.IsRead {
			unreadIDs = append(unreadIDs, apply.Id)
		}

		if item := buildFriendApplyItemProto(apply); item != nil {
			items = append(items, item)
		}
	}

	// 列表读取后异步消已读，只做尽力而为，不阻塞主流程。
	if len(unreadIDs) > 0 {
		s.applyRepo.MarkAsReadAsync(ctx, unreadIDs)
	}
	if err := s.applyRepo.ClearUnreadCount(ctx, currentUserUUID); err != nil {
		logger.Warn(ctx, "清除好友申请未读数量失败",
			logger.String("user_uuid", currentUserUUID),
			logger.ErrorField("error", err),
		)
	}

	return &pb.GetFriendApplyListResponse{
		Items:      items,
		Pagination: buildPagination(page, pageSize, total),
	}, nil
}

// GetSentApplyList 获取当前用户发出的好友申请列表。
//
// 与 GetFriendApplyList 类似，此处只拼装 target_uuid，不跨服务补齐资料字段。
func (s *friendServiceImpl) GetSentApplyList(ctx context.Context, req *pb.GetSentApplyListRequest) (*pb.GetSentApplyListResponse, error) {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	page := int32(1)
	pageSize := int32(20)
	status := int32(-1)
	if req != nil {
		status = req.Status
		if req.Page > 0 {
			page = req.Page
		}
		if req.PageSize > 0 {
			pageSize = req.PageSize
		}
	}

	applies, total, err := s.applyRepo.GetSentList(ctx, currentUserUUID, int(status), int(page), int(pageSize))
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取发出的申请列表失败")
	}

	if len(applies) == 0 {
		return &pb.GetSentApplyListResponse{
			Items:      []*pb.SentApplyItem{},
			Pagination: buildPagination(page, pageSize, total),
		}, nil
	}

	items := make([]*pb.SentApplyItem, 0, len(applies))
	for _, apply := range applies {
		if item := buildSentApplyItemProto(apply); item != nil {
			items = append(items, item)
		}
	}

	return &pb.GetSentApplyListResponse{
		Items:      items,
		Pagination: buildPagination(page, pageSize, total),
	}, nil
}

// HandleFriendApply 处理好友申请。
//
// action=1 时走“同意 + 建立双向关系”的事务路径；
// action=2 时仅将申请标记为拒绝。
func (s *friendServiceImpl) HandleFriendApply(ctx context.Context, req *pb.HandleFriendApplyRequest) error {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	if req == nil || req.ApplyId <= 0 {
		return apperr.New(consts.CodeParamError)
	}

	apply, err := s.applyRepo.GetByID(ctx, req.ApplyId)
	if err != nil || apply == nil {
		return apperr.New(consts.CodeApplyNotFoundOrHandle)
	}
	if apply.TargetUuid != currentUserUUID {
		return apperr.New(consts.CodeNoPermission)
	}

	if req.Action == 1 {
		_, err := s.applyRepo.AcceptApplyAndCreateRelation(ctx, req.ApplyId, currentUserUUID, apply.ApplicantUuid, req.Remark)
		if err != nil {
			return apperr.Wrap(err, consts.CodeInternalError, "同意好友申请失败")
		}
		return nil
	}

	err = s.applyRepo.UpdateStatus(ctx, req.ApplyId, int(req.Action), req.Remark)
	if err != nil {
		// 已处理的申请在业务上视为幂等成功，不再向上抛错。
		if err == repository.ErrApplyNotFound {
			return nil
		}
		return apperr.Wrap(err, consts.CodeInternalError, "拒绝好友申请失败")
	}
	return nil
}

// GetUnreadApplyCount 返回当前用户未读好友申请数量。
//
// 当前 DB-first 实现直接查询数据库；若后续恢复 Redis 红点缓存，可在 repository 内部透明优化。
func (s *friendServiceImpl) GetUnreadApplyCount(ctx context.Context, req *pb.GetUnreadApplyCountRequest) (*pb.GetUnreadApplyCountResponse, error) {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	count, err := s.applyRepo.GetUnreadCount(ctx, currentUserUUID)
	if err != nil {
		logger.Warn(ctx, "获取好友申请未读数量失败，降级返回 0",
			logger.String("user_uuid", currentUserUUID),
			logger.ErrorField("error", err),
		)
		count = 0
	}

	return &pb.GetUnreadApplyCountResponse{UnreadCount: int32(count)}, nil
}

// MarkApplyAsRead 将指定申请或全部申请标记为已读。
//
// apply_ids 为空时表示“全部已读”，否则只对指定记录做批量更新。
func (s *friendServiceImpl) MarkApplyAsRead(ctx context.Context, req *pb.MarkApplyAsReadRequest) error {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}

	if req == nil || len(req.ApplyIds) == 0 {
		if _, err := s.applyRepo.MarkAllAsRead(ctx, currentUserUUID); err != nil {
			return apperr.Wrap(err, consts.CodeInternalError, "标记全部申请已读失败")
		}
	} else {
		if _, err := s.applyRepo.MarkAsRead(ctx, currentUserUUID, req.ApplyIds); err != nil {
			return apperr.Wrap(err, consts.CodeInternalError, "标记申请已读失败")
		}
	}

	if err := s.applyRepo.ClearUnreadCount(ctx, currentUserUUID); err != nil {
		logger.Warn(ctx, "清除好友申请未读数量失败",
			logger.String("user_uuid", currentUserUUID),
			logger.ErrorField("error", err),
		)
	}
	return nil
}

// GetFriendList 获取当前用户好友列表。
//
// 当前实现只返回 relation 表中的关系域字段；资料字段保留为空，等待后续跨服务聚合补齐。
func (s *friendServiceImpl) GetFriendList(ctx context.Context, req *pb.GetFriendListRequest) (*pb.GetFriendListResponse, error) {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	page := int32(1)
	pageSize := int32(20)
	groupTag := ""
	if req != nil {
		groupTag = req.GroupTag
		if req.Page > 0 {
			page = req.Page
		}
		if req.PageSize > 0 {
			pageSize = req.PageSize
		}
	}

	relations, total, version, err := s.friendRepo.GetFriendList(ctx, currentUserUUID, groupTag, int(page), int(pageSize))
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取好友列表失败")
	}

	if len(relations) == 0 {
		return &pb.GetFriendListResponse{
			Items:      []*pb.FriendItem{},
			Pagination: buildPagination(page, pageSize, total),
			Version:    version,
		}, nil
	}

	items := make([]*pb.FriendItem, 0, len(relations))
	for _, relation := range relations {
		if item := buildFriendItemProto(relation); item != nil {
			items = append(items, item)
		}
	}

	return &pb.GetFriendListResponse{
		Items:      items,
		Pagination: buildPagination(page, pageSize, total),
		Version:    version,
	}, nil
}

// SyncFriendList 增量同步好友列表。
//
// 返回的 change_type 由 relation 状态与时间共同决定：
//  1. deleted_at 有效或 status=2 => delete；
//  2. created_at 晚于请求 version => add；
//  3. 其余仍存在关系的变更 => update。
func (s *friendServiceImpl) SyncFriendList(ctx context.Context, req *pb.SyncFriendListRequest) (*pb.SyncFriendListResponse, error) {
	const syncVersionRollbackMs int64 = 2000

	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	limit := 100
	version := int64(0)
	if req != nil {
		if req.Limit > 0 {
			limit = int(req.Limit)
		}
		if req.Version > 0 {
			version = req.Version
		}
	}
	if limit > 500 {
		limit = 500
	}

	relations, latestDBVersion, hasMore, err := s.friendRepo.SyncFriendList(ctx, currentUserUUID, version, limit)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "增量同步好友列表失败")
	}

	if len(relations) == 0 {
		latestVersion := time.Now().UnixMilli() - syncVersionRollbackMs
		if latestVersion < 0 {
			latestVersion = 0
		}
		return &pb.SyncFriendListResponse{
			Changes:       []*pb.FriendChange{},
			HasMore:       false,
			LatestVersion: latestVersion,
		}, nil
	}

	versionTime := time.UnixMilli(version)
	changes := make([]*pb.FriendChange, 0, len(relations))
	var lastChangedAt int64
	for _, relation := range relations {
		if relation == nil {
			continue
		}

		changeType := "update"
		changedAt := relation.UpdatedAt.UnixMilli()

		// 已删除关系优先映射为 delete，这样客户端能够正确清理本地好友项。
		if relation.DeletedAt.Valid || relation.Status == 2 {
			changeType = "delete"
			if relation.DeletedAt.Valid {
				changedAt = relation.DeletedAt.Time.UnixMilli()
			}
		} else if relation.CreatedAt.After(versionTime) {
			changeType = "add"
		}

		if item := buildFriendChangeProto(relation, changeType, changedAt); item != nil {
			changes = append(changes, item)
		}
		lastChangedAt = changedAt
	}

	latestVersion := latestDBVersion
	if hasMore {
		// 分页场景以下一批起点为准，避免越过未返回的数据。
		latestVersion = lastChangedAt
	} else {
		// 完整返回当前批次后回退一小段时间，降低边界时间戳漏数据的风险。
		latestVersion = time.Now().UnixMilli() - syncVersionRollbackMs
		if latestVersion < 0 {
			latestVersion = 0
		}
	}

	return &pb.SyncFriendListResponse{
		Changes:       changes,
		HasMore:       hasMore,
		LatestVersion: latestVersion,
	}, nil
}

// DeleteFriend 删除一条单向好友关系。
//
// 当前仓储模型是单向关系，因此这里只删除当前用户视角下的数据，保持与旧实现一致。
func (s *friendServiceImpl) DeleteFriend(ctx context.Context, req *pb.DeleteFriendRequest) error {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	if req == nil || req.UserUuid == "" {
		return apperr.New(consts.CodeParamError)
	}

	if err := s.friendRepo.DeleteFriendRelation(ctx, currentUserUUID, req.UserUuid); err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return apperr.New(consts.CodeNotFriend)
		}
		return apperr.Wrap(err, consts.CodeInternalError, "删除好友关系失败")
	}
	return nil
}

// SetFriendRemark 设置好友备注。
//
// 备注是单向展示属性，仅影响当前登录用户自己的好友列表视图。
func (s *friendServiceImpl) SetFriendRemark(ctx context.Context, req *pb.SetFriendRemarkRequest) error {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	if req == nil || req.UserUuid == "" {
		return apperr.New(consts.CodeParamError)
	}

	if err := s.friendRepo.SetFriendRemark(ctx, currentUserUUID, req.UserUuid, req.Remark); err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return apperr.New(consts.CodeNotFriend)
		}
		return apperr.Wrap(err, consts.CodeInternalError, "设置好友备注失败")
	}
	return nil
}

// SetFriendTag 设置好友标签。
//
// 标签同样是当前用户视角下的单向属性，不会同步修改对方的 relation 记录。
func (s *friendServiceImpl) SetFriendTag(ctx context.Context, req *pb.SetFriendTagRequest) error {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	if req == nil || req.UserUuid == "" {
		return apperr.New(consts.CodeParamError)
	}

	if err := s.friendRepo.SetFriendTag(ctx, currentUserUUID, req.UserUuid, req.GroupTag); err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return apperr.New(consts.CodeNotFriend)
		}
		return apperr.Wrap(err, consts.CodeInternalError, "设置好友标签失败")
	}
	return nil
}

// GetTagList 获取标签列表。
//
// 旧单体中的该接口尚未完整实现标签计数能力。为了保持现有行为一致，当前仍返回
// MethodNotAllowed，避免给调用方制造“看似可用但统计不准确”的假象。
func (s *friendServiceImpl) GetTagList(ctx context.Context, req *pb.GetTagListRequest) (*pb.GetTagListResponse, error) {
	return nil, apperr.NewWithMessage(consts.CodeMethodNotAllowed, "获取标签列表功能暂未实现")
}

// CheckIsFriend 判断两个用户之间是否存在好友关系。
//
// 该接口主要面向其他服务或网关的关系判定，不依赖当前登录上下文。
func (s *friendServiceImpl) CheckIsFriend(ctx context.Context, req *pb.CheckIsFriendRequest) (*pb.CheckIsFriendResponse, error) {
	if req == nil || req.UserUuid == "" || req.PeerUuid == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	isFriend, err := s.friendRepo.CheckIsFriendRelation(ctx, req.UserUuid, req.PeerUuid)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "判断是否好友失败")
	}
	return &pb.CheckIsFriendResponse{IsFriend: isFriend}, nil
}

// BatchCheckIsFriend 批量判断好友关系。
//
// 返回结果按请求中的 peer_uuids 顺序重组，方便调用方直接和原始请求一一对应。
func (s *friendServiceImpl) BatchCheckIsFriend(ctx context.Context, req *pb.BatchCheckIsFriendRequest) (*pb.BatchCheckIsFriendResponse, error) {
	if req == nil || req.UserUuid == "" || len(req.PeerUuids) == 0 {
		return &pb.BatchCheckIsFriendResponse{Items: []*pb.FriendCheckItem{}}, nil
	}

	result, err := s.friendRepo.BatchCheckIsFriend(ctx, req.UserUuid, req.PeerUuids)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "批量判断是否好友失败")
	}

	items := make([]*pb.FriendCheckItem, 0, len(req.PeerUuids))
	for _, peerUUID := range req.PeerUuids {
		if item := buildFriendCheckItemProto(peerUUID, result[peerUUID]); item != nil {
			items = append(items, item)
		}
	}

	return &pb.BatchCheckIsFriendResponse{Items: items}, nil
}

// GetRelationStatus 获取两个用户之间的关系状态。
//
// 响应分为四种语义：
//  1. none：从未建立过关系；
//  2. friend：当前为好友；
//  3. blacklist：当前处于黑名单；
//  4. deleted：曾经存在好友关系但已删除。
func (s *friendServiceImpl) GetRelationStatus(ctx context.Context, req *pb.GetRelationStatusRequest) (*pb.GetRelationStatusResponse, error) {
	if req == nil || req.UserUuid == "" || req.PeerUuid == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	relation, err := s.friendRepo.GetRelationStatus(ctx, req.UserUuid, req.PeerUuid)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取关系状态失败")
	}

	resp := buildRelationStatusResponse("none", false, false, "", "")
	if relation == nil {
		return resp, nil
	}

	if relation.DeletedAt.Valid || relation.Status == 2 {
		resp.Relation = "deleted"
		return resp, nil
	}

	switch relation.Status {
	case 0:
		resp.Relation = "friend"
		resp.IsFriend = true
		resp.Remark = relation.Remark
		resp.GroupTag = relation.GroupTag
	case 1, 3:
		resp.Relation = "blacklist"
		resp.IsBlacklist = true
	default:
		resp.Relation = "none"
	}

	return resp, nil
}
