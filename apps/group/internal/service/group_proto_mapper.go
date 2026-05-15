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

// buildMyJoinGroupApplicationProto 将“我的入群申请状态”模型转换为 proto 响应。
//
// 这里直接暴露申请单当前最新事实，避免 gateway 再拼装状态字段，
// 保证待审批、已通过、已拒绝、已撤销四类状态都由 group-service 单点输出。
func buildMyJoinGroupApplicationProto(request *model.GroupJoinRequest) *pb.MyJoinGroupApplication {
	if request == nil || request.Id <= 0 {
		return nil
	}
	item := &pb.MyJoinGroupApplication{
		ApplyId:      request.Id,
		Status:       int32(request.Status),
		Reason:       request.Reason,
		ReviewerUuid: request.ReviewerUuid,
		ReviewRemark: request.ReviewRemark,
		CreatedAt:    request.CreatedAt.UnixMilli(),
	}
	if request.ReviewedAt != nil {
		item.ReviewedAt = request.ReviewedAt.UnixMilli()
	}
	return item
}

// buildMyJoinGroupApplicationListItemProto 将“我的入群申请列表”项组装成 proto 响应。
//
// 这里把申请事实和群展示资料聚合到一起，避免 gateway 为了列表展示再次回查群资料，
// 让用户侧可以直接拿到群名称、头像和申请流转状态。
func buildMyJoinGroupApplicationListItemProto(request *model.GroupJoinRequest, group *model.GroupInfo) *pb.MyJoinGroupApplicationListItem {
	if request == nil || request.Id <= 0 || request.GroupUuid == "" {
		return nil
	}
	item := &pb.MyJoinGroupApplicationListItem{
		ApplyId:      request.Id,
		GroupUuid:    request.GroupUuid,
		Status:       int32(request.Status),
		Reason:       request.Reason,
		ReviewerUuid: request.ReviewerUuid,
		ReviewRemark: request.ReviewRemark,
		CreatedAt:    request.CreatedAt.UnixMilli(),
	}
	if request.ReviewedAt != nil {
		item.ReviewedAt = request.ReviewedAt.UnixMilli()
	}
	if group != nil {
		item.GroupName = group.Name
		item.GroupAvatar = group.Avatar
	}
	return item
}

// buildReviewedJoinRequestItemProto 将审批记录与申请人资料组装成 proto 响应项。
//
// 审批记录列表不仅要展示申请人信息，还要回显审批结果与处理备注，
// 因此这里统一输出完整的审批事实读模型。
func buildReviewedJoinRequestItemProto(request *model.GroupJoinRequest, profile *model.UserProfile) *pb.ReviewedJoinRequestItem {
	if request == nil || request.Id <= 0 || request.ApplicantUuid == "" {
		return nil
	}
	item := &pb.ReviewedJoinRequestItem{
		ApplyId:       request.Id,
		ApplicantUuid: request.ApplicantUuid,
		Status:        int32(request.Status),
		Reason:        request.Reason,
		ReviewerUuid:  request.ReviewerUuid,
		ReviewRemark:  request.ReviewRemark,
		CreatedAt:     request.CreatedAt.UnixMilli(),
	}
	if request.ReviewedAt != nil {
		item.ReviewedAt = request.ReviewedAt.UnixMilli()
	}
	if profile != nil {
		item.Nickname = profile.Nickname
		item.Avatar = profile.Avatar
	}
	return item
}
