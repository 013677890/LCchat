package dto

import (
	pb "ChatServer/apps/user/pb"
	"ChatServer/model"
	"time"
)

// ==================== 好友服务 DTO ====================

// SearchUserRequest 搜索用户请求DTO
type SearchUserRequest struct {
	Keyword  string // 搜索关键字
	Page     int32  // 页码
	PageSize int32  // 每页大小
}

// SearchUserResponse 搜索用户响应DTO
type SearchUserResponse struct {
	Items      []*SimpleUserItem  // 用户列表
	Pagination *pb.PaginationInfo // 分页信息
}

// SimpleUserItem 简化用户信息DTO（搜索结果）
type SimpleUserItem struct {
	UUID      string // 用户UUID
	Nickname  string // 昵称
	Email     string // 邮箱
	Avatar    string // 头像
	Signature string // 个性签名
	IsFriend  bool   // 是否好友
}

// SendFriendApplyRequest 发送好友申请请求DTO
type SendFriendApplyRequest struct {
	TargetUUID string // 目标用户UUID
	Reason     string // 申请理由
	Source     string // 来源
}

// SendFriendApplyResponse 发送好友申请响应DTO
type SendFriendApplyResponse struct {
	ApplyID int64 // 申请ID
}

// GetFriendApplyListRequest 获取好友申请列表请求DTO
type GetFriendApplyListRequest struct {
	Status   int32 // 状态(0:待处理 1:已同意 2:已拒绝)
	Page     int32 // 页码
	PageSize int32 // 每页大小
}

// FriendApplyItem 好友申请信息DTO
type FriendApplyItem struct {
	ApplyID           int64     // 申请ID
	ApplicantUUID     string    // 申请人UUID
	ApplicantNickname string    // 申请人昵称
	ApplicantAvatar   string    // 申请人头像
	Reason            string    // 申请理由
	Source            string    // 来源
	Status            int32     // 状态
	IsRead            bool      // 是否已读
	CreatedAt         time.Time // 申请时间
}

// GetFriendApplyListResponse 获取好友申请列表响应DTO
type GetFriendApplyListResponse struct {
	Items      []*FriendApplyItem // 好友申请列表
	Pagination *pb.PaginationInfo // 分页信息
}

// GetSentApplyListRequest 获取发出的申请列表请求DTO
type GetSentApplyListRequest struct {
	Status   int32 // 状态
	Page     int32 // 页码
	PageSize int32 // 每页大小
}

// SentApplyItem 发出的申请项DTO
type SentApplyItem struct {
	ApplyID    int64           // 申请ID
	TargetUUID string          // 目标用户UUID
	TargetInfo *SimpleUserInfo // 目标用户信息
	Status     int32           // 状态
	CreatedAt  time.Time       // 申请时间
}

// GetSentApplyListResponse 获取发出的申请列表响应DTO
type GetSentApplyListResponse struct {
	Items      []*SentApplyItem   // 发出的申请列表
	Pagination *pb.PaginationInfo // 分页信息
}

// HandleFriendApplyRequest 处理好友申请请求DTO
type HandleFriendApplyRequest struct {
	ApplyID int64  // 申请ID
	Action  int32  // 操作(1:同意 2:拒绝)
	Remark  string // 处理备注
}

// HandleFriendApplyResponse 处理好友申请响应DTO
type HandleFriendApplyResponse struct{}

// GetUnreadApplyCountRequest 获取未读申请数量请求DTO
type GetUnreadApplyCountRequest struct{}

// GetUnreadApplyCountResponse 获取未读申请数量响应DTO
type GetUnreadApplyCountResponse struct {
	UnreadCount int32 // 未读数量
}

// MarkApplyAsReadRequest 标记申请已读请求DTO
type MarkApplyAsReadRequest struct {
	ApplyIDs []int64 // 申请ID列表
}

// MarkApplyAsReadResponse 标记申请已读响应DTO
type MarkApplyAsReadResponse struct{}

// GetFriendListRequest 获取好友列表请求DTO
type GetFriendListRequest struct {
	GroupTag string // 标签
	Page     int32  // 页码
	PageSize int32  // 每页大小
}

// FriendItem 好友信息DTO
type FriendItem struct {
	UUID      string    // 好友UUID
	Nickname  string    // 昵称
	Avatar    string    // 头像
	Gender    int32     // 性别
	Signature string    // 个性签名
	Remark    string    // 备注名
	GroupTag  string    // 标签
	Source    string    // 来源
	CreatedAt time.Time // 添加好友时间
}

// GetFriendListResponse 获取好友列表响应DTO
type GetFriendListResponse struct {
	Items      []*FriendItem      // 好友列表
	Pagination *pb.PaginationInfo // 分页信息
	Version    int64              // 版本号
}

// SyncFriendListRequest 增量同步请求DTO
type SyncFriendListRequest struct {
	Version int64 // 版本号
	Limit   int32 // 每次同步数量
}

// FriendChange 好友变更DTO
type FriendChange struct {
	UUID       string // 好友UUID
	Nickname   string // 昵称
	Avatar     string // 头像
	Gender     int32  // 性别
	Signature  string // 个性签名
	Remark     string // 备注名
	GroupTag   string // 标签
	Source     string // 来源
	ChangeType string // 变更类型(add/update/delete)
	ChangedAt  int64  // 变更时间
}

// SyncFriendListResponse 增量同步响应DTO
type SyncFriendListResponse struct {
	Changes       []*FriendChange // 变更列表
	HasMore       bool            // 是否还有更多
	LatestVersion int64           // 最新版本号
}

// DeleteFriendRequest 删除好友请求DTO
type DeleteFriendRequest struct {
	UserUUID string // 当前用户UUID
}

// DeleteFriendResponse 删除好友响应DTO
type DeleteFriendResponse struct{}

// SetFriendRemarkRequest 设置好友备注请求DTO
type SetFriendRemarkRequest struct {
	UserUUID string // 用户UUID
	Remark   string // 备注名
}

// SetFriendRemarkResponse 设置好友备注响应DTO
type SetFriendRemarkResponse struct{}

// SetFriendTagRequest 设置好友标签请求DTO
type SetFriendTagRequest struct {
	UserUUID string // 用户UUID
	GroupTag string // 标签
}

// SetFriendTagResponse 设置好友标签响应DTO
type SetFriendTagResponse struct{}

// GetTagListRequest 获取标签列表请求DTO
type GetTagListRequest struct{}

// TagItem 标签项DTO
type TagItem struct {
	TagName string // 标签名
	Count   int32  // 数量
}

// GetTagListResponse 获取标签列表响应DTO
type GetTagListResponse struct {
	Tags []*TagItem // 标签列表
}

// CheckIsFriendRequest 判断是否好友请求DTO
type CheckIsFriendRequest struct {
	UserUUID string // 当前用户UUID
	PeerUUID string // 目标用户UUID
}

// CheckIsFriendResponse 判断是否好友响应DTO
type CheckIsFriendResponse struct {
	IsFriend bool // 是否好友
}

// GetRelationStatusRequest 获取关系状态请求DTO
type GetRelationStatusRequest struct {
	UserUUID string // 当前用户UUID
	PeerUUID string // 目标用户UUID
}

// GetRelationStatusResponse 获取关系状态响应DTO
type GetRelationStatusResponse struct {
	Relation    string // 关系(none/friend/blacklist/deleted)
	IsFriend    bool   // 是否好友
	IsBlacklist bool   // 是否拉黑
	Remark      string // 备注名
	GroupTag    string // 标签
}

// ==================== 转换函数 ====================

// ConvertSimpleUserModelToDTO 将数据库模型转换为简化用户DTO
func ConvertSimpleUserModelToDTO(user *model.UserInfo, isFriend bool) *SimpleUserItem {
	if user == nil {
		return nil
	}
	return &SimpleUserItem{
		UUID:      user.Uuid,
		Nickname:  user.Nickname,
		Email:     user.Email,
		Avatar:    user.Avatar,
		Signature: user.Signature,
		IsFriend:  isFriend,
	}
}

// ConvertSimpleUserModelsToDTO 批量转换
func ConvertSimpleUserModelsToDTO(users []*model.UserInfo, isFriendMap map[string]bool) []*SimpleUserItem {
	if users == nil {
		return []*SimpleUserItem{}
	}

	result := make([]*SimpleUserItem, 0, len(users))
	for _, user := range users {
		isFriend := false
		if isFriendMap != nil {
			isFriend = isFriendMap[user.Uuid]
		}
		result = append(result, ConvertSimpleUserModelToDTO(user, isFriend))
	}
	return result
}

// ConvertSimpleUserItemToProto 将DTO转换为Protobuf消息
func ConvertSimpleUserItemToProto(item *SimpleUserItem) *pb.SimpleUserItem {
	if item == nil {
		return nil
	}
	return &pb.SimpleUserItem{
		Uuid:      item.UUID,
		Nickname:  item.Nickname,
		Email:     item.Email,
		Avatar:    item.Avatar,
		Signature: item.Signature,
		IsFriend:  item.IsFriend,
	}
}

// ConvertSimpleUserItemListToProto 批量转换
func ConvertSimpleUserItemListToProto(items []*SimpleUserItem) []*pb.SimpleUserItem {
	if items == nil {
		return []*pb.SimpleUserItem{}
	}

	result := make([]*pb.SimpleUserItem, 0, len(items))
	for _, item := range items {
		result = append(result, ConvertSimpleUserItemToProto(item))
	}
	return result
}

// ConvertFriendApplyModelToDTO 将数据库模型转换为好友申请DTO
func ConvertFriendApplyModelToDTO(apply *model.ApplyRequest, user *model.UserInfo) *FriendApplyItem {
	if apply == nil {
		return nil
	}

	item := &FriendApplyItem{
		ApplyID:   apply.Id,
		Reason:    apply.Reason,
		Source:    apply.HandleUserUuid,
		Status:    int32(apply.Status),
		IsRead:    apply.IsRead,
		CreatedAt: apply.CreatedAt,
	}

	if user != nil {
		item.ApplicantUUID = user.Uuid
		item.ApplicantNickname = user.Nickname
		item.ApplicantAvatar = user.Avatar
	}

	return item
}

// ConvertFriendApplyModelsToDTO 批量转换
func ConvertFriendApplyModelsToDTO(applies []*model.ApplyRequest, users []*model.UserInfo) []*FriendApplyItem {
	if applies == nil {
		return []*FriendApplyItem{}
	}

	// 创建用户映射
	userMap := make(map[string]*model.UserInfo)
	for _, user := range users {
		userMap[user.Uuid] = user
	}

	result := make([]*FriendApplyItem, 0, len(applies))
	for _, apply := range applies {
		user := userMap[apply.ApplicantUuid]
		result = append(result, ConvertFriendApplyModelToDTO(apply, user))
	}
	return result
}

// ConvertFriendApplyItemToProto 将DTO转换为Protobuf消息
func ConvertFriendApplyItemToProto(item *FriendApplyItem) *pb.FriendApplyItem {
	if item == nil {
		return nil
	}
	return &pb.FriendApplyItem{
		ApplyId: item.ApplyID,
		ApplicantInfo: &pb.SimpleUserInfo{
			Uuid:     item.ApplicantUUID,
			Nickname: item.ApplicantNickname,
			Avatar:   item.ApplicantAvatar,
		},
		Reason:    item.Reason,
		Source:    item.Source,
		Status:    item.Status,
		IsRead:    item.IsRead,
		CreatedAt: item.CreatedAt.Unix() * 1000,
	}
}

// ConvertFriendApplyItemListToProto 批量转换
func ConvertFriendApplyItemListToProto(items []*FriendApplyItem) []*pb.FriendApplyItem {
	if items == nil {
		return []*pb.FriendApplyItem{}
	}

	result := make([]*pb.FriendApplyItem, 0, len(items))
	for _, item := range items {
		result = append(result, ConvertFriendApplyItemToProto(item))
	}
	return result
}

// ConvertFriendRelationModelToDTO 将数据库模型转换为好友信息DTO
func ConvertFriendRelationModelToDTO(relation *model.UserRelation, user *model.UserInfo) *FriendItem {
	if relation == nil {
		return nil
	}

	item := &FriendItem{
		UUID:      relation.PeerUuid,
		Remark:    relation.Remark,
		GroupTag:  relation.GroupTag,
		Source:    relation.Source,
		CreatedAt: relation.CreatedAt,
	}

	if user != nil {
		item.Nickname = user.Nickname
		item.Avatar = user.Avatar
		item.Gender = int32(user.Gender)
		item.Signature = user.Signature
	}

	return item
}

// ConvertFriendRelationModelsToDTO 批量转换
func ConvertFriendRelationModelsToDTO(relations []*model.UserRelation, users []*model.UserInfo) []*FriendItem {
	if relations == nil {
		return []*FriendItem{}
	}

	// 创建用户映射
	userMap := make(map[string]*model.UserInfo)
	for _, user := range users {
		userMap[user.Uuid] = user
	}

	result := make([]*FriendItem, 0, len(relations))
	for _, relation := range relations {
		user := userMap[relation.PeerUuid]
		result = append(result, ConvertFriendRelationModelToDTO(relation, user))
	}
	return result
}

// ConvertFriendItemToProto 将DTO转换为Protobuf消息
func ConvertFriendItemToProto(item *FriendItem) *pb.FriendItem {
	if item == nil {
		return nil
	}
	return &pb.FriendItem{
		Uuid:      item.UUID,
		Nickname:  item.Nickname,
		Avatar:    item.Avatar,
		Gender:    item.Gender,
		Signature: item.Signature,
		Remark:    item.Remark,
		GroupTag:  item.GroupTag,
		Source:    item.Source,
		CreatedAt: item.CreatedAt.Unix() * 1000,
	}
}

// ConvertFriendItemListToProto 批量转换
func ConvertFriendItemListToProto(items []*FriendItem) []*pb.FriendItem {
	if items == nil {
		return []*pb.FriendItem{}
	}

	result := make([]*pb.FriendItem, 0, len(items))
	for _, item := range items {
		result = append(result, ConvertFriendItemToProto(item))
	}
	return result
}

// ConvertFriendRelationModelToChange 将数据库模型转换为变更DTO
func ConvertFriendRelationModelToChange(relation *model.UserRelation, changeType string) *FriendChange {
	if relation == nil {
		return nil
	}
	return &FriendChange{
		UUID:       relation.PeerUuid,
		Remark:     relation.Remark,
		GroupTag:   relation.GroupTag,
		Source:     relation.Source,
		ChangeType: changeType,
		ChangedAt:  relation.UpdatedAt.Unix(),
	}
}

// ConvertFriendChangeToProto 将DTO转换为Protobuf消息
func ConvertFriendChangeToProto(change *FriendChange) *pb.FriendChange {
	if change == nil {
		return nil
	}
	return &pb.FriendChange{
		Uuid:       change.UUID,
		Nickname:   change.Nickname,
		Avatar:     change.Avatar,
		Gender:     change.Gender,
		Signature:  change.Signature,
		Remark:     change.Remark,
		GroupTag:   change.GroupTag,
		Source:     change.Source,
		ChangeType: change.ChangeType,
		ChangedAt:  change.ChangedAt,
	}
}

// ConvertFriendChangeListToProto 批量转换
func ConvertFriendChangeListToProto(changes []*FriendChange) []*pb.FriendChange {
	if changes == nil {
		return []*pb.FriendChange{}
	}

	result := make([]*pb.FriendChange, 0, len(changes))
	for _, change := range changes {
		result = append(result, ConvertFriendChangeToProto(change))
	}
	return result
}

// ConvertTagToProto 将标签DTO转换为Protobuf消息
func ConvertTagToProto(tag *TagItem) *pb.TagItem {
	if tag == nil {
		return nil
	}
	return &pb.TagItem{
		TagName: tag.TagName,
		Count:   tag.Count,
	}
}

// ConvertTagListToProto 批量转换
func ConvertTagListToProto(tags []*TagItem) []*pb.TagItem {
	if tags == nil {
		return []*pb.TagItem{}
	}

	result := make([]*pb.TagItem, 0, len(tags))
	for _, tag := range tags {
		result = append(result, ConvertTagToProto(tag))
	}
	return result
}
