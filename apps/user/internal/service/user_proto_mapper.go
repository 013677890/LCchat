package service

import (
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/model"
)

// buildUserInfoProto 将资料域模型转换为对外的用户资料 proto。
func buildUserInfoProto(userInfo *model.UserInfo) *pb.UserInfo {
	if userInfo == nil {
		return nil
	}

	birthday := ""
	if userInfo.Birthday != nil {
		birthday = userInfo.Birthday.Format("2006-01-02")
	}

	return &pb.UserInfo{
		Uuid:      userInfo.Uuid,
		Nickname:  userInfo.Nickname,
		Avatar:    userInfo.Avatar,
		Gender:    int32(userInfo.Gender),
		Signature: userInfo.Signature,
		Birthday:  birthday,
	}
}

// buildSimpleUserInfoProto 将资料模型转换为批量资料查询使用的精简 proto。
func buildSimpleUserInfoProto(userInfo *model.UserInfo) *pb.SimpleUserInfo {
	if userInfo == nil {
		return nil
	}

	return &pb.SimpleUserInfo{
		Uuid:      userInfo.Uuid,
		Nickname:  userInfo.Nickname,
		Avatar:    userInfo.Avatar,
		Gender:    int32(userInfo.Gender),
		Signature: userInfo.Signature,
	}
}

// buildSearchUserItemProto 将搜索结果资料转换为公开搜索项 proto。
func buildSearchUserItemProto(userInfo *model.UserInfo, isFriend bool) *pb.SimpleUserItem {
	if userInfo == nil {
		return nil
	}

	return &pb.SimpleUserItem{
		Uuid:      userInfo.Uuid,
		Nickname:  userInfo.Nickname,
		Avatar:    userInfo.Avatar,
		Signature: userInfo.Signature,
		IsFriend:  isFriend,
	}
}

// buildUserPaginationInfoProto 构建 user 域本地分页 proto。
func buildUserPaginationInfoProto(page, pageSize int32, total int64) *pb.PaginationInfo {
	totalPages := int32(0)
	if pageSize > 0 {
		totalPages = int32((total + int64(pageSize) - 1) / int64(pageSize))
	}

	return &pb.PaginationInfo{
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	}
}

// buildUserCardProto 将资料模型转换为内部卡片 proto。
func buildUserCardProto(userInfo *model.UserInfo) *pb.UserCard {
	if userInfo == nil {
		return nil
	}

	return &pb.UserCard{
		Uuid:     userInfo.Uuid,
		Nickname: userInfo.Nickname,
		Avatar:   userInfo.Avatar,
	}
}

// buildPublicProfileProto 将资料模型转换为内部公开资料 proto。
func buildPublicProfileProto(userInfo *model.UserInfo) *pb.PublicProfile {
	if userInfo == nil {
		return nil
	}

	return &pb.PublicProfile{
		Uuid:      userInfo.Uuid,
		Nickname:  userInfo.Nickname,
		Avatar:    userInfo.Avatar,
		Gender:    int32(userInfo.Gender),
		Signature: userInfo.Signature,
	}
}

// buildGroupMemberItemProto 将群成员模型转换为只读群成员 proto。
func buildGroupMemberItemProto(member *model.GroupMember) *pb.GroupMemberItem {
	if member == nil || member.UserUuid == "" {
		return nil
	}

	return &pb.GroupMemberItem{
		UserUuid: member.UserUuid,
		Role:     int32(member.Role),
	}
}
