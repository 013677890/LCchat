package service

import (
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/model"
)

// buildUserInfoProto 将资料域模型转换为对外的用户资料 proto。
func buildUserInfoProto(profile *model.UserProfile) *pb.UserInfo {
	if profile == nil {
		return nil
	}

	birthday := ""
	if profile.Birthday != nil {
		birthday = profile.Birthday.Format("2006-01-02")
	}

	return &pb.UserInfo{
		Uuid:      profile.UserUuid,
		Nickname:  profile.Nickname,
		Avatar:    profile.Avatar,
		Gender:    int32(profile.Gender),
		Signature: profile.Signature,
		Birthday:  birthday,
	}
}

// buildSimpleUserInfoProto 将资料模型转换为批量资料查询使用的精简 proto。
func buildSimpleUserInfoProto(profile *model.UserProfile) *pb.SimpleUserInfo {
	if profile == nil {
		return nil
	}

	return &pb.SimpleUserInfo{
		Uuid:      profile.UserUuid,
		Nickname:  profile.Nickname,
		Avatar:    profile.Avatar,
		Gender:    int32(profile.Gender),
		Signature: profile.Signature,
	}
}

// buildSearchUserItemProto 将搜索结果资料转换为公开搜索项 proto。
func buildSearchUserItemProto(profile *model.UserProfile, isFriend bool) *pb.SimpleUserItem {
	if profile == nil {
		return nil
	}

	return &pb.SimpleUserItem{
		Uuid:      profile.UserUuid,
		Nickname:  profile.Nickname,
		Avatar:    profile.Avatar,
		Signature: profile.Signature,
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
func buildUserCardProto(profile *model.UserProfile) *pb.UserCard {
	if profile == nil {
		return nil
	}

	return &pb.UserCard{
		Uuid:     profile.UserUuid,
		Nickname: profile.Nickname,
		Avatar:   profile.Avatar,
	}
}

// buildPublicProfileProto 将资料模型转换为内部公开资料 proto。
func buildPublicProfileProto(profile *model.UserProfile) *pb.PublicProfile {
	if profile == nil {
		return nil
	}

	return &pb.PublicProfile{
		Uuid:      profile.UserUuid,
		Nickname:  profile.Nickname,
		Avatar:    profile.Avatar,
		Gender:    int32(profile.Gender),
		Signature: profile.Signature,
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
