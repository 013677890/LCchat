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

// AddMember 添加群成员。
func (s *GroupServiceImpl) AddMember(ctx context.Context, req *dto.AddGroupMemberRequest) error {
	_, err := s.groupClient.AddMember(ctx, dto.ConvertToProtoAddGroupMemberRequest(req))
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
