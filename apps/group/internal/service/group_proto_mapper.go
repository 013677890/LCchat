package service

import (
	"github.com/013677890/LCchat-Backend/apps/group/pb"
	"github.com/013677890/LCchat-Backend/model"
)

// buildGroupInfoProto 将群资料模型转换为 group proto。
//
// 第二批资料接口已经把公告和加群方式纳入正式模型，
// 因此这里直接输出完整读模型，避免 gateway 再拼装默认值或兼容旧字段。
func buildGroupInfoProto(groupInfo *model.GroupInfo) *pb.GetGroupInfoResponse {
	if groupInfo == nil || groupInfo.Uuid == "" {
		return nil
	}
	return &pb.GetGroupInfoResponse{
		GroupUuid:   groupInfo.Uuid,
		Name:        groupInfo.Name,
		Avatar:      groupInfo.Avatar,
		OwnerUuid:   groupInfo.OwnerUuid,
		MemberCount: int32(groupInfo.MemberCnt),
		Notice:      groupInfo.Notice,
		AddMode:     int32(groupInfo.AddMode),
	}
}

// buildGroupMemberItemProto 将群成员模型与用户资料快照组装成群成员 proto。
//
// 说明：
//  1. role 来自群成员关系表，是权限判断的权威来源；
//  2. nickname / avatar 来自 user_profile，是展示字段；
//  3. 若资料缺失，仍返回成员基础信息，避免因为资料不完整导致整个列表不可用。
func buildGroupMemberItemProto(member *model.GroupMember, profile *model.UserProfile) *pb.GroupMemberItem {
	if member == nil || member.UserUuid == "" {
		return nil
	}
	item := &pb.GroupMemberItem{
		UserUuid: member.UserUuid,
		Role:     int32(member.Role),
	}
	if profile != nil {
		item.Nickname = profile.Nickname
		item.Avatar = profile.Avatar
	}
	return item
}

// buildGroupListResponse 将群列表模型转换为响应对象。
//
// 返回空切片而不是 nil，方便上游直接遍历，减少空值判断分支。
func buildGroupListResponse(groups []*model.GroupInfo) *pb.GetGroupListResponse {
	items := make([]*pb.GetGroupInfoResponse, 0, len(groups))
	for _, groupInfo := range groups {
		if item := buildGroupInfoProto(groupInfo); item != nil {
			items = append(items, item)
		}
	}
	return &pb.GetGroupListResponse{Groups: items}
}

// buildGroupJoinRequestItemProto 将入群申请与申请人资料组装成 proto 响应项。
//
// 申请列表对审批人来说本质上是“申请事实 + 申请人展示信息”的聚合视图，
// 因此这里继续保持 service 层聚合、proto 层只承载最终读模型的风格。
func buildGroupJoinRequestItemProto(request *model.GroupJoinRequest, profile *model.UserProfile) *pb.GroupJoinRequestItem {
	if request == nil || request.ApplicantUuid == "" {
		return nil
	}
	item := &pb.GroupJoinRequestItem{
		ApplyId:       request.Id,
		ApplicantUuid: request.ApplicantUuid,
		Reason:        request.Reason,
		CreatedAt:     request.CreatedAt.UnixMilli(),
	}
	if profile != nil {
		item.Nickname = profile.Nickname
		item.Avatar = profile.Avatar
	}
	return item
}
