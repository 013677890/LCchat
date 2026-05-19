package service

import (
	"context"
	pb "github.com/013677890/LCchat-Backend/apps/group/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/realtimepush"
	"github.com/013677890/LCchat-Backend/pkg/util"
	"strings"
	"time"
)

// LeaveGroup 处理当前登录用户主动退群。
//
// 退群必须从认证上下文读取操作者身份，避免客户端伪造 user_uuid；
// 群主不能直接退群等规则由 repository 在事务锁内统一裁决。
func (s *groupServiceImpl) LeaveGroup(ctx context.Context, req *pb.LeaveGroupRequest) error {
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
	if err := mapGroupWriteError(
		s.groupRepo.LeaveGroup(ctx, groupUUID, currentUserUUID),
		"退出群聊失败",
	); err != nil {
		return err
	}
	s.publishGroupMemberRemoved(ctx, groupUUID, currentUserUUID, currentUserUUID)
	s.publishGroupStateChangedToMembers(ctx, groupUUID, []string{realtimepush.GroupChangedMembers})
	return nil
}

// SearchGroupMembers 搜索群成员并返回分页读模型。
//
// service 层只负责登录态、分页和展示资料聚合；是否允许搜索由 repository
// 基于群状态和成员身份判断，避免非成员枚举群成员。
func (s *groupServiceImpl) SearchGroupMembers(ctx context.Context, req *pb.SearchGroupMembersRequest) (*pb.SearchGroupMembersResponse, error) {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}
	if req == nil {
		return nil, apperr.New(consts.CodeParamError)
	}
	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	keyword := strings.TrimSpace(req.GetKeyword())
	if groupUUID == "" || len([]rune(keyword)) > 64 {
		return nil, apperr.New(consts.CodeParamError)
	}
	page, pageSize := normalizeJoinRequestPage(req.GetPage(), req.GetPageSize())
	members, total, err := s.groupRepo.SearchGroupMembers(ctx, groupUUID, currentUserUUID, keyword, page, pageSize)
	if err != nil {
		return nil, mapGroupWriteError(err, "搜索群成员失败")
	}
	profiles, err := s.groupRepo.GetUserProfiles(ctx, collectMemberUUIDs(members))
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询群成员资料失败")
	}
	items := make([]*pb.GroupMemberItem, 0, len(members))
	for _, member := range members {
		if member == nil {
			continue
		}
		if item := buildGroupMemberItemProto(member, profiles[member.UserUuid]); item != nil {
			items = append(items, item)
		}
	}
	return &pb.SearchGroupMembersResponse{Members: items, Total: total, Page: int32(page), PageSize: int32(pageSize)}, nil
}

// SearchGroups 按群名或完整群号搜索正常群。
//
// 该接口服务于用户主动找群场景，仍要求登录态，避免匿名请求批量枚举群信息；
// 空 keyword 返回空分页结果，不把搜索接口降级成全量群列表。
func (s *groupServiceImpl) SearchGroups(ctx context.Context, req *pb.SearchGroupsRequest) (*pb.SearchGroupsResponse, error) {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}
	if req == nil {
		return nil, apperr.New(consts.CodeParamError)
	}
	keyword := strings.TrimSpace(req.GetKeyword())
	if len([]rune(keyword)) > 64 {
		return nil, apperr.New(consts.CodeParamError)
	}
	page, pageSize := normalizeJoinRequestPage(req.GetPage(), req.GetPageSize())
	groups, total, err := s.groupRepo.SearchGroups(ctx, keyword, page, pageSize)
	if err != nil {
		return nil, mapGroupWriteError(err, "搜索群聊失败")
	}
	items := make([]*pb.GroupSearchItem, 0, len(groups))
	for _, group := range groups {
		if group == nil || group.Uuid == "" {
			continue
		}
		items = append(items, &pb.GroupSearchItem{
			GroupUuid:   group.Uuid,
			Name:        group.Name,
			Avatar:      group.Avatar,
			MemberCount: int32(group.MemberCnt),
			AddMode:     int32(group.AddMode),
		})
	}
	return &pb.SearchGroupsResponse{Groups: items, Total: total, Page: int32(page), PageSize: int32(pageSize)}, nil
}

// UpdateMyGroupNickname 更新当前登录用户自己的群名片。
//
// 群名片允许显式传空字符串来清空；这里仅做长度约束，具体成员是否存在、
// 群是否可写由 repository 在事务内二次校验。
func (s *groupServiceImpl) UpdateMyGroupNickname(ctx context.Context, req *pb.UpdateMyGroupNicknameRequest) error {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	if req == nil {
		return apperr.New(consts.CodeParamError)
	}
	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	groupNickname := strings.TrimSpace(req.GetGroupNickname())
	if groupUUID == "" || len([]rune(groupNickname)) > 64 {
		return apperr.New(consts.CodeParamError)
	}
	if err := mapGroupWriteError(
		s.groupRepo.UpdateMyGroupNickname(ctx, groupUUID, currentUserUUID, groupNickname),
		"更新群名片失败",
	); err != nil {
		return err
	}
	s.publishGroupStateChangedToMembers(ctx, groupUUID, []string{realtimepush.GroupChangedMembers})
	return nil
}

// UpdateGroupMemberNickname 管理员或群主修改指定成员群名片。
//
// 操作者身份只取认证上下文，目标成员来自请求参数；权限矩阵在 repository 事务中
// 基于最新成员角色裁决，防止并发角色变化导致越权代改名片。
func (s *groupServiceImpl) UpdateGroupMemberNickname(ctx context.Context, req *pb.UpdateGroupMemberNicknameRequest) error {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	if req == nil {
		return apperr.New(consts.CodeParamError)
	}
	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	targetUserUUID := strings.TrimSpace(req.GetUserUuid())
	groupNickname := strings.TrimSpace(req.GetGroupNickname())
	if groupUUID == "" || targetUserUUID == "" || len([]rune(groupNickname)) > 64 {
		return apperr.New(consts.CodeParamError)
	}
	if err := mapGroupWriteError(
		s.groupRepo.UpdateGroupMemberNickname(ctx, groupUUID, currentUserUUID, targetUserUUID, groupNickname),
		"更新成员群名片失败",
	); err != nil {
		return err
	}
	s.publishGroupStateChangedToMembers(ctx, groupUUID, []string{realtimepush.GroupChangedMembers})
	return nil
}

// MuteGroupMember 设置或取消指定成员单人禁言。
//
// mute_until=0 表示解除禁言；大于 0 时按 Unix 毫秒转换为截止时间，
// 权限矩阵和目标成员合法性统一交给 repository 事务处理。
func (s *groupServiceImpl) MuteGroupMember(ctx context.Context, req *pb.MuteGroupMemberRequest) error {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	if req == nil {
		return apperr.New(consts.CodeParamError)
	}
	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	targetUserUUID := strings.TrimSpace(req.GetUserUuid())
	muteUntil, err := normalizeMuteUntil(req.GetMuteUntil())
	if err != nil || groupUUID == "" || targetUserUUID == "" {
		return apperr.New(consts.CodeParamError)
	}
	if err := mapGroupWriteError(
		s.groupRepo.MuteGroupMember(ctx, groupUUID, currentUserUUID, targetUserUUID, muteUntil),
		"更新成员禁言状态失败",
	); err != nil {
		return err
	}
	s.publishGroupMemberMuted(ctx, groupUUID, targetUserUUID, req.GetMuteUntil())
	s.publishGroupStateChangedToMembers(ctx, groupUUID, []string{realtimepush.GroupChangedMemberMute})
	return nil
}

// UpdateGroupMuteSetting 更新群全员禁言开关。
//
// 全员禁言属于管理开关，service 不在这里区分群主和管理员，
// 统一由 repository 基于最新成员角色做权限判断。
func (s *groupServiceImpl) UpdateGroupMuteSetting(ctx context.Context, req *pb.UpdateGroupMuteSettingRequest) error {
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
	if err := mapGroupWriteError(
		s.groupRepo.UpdateGroupMuteSetting(ctx, groupUUID, currentUserUUID, req.GetMuteAll()),
		"更新全员禁言设置失败",
	); err != nil {
		return err
	}
	s.publishGroupStateChangedToMembers(ctx, groupUUID, []string{realtimepush.GroupChangedMuteAll})
	return nil
}

// CheckGroupSendPermission 检查指定用户是否允许发送群消息。
//
// 该接口面向 msg-service 等内部调用方，不依赖当前 HTTP 登录态；
// 非成员、全员禁言和单人禁言都作为结构化结果返回，便于消息链路稳定拒绝发送。
func (s *groupServiceImpl) CheckGroupSendPermission(ctx context.Context, req *pb.CheckGroupSendPermissionRequest) (*pb.CheckGroupSendPermissionResponse, error) {
	if req == nil {
		return nil, apperr.New(consts.CodeParamError)
	}
	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	userUUID := strings.TrimSpace(req.GetUserUuid())
	if groupUUID == "" || userUUID == "" {
		return nil, apperr.New(consts.CodeParamError)
	}
	permission, err := s.groupRepo.CheckGroupSendPermission(ctx, groupUUID, userUUID)
	if err != nil {
		return nil, mapGroupWriteError(err, "检查群发言权限失败")
	}
	return &pb.CheckGroupSendPermissionResponse{
		CanSend:   permission.CanSend,
		Role:      int32(permission.Role),
		Reason:    permission.Reason,
		MuteUntil: permission.MuteUntil,
		MuteAll:   permission.MuteAll,
	}, nil
}

// GetJoinRequestPendingCount 获取群待审批入群申请数量。
//
// 待读数量是管理视角数据，必须绑定当前登录用户，具体是否为管理员或群主
// 由 repository 统一检查，避免普通成员窥探申请活跃度。
func (s *groupServiceImpl) GetJoinRequestPendingCount(ctx context.Context, req *pb.GetJoinRequestPendingCountRequest) (*pb.GetJoinRequestPendingCountResponse, error) {
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
	count, err := s.groupRepo.GetJoinRequestPendingCount(ctx, groupUUID, currentUserUUID)
	if err != nil {
		return nil, mapGroupWriteError(err, "获取待审批入群申请数量失败")
	}
	return &pb.GetJoinRequestPendingCountResponse{Count: count}, nil
}

// normalizeMuteUntil 把请求里的 Unix 毫秒禁言截止时间转换为可空时间。
func normalizeMuteUntil(raw int64) (*time.Time, error) {
	if raw < 0 {
		return nil, apperr.New(consts.CodeParamError)
	}
	if raw == 0 {
		return nil, nil
	}
	muteUntil := time.UnixMilli(raw)
	return &muteUntil, nil
}
