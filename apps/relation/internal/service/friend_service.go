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
//
// 错误码映射：
//   - codes.Unauthenticated: 未登录
//   - codes.InvalidArgument: 参数错误或给自己发申请
//   - codes.AlreadyExists: 已是好友或已有待处理申请
//   - codes.PermissionDenied: 任一方向存在拉黑关系
//   - codes.Internal: 系统内部错误
func (s *friendServiceImpl) SendFriendApply(ctx context.Context, req *pb.SendFriendApplyRequest) (*pb.SendFriendApplyResponse, error) {
	// 先取当前登录用户；好友关系写操作不允许匿名调用。
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}
	// target_uuid 为空时无法定位申请目标，直接按参数错误拦截。
	if req == nil || req.TargetUuid == "" {
		return nil, apperr.New(consts.CodeParamError)
	}
	// 给自己发好友申请没有业务意义，单独返回明确错误码。
	if currentUserUUID == req.TargetUuid {
		return nil, apperr.New(consts.CodeCannotAddSelf)
	}

	// 先判断当前是否已经是好友；命中后无需再做后续申请和黑名单校验。
	isFriend, err := s.friendRepo.IsFriend(ctx, currentUserUUID, req.TargetUuid)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "检查是否好友失败")
	}
	if isFriend {
		return nil, apperr.New(consts.CodeAlreadyFriend)
	}

	// 再判断是否已有待处理申请，避免重复插入语义相同的申请记录。
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

	// 只有全部前置校验通过后，才构造 relation 域申请模型准备落库。
	apply := &model.ApplyRequest{
		ApplyType:     0,
		ApplicantUuid: currentUserUUID,
		TargetUuid:    req.TargetUuid,
		Status:        0,
		IsRead:        false,
		Reason:        req.Reason,
		Source:        req.Source,
	}

	// 真正写库放在最后一步，保证数据库里不会留下无效申请事实。
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
//
// 错误码映射：
//   - codes.Unauthenticated: 未登录
//   - codes.Internal: 系统内部错误
func (s *friendServiceImpl) GetFriendApplyList(ctx context.Context, req *pb.GetFriendApplyListRequest) (*pb.GetFriendApplyListResponse, error) {
	// 收件箱属于当前用户私有视图，必须先确认登录身份。
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 先准备与旧接口一致的默认分页参数，避免空请求下行为漂移。
	page := int32(1)
	pageSize := int32(20)
	status := int32(-1)
	if req != nil {
		// status/page/pageSize 只有显式传值时才覆盖默认值。
		status = req.Status
		if req.Page > 0 {
			page = req.Page
		}
		if req.PageSize > 0 {
			pageSize = req.PageSize
		}
	}

	// 统一拉取当前页申请和 total，后续空列表分支与分页响应都依赖该结果。
	applies, total, err := s.applyRepo.GetPendingList(ctx, currentUserUUID, int(status), int(page), int(pageSize))
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取好友申请列表失败")
	}

	if len(applies) == 0 {
		// 列表为空时仍尽力清空未读红点，保证列表页与角标状态一致。
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

	// 预分配切片，减少遍历时的扩容开销。
	items := make([]*pb.FriendApplyItem, 0, len(applies))
	unreadIDs := make([]int64, 0, len(applies))
	for _, apply := range applies {
		// 防御仓储层返回空元素，避免后续字段访问 panic。
		if apply == nil {
			continue
		}
		// 只收集未读申请 id，供后面的异步已读更新使用。
		if !apply.IsRead {
			unreadIDs = append(unreadIDs, apply.Id)
		}

		// 单条 proto 组装失败时跳过该项，不影响整页其他记录返回。
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
//
// 错误码映射：
//   - codes.Unauthenticated: 未登录
//   - codes.Internal: 系统内部错误
func (s *friendServiceImpl) GetSentApplyList(ctx context.Context, req *pb.GetSentApplyListRequest) (*pb.GetSentApplyListResponse, error) {
	// 发件箱同样是当前用户私有视图，没有登录态时直接拒绝访问。
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 默认分页参数保持与收件箱一致，便于上游统一处理。
	page := int32(1)
	pageSize := int32(20)
	status := int32(-1)
	if req != nil {
		// 请求里显式传入的分页参数才覆盖默认值。
		status = req.Status
		if req.Page > 0 {
			page = req.Page
		}
		if req.PageSize > 0 {
			pageSize = req.PageSize
		}
	}

	// 发件箱只查当前用户发出的申请记录，不额外补资料域数据。
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
		// 单条坏数据只跳过自身，避免整页失败。
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
//
// 错误码映射：
//   - codes.Unauthenticated: 未登录
//   - codes.InvalidArgument: 参数错误
//   - codes.PermissionDenied: 当前用户无权处理该申请
//   - codes.NotFound: 申请不存在或已处理
//   - codes.Internal: 系统内部错误
func (s *friendServiceImpl) HandleFriendApply(ctx context.Context, req *pb.HandleFriendApplyRequest) error {
	// 处理动作只能由已登录用户发起，匿名请求不允许修改申请状态。
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	// apply_id 非法时无法定位目标申请，直接按参数错误返回。
	if req == nil || req.ApplyId <= 0 {
		return apperr.New(consts.CodeParamError)
	}

	// 先取申请快照；后续权限判断和 accept/reject 分支都依赖这条记录。
	apply, err := s.applyRepo.GetByID(ctx, req.ApplyId)
	if err != nil || apply == nil {
		return apperr.New(consts.CodeApplyNotFoundOrHandle)
	}
	// 只有申请接收方本人才能处理该申请。
	if apply.TargetUuid != currentUserUUID {
		return apperr.New(consts.CodeNoPermission)
	}

	if req.Action == 1 {
		// 同意时走事务路径：修改申请状态并创建双向好友关系。
		_, err := s.applyRepo.AcceptApplyAndCreateRelation(ctx, req.ApplyId, currentUserUUID, apply.ApplicantUuid, req.Remark)
		if err != nil {
			return apperr.Wrap(err, consts.CodeInternalError, "同意好友申请失败")
		}
		return nil
	}

	// 非同意动作统一交给仓储层做状态写入。
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
//
// 错误码映射：
//   - codes.Unauthenticated: 未登录
//   - codes.Internal: 仅在无法降级时返回系统错误
func (s *friendServiceImpl) GetUnreadApplyCount(ctx context.Context, req *pb.GetUnreadApplyCountRequest) (*pb.GetUnreadApplyCountResponse, error) {
	// 红点数量属于当前用户私有数据，没有登录态时不返回任何关系信息。
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 当前实现以数据库结果为准；查失败时降级为 0，避免阻塞页面主流程。
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
//
// 错误码映射：
//   - codes.Unauthenticated: 未登录
//   - codes.Internal: 系统内部错误
func (s *friendServiceImpl) MarkApplyAsRead(ctx context.Context, req *pb.MarkApplyAsReadRequest) error {
	// 已读状态修改只允许作用在当前登录用户自己的申请收件箱上。
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}

	if req == nil || len(req.ApplyIds) == 0 {
		// 空 id 列表的语义是“全部已读”，走全量更新路径。
		if _, err := s.applyRepo.MarkAllAsRead(ctx, currentUserUUID); err != nil {
			return apperr.Wrap(err, consts.CodeInternalError, "标记全部申请已读失败")
		}
	} else {
		// 指定了 apply_ids 时，仅更新调用方传入的那些申请记录。
		if _, err := s.applyRepo.MarkAsRead(ctx, currentUserUUID, req.ApplyIds); err != nil {
			return apperr.Wrap(err, consts.CodeInternalError, "标记申请已读失败")
		}
	}

	// 主状态修改成功后，再尽力清理未读计数；失败仅记降级日志。
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
//
// 错误码映射：
//   - codes.Unauthenticated: 未登录
//   - codes.Internal: 系统内部错误
func (s *friendServiceImpl) GetFriendList(ctx context.Context, req *pb.GetFriendListRequest) (*pb.GetFriendListResponse, error) {
	// 好友列表必须以当前登录用户视角查询，不能用匿名身份读取。
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 默认分页和分组参数保持与旧接口一致，避免行为漂移。
	page := int32(1)
	pageSize := int32(20)
	groupTag := ""
	if req != nil {
		// groupTag 只过滤当前用户自己维护的好友标签分组。
		groupTag = req.GroupTag
		if req.Page > 0 {
			page = req.Page
		}
		if req.PageSize > 0 {
			pageSize = req.PageSize
		}
	}

	// 仓储层返回关系记录、总数和版本锚点；service 层只做响应重组。
	relations, total, version, err := s.friendRepo.GetFriendList(ctx, currentUserUUID, groupTag, int(page), int(pageSize))
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取好友列表失败")
	}

	if len(relations) == 0 {
		// 当前页没有好友时仍返回分页信息和版本锚点，方便客户端保持本地游标一致。
		return &pb.GetFriendListResponse{
			Items:      []*pb.FriendItem{},
			Pagination: buildPagination(page, pageSize, total),
			Version:    version,
		}, nil
	}

	// 预分配好友项切片，避免列表较大时频繁扩容。
	items := make([]*pb.FriendItem, 0, len(relations))
	for _, relation := range relations {
		// 单条关系 proto 组装失败时仅跳过该条，避免整页列表失败。
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
//
// 错误码映射：
//   - codes.Unauthenticated: 未登录
//   - codes.Internal: 系统内部错误
func (s *friendServiceImpl) SyncFriendList(ctx context.Context, req *pb.SyncFriendListRequest) (*pb.SyncFriendListResponse, error) {
	const syncVersionRollbackMs int64 = 2000

	// 增量同步结果是用户私有数据，必须绑定当前登录用户身份。
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 使用保守默认值和上限，避免单次请求拉取过多变更压垮仓储层。
	limit := 100
	version := int64(0)
	if req != nil {
		// 只有显式传入正值时才覆盖默认 limit，避免 0 值破坏既有分页语义。
		if req.Limit > 0 {
			limit = int(req.Limit)
		}
		// version=0 表示全量起点；只有正数版本才参与增量过滤。
		if req.Version > 0 {
			version = req.Version
		}
	}
	// 对单次同步上限做保护，避免客户端把 limit 拉得过大压垮仓储层查询。
	if limit > 500 {
		limit = 500
	}

	// 仓储层返回本批关系变更、数据库最新版本以及是否还有更多分页数据。
	relations, latestDBVersion, hasMore, err := s.friendRepo.SyncFriendList(ctx, currentUserUUID, version, limit)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "增量同步好友列表失败")
	}

	if len(relations) == 0 {
		// 没有增量时仍回退一个小时间窗，降低边界时间戳漏数据风险。
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
		// 防御空元素，避免单条脏数据影响整批同步。
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
//
// 错误码映射：
//   - codes.Unauthenticated: 未登录
//   - codes.InvalidArgument: 参数错误
//   - codes.NotFound: 当前不是好友
//   - codes.Internal: 系统内部错误
func (s *friendServiceImpl) DeleteFriend(ctx context.Context, req *pb.DeleteFriendRequest) error {
	// 删除动作只允许当前登录用户操作自己视角下的好友关系。
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	// peer uuid 为空时无法定位要删除的好友记录。
	if req == nil || req.UserUuid == "" {
		return apperr.New(consts.CodeParamError)
	}

	// 仓储层按“当前用户 -> 对端用户”的单向关系删除；不存在时映射为不是好友。
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
//
// 错误码映射：
//   - codes.Unauthenticated: 未登录
//   - codes.InvalidArgument: 参数错误
//   - codes.NotFound: 当前不是好友
//   - codes.Internal: 系统内部错误
func (s *friendServiceImpl) SetFriendRemark(ctx context.Context, req *pb.SetFriendRemarkRequest) error {
	// 备注属于用户私有展示能力，必须依附当前登录身份。
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	// 对端用户为空时，无法知道要给谁设置备注。
	if req == nil || req.UserUuid == "" {
		return apperr.New(consts.CodeParamError)
	}

	// 备注只更新当前用户这一侧的 relation 记录；不存在时说明当前不是好友。
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
//
// 错误码映射：
//   - codes.Unauthenticated: 未登录
//   - codes.InvalidArgument: 参数错误
//   - codes.NotFound: 当前不是好友
//   - codes.Internal: 系统内部错误
func (s *friendServiceImpl) SetFriendTag(ctx context.Context, req *pb.SetFriendTagRequest) error {
	// 标签归类属于当前用户自己的好友管理视图，必须先确认登录态。
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	// 对端用户为空时，无法定位需要打标签的好友关系。
	if req == nil || req.UserUuid == "" {
		return apperr.New(consts.CodeParamError)
	}

	// groupTag 可以为空，表示清空标签；但 relation 不存在时仍视为不是好友。
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
//
// 错误码映射：
//   - codes.MethodNotAllowed: 功能暂未实现
func (s *friendServiceImpl) GetTagList(ctx context.Context, req *pb.GetTagListRequest) (*pb.GetTagListResponse, error) {
	// 这里显式拒绝而不是返回空列表，避免调用方误判为“当前没有标签”。
	return nil, apperr.NewWithMessage(consts.CodeMethodNotAllowed, "获取标签列表功能暂未实现")
}

// CheckIsFriend 判断两个用户之间是否存在好友关系。
//
// 该接口主要面向其他服务或网关的关系判定，不依赖当前登录上下文。
//
// 错误码映射：
//   - codes.InvalidArgument: 参数错误
//   - codes.Internal: 系统内部错误
func (s *friendServiceImpl) CheckIsFriend(ctx context.Context, req *pb.CheckIsFriendRequest) (*pb.CheckIsFriendResponse, error) {
	// 该接口是内部关系查询接口，因此直接要求 user_uuid / peer_uuid 同时齐全。
	if req == nil || req.UserUuid == "" || req.PeerUuid == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	// 仓储层返回关系事实；service 层只负责错误映射与 proto 封装。
	isFriend, err := s.friendRepo.CheckIsFriendRelation(ctx, req.UserUuid, req.PeerUuid)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "判断是否好友失败")
	}
	return &pb.CheckIsFriendResponse{IsFriend: isFriend}, nil
}

// BatchCheckIsFriend 批量判断好友关系。
//
// 返回结果按请求中的 peer_uuids 顺序重组，方便调用方直接和原始请求一一对应。
//
// 错误码映射：
//   - codes.InvalidArgument: 参数错误
//   - codes.Internal: 系统内部错误
func (s *friendServiceImpl) BatchCheckIsFriend(ctx context.Context, req *pb.BatchCheckIsFriendRequest) (*pb.BatchCheckIsFriendResponse, error) {
	// 空请求按空结果返回，方便调用方直接透传，不需要额外判空。
	if req == nil || req.UserUuid == "" || len(req.PeerUuids) == 0 {
		return &pb.BatchCheckIsFriendResponse{Items: []*pb.FriendCheckItem{}}, nil
	}

	// 先批量拿 map 结果，再按请求顺序重组，保证响应顺序稳定。
	result, err := s.friendRepo.BatchCheckIsFriend(ctx, req.UserUuid, req.PeerUuids)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "批量判断是否好友失败")
	}

	items := make([]*pb.FriendCheckItem, 0, len(req.PeerUuids))
	for _, peerUUID := range req.PeerUuids {
		// 即使底层 map 没有该 key，也按 false 回填一项，保持与请求一一对应。
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
//
// 错误码映射：
//   - codes.InvalidArgument: 参数错误
//   - codes.Internal: 系统内部错误
func (s *friendServiceImpl) GetRelationStatus(ctx context.Context, req *pb.GetRelationStatusRequest) (*pb.GetRelationStatusResponse, error) {
	// 该接口是显式双边查询，因此 user_uuid 和 peer_uuid 都必须提供。
	if req == nil || req.UserUuid == "" || req.PeerUuid == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	// 仓储层统一负责从 relation 表推导当前视角下的关系快照。
	relation, err := s.friendRepo.GetRelationStatus(ctx, req.UserUuid, req.PeerUuid)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取关系状态失败")
	}

	// 先构造 none 响应；只有命中关系记录时才覆盖具体状态字段。
	resp := buildRelationStatusResponse("none", false, false, "", "")
	if relation == nil {
		return resp, nil
	}

	// 已删除关系优先返回 deleted，避免被旧 remark/tag 等展示字段干扰。
	if relation.DeletedAt.Valid || relation.Status == 2 {
		resp.Relation = "deleted"
		return resp, nil
	}

	switch relation.Status {
	case 0:
		// status=0 表示当前是好友，同时返回当前用户侧维护的备注与标签。
		resp.Relation = "friend"
		resp.IsFriend = true
		resp.Remark = relation.Remark
		resp.GroupTag = relation.GroupTag
	case 1, 3:
		// status=1/3 统一视为黑名单态；更细的单向语义留给 relation 数据本身表达。
		resp.Relation = "blacklist"
		resp.IsBlacklist = true
	default:
		// 其余未知状态保守回退为 none，避免把脏数据升级成更强语义。
		resp.Relation = "none"
	}

	return resp, nil
}
