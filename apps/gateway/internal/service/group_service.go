package service

import (
	"context"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/pb"
	grouppb "github.com/013677890/LCchat-Backend/apps/group/pb"
)

// GroupServiceImpl 是 gateway 到 group-service 的群能力适配层。
//
// 这里保持“DTO 转 proto、响应再转回 DTO”的单一职责，
// 避免在 gateway 再复制一份群业务规则，保证真实规则只维护在 group-service。
type GroupServiceImpl struct {
	groupClient pb.GroupServiceClient
}

// NewGroupService 创建群组服务实例。
func NewGroupService(groupClient pb.GroupServiceClient) GroupService {
	return &GroupServiceImpl{groupClient: groupClient}
}

// CreateGroup 创建群。
func (s *GroupServiceImpl) CreateGroup(ctx context.Context, req *dto.CreateGroupRequest) (*dto.CreateGroupResponse, error) {
	resp, err := s.groupClient.CreateGroup(ctx, dto.ConvertToProtoCreateGroupRequest(req))
	if err != nil {
		return nil, err
	}
	return dto.ConvertCreateGroupResponseFromProto(resp), nil
}

// DismissGroup 解散群。
func (s *GroupServiceImpl) DismissGroup(ctx context.Context, req *dto.DismissGroupRequest) error {
	_, err := s.groupClient.DismissGroup(ctx, dto.ConvertToProtoDismissGroupRequest(req))
	return err
}

// GetGroupInfo 获取群资料。
func (s *GroupServiceImpl) GetGroupInfo(ctx context.Context, req *dto.GetGroupInfoRequest) (*dto.GroupInfoDTO, error) {
	resp, err := s.groupClient.GetGroupInfo(ctx, dto.ConvertToProtoGetGroupInfoRequest(req))
	if err != nil {
		return nil, err
	}
	return dto.ConvertGroupInfoFromProto(resp), nil
}

// UpdateGroupInfo 更新群资料。
func (s *GroupServiceImpl) UpdateGroupInfo(ctx context.Context, req *dto.UpdateGroupInfoRequest) error {
	_, err := s.groupClient.UpdateGroupInfo(ctx, dto.ConvertToProtoUpdateGroupInfoRequest(req))
	return err
}

// UpdateGroupNotice 独立更新群公告。
func (s *GroupServiceImpl) UpdateGroupNotice(ctx context.Context, req *dto.UpdateGroupNoticeRequest) error {
	_, err := s.groupClient.UpdateGroupNotice(ctx, dto.ConvertToProtoUpdateGroupNoticeRequest(req))
	return err
}

// TransferGroupOwner 转让群主。
func (s *GroupServiceImpl) TransferGroupOwner(ctx context.Context, req *dto.TransferGroupOwnerRequest) error {
	_, err := s.groupClient.TransferGroupOwner(ctx, dto.ConvertToProtoTransferGroupOwnerRequest(req))
	return err
}

// UpdateMemberRole 更新群成员角色。
func (s *GroupServiceImpl) UpdateMemberRole(ctx context.Context, req *dto.UpdateGroupMemberRoleRequest) error {
	_, err := s.groupClient.UpdateMemberRole(ctx, dto.ConvertToProtoUpdateGroupMemberRoleRequest(req))
	return err
}

// ApplyJoinGroup 申请加入群聊。
func (s *GroupServiceImpl) ApplyJoinGroup(ctx context.Context, req *dto.ApplyJoinGroupRequest) (*dto.ApplyJoinGroupResponse, error) {
	resp, err := s.groupClient.ApplyJoinGroup(ctx, dto.ConvertToProtoApplyJoinGroupRequest(req))
	if err != nil {
		return nil, err
	}
	return dto.ConvertApplyJoinGroupResponseFromProto(resp), nil
}

// CancelJoinGroupApplication 撤销当前用户自己的待审批入群申请。
func (s *GroupServiceImpl) CancelJoinGroupApplication(ctx context.Context, req *dto.CancelJoinGroupApplicationRequest) error {
	_, err := s.groupClient.CancelJoinGroupApplication(ctx, dto.ConvertToProtoCancelJoinGroupApplicationRequest(req))
	return err
}

// GetMyJoinGroupApplication 获取当前用户在指定群的最新申请状态。
func (s *GroupServiceImpl) GetMyJoinGroupApplication(ctx context.Context, req *dto.GetMyJoinGroupApplicationRequest) (*dto.GetMyJoinGroupApplicationResponse, error) {
	resp, err := s.groupClient.GetMyJoinGroupApplication(ctx, dto.ConvertToProtoGetMyJoinGroupApplicationRequest(req))
	if err != nil {
		return nil, err
	}
	return dto.ConvertGetMyJoinGroupApplicationResponseFromProto(resp), nil
}

// ListMyJoinGroupApplications 获取当前用户发起的入群申请列表。
func (s *GroupServiceImpl) ListMyJoinGroupApplications(ctx context.Context, req *dto.ListMyJoinGroupApplicationsRequest) (*dto.ListMyJoinGroupApplicationsResponse, error) {
	// gateway 只负责转发分页参数，不在这里二次聚合群资料，
	// 这样可以保证列表读模型始终由 group-service 单点维护。
	resp, err := s.groupClient.ListMyJoinGroupApplications(ctx, dto.ConvertToProtoListMyJoinGroupApplicationsRequest(req))
	if err != nil {
		return nil, err
	}
	return dto.ConvertListMyJoinGroupApplicationsResponseFromProto(resp), nil
}

// ReviewJoinGroup 审批入群申请。
func (s *GroupServiceImpl) ReviewJoinGroup(ctx context.Context, req *dto.ReviewJoinGroupRequest) error {
	_, err := s.groupClient.ReviewJoinGroup(ctx, dto.ConvertToProtoReviewJoinGroupRequest(req))
	return err
}

// ListJoinRequests 获取待审批入群申请列表。
func (s *GroupServiceImpl) ListJoinRequests(ctx context.Context, req *dto.ListJoinRequestsRequest) (*dto.ListJoinRequestsResponse, error) {
	resp, err := s.groupClient.ListJoinRequests(ctx, dto.ConvertToProtoListJoinRequestsRequest(req))
	if err != nil {
		return nil, err
	}
	return dto.ConvertListJoinRequestsResponseFromProto(resp), nil
}

// ListReviewedJoinRequests 获取群已审批申请列表。
func (s *GroupServiceImpl) ListReviewedJoinRequests(ctx context.Context, req *dto.ListReviewedJoinRequestsRequest) (*dto.ListReviewedJoinRequestsResponse, error) {
	// 审批记录的申请人资料和审批结果都由下游一次性组装完成，
	// gateway 只做协议转换，避免把管理规则复制到接入层。
	resp, err := s.groupClient.ListReviewedJoinRequests(ctx, dto.ConvertToProtoListReviewedJoinRequestsRequest(req))
	if err != nil {
		return nil, err
	}
	return dto.ConvertListReviewedJoinRequestsResponseFromProto(resp), nil
}

// AddMember 添加群成员。
func (s *GroupServiceImpl) AddMember(ctx context.Context, req *dto.AddGroupMemberRequest) error {
	_, err := s.groupClient.AddMember(ctx, dto.ConvertToProtoAddGroupMemberRequest(req))
	return err
}

// LeaveGroup 当前用户主动退出群聊。
func (s *GroupServiceImpl) LeaveGroup(ctx context.Context, req *dto.LeaveGroupRequest) error {
	_, err := s.groupClient.LeaveGroup(ctx, dto.ConvertToProtoLeaveGroupRequest(req))
	return err
}

// RemoveMember 移除群成员。
func (s *GroupServiceImpl) RemoveMember(ctx context.Context, req *dto.RemoveGroupMemberRequest) error {
	_, err := s.groupClient.RemoveMember(ctx, dto.ConvertToProtoRemoveGroupMemberRequest(req))
	return err
}

// GetMemberList 获取群成员列表。
func (s *GroupServiceImpl) GetMemberList(ctx context.Context, req *dto.GetGroupMemberListRequest) (*dto.GetGroupMemberListResponse, error) {
	resp, err := s.groupClient.GetMemberList(ctx, dto.ConvertToProtoGetGroupMemberListRequest(req))
	if err != nil {
		return nil, err
	}
	return dto.ConvertGroupMemberListResponseFromProto(resp), nil
}

// SearchGroupMembers 搜索群成员。
func (s *GroupServiceImpl) SearchGroupMembers(ctx context.Context, req *dto.SearchGroupMembersRequest) (*dto.SearchGroupMembersResponse, error) {
	resp, err := s.groupClient.SearchGroupMembers(ctx, dto.ConvertToProtoSearchGroupMembersRequest(req))
	if err != nil {
		return nil, err
	}
	return dto.ConvertSearchGroupMembersResponseFromProto(resp), nil
}

// SearchGroups 按群名或完整群号搜索正常群。
func (s *GroupServiceImpl) SearchGroups(ctx context.Context, req *dto.SearchGroupsRequest) (*dto.SearchGroupsResponse, error) {
	resp, err := s.groupClient.SearchGroups(ctx, dto.ConvertToProtoSearchGroupsRequest(req))
	if err != nil {
		return nil, err
	}
	return dto.ConvertSearchGroupsResponseFromProto(resp), nil
}

// UpdateMyGroupNickname 更新当前用户自己的群名片。
func (s *GroupServiceImpl) UpdateMyGroupNickname(ctx context.Context, req *dto.UpdateMyGroupNicknameRequest) error {
	_, err := s.groupClient.UpdateMyGroupNickname(ctx, dto.ConvertToProtoUpdateMyGroupNicknameRequest(req))
	return err
}

// UpdateGroupMemberNickname 管理员或群主修改指定成员群名片。
func (s *GroupServiceImpl) UpdateGroupMemberNickname(ctx context.Context, req *dto.UpdateGroupMemberNicknameRequest) error {
	_, err := s.groupClient.UpdateGroupMemberNickname(ctx, dto.ConvertToProtoUpdateGroupMemberNicknameRequest(req))
	return err
}

// MuteGroupMember 设置或取消成员单人禁言。
func (s *GroupServiceImpl) MuteGroupMember(ctx context.Context, req *dto.MuteGroupMemberRequest) error {
	_, err := s.groupClient.MuteGroupMember(ctx, dto.ConvertToProtoMuteGroupMemberRequest(req))
	return err
}

// UpdateGroupMuteSetting 更新全员禁言开关。
func (s *GroupServiceImpl) UpdateGroupMuteSetting(ctx context.Context, req *dto.UpdateGroupMuteSettingRequest) error {
	_, err := s.groupClient.UpdateGroupMuteSetting(ctx, dto.ConvertToProtoUpdateGroupMuteSettingRequest(req))
	return err
}

// GetGroupList 获取当前用户的群列表。
//
// 下游接口当前不需要业务参数，gateway 在这里统一屏蔽空 proto 请求的构造细节。
func (s *GroupServiceImpl) GetGroupList(ctx context.Context) (*dto.GetGroupListResponse, error) {
	resp, err := s.groupClient.GetGroupList(ctx, &grouppb.GetGroupListRequest{})
	if err != nil {
		return nil, err
	}
	return dto.ConvertGroupListResponseFromProto(resp), nil
}

// GetGroupMemberIDs 获取群成员 UUID 列表。
func (s *GroupServiceImpl) GetGroupMemberIDs(ctx context.Context, req *dto.GetGroupMemberIDsRequest) (*dto.GetGroupMemberIDsResponse, error) {
	resp, err := s.groupClient.GetGroupMemberIds(ctx, dto.ConvertToProtoGetGroupMemberIDsRequest(req))
	if err != nil {
		return nil, err
	}
	return dto.ConvertGroupMemberIDsResponseFromProto(resp), nil
}

// GetJoinRequestPendingCount 获取群待审批入群申请数量。
func (s *GroupServiceImpl) GetJoinRequestPendingCount(ctx context.Context, req *dto.GetJoinRequestPendingCountRequest) (*dto.GetJoinRequestPendingCountResponse, error) {
	resp, err := s.groupClient.GetJoinRequestPendingCount(ctx, dto.ConvertToProtoGetJoinRequestPendingCountRequest(req))
	if err != nil {
		return nil, err
	}
	return dto.ConvertGetJoinRequestPendingCountResponseFromProto(resp), nil
}
