package service

import (
	pb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	"github.com/013677890/LCchat-Backend/model"
	commonpb "github.com/013677890/LCchat-Backend/pkg/commonpb"
)

// buildPagination 构建跨服务共享的分页结构。
func buildPagination(page, pageSize int32, total int64) *commonpb.PaginationInfo {
	totalPages := int32(0)
	if pageSize > 0 {
		totalPages = int32((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return &commonpb.PaginationInfo{
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	}
}

// buildFriendApplyItemProto 将好友申请模型转换为收到的申请列表项。
func buildFriendApplyItemProto(apply *model.ApplyRequest) *pb.FriendApplyItem {
	if apply == nil {
		return nil
	}

	return &pb.FriendApplyItem{
		ApplyId:       apply.Id,
		ApplicantUuid: apply.ApplicantUuid,
		ApplicantInfo: &pb.SimpleUserInfo{Uuid: apply.ApplicantUuid},
		Reason:        apply.Reason,
		Source:        apply.Source,
		Status:        int32(apply.Status),
		IsRead:        apply.IsRead,
		CreatedAt:     apply.CreatedAt.UnixMilli(),
	}
}

// buildSentApplyItemProto 将好友申请模型转换为发出的申请列表项。
func buildSentApplyItemProto(apply *model.ApplyRequest) *pb.SentApplyItem {
	if apply == nil {
		return nil
	}

	return &pb.SentApplyItem{
		ApplyId:    apply.Id,
		TargetUuid: apply.TargetUuid,
		TargetInfo: &pb.SimpleUserInfo{Uuid: apply.TargetUuid},
		Reason:     apply.Reason,
		Source:     apply.Source,
		Status:     int32(apply.Status),
		IsRead:     apply.IsRead,
		CreatedAt:  apply.CreatedAt.UnixMilli(),
	}
}

// buildFriendItemProto 将单向关系模型转换为好友列表项。
func buildFriendItemProto(relation *model.UserRelation) *pb.FriendItem {
	if relation == nil {
		return nil
	}

	return &pb.FriendItem{
		Uuid:      relation.PeerUuid,
		Remark:    relation.Remark,
		GroupTag:  relation.GroupTag,
		Source:    relation.Source,
		CreatedAt: relation.CreatedAt.UnixMilli(),
	}
}

// buildFriendChangeProto 将关系变更模型转换为增量同步项。
func buildFriendChangeProto(relation *model.UserRelation, changeType string, changedAt int64) *pb.FriendChange {
	if relation == nil {
		return nil
	}

	return &pb.FriendChange{
		Uuid:       relation.PeerUuid,
		Remark:     relation.Remark,
		GroupTag:   relation.GroupTag,
		Source:     relation.Source,
		ChangeType: changeType,
		ChangedAt:  changedAt,
	}
}

// buildFriendCheckItemProto 将好友关系判断结果转换为响应项。
func buildFriendCheckItemProto(peerUUID string, isFriend bool) *pb.FriendCheckItem {
	if peerUUID == "" {
		return nil
	}

	return &pb.FriendCheckItem{
		PeerUuid: peerUUID,
		IsFriend: isFriend,
	}
}

// buildBlacklistItemProto 将黑名单关系模型转换为黑名单列表项。
func buildBlacklistItemProto(relation *model.UserRelation) *pb.BlacklistItem {
	if relation == nil {
		return nil
	}

	blacklistedAt := relation.UpdatedAt
	if relation.BlacklistedAt != nil {
		blacklistedAt = *relation.BlacklistedAt
	}

	return &pb.BlacklistItem{
		Uuid:          relation.PeerUuid,
		BlacklistedAt: blacklistedAt.UnixMilli(),
	}
}

// buildBlacklistListResponse 组装黑名单列表响应。
func buildBlacklistListResponse(items []*pb.BlacklistItem, page, pageSize int32, total int64) *pb.GetBlacklistListResponse {
	if items == nil {
		items = []*pb.BlacklistItem{}
	}

	return &pb.GetBlacklistListResponse{
		Items:      items,
		Pagination: buildPagination(page, pageSize, total),
	}
}

// buildCheckIsBlacklistResponse 组装是否拉黑响应。
func buildCheckIsBlacklistResponse(isBlocked bool) *pb.CheckIsBlacklistResponse {
	return &pb.CheckIsBlacklistResponse{IsBlacklist: isBlocked}
}

// buildRelationStatusResponse 组装关系状态响应。
func buildRelationStatusResponse(relation string, isFriend, isBlacklist bool, remark, groupTag string) *pb.GetRelationStatusResponse {
	return &pb.GetRelationStatusResponse{
		Relation:    relation,
		IsFriend:    isFriend,
		IsBlacklist: isBlacklist,
		Remark:      remark,
		GroupTag:    groupTag,
	}
}
