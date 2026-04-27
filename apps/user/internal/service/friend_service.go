package service

import (
	"context"
	"errors"
	"time"

	"github.com/013677890/LCchat-Backend/apps/user/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/util"
)

// friendServiceImpl 好友关系服务实现
type friendServiceImpl struct {
	friendRepo    repository.IFriendRepository
	applyRepo     repository.IApplyRepository
	blacklistRepo repository.IBlacklistRepository
}

// NewFriendService 创建好友服务实例
func NewFriendService(
	friendRepo repository.IFriendRepository,
	applyRepo repository.IApplyRepository,
	blacklistRepo repository.IBlacklistRepository,
) FriendService {
	return &friendServiceImpl{
		friendRepo:    friendRepo,
		applyRepo:     applyRepo,
		blacklistRepo: blacklistRepo,
	}
}

// SendFriendApply 发送好友申请
// 业务流程：
//  1. 从context中获取当前用户UUID（申请人）
//  2. 检查不能添加自己为好友
//  3. 检查是否已经是好友
//  4. 检查是否存在待处理的申请
//  5. 检查对方是否已将你拉黑
//  6. 检查你是否已将对方拉黑
//  7. 创建好友申请记录
//  8. 返回申请ID
//
// 错误码映射：
//   - codes.InvalidArgument: 不能添加自己为好友
//   - codes.AlreadyExists: 已经是好友、申请已发送
//   - codes.FailedPrecondition: 对方已将你拉黑、你已将对方拉黑
//   - codes.Internal: 系统内部错误
func (s *friendServiceImpl) SendFriendApply(ctx context.Context, req *pb.SendFriendApplyRequest) (*pb.SendFriendApplyResponse, error) {
	// 1. 从context中获取当前用户UUID（申请人）
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 2. 检查不能添加自己为好友
	if currentUserUUID == req.TargetUuid {
		return nil, apperr.New(consts.CodeCannotAddSelf)
	}

	// 3. 检查是否已经是好友
	isFriend, err := s.friendRepo.IsFriend(ctx, currentUserUUID, req.TargetUuid)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "检查是否好友失败")
	}

	if isFriend {
		return nil, apperr.New(consts.CodeAlreadyFriend)
	}

	// 4. 检查是否存在待处理的申请
	exists, err := s.applyRepo.ExistsPendingRequest(ctx, currentUserUUID, req.TargetUuid)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "检查待处理申请失败")
	}

	if exists {
		return nil, apperr.New(consts.CodeFriendRequestSent)
	}

	// 5. 检查对方是否已将你拉黑
	isBlockedByTarget, err := s.blacklistRepo.IsBlocked(ctx, req.TargetUuid, currentUserUUID)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "检查是否被拉黑失败")
	}

	if isBlockedByTarget {
		return nil, apperr.New(consts.CodePeerBlacklistYou)
	}

	// 6. 检查你是否已将对方拉黑
	isBlocked, err := s.blacklistRepo.IsBlocked(ctx, currentUserUUID, req.TargetUuid)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "检查拉黑状态失败")
	}

	if isBlocked {
		return nil, apperr.New(consts.CodeYouBlacklistPeer)
	}

	// 7. 创建好友申请记录
	apply := &model.ApplyRequest{
		ApplyType:     0, // 0=好友申请
		ApplicantUuid: currentUserUUID,
		TargetUuid:    req.TargetUuid,
		Status:        0, // 0=待处理
		IsRead:        false,
		Reason:        req.Reason,
		Source:        req.Source,
	}

	createdApply, err := s.applyRepo.Create(ctx, apply)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "创建好友申请失败")
	}

	// 8. 返回申请ID
	return &pb.SendFriendApplyResponse{
		ApplyId: createdApply.Id,
	}, nil
}

// GetFriendApplyList 获取好友申请列表
func (s *friendServiceImpl) GetFriendApplyList(ctx context.Context, req *pb.GetFriendApplyListRequest) (*pb.GetFriendApplyListResponse, error) {
	// 从上下文获取当前用户
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 兜底分页参数（即使网关做了默认值，这里也防御性处理）
	page := req.Page
	pageSize := req.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	// 查询申请列表（status<0 表示全部状态）
	applies, total, err := s.applyRepo.GetPendingList(ctx, currentUserUUID, int(req.Status), int(page), int(pageSize))
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取好友申请列表失败")
	}

	if len(applies) == 0 {
		// 空列表也需要清除未读数量红点（尽力而为）
		if err := s.applyRepo.ClearUnreadCount(ctx, currentUserUUID); err != nil {
			logger.Warn(ctx, "清除好友申请未读数量失败",
				logger.String("user_uuid", currentUserUUID),
				logger.ErrorField("error", err),
			)
		}
		// 空列表直接返回，避免后续无意义的批量查询
		return &pb.GetFriendApplyListResponse{
			Items: []*pb.FriendApplyItem{},
			Pagination: &pb.PaginationInfo{
				Page:       page,
				PageSize:   pageSize,
				Total:      total,
				TotalPages: int32((total + int64(pageSize) - 1) / int64(pageSize)),
			},
		}, nil
	}

	// 组装返回项（申请记录 + 申请人简要信息）
	items := make([]*pb.FriendApplyItem, 0, len(applies))
	unreadIDs := make([]int64, 0) // 收集未读申请的 ID

	for _, apply := range applies {
		if apply == nil {
			continue
		}

		// 收集未读申请 ID
		if !apply.IsRead {
			unreadIDs = append(unreadIDs, apply.Id)
		}

		applicantInfo := &pb.SimpleUserInfo{
			Uuid: apply.ApplicantUuid,
		}

		// created_at 使用毫秒时间戳（与网关 DTO 一致）
		items = append(items, &pb.FriendApplyItem{
			ApplyId:       apply.Id,
			ApplicantUuid: apply.ApplicantUuid,
			ApplicantInfo: applicantInfo,
			Reason:        apply.Reason,
			Source:        apply.Source,
			Status:        int32(apply.Status),
			IsRead:        apply.IsRead,
			CreatedAt:     apply.CreatedAt.UnixMilli(),
		})
	}

	// 异步标记已读（不阻塞响应）
	if len(unreadIDs) > 0 {
		s.applyRepo.MarkAsReadAsync(ctx, unreadIDs)
	}

	// 清除未读数量红点（尽力而为）
	if err := s.applyRepo.ClearUnreadCount(ctx, currentUserUUID); err != nil {
		logger.Warn(ctx, "清除好友申请未读数量失败",
			logger.String("user_uuid", currentUserUUID),
			logger.ErrorField("error", err),
		)
	}

	return &pb.GetFriendApplyListResponse{
		Items: items,
		Pagination: &pb.PaginationInfo{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: int32((total + int64(pageSize) - 1) / int64(pageSize)),
		},
	}, nil
}

// GetSentApplyList 获取发出的申请列表
func (s *friendServiceImpl) GetSentApplyList(ctx context.Context, req *pb.GetSentApplyListRequest) (*pb.GetSentApplyListResponse, error) {
	// 从上下文获取当前用户（申请人）
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 兜底分页参数（即使网关做了默认值，这里也防御性处理）
	page := req.Page
	pageSize := req.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	// 查询发出的申请列表（status<0 表示全部状态）
	applies, total, err := s.applyRepo.GetSentList(ctx, currentUserUUID, int(req.Status), int(page), int(pageSize))
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取发出的申请列表失败")
	}

	if len(applies) == 0 {
		// 空列表直接返回，避免后续无意义的批量查询
		return &pb.GetSentApplyListResponse{
			Items: []*pb.SentApplyItem{},
			Pagination: &pb.PaginationInfo{
				Page:       page,
				PageSize:   pageSize,
				Total:      total,
				TotalPages: int32((total + int64(pageSize) - 1) / int64(pageSize)),
			},
		}, nil
	}

	// 组装返回项（申请记录 + 目标用户简要信息）
	items := make([]*pb.SentApplyItem, 0, len(applies))
	for _, apply := range applies {
		if apply == nil {
			continue
		}

		targetInfo := &pb.SimpleUserInfo{
			Uuid: apply.TargetUuid,
		}

		items = append(items, &pb.SentApplyItem{
			ApplyId:    apply.Id,
			TargetUuid: apply.TargetUuid,
			TargetInfo: targetInfo,
			Reason:     apply.Reason,
			Source:     apply.Source,
			Status:     int32(apply.Status),
			IsRead:     apply.IsRead,
			CreatedAt:  apply.CreatedAt.UnixMilli(),
		})
	}

	return &pb.GetSentApplyListResponse{
		Items: items,
		Pagination: &pb.PaginationInfo{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: int32((total + int64(pageSize) - 1) / int64(pageSize)),
		},
	}, nil
}

// HandleFriendApply 处理好友申请
// 业务流程：
//  1. 从context获取当前用户UUID
//  2. 根据applyId获取申请详情
//  3. 验证当前用户是否为申请的目标用户（有权限处理）
//  4. 同意：调用 AcceptApplyAndCreateRelation（事务 + CAS幂等）
//     拒绝：调用 UpdateStatus（CAS幂等）
func (s *friendServiceImpl) HandleFriendApply(ctx context.Context, req *pb.HandleFriendApplyRequest) error {
	// 1. 从context获取当前用户UUID（处理人）
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}

	// 2. 根据applyId获取申请详情
	apply, err := s.applyRepo.GetByID(ctx, req.ApplyId)
	if err != nil {
		return apperr.New(consts.CodeApplyNotFoundOrHandle)
	}
	if apply == nil {
		return apperr.New(consts.CodeApplyNotFoundOrHandle)
	}

	// 3. 验证当前用户是否有权限处理该申请
	if apply.TargetUuid != currentUserUUID {
		return apperr.New(consts.CodeNoPermission)
	}

	// 4. 处理申请
	if req.Action == 1 {
		// 同意：事务性更新申请状态 + 创建好友关系
		_, err := s.applyRepo.AcceptApplyAndCreateRelation(ctx, req.ApplyId, currentUserUUID, apply.ApplicantUuid, req.Remark)
		if err != nil {
			return apperr.Wrap(err, consts.CodeInternalError, "同意好友申请失败")
		}

	} else {
		// 拒绝：只更新申请状态
		err = s.applyRepo.UpdateStatus(ctx, req.ApplyId, int(req.Action), req.Remark)
		if err != nil {
			// ErrApplyNotFound 也是幂等成功
			if err == repository.ErrApplyNotFound {
				return nil
			}
			return apperr.Wrap(err, consts.CodeInternalError, "拒绝好友申请失败")
		}

	}

	return nil
}

// GetUnreadApplyCount 获取未读申请数量
func (s *friendServiceImpl) GetUnreadApplyCount(ctx context.Context, req *pb.GetUnreadApplyCountRequest) (*pb.GetUnreadApplyCountResponse, error) {
	// 1. 获取当前用户 UUID
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 2. 只读 Redis 未读数量（不命中直接返回 0）
	count, err := s.applyRepo.GetUnreadCount(ctx, currentUserUUID)
	if err != nil {
		logger.Warn(ctx, "获取好友申请未读数量失败，降级返回 0",
			logger.String("user_uuid", currentUserUUID),
			logger.ErrorField("error", err),
		)
		count = 0
	}

	return &pb.GetUnreadApplyCountResponse{
		UnreadCount: int32(count),
	}, nil
}

// MarkApplyAsRead 标记申请已读
func (s *friendServiceImpl) MarkApplyAsRead(ctx context.Context, req *pb.MarkApplyAsReadRequest) error {
	// 1. 获取当前用户 UUID
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}

	// 2. 标记已读（applyIds 为空则标记全部）
	if len(req.ApplyIds) == 0 {
		if _, err := s.applyRepo.MarkAllAsRead(ctx, currentUserUUID); err != nil {
			return apperr.Wrap(err, consts.CodeInternalError, "标记全部申请已读失败")
		}
	} else {
		if _, err := s.applyRepo.MarkAsRead(ctx, currentUserUUID, req.ApplyIds); err != nil {
			return apperr.Wrap(err, consts.CodeInternalError, "标记申请已读失败")
		}
	}

	// 3. 清除未读数量红点（尽力而为）
	if err := s.applyRepo.ClearUnreadCount(ctx, currentUserUUID); err != nil {
		logger.Warn(ctx, "清除好友申请未读数量失败",
			logger.String("user_uuid", currentUserUUID),
			logger.ErrorField("error", err),
		)
	}

	return nil
}

// GetFriendList 获取好友列表
func (s *friendServiceImpl) GetFriendList(ctx context.Context, req *pb.GetFriendListRequest) (*pb.GetFriendListResponse, error) {
	// 1. 从context中获取当前用户UUID
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 2. 兜底分页参数（即使网关做了默认值，这里也防御性处理）
	page := req.Page
	pageSize := req.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	// 3. 获取好友关系列表
	relations, total, version, err := s.friendRepo.GetFriendList(ctx, currentUserUUID, req.GroupTag, int(page), int(pageSize))
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取好友列表失败")
	}

	if len(relations) == 0 {
		return &pb.GetFriendListResponse{
			Items: []*pb.FriendItem{},
			Pagination: &pb.PaginationInfo{
				Page:       page,
				PageSize:   pageSize,
				Total:      total,
				TotalPages: int32((total + int64(pageSize) - 1) / int64(pageSize)),
			},
			Version: version,
		}, nil
	}

	// 4. 组装返回项（好友关系数据）
	items := make([]*pb.FriendItem, 0, len(relations))
	for _, relation := range relations {
		if relation == nil {
			continue
		}

		item := &pb.FriendItem{
			Uuid:      relation.PeerUuid,
			Remark:    relation.Remark,
			GroupTag:  relation.GroupTag,
			Source:    relation.Source,
			CreatedAt: relation.CreatedAt.UnixMilli(),
		}

		items = append(items, item)
	}

	return &pb.GetFriendListResponse{
		Items: items,
		Pagination: &pb.PaginationInfo{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: int32((total + int64(pageSize) - 1) / int64(pageSize)),
		},
		Version: version,
	}, nil
}

// SyncFriendList 好友增量同步
func (s *friendServiceImpl) SyncFriendList(ctx context.Context, req *pb.SyncFriendListRequest) (*pb.SyncFriendListResponse, error) {
	const syncVersionRollbackMs int64 = 2000 // 回退 2s，避免事务时间差漏数据

	// 1. 从context中获取当前用户UUID
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 2. 兜底同步参数
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	version := req.Version
	if version < 0 {
		version = 0
	}

	// 3. 查询增量变更（按时间升序）
	relations, serverTime, hasMore, err := s.friendRepo.SyncFriendList(ctx, currentUserUUID, version, limit)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "增量同步好友列表失败")
	}

	// 4. 无变更：直接返回（latestVersion 使用服务器时间回退一小段）
	if len(relations) == 0 {
		latestVersion := serverTime - syncVersionRollbackMs
		if latestVersion < 0 {
			latestVersion = 0
		}
		return &pb.SyncFriendListResponse{
			Changes:       []*pb.FriendChange{},
			HasMore:       false,
			LatestVersion: latestVersion,
		}, nil
	}

	// 5. 判断是否还有更多
	//if len(relations) > limit {
	//	hasMore = true
	//	relations = relations[:limit]
	//}

	// 6. 组装变更列表
	versionTime := time.UnixMilli(version)
	changes := make([]*pb.FriendChange, 0, len(relations))
	var lastChangedAt int64

	for _, relation := range relations {
		if relation == nil {
			continue
		}

		changeType := "update"
		changedAt := relation.UpdatedAt.UnixMilli()

		if relation.DeletedAt.Valid {
			changeType = "delete"
			changedAt = relation.DeletedAt.Time.UnixMilli()
		} else if relation.CreatedAt.After(versionTime) {
			changeType = "add"
		}

		change := &pb.FriendChange{
			Uuid:       relation.PeerUuid,
			Remark:     relation.Remark,
			GroupTag:   relation.GroupTag,
			Source:     relation.Source,
			ChangeType: changeType,
			ChangedAt:  changedAt,
		}

		changes = append(changes, change)
		lastChangedAt = changedAt
	}

	// 7. latestVersion 规则：
	// - hasMore=true：取本批次最后一条的 changedAt
	// - hasMore=false：取服务器当前时间并回退一小段
	var latestVersion int64
	if hasMore {
		latestVersion = lastChangedAt
	} else {
		latestVersion = serverTime - syncVersionRollbackMs
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

// DeleteFriend 删除好友
func (s *friendServiceImpl) DeleteFriend(ctx context.Context, req *pb.DeleteFriendRequest) error {
	// 1. 从context中获取当前用户UUID
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}

	// 2. 参数校验
	if req == nil || req.UserUuid == "" {
		return apperr.New(consts.CodeParamError)
	}

	// 3. 删除好友关系（单向）
	if err := s.friendRepo.DeleteFriendRelation(ctx, currentUserUUID, req.UserUuid); err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return apperr.New(consts.CodeNotFriend)
		}
		return apperr.Wrap(err, consts.CodeInternalError, "删除好友关系失败")
	}

	return nil
}

// SetFriendRemark 设置好友备注
func (s *friendServiceImpl) SetFriendRemark(ctx context.Context, req *pb.SetFriendRemarkRequest) error {
	// 1. 从context中获取当前用户UUID
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}

	// 2. 参数校验
	if req == nil || req.UserUuid == "" {
		return apperr.New(consts.CodeParamError)
	}

	// 3. 设置好友备注
	if err := s.friendRepo.SetFriendRemark(ctx, currentUserUUID, req.UserUuid, req.Remark); err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return apperr.New(consts.CodeNotFriend)
		}
		return apperr.Wrap(err, consts.CodeInternalError, "设置好友备注失败")
	}

	return nil
}

// SetFriendTag 设置好友标签
func (s *friendServiceImpl) SetFriendTag(ctx context.Context, req *pb.SetFriendTagRequest) error {
	// 1. 从context中获取当前用户UUID
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}

	// 2. 参数校验
	if req == nil || req.UserUuid == "" {
		return apperr.New(consts.CodeParamError)
	}

	// 3. 设置好友标签
	if err := s.friendRepo.SetFriendTag(ctx, currentUserUUID, req.UserUuid, req.GroupTag); err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return apperr.New(consts.CodeNotFriend)
		}
		return apperr.Wrap(err, consts.CodeInternalError, "设置好友标签失败")
	}

	return nil
}

// GetTagList 获取标签列表
func (s *friendServiceImpl) GetTagList(ctx context.Context, req *pb.GetTagListRequest) (*pb.GetTagListResponse, error) {
	return nil, apperr.NewWithMessage(consts.CodeMethodNotAllowed, "获取标签列表功能暂未实现")
}

// CheckIsFriend 判断是否好友
func (s *friendServiceImpl) CheckIsFriend(ctx context.Context, req *pb.CheckIsFriendRequest) (*pb.CheckIsFriendResponse, error) {
	isFriend, err := s.friendRepo.CheckIsFriendRelation(ctx, req.UserUuid, req.PeerUuid)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "判断是否好友失败")
	}
	return &pb.CheckIsFriendResponse{
		IsFriend: isFriend,
	}, nil
}

// BatchCheckIsFriend 批量判断是否好友
func (s *friendServiceImpl) BatchCheckIsFriend(ctx context.Context, req *pb.BatchCheckIsFriendRequest) (*pb.BatchCheckIsFriendResponse, error) {
	if req == nil || len(req.PeerUuids) == 0 {
		return &pb.BatchCheckIsFriendResponse{
			Items: []*pb.FriendCheckItem{},
		}, nil
	}

	result, err := s.friendRepo.BatchCheckIsFriend(ctx, req.UserUuid, req.PeerUuids)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "批量判断是否好友失败")
	}

	items := make([]*pb.FriendCheckItem, 0, len(req.PeerUuids))
	for _, peerUUID := range req.PeerUuids {
		if peerUUID == "" {
			continue
		}
		items = append(items, &pb.FriendCheckItem{
			PeerUuid: peerUUID,
			IsFriend: result[peerUUID],
		})
	}

	return &pb.BatchCheckIsFriendResponse{
		Items: items,
	}, nil
}

// GetRelationStatus 获取关系状态
func (s *friendServiceImpl) GetRelationStatus(ctx context.Context, req *pb.GetRelationStatusRequest) (*pb.GetRelationStatusResponse, error) {
	if req == nil || req.UserUuid == "" || req.PeerUuid == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	relation, err := s.friendRepo.GetRelationStatus(ctx, req.UserUuid, req.PeerUuid)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取关系状态失败")
	}

	resp := &pb.GetRelationStatusResponse{
		Relation:    "none",
		IsFriend:    false,
		IsBlacklist: false,
		Remark:      "",
		GroupTag:    "",
	}

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
