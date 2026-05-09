package service

import (
	"github.com/013677890/LCchat-Backend/apps/group/pb"
	"github.com/013677890/LCchat-Backend/model"
)

// buildGroupInfoProto 将群资料模型转换为 group proto。
//
// 当前 proto 的群资料字段较精简，因此这里只映射：
//  1. 群 UUID；
//  2. 群名称；
//  3. 群头像；
//  4. 群主 UUID；
//  5. 当前成员数量。
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
