package converter

import (
	"time"

	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/util"
)

// ==================== UserInfo 转换函数 ====================

// ModelToProtoUserInfo 将 UserInfo Model 转换为 Proto
// 注意：不包含敏感字段（Password、IsAdmin）
func ModelToProtoUserInfo(user *model.UserInfo) *pb.UserInfo {
	if user == nil {
		return nil
	}
	return &pb.UserInfo{
		Uuid:      user.Uuid,
		Nickname:  user.Nickname,
		Avatar:    user.Avatar,
		Gender:    int32(user.Gender),
		Signature: user.Signature,
		Birthday:  formatBirthday(user.Birthday),
	}
}

// ModelListToProtoUserInfoList 批量转换 UserInfo
func ModelListToProtoUserInfoList(users []*model.UserInfo) []*pb.UserInfo {
	if users == nil {
		return []*pb.UserInfo{}
	}

	result := make([]*pb.UserInfo, 0, len(users))
	for _, user := range users {
		result = append(result, ModelToProtoUserInfo(user))
	}
	return result
}

// ModelToProtoSimpleUserInfo 将 UserInfo Model 转换为 SimpleUserInfo Proto
func ModelToProtoSimpleUserInfo(user *model.UserInfo) *pb.SimpleUserInfo {
	if user == nil {
		return nil
	}
	return &pb.SimpleUserInfo{
		Uuid:      user.Uuid,
		Nickname:  user.Nickname,
		Avatar:    user.Avatar,
		Gender:    int32(user.Gender),
		Signature: user.Signature,
	}
}

// ModelListToProtoSimpleUserInfoList 批量转换 SimpleUserInfo
func ModelListToProtoSimpleUserInfoList(users []*model.UserInfo) []*pb.SimpleUserInfo {
	if users == nil {
		return []*pb.SimpleUserInfo{}
	}

	result := make([]*pb.SimpleUserInfo, 0, len(users))
	for _, user := range users {
		result = append(result, ModelToProtoSimpleUserInfo(user))
	}
	return result
}

// ==================== Friend 相关转换函数 ====================

// ModelToProtoSimpleUserItem 将 UserInfo Model 转换为 SimpleUserItem Proto（搜索结果）
func ModelToProtoSimpleUserItem(user *model.UserInfo, isFriend bool) *pb.SimpleUserItem {
	if user == nil {
		return nil
	}
	return &pb.SimpleUserItem{
		Uuid:      user.Uuid,
		Nickname:  user.Nickname,
		Avatar:    user.Avatar,
		Signature: user.Signature,
		IsFriend:  isFriend,
	}
}

// ModelListToProtoSimpleUserItemList 批量转换 SimpleUserItem
func ModelListToProtoSimpleUserItemList(users []*model.UserInfo, isFriendMap map[string]bool) []*pb.SimpleUserItem {
	if users == nil {
		return []*pb.SimpleUserItem{}
	}

	result := make([]*pb.SimpleUserItem, 0, len(users))
	for _, user := range users {
		isFriend := false
		if isFriendMap != nil {
			isFriend = isFriendMap[user.Uuid]
		}
		result = append(result, ModelToProtoSimpleUserItem(user, isFriend))
	}
	return result
}

// ModelToProtoFriendApplyItem 将 ApplyRequest Model 和 UserInfo Model 转换为 FriendApplyItem Proto
func ModelToProtoFriendApplyItem(apply *model.ApplyRequest, applicant *model.UserInfo) *pb.FriendApplyItem {
	if apply == nil {
		return nil
	}

	applicantInfo := &pb.SimpleUserInfo{}
	if applicant != nil {
		applicantInfo.Uuid = applicant.Uuid
		applicantInfo.Nickname = applicant.Nickname
		applicantInfo.Avatar = applicant.Avatar
		applicantInfo.Gender = int32(applicant.Gender)
		applicantInfo.Signature = applicant.Signature
	}

	return &pb.FriendApplyItem{
		ApplyId:       apply.Id,
		ApplicantInfo: applicantInfo,
		Reason:        apply.Reason,
		Source:        apply.Source,
		Status:        int32(apply.Status),
		IsRead:        apply.IsRead,
		CreatedAt:     apply.CreatedAt.Unix() * 1000,
	}
}

// ModelsToProtoFriendApplyItemList 批量转换 FriendApplyItem
func ModelsToProtoFriendApplyItemList(applies []*model.ApplyRequest, users []*model.UserInfo) []*pb.FriendApplyItem {
	if applies == nil {
		return []*pb.FriendApplyItem{}
	}

	// 创建用户映射
	userMap := make(map[string]*model.UserInfo)
	for _, user := range users {
		userMap[user.Uuid] = user
	}

	result := make([]*pb.FriendApplyItem, 0, len(applies))
	for _, apply := range applies {
		user := userMap[apply.ApplicantUuid]
		result = append(result, ModelToProtoFriendApplyItem(apply, user))
	}
	return result
}

// ModelToProtoFriendItem 将 UserRelation Model 和 UserInfo Model 转换为 FriendItem Proto
func ModelToProtoFriendItem(relation *model.UserRelation, user *model.UserInfo) *pb.FriendItem {
	if relation == nil {
		return nil
	}

	item := &pb.FriendItem{
		Uuid:      relation.PeerUuid,
		Remark:    relation.Remark,
		GroupTag:  relation.GroupTag,
		Source:    relation.Source,
		CreatedAt: relation.CreatedAt.Unix() * 1000,
	}

	if user != nil {
		item.Nickname = user.Nickname
		item.Avatar = user.Avatar
		item.Gender = int32(user.Gender)
		item.Signature = user.Signature
	}

	return item
}

// ModelsToProtoFriendItemList 批量转换 FriendItem
func ModelsToProtoFriendItemList(relations []*model.UserRelation, users []*model.UserInfo) []*pb.FriendItem {
	if relations == nil {
		return []*pb.FriendItem{}
	}

	// 创建用户映射
	userMap := make(map[string]*model.UserInfo)
	for _, user := range users {
		userMap[user.Uuid] = user
	}

	result := make([]*pb.FriendItem, 0, len(relations))
	for _, relation := range relations {
		user := userMap[relation.PeerUuid]
		result = append(result, ModelToProtoFriendItem(relation, user))
	}
	return result
}

// ModelToProtoFriendChange 将 UserRelation Model 转换为 FriendChange Proto
func ModelToProtoFriendChange(relation *model.UserRelation, user *model.UserInfo, changeType string) *pb.FriendChange {
	if relation == nil {
		return nil
	}

	changedAt := relation.UpdatedAt.UnixMilli()
	if changeType == "delete" && relation.DeletedAt.Valid {
		changedAt = relation.DeletedAt.Time.UnixMilli()
	}

	change := &pb.FriendChange{
		Uuid:       relation.PeerUuid,
		Remark:     relation.Remark,
		GroupTag:   relation.GroupTag,
		Source:     relation.Source,
		ChangeType: changeType,
		ChangedAt:  changedAt,
	}

	if user != nil {
		change.Nickname = user.Nickname
		change.Avatar = user.Avatar
		change.Gender = int32(user.Gender)
		change.Signature = user.Signature
	}

	return change
}

// ==================== Blacklist 相关转换函数 ====================

// ModelToProtoBlacklistItem 将 UserRelation Model 和 UserInfo Model 转换为 BlacklistItem Proto
func ModelToProtoBlacklistItem(relation *model.UserRelation, user *model.UserInfo) *pb.BlacklistItem {
	if relation == nil {
		return nil
	}

	item := &pb.BlacklistItem{
		BlacklistedAt: relation.UpdatedAt.Unix() * 1000,
	}

	if user != nil {
		item.Uuid = user.Uuid
		item.Nickname = user.Nickname
		item.Avatar = user.Avatar
	}

	return item
}

// ModelsToProtoBlacklistItemList 批量转换 BlacklistItem
func ModelsToProtoBlacklistItemList(relations []*model.UserRelation, users []*model.UserInfo) []*pb.BlacklistItem {
	if relations == nil {
		return []*pb.BlacklistItem{}
	}

	// 创建用户映射
	userMap := make(map[string]*model.UserInfo)
	for _, user := range users {
		userMap[user.Uuid] = user
	}

	result := make([]*pb.BlacklistItem, 0, len(relations))
	for _, relation := range relations {
		user := userMap[relation.PeerUuid]
		result = append(result, ModelToProtoBlacklistItem(relation, user))
	}
	return result
}

// ==================== Device 相关转换函数 ====================

// ModelToProtoDeviceItem 将 DeviceSession Model 转换为 DeviceItem Proto
func ModelToProtoDeviceItem(session *model.DeviceSession, currentDeviceID string) *pb.DeviceItem {
	if session == nil {
		return nil
	}

	item := &pb.DeviceItem{
		DeviceId:        session.DeviceId,
		DeviceName:      session.DeviceName,
		Platform:        session.Platform,
		AppVersion:      session.AppVersion,
		Status:          int32(session.Status),
		IsCurrentDevice: session.DeviceId == currentDeviceID,
	}

	item.LastSeenAt = util.TimeToUnixMilli(session.UpdatedAt)
	if item.LastSeenAt == 0 {
		item.LastSeenAt = util.TimeToUnixMilli(session.CreatedAt)
	}

	return item
}

// ModelsToProtoDeviceItemList 批量转换 DeviceItem
func ModelsToProtoDeviceItemList(sessions []*model.DeviceSession, currentDeviceID string) []*pb.DeviceItem {
	if sessions == nil {
		return []*pb.DeviceItem{}
	}

	result := make([]*pb.DeviceItem, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, ModelToProtoDeviceItem(session, currentDeviceID))
	}
	return result
}

// ==================== Proto to Model 转换函数 ====================

// ProtoToModelDeviceInfo 将 DeviceInfo Proto 转换为创建 DeviceSession Model 所需的字段
func ProtoToModelDeviceInfo(deviceInfo *pb.DeviceInfo) (deviceName, platform, osVersion, appVersion string) {
	if deviceInfo == nil {
		return "", "", "", ""
	}
	return deviceInfo.DeviceName, deviceInfo.Platform, deviceInfo.OsVersion, deviceInfo.AppVersion
}

// ProtoUpdateProfileToModelFields 将 UpdateProfile Proto 请求转换为 Model 更新字段
func ProtoUpdateProfileToModelFields(req *pb.UpdateProfileRequest) (nickname, birthday, signature string, gender int8) {
	if req == nil {
		return "", "", "", 0
	}
	return req.Nickname, req.Birthday, req.Signature, int8(req.Gender)
}

// ==================== 辅助函数 ====================

// TimeToMillis 将 time.Time 转换为毫秒时间戳
func TimeToMillis(t time.Time) int64 {
	return t.Unix() * 1000
}

// TimePointerToMillis 将 *time.Time 转换为毫秒时间戳
func TimePointerToMillis(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return t.Unix() * 1000
}

func formatBirthday(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}
