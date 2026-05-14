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

// ConvertToProtoCreateGroupRequest 把创建群 DTO 转成 group proto 请求。
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

// ConvertCreateGroupResponseFromProto 把创建群 proto 响应转成 HTTP DTO。
func ConvertCreateGroupResponseFromProto(pb *grouppb.CreateGroupResponse) *CreateGroupResponse {
	if pb == nil {
		return nil
	}
	return &CreateGroupResponse{GroupUUID: pb.GetGroupUuid()}
}

// UpdateGroupInfoRequest 更新群资料请求。
type UpdateGroupInfoRequest struct {
	GroupUUID string  `json:"groupUuid"`
	Name      *string `json:"name" binding:"omitempty,max=64"`
	Avatar    *string `json:"avatar"`
	AddMode   *int32  `json:"addMode" binding:"omitempty,oneof=0 1"`
}

// ConvertToProtoUpdateGroupInfoRequest 把资料更新 DTO 转成 proto 请求。
//
// 这里保留 optional 字段的 nil 语义，避免 gateway 把“未传字段”错误转换成空字符串更新。
func ConvertToProtoUpdateGroupInfoRequest(dto *UpdateGroupInfoRequest) *grouppb.UpdateGroupInfoRequest {
	if dto == nil {
		return nil
	}
	req := &grouppb.UpdateGroupInfoRequest{GroupUuid: dto.GroupUUID}
	if dto.Name != nil {
		req.Name = dto.Name
	}
	if dto.Avatar != nil {
		req.Avatar = dto.Avatar
	}
	if dto.AddMode != nil {
		req.AddMode = dto.AddMode
	}
	return req
}

// UpdateGroupNoticeRequest 独立更新群公告请求。
type UpdateGroupNoticeRequest struct {
	GroupUUID string `json:"groupUuid"`
	Notice    string `json:"notice" binding:"max=500"`
}

// ConvertToProtoUpdateGroupNoticeRequest 把群公告更新 DTO 转成 proto 请求。
func ConvertToProtoUpdateGroupNoticeRequest(dto *UpdateGroupNoticeRequest) *grouppb.UpdateGroupNoticeRequest {
	if dto == nil {
		return nil
	}
	return &grouppb.UpdateGroupNoticeRequest{
		GroupUuid: dto.GroupUUID,
		Notice:    dto.Notice,
	}
}

// ApplyJoinGroupRequest 申请加入群聊请求。
type ApplyJoinGroupRequest struct {
	GroupUUID string `json:"groupUuid"`
	Reason    string `json:"reason" binding:"omitempty,max=255"`
}

// ApplyJoinGroupResponse 申请加入群聊响应。
type ApplyJoinGroupResponse struct {
	ApplyID        int64 `json:"applyId"`
	JoinedDirectly bool  `json:"joinedDirectly"`
}

// ConvertToProtoApplyJoinGroupRequest 把入群申请 DTO 转成 proto 请求。
func ConvertToProtoApplyJoinGroupRequest(dto *ApplyJoinGroupRequest) *grouppb.ApplyJoinGroupRequest {
	if dto == nil {
		return nil
	}
	return &grouppb.ApplyJoinGroupRequest{
		GroupUuid: dto.GroupUUID,
		Reason:    dto.Reason,
	}
}

// ConvertApplyJoinGroupResponseFromProto 把入群申请 proto 响应转成 HTTP DTO。
func ConvertApplyJoinGroupResponseFromProto(pb *grouppb.ApplyJoinGroupResponse) *ApplyJoinGroupResponse {
	if pb == nil {
		return nil
	}
	return &ApplyJoinGroupResponse{
		ApplyID:        pb.GetApplyId(),
		JoinedDirectly: pb.GetJoinedDirectly(),
	}
}

// ReviewJoinGroupRequest 审批入群申请请求。
type ReviewJoinGroupRequest struct {
	GroupUUID string `json:"groupUuid"`
	ApplyID   int64  `json:"applyId"`
	Action    int32  `json:"action" binding:"required,oneof=1 2"`
	Remark    string `json:"remark" binding:"omitempty,max=255"`
}

// ConvertToProtoReviewJoinGroupRequest 把审批 DTO 转成 proto 请求。
func ConvertToProtoReviewJoinGroupRequest(dto *ReviewJoinGroupRequest) *grouppb.ReviewJoinGroupRequest {
	if dto == nil {
		return nil
	}
	return &grouppb.ReviewJoinGroupRequest{
		GroupUuid: dto.GroupUUID,
		ApplyId:   dto.ApplyID,
		Action:    dto.Action,
		Remark:    dto.Remark,
	}
}

// GroupJoinRequestItemDTO 待审批入群申请项 DTO。
type GroupJoinRequestItemDTO struct {
	ApplyID        int64  `json:"applyId"`
	ApplicantUUID  string `json:"applicantUuid"`
	Nickname       string `json:"nickname"`
	Avatar         string `json:"avatar"`
	Reason         string `json:"reason"`
	CreatedAtMilli int64  `json:"createdAt"`
}

// ListJoinRequestsRequest 获取待审批入群申请列表请求。
type ListJoinRequestsRequest struct {
	GroupUUID string `json:"groupUuid"`
	Page      int32  `form:"page" json:"page" binding:"omitempty,min=1"`
	PageSize  int32  `form:"pageSize" json:"pageSize" binding:"omitempty,min=1,max=100"`
}

// ListJoinRequestsResponse 获取待审批入群申请列表响应。
type ListJoinRequestsResponse struct {
	Items    []*GroupJoinRequestItemDTO `json:"items"`
	Total    int64                      `json:"total"`
	Page     int32                      `json:"page"`
	PageSize int32                      `json:"pageSize"`
}

// ConvertToProtoListJoinRequestsRequest 把申请列表 DTO 转成 proto 请求。
func ConvertToProtoListJoinRequestsRequest(dto *ListJoinRequestsRequest) *grouppb.ListJoinRequestsRequest {
	if dto == nil {
		return nil
	}
	return &grouppb.ListJoinRequestsRequest{
		GroupUuid: dto.GroupUUID,
		Page:      dto.Page,
		PageSize:  dto.PageSize,
	}
}

// ConvertListJoinRequestsResponseFromProto 把申请列表 proto 响应转成 HTTP DTO。
func ConvertListJoinRequestsResponseFromProto(pb *grouppb.ListJoinRequestsResponse) *ListJoinRequestsResponse {
	if pb == nil {
		return nil
	}
	items := make([]*GroupJoinRequestItemDTO, 0, len(pb.GetItems()))
	for _, item := range pb.GetItems() {
		if item == nil {
			continue
		}
		items = append(items, &GroupJoinRequestItemDTO{
			ApplyID:        item.GetApplyId(),
			ApplicantUUID:  item.GetApplicantUuid(),
			Nickname:       item.GetNickname(),
			Avatar:         item.GetAvatar(),
			Reason:         item.GetReason(),
			CreatedAtMilli: item.GetCreatedAt(),
		})
	}
	return &ListJoinRequestsResponse{
		Items:    items,
		Total:    pb.GetTotal(),
		Page:     pb.GetPage(),
		PageSize: pb.GetPageSize(),
	}
}

// TransferGroupOwnerRequest 转让群主请求。
type TransferGroupOwnerRequest struct {
	GroupUUID      string `json:"groupUuid"`
	TargetUserUUID string `json:"targetUserUuid" binding:"required"`
}

// ConvertToProtoTransferGroupOwnerRequest 把群主转让 DTO 转成 proto 请求。
func ConvertToProtoTransferGroupOwnerRequest(dto *TransferGroupOwnerRequest) *grouppb.TransferGroupOwnerRequest {
	if dto == nil {
		return nil
	}
	return &grouppb.TransferGroupOwnerRequest{
		GroupUuid:      dto.GroupUUID,
		TargetUserUuid: dto.TargetUserUUID,
	}
}

// UpdateGroupMemberRoleRequest 更新群成员角色请求。
type UpdateGroupMemberRoleRequest struct {
	GroupUUID string `json:"groupUuid"`
	UserUUID  string `json:"userUuid"`
	Role      *int32 `json:"role" binding:"required,oneof=0 1"`
}

// ConvertToProtoUpdateGroupMemberRoleRequest 把成员角色更新 DTO 转成 proto 请求。
func ConvertToProtoUpdateGroupMemberRoleRequest(dto *UpdateGroupMemberRoleRequest) *grouppb.UpdateMemberRoleRequest {
	if dto == nil || dto.Role == nil {
		return nil
	}
	return &grouppb.UpdateMemberRoleRequest{
		GroupUuid: dto.GroupUUID,
		UserUuid:  dto.UserUUID,
		Role:      *dto.Role,
	}
}

// AddGroupMemberRequest 添加群成员请求。
type AddGroupMemberRequest struct {
	GroupUUID string   `json:"groupUuid"`
	UserUUIDs []string `json:"userUuids" binding:"required,min=1"`
}

// ConvertToProtoAddGroupMemberRequest 把添加成员 DTO 转成 proto 请求。
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

// ConvertToProtoRemoveGroupMemberRequest 把移除成员 DTO 转成 proto 请求。
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

// ConvertToProtoDismissGroupRequest 把解散群 DTO 转成 proto 请求。
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
	Notice      string `json:"notice"`
	OwnerUUID   string `json:"ownerUuid"`
	MemberCount int32  `json:"memberCount"`
	AddMode     int32  `json:"addMode"`
}

// ConvertToProtoGetGroupInfoRequest 把查群资料 DTO 转成 proto 请求。
func ConvertToProtoGetGroupInfoRequest(dto *GetGroupInfoRequest) *grouppb.GetGroupInfoRequest {
	if dto == nil {
		return nil
	}
	return &grouppb.GetGroupInfoRequest{GroupUuid: dto.GroupUUID}
}

// ConvertGroupInfoFromProto 把群资料 proto 响应转成 HTTP DTO。
func ConvertGroupInfoFromProto(pb *grouppb.GetGroupInfoResponse) *GroupInfoDTO {
	if pb == nil {
		return nil
	}
	return &GroupInfoDTO{
		GroupUUID:   pb.GetGroupUuid(),
		Name:        pb.GetName(),
		Avatar:      pb.GetAvatar(),
		Notice:      pb.GetNotice(),
		OwnerUUID:   pb.GetOwnerUuid(),
		MemberCount: pb.GetMemberCount(),
		AddMode:     pb.GetAddMode(),
	}
}

// GetGroupListResponse 当前用户群列表响应。
type GetGroupListResponse struct {
	Groups []*GroupInfoDTO `json:"groups"`
}

// ConvertGroupListResponseFromProto 把群列表 proto 响应转成 HTTP DTO。
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

// ConvertToProtoGetGroupMemberListRequest 把群成员列表 DTO 转成 proto 请求。
func ConvertToProtoGetGroupMemberListRequest(dto *GetGroupMemberListRequest) *grouppb.GetMemberListRequest {
	if dto == nil {
		return nil
	}
	return &grouppb.GetMemberListRequest{GroupUuid: dto.GroupUUID}
}

// ConvertGroupMemberListResponseFromProto 把群成员列表 proto 响应转成 HTTP DTO。
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

// ConvertToProtoGetGroupMemberIDsRequest 把群成员 ID 列表 DTO 转成 proto 请求。
func ConvertToProtoGetGroupMemberIDsRequest(dto *GetGroupMemberIDsRequest) *grouppb.GetGroupMemberIdsRequest {
	if dto == nil {
		return nil
	}
	return &grouppb.GetGroupMemberIdsRequest{GroupUuid: dto.GroupUUID}
}

// ConvertGroupMemberIDsResponseFromProto 把群成员 ID proto 响应转成 HTTP DTO。
func ConvertGroupMemberIDsResponseFromProto(pb *grouppb.GetGroupMemberIdsResponse) *GetGroupMemberIDsResponse {
	if pb == nil {
		return nil
	}
	return &GetGroupMemberIDsResponse{UserUUIDs: pb.GetUserUuids()}
}
