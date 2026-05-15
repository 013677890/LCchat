package service

import (
	"context"
	pb "github.com/013677890/LCchat-Backend/apps/group/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
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
	return mapGroupWriteError(
		s.groupRepo.UpdateGroupNotice(ctx, groupUUID, currentUserUUID, notice),
		"更新群公告失败",
	)
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
	return mapGroupWriteError(
		s.groupRepo.ReviewJoinGroup(ctx, groupUUID, currentUserUUID, req.GetApplyId(), action, remark),
		"审批入群申请失败",
	)
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

// normalizeJoinRequestPage 归一化入群申请分页参数。
func normalizeJoinRequestPage(page, pageSize int32) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return int(page), int(pageSize)
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
