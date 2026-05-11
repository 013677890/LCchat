package dto

import grouppb "github.com/013677890/LCchat-Backend/apps/group/pb"

// ==================== 群组 DTO ====================

// CreateGroupRequest 创建群请求。
type CreateGroupRequest struct {
	Name        string   `json:"name" binding:"required,min=1,max=64"`
	Avatar      string   `json:"avatar"`
	MemberUUIDs []string `json:"memberUuids"`
}

// CreateGroupResponse 创建群响应。
type CreateGroupResponse struct {
	GroupUUID string `json:"groupUuid"`
}

func ConvertToProtoCreateGroupRequest(dto *CreateGroupRequest) *grouppb.CreateGroupRequest {
	if dto == nil {
		return nil
	}
	return &grouppb.CreateGroupRequest{
		Name:        dto.Name,
		Avatar:      dto.Avatar,
		MemberUuids: dto.MemberUUIDs,
	}
}

func ConvertCreateGroupResponseFromProto(pb *grouppb.CreateGroupResponse) *CreateGroupResponse {
	if pb == nil {
		return nil
	}
	return &CreateGroupResponse{GroupUUID: pb.GetGroupUuid()}
}

// UpdateGroupInfoRequest 更新群资料请求。
type UpdateGroupInfoRequest struct {
	GroupUUID string `json:"groupUuid"`
	Name      string `json:"name" binding:"omitempty,max=64"`
	Avatar    string `json:"avatar"`
}

func ConvertToProtoUpdateGroupInfoRequest(dto *UpdateGroupInfoRequest) *grouppb.UpdateGroupInfoRequest {
	if dto == nil {
		return nil
	}
	return &grouppb.UpdateGroupInfoRequest{
		GroupUuid: dto.GroupUUID,
		Name:      dto.Name,
		Avatar:    dto.Avatar,
	}
}

// AddGroupMemberRequest 添加群成员请求。
type AddGroupMemberRequest struct {
	GroupUUID string   `json:"groupUuid"`
	UserUUIDs []string `json:"userUuids" binding:"required,min=1"`
}

func ConvertToProtoAddGroupMemberRequest(dto *AddGroupMemberRequest) *grouppb.AddMemberRequest {
	if dto == nil {
		return nil
	}
	return &grouppb.AddMemberRequest{
		GroupUuid: dto.GroupUUID,
		UserUuids: dto.UserUUIDs,
	}
}

// RemoveGroupMemberRequest 移除群成员请求。
type RemoveGroupMemberRequest struct {
	GroupUUID string `json:"groupUuid"`
	UserUUID  string `json:"userUuid"`
}

func ConvertToProtoRemoveGroupMemberRequest(dto *RemoveGroupMemberRequest) *grouppb.RemoveMemberRequest {
	if dto == nil {
		return nil
	}
	return &grouppb.RemoveMemberRequest{
		GroupUuid: dto.GroupUUID,
		UserUuid:  dto.UserUUID,
	}
}

// DismissGroupRequest 解散群请求。
type DismissGroupRequest struct {
	GroupUUID string `json:"groupUuid"`
}

func ConvertToProtoDismissGroupRequest(dto *DismissGroupRequest) *grouppb.DismissGroupRequest {
	if dto == nil {
		return nil
	}
	return &grouppb.DismissGroupRequest{GroupUuid: dto.GroupUUID}
}

// GetGroupInfoRequest 获取群资料请求。
type GetGroupInfoRequest struct {
	GroupUUID string `json:"groupUuid"`
}

// GroupInfoDTO 群资料 DTO。
type GroupInfoDTO struct {
	GroupUUID   string `json:"groupUuid"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar"`
	OwnerUUID   string `json:"ownerUuid"`
	MemberCount int32  `json:"memberCount"`
}

func ConvertToProtoGetGroupInfoRequest(dto *GetGroupInfoRequest) *grouppb.GetGroupInfoRequest {
	if dto == nil {
		return nil
	}
	return &grouppb.GetGroupInfoRequest{GroupUuid: dto.GroupUUID}
}

func ConvertGroupInfoFromProto(pb *grouppb.GetGroupInfoResponse) *GroupInfoDTO {
	if pb == nil {
		return nil
	}
	return &GroupInfoDTO{
		GroupUUID:   pb.GetGroupUuid(),
		Name:        pb.GetName(),
		Avatar:      pb.GetAvatar(),
		OwnerUUID:   pb.GetOwnerUuid(),
		MemberCount: pb.GetMemberCount(),
	}
}

// GetGroupListResponse 当前用户群列表响应。
type GetGroupListResponse struct {
	Groups []*GroupInfoDTO `json:"groups"`
}

func ConvertGroupListResponseFromProto(pb *grouppb.GetGroupListResponse) *GetGroupListResponse {
	if pb == nil {
		return nil
	}
	groups := make([]*GroupInfoDTO, 0, len(pb.GetGroups()))
	for _, group := range pb.GetGroups() {
		groups = append(groups, ConvertGroupInfoFromProto(group))
	}
	return &GetGroupListResponse{Groups: groups}
}

// GetGroupMemberListRequest 获取群成员列表请求。
type GetGroupMemberListRequest struct {
	GroupUUID string `json:"groupUuid"`
}

// GroupMemberItemDTO 群成员项 DTO。
type GroupMemberItemDTO struct {
	UserUUID string `json:"userUuid"`
	Role     int32  `json:"role"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// GetGroupMemberListResponse 获取群成员列表响应。
type GetGroupMemberListResponse struct {
	Members []*GroupMemberItemDTO `json:"members"`
}

func ConvertToProtoGetGroupMemberListRequest(dto *GetGroupMemberListRequest) *grouppb.GetMemberListRequest {
	if dto == nil {
		return nil
	}
	return &grouppb.GetMemberListRequest{GroupUuid: dto.GroupUUID}
}

func ConvertGroupMemberListResponseFromProto(pb *grouppb.GetMemberListResponse) *GetGroupMemberListResponse {
	if pb == nil {
		return nil
	}
	members := make([]*GroupMemberItemDTO, 0, len(pb.GetMembers()))
	for _, member := range pb.GetMembers() {
		if member == nil {
			continue
		}
		members = append(members, &GroupMemberItemDTO{
			UserUUID: member.GetUserUuid(),
			Role:     member.GetRole(),
			Nickname: member.GetNickname(),
			Avatar:   member.GetAvatar(),
		})
	}
	return &GetGroupMemberListResponse{Members: members}
}

// GetGroupMemberIDsRequest 获取群成员 UUID 请求。
type GetGroupMemberIDsRequest struct {
	GroupUUID string `json:"groupUuid"`
}

// GetGroupMemberIDsResponse 获取群成员 UUID 响应。
type GetGroupMemberIDsResponse struct {
	UserUUIDs []string `json:"userUuids"`
}

func ConvertToProtoGetGroupMemberIDsRequest(dto *GetGroupMemberIDsRequest) *grouppb.GetGroupMemberIdsRequest {
	if dto == nil {
		return nil
	}
	return &grouppb.GetGroupMemberIdsRequest{GroupUuid: dto.GroupUUID}
}

func ConvertGroupMemberIDsResponseFromProto(pb *grouppb.GetGroupMemberIdsResponse) *GetGroupMemberIDsResponse {
	if pb == nil {
		return nil
	}
	return &GetGroupMemberIDsResponse{UserUUIDs: pb.GetUserUuids()}
}
