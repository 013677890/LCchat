package service

import (
	"context"
	pb "github.com/013677890/LCchat-Backend/apps/group/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/realtimepush"
	"github.com/013677890/LCchat-Backend/pkg/util"
	"strings"
)

const (
	joinRequestActionApprove int8 = 1
	joinRequestActionReject  int8 = 2
)

// UpdateGroupNotice 独立更新群公告。
func (s *groupServiceImpl) UpdateGroupNotice(ctx context.Context, req *pb.UpdateGroupNoticeRequest) error {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	if req == nil {
		return apperr.New(consts.CodeParamError)
	}
	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	if groupUUID == "" {
		return apperr.New(consts.CodeParamError)
	}
	notice := strings.TrimSpace(req.GetNotice())
	if len([]rune(notice)) > 500 {
		return apperr.New(consts.CodeGroupNoticeTooLong)
	}
	if err := mapGroupWriteError(
		s.groupRepo.UpdateGroupNotice(ctx, groupUUID, currentUserUUID, notice),
		"更新群公告失败",
	); err != nil {
		return err
	}
	s.publishGroupStateChangedToMembers(ctx, groupUUID, []string{realtimepush.GroupChangedNotice})
	return nil
}

// ApplyJoinGroup 根据 add_mode 执行直加入群或创建申请。
func (s *groupServiceImpl) ApplyJoinGroup(ctx context.Context, req *pb.ApplyJoinGroupRequest) (*pb.ApplyJoinGroupResponse, error) {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}
	if req == nil {
		return nil, apperr.New(consts.CodeParamError)
	}
	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	if groupUUID == "" {
		return nil, apperr.New(consts.CodeParamError)
	}
	reason := strings.TrimSpace(req.GetReason())
	if len([]rune(reason)) > 255 {
		return nil, apperr.New(consts.CodeReasonTooLong)
	}
	result, err := s.groupRepo.ApplyJoinGroup(ctx, groupUUID, currentUserUUID, reason)
	if err != nil {
		return nil, mapGroupWriteError(err, "申请加入群聊失败")
	}
	if result.ApplyID > 0 && !result.JoinedDirectly {
		s.publishGroupJoinRequestCreated(ctx, groupUUID, currentUserUUID, result.ApplyID)
	}
	if result.JoinedDirectly {
		s.publishGroupStateChangedToMembers(ctx, groupUUID, []string{realtimepush.GroupChangedMembers})
	}
	return &pb.ApplyJoinGroupResponse{ApplyId: result.ApplyID, JoinedDirectly: result.JoinedDirectly}, nil
}

// CancelJoinGroupApplication 撤销当前用户自己的待审批入群申请。
func (s *groupServiceImpl) CancelJoinGroupApplication(ctx context.Context, req *pb.CancelJoinGroupApplicationRequest) error {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	if req == nil {
		return apperr.New(consts.CodeParamError)
	}
	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	if groupUUID == "" {
		return apperr.New(consts.CodeParamError)
	}
	return mapGroupWriteError(
		s.groupRepo.CancelJoinGroupApplication(ctx, groupUUID, currentUserUUID),
		"撤销入群申请失败",
	)
}

// GetMyJoinGroupApplication 获取当前用户在指定群的最新申请状态。
func (s *groupServiceImpl) GetMyJoinGroupApplication(ctx context.Context, req *pb.GetMyJoinGroupApplicationRequest) (*pb.GetMyJoinGroupApplicationResponse, error) {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}
	if req == nil {
		return nil, apperr.New(consts.CodeParamError)
	}
	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	if groupUUID == "" {
		return nil, apperr.New(consts.CodeParamError)
	}
	joinRequest, err := s.groupRepo.GetMyJoinGroupApplication(ctx, groupUUID, currentUserUUID)
	if err != nil {
		return nil, mapGroupWriteError(err, "获取我的入群申请状态失败")
	}
	if joinRequest == nil {
		return &pb.GetMyJoinGroupApplicationResponse{HasApplication: false}, nil
	}
	return &pb.GetMyJoinGroupApplicationResponse{
		HasApplication: true,
		Application:    buildMyJoinGroupApplicationProto(joinRequest),
	}, nil
}

// ListMyJoinGroupApplications 获取当前用户发起的入群申请列表。
func (s *groupServiceImpl) ListMyJoinGroupApplications(ctx context.Context, req *pb.ListMyJoinGroupApplicationsRequest) (*pb.ListMyJoinGroupApplicationsResponse, error) {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}
	if req == nil {
		return nil, apperr.New(consts.CodeParamError)
	}
	page, pageSize := normalizeJoinRequestPage(req.GetPage(), req.GetPageSize())
	status, err := normalizeOptionalJoinRequestStatus(req.Status)
	if err != nil {
		return nil, err
	}
	items, total, err := s.groupRepo.ListMyJoinGroupApplications(ctx, currentUserUUID, status, page, pageSize)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取我的入群申请列表失败")
	}

	// 用户侧列表需要直接展示群名称和头像，因此在 service 层一次性补齐群资料，
	// 避免 gateway 为了渲染列表再额外回查一轮群信息。
	groupUUIDs := collectJoinRequestGroupUUIDs(items)
	groups, err := s.groupRepo.GetGroupsByUUIDs(ctx, groupUUIDs)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询申请关联群资料失败")
	}
	respItems := make([]*pb.MyJoinGroupApplicationListItem, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		respItems = append(respItems, buildMyJoinGroupApplicationListItemProto(item, groups[item.GroupUuid]))
	}

	return &pb.ListMyJoinGroupApplicationsResponse{Items: respItems, Total: total, Page: int32(page), PageSize: int32(pageSize)}, nil
}

// ReviewJoinGroup 审批入群申请。
func (s *groupServiceImpl) ReviewJoinGroup(ctx context.Context, req *pb.ReviewJoinGroupRequest) error {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	if req == nil {
		return apperr.New(consts.CodeParamError)
	}
	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	if groupUUID == "" || req.GetApplyId() <= 0 {
		return apperr.New(consts.CodeParamError)
	}
	action := int8(req.GetAction())
	if action != joinRequestActionApprove && action != joinRequestActionReject {
		return apperr.New(consts.CodeParamError)
	}
	remark := strings.TrimSpace(req.GetRemark())
	if len([]rune(remark)) > 255 {
		return apperr.New(consts.CodeRemarkTooLong)
	}
	applicantUUID := s.loadJoinRequestApplicantForRealtime(ctx, groupUUID, req.GetApplyId())
	if err := mapGroupWriteError(
		s.groupRepo.ReviewJoinGroup(ctx, groupUUID, currentUserUUID, req.GetApplyId(), action, remark),
		"审批入群申请失败",
	); err != nil {
		return err
	}

	// 审批后通知申请人刷新状态；同意时再通知群成员刷新成员列表。
	if applicantUUID != "" {
		s.publishGroupJoinRequestReviewed(ctx, groupUUID, applicantUUID, currentUserUUID, req.GetApplyId(), action)
	}
	if action == joinRequestActionApprove {
		s.publishGroupStateChangedToMembers(ctx, groupUUID, []string{realtimepush.GroupChangedMembers})
	}
	return nil
}

// ListJoinRequests 获取群待审批申请列表。
func (s *groupServiceImpl) ListJoinRequests(ctx context.Context, req *pb.ListJoinRequestsRequest) (*pb.ListJoinRequestsResponse, error) {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}
	if req == nil {
		return nil, apperr.New(consts.CodeParamError)
	}
	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	if groupUUID == "" {
		return nil, apperr.New(consts.CodeParamError)
	}
	page, pageSize := normalizeJoinRequestPage(req.GetPage(), req.GetPageSize())
	items, total, err := s.groupRepo.ListJoinRequests(ctx, groupUUID, currentUserUUID, page, pageSize)
	if err != nil {
		return nil, mapGroupWriteError(err, "获取入群申请列表失败")
	}

	// 审批侧列表按“申请事实 + 申请人资料”输出，先批量补齐资料可以避免逐条查询造成 N+1。
	userUUIDs := collectJoinRequestApplicantUUIDs(items)
	profiles, err := s.groupRepo.GetUserProfiles(ctx, userUUIDs)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询申请人资料失败")
	}
	respItems := make([]*pb.GroupJoinRequestItem, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		respItems = append(respItems, buildGroupJoinRequestItemProto(item, profiles[item.ApplicantUuid]))
	}

	return &pb.ListJoinRequestsResponse{Items: respItems, Total: total, Page: int32(page), PageSize: int32(pageSize)}, nil
}

// ListReviewedJoinRequests 获取群已审批申请列表。
func (s *groupServiceImpl) ListReviewedJoinRequests(ctx context.Context, req *pb.ListReviewedJoinRequestsRequest) (*pb.ListReviewedJoinRequestsResponse, error) {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}
	if req == nil {
		return nil, apperr.New(consts.CodeParamError)
	}
	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	if groupUUID == "" {
		return nil, apperr.New(consts.CodeParamError)
	}
	page, pageSize := normalizeJoinRequestPage(req.GetPage(), req.GetPageSize())
	status, err := normalizeOptionalReviewedJoinRequestStatus(req.Status)
	if err != nil {
		return nil, err
	}
	items, total, err := s.groupRepo.ListReviewedJoinRequests(ctx, groupUUID, currentUserUUID, status, page, pageSize)
	if err != nil {
		return nil, mapGroupWriteError(err, "获取审批记录列表失败")
	}

	// 审批记录列表同样需要回显申请人资料，这里复用批量聚合策略，避免管理端列表退化成多次回源。
	userUUIDs := collectJoinRequestApplicantUUIDs(items)
	profiles, err := s.groupRepo.GetUserProfiles(ctx, userUUIDs)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询审批记录申请人资料失败")
	}
	respItems := make([]*pb.ReviewedJoinRequestItem, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		respItems = append(respItems, buildReviewedJoinRequestItemProto(item, profiles[item.ApplicantUuid]))
	}

	return &pb.ListReviewedJoinRequestsResponse{Items: respItems, Total: total, Page: int32(page), PageSize: int32(pageSize)}, nil
}

// normalizeJoinRequestPage 归一化入群申请分页参数。
func normalizeJoinRequestPage(page, pageSize int32) (int, int) {
	if page <= 0 {
		// 列表接口统一回退到第一页，避免把无效页码继续透传到仓储层。
		page = 1
	}
	if pageSize <= 0 {
		// 默认页大小取 20，兼顾管理列表可读性和单次查询成本。
		pageSize = 20
	}
	if pageSize > 100 {
		// 上限收敛到 100，避免一次拉太多审批记录压垮数据库和网络负载。
		pageSize = 100
	}
	return int(page), int(pageSize)
}

// normalizeOptionalJoinRequestStatus 校验“我的申请列表”的可选状态筛选。
//
// nil 表示不筛选，传值时允许覆盖申请流的四种终态：
// 0=待审批、1=已通过、2=已拒绝、3=已撤销。
func normalizeOptionalJoinRequestStatus(status *int32) (*int8, error) {
	if status == nil {
		return nil, nil
	}
	if *status < 0 || *status > 3 {
		return nil, apperr.New(consts.CodeParamError)
	}
	value := int8(*status)
	return &value, nil
}

// normalizeOptionalReviewedJoinRequestStatus 校验审批记录列表的可选状态筛选。
//
// 审批记录只包含管理员处理过的已通过/已拒绝，撤销属于申请人动作，不进入审批历史。
func normalizeOptionalReviewedJoinRequestStatus(status *int32) (*int8, error) {
	if status == nil {
		return nil, nil
	}
	if *status != int32(joinRequestActionApprove) && *status != int32(joinRequestActionReject) {
		return nil, apperr.New(consts.CodeParamError)
	}
	value := int8(*status)
	return &value, nil
}

// collectJoinRequestApplicantUUIDs 提取申请人 UUID，供批量补齐资料。
func collectJoinRequestApplicantUUIDs(items []*model.GroupJoinRequest) []string {
	if len(items) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item == nil || item.ApplicantUuid == "" {
			continue
		}
		if _, exists := seen[item.ApplicantUuid]; exists {
			continue
		}
		seen[item.ApplicantUuid] = struct{}{}
		result = append(result, item.ApplicantUuid)
	}
	return result
}

// collectJoinRequestGroupUUIDs 提取申请关联的群 UUID，供批量补齐群展示资料。
func collectJoinRequestGroupUUIDs(items []*model.GroupJoinRequest) []string {
	if len(items) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item == nil || item.GroupUuid == "" {
			continue
		}
		if _, exists := seen[item.GroupUuid]; exists {
			continue
		}
		seen[item.GroupUuid] = struct{}{}
		result = append(result, item.GroupUuid)
	}
	return result
}
