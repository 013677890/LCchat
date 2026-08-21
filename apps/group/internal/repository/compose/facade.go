package compose

import (
	"context"
	"time"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/apps/group/internal/repository/cache"
	"github.com/013677890/LCchat-Backend/apps/group/internal/repository/store"
	"github.com/013677890/LCchat-Backend/model"
)

// Facade 是 group-service 的同步仓储门面。
//
// 写命令全部交给 MySQL 权威 store，并在同一事务内写 Outbox；
// 展示与权限读取走最终一致的 cache。service 只依赖这个组合后的 IGroupRepository。
//
// 本类型必须留在 compose 子包：父包 repository 若再 import cache，会与
// cache → repository 协议形成循环依赖。
type Facade struct {
	store *store.Store
	cache *cache.Reader
}

// New 组合权威写与同步读缓存。
func New(mysqlStore *store.Store, reader *cache.Reader) *Facade {
	return &Facade{store: mysqlStore, cache: reader}
}

// CreateGroup 把建群事务交给 store：业务表、cache_version 与 Outbox 同事务提交。
func (f *Facade) CreateGroup(ctx context.Context, group *model.GroupInfo, members []*model.GroupMember) error {
	return f.store.CreateGroup(ctx, group, members)
}

// AddMembers 把加人事务交给 store。
func (f *Facade) AddMembers(ctx context.Context, groupUUID, operatorUUID string, members []*model.GroupMember) error {
	return f.store.AddMembers(ctx, groupUUID, operatorUUID, members)
}

// RemoveMember 把踢人事务交给 store。
func (f *Facade) RemoveMember(ctx context.Context, groupUUID, operatorUUID, targetUUID string) error {
	return f.store.RemoveMember(ctx, groupUUID, operatorUUID, targetUUID)
}

// LeaveGroup 把主动退群事务交给 store。
func (f *Facade) LeaveGroup(ctx context.Context, groupUUID, operatorUUID string) error {
	return f.store.LeaveGroup(ctx, groupUUID, operatorUUID)
}

// DismissGroup 把解散群事务交给 store。
func (f *Facade) DismissGroup(ctx context.Context, groupUUID, operatorUUID string) error {
	return f.store.DismissGroup(ctx, groupUUID, operatorUUID)
}

// UpdateGroupInfo 把群资料更新交给 store。
func (f *Facade) UpdateGroupInfo(ctx context.Context, groupUUID, operatorUUID string, updates repository.GroupInfoUpdates) error {
	return f.store.UpdateGroupInfo(ctx, groupUUID, operatorUUID, updates)
}

// UpdateGroupNotice 把群公告更新交给 store。
func (f *Facade) UpdateGroupNotice(ctx context.Context, groupUUID, operatorUUID, notice string) error {
	return f.store.UpdateGroupNotice(ctx, groupUUID, operatorUUID, notice)
}

// TransferGroupOwner 把转让群主事务交给 store。
func (f *Facade) TransferGroupOwner(ctx context.Context, groupUUID, operatorUUID, targetUserUUID string) error {
	return f.store.TransferGroupOwner(ctx, groupUUID, operatorUUID, targetUserUUID)
}

// UpdateMemberRole 把角色变更事务交给 store。
func (f *Facade) UpdateMemberRole(ctx context.Context, groupUUID, operatorUUID, targetUserUUID string, role int8) error {
	return f.store.UpdateMemberRole(ctx, groupUUID, operatorUUID, targetUserUUID, role)
}

// SearchGroupMembers 按关键词搜索成员，走 MySQL 权威分页，不读成员 Hash 全量。
func (f *Facade) SearchGroupMembers(ctx context.Context, groupUUID, operatorUUID, keyword string, page, pageSize int) ([]*model.GroupMember, int64, error) {
	return f.store.SearchGroupMembers(ctx, groupUUID, operatorUUID, keyword, page, pageSize)
}

// UpdateMyGroupNickname 把本人改群名片交给 store。
func (f *Facade) UpdateMyGroupNickname(ctx context.Context, groupUUID, userUUID, nickname string) error {
	return f.store.UpdateMyGroupNickname(ctx, groupUUID, userUUID, nickname)
}

// UpdateGroupMemberNickname 把管理员改他人群名片交给 store。
func (f *Facade) UpdateGroupMemberNickname(ctx context.Context, groupUUID, operatorUUID, targetUserUUID, nickname string) error {
	return f.store.UpdateGroupMemberNickname(ctx, groupUUID, operatorUUID, targetUserUUID, nickname)
}

// MuteGroupMember 把单人禁言事务交给 store。
func (f *Facade) MuteGroupMember(ctx context.Context, groupUUID, operatorUUID, targetUserUUID string, muteUntil *time.Time) error {
	return f.store.MuteGroupMember(ctx, groupUUID, operatorUUID, targetUserUUID, muteUntil)
}

// UpdateGroupMuteSetting 把全员禁言开关交给 store。
func (f *Facade) UpdateGroupMuteSetting(ctx context.Context, groupUUID, operatorUUID string, muteAll bool) error {
	return f.store.UpdateGroupMuteSetting(ctx, groupUUID, operatorUUID, muteAll)
}

// ApplyJoinGroup 把入群申请或直加事务交给 store。
func (f *Facade) ApplyJoinGroup(ctx context.Context, groupUUID, applicantUUID, reason string) (repository.ApplyJoinGroupResult, error) {
	return f.store.ApplyJoinGroup(ctx, groupUUID, applicantUUID, reason)
}

// CancelJoinGroupApplication 把撤销申请交给 store。
func (f *Facade) CancelJoinGroupApplication(ctx context.Context, groupUUID, applicantUUID string) error {
	return f.store.CancelJoinGroupApplication(ctx, groupUUID, applicantUUID)
}

// GetMyJoinGroupApplication 读取当前用户在指定群的最新申请，走 MySQL 权威行。
func (f *Facade) GetMyJoinGroupApplication(ctx context.Context, groupUUID, applicantUUID string) (*model.GroupJoinRequest, error) {
	return f.store.GetMyJoinGroupApplication(ctx, groupUUID, applicantUUID)
}

// GetJoinRequestApplicant 读取指定申请的申请人 UUID，走 MySQL 权威行。
func (f *Facade) GetJoinRequestApplicant(ctx context.Context, groupUUID string, applyID int64) (string, error) {
	return f.store.GetJoinRequestApplicant(ctx, groupUUID, applyID)
}

// ListMyJoinGroupApplications 列出当前用户发起的申请，走 MySQL 分页。
func (f *Facade) ListMyJoinGroupApplications(ctx context.Context, applicantUUID string, status *int8, page, pageSize int) ([]*model.GroupJoinRequest, int64, error) {
	return f.store.ListMyJoinGroupApplications(ctx, applicantUUID, status, page, pageSize)
}

// ReviewJoinGroup 把审批入群申请事务交给 store。
func (f *Facade) ReviewJoinGroup(ctx context.Context, groupUUID, operatorUUID string, applyID int64, action int8, remark string) error {
	return f.store.ReviewJoinGroup(ctx, groupUUID, operatorUUID, applyID, action, remark)
}

// ListJoinRequests 读取待审批列表：群状态走最终一致缓存，操作者角色仍回源 MySQL。
func (f *Facade) ListJoinRequests(ctx context.Context, groupUUID, operatorUUID string, page, pageSize int) ([]*model.GroupJoinRequest, int64, error) {
	return f.cache.ListJoinRequests(ctx, groupUUID, operatorUUID, page, pageSize)
}

// ListReviewedJoinRequests 读取已审批历史，走 MySQL 分页。
func (f *Facade) ListReviewedJoinRequests(ctx context.Context, groupUUID, operatorUUID string, status *int8, page, pageSize int) ([]*model.GroupJoinRequest, int64, error) {
	return f.store.ListReviewedJoinRequests(ctx, groupUUID, operatorUUID, status, page, pageSize)
}

// GetJoinRequestPendingCount 统计待审批数量，走 MySQL 权威计数。
func (f *Facade) GetJoinRequestPendingCount(ctx context.Context, groupUUID, operatorUUID string) (int64, error) {
	return f.store.GetJoinRequestPendingCount(ctx, groupUUID, operatorUUID)
}

// GetGroupInfo 读取有效群资料，走最终一致的 group:info 缓存。
func (f *Facade) GetGroupInfo(ctx context.Context, groupUUID string) (*model.GroupInfo, error) {
	return f.cache.GetGroupInfo(ctx, groupUUID)
}

// GetGroupsByUUIDs 按 UUID 批量补齐群资料，走 Bloom 过滤后的 MySQL 批量查询。
func (f *Facade) GetGroupsByUUIDs(ctx context.Context, groupUUIDs []string) (map[string]*model.GroupInfo, error) {
	return f.store.GetGroupsByUUIDs(ctx, groupUUIDs)
}

// GetGroupMembers 读取群内有效成员列表，走最终一致的成员 Hash。
func (f *Facade) GetGroupMembers(ctx context.Context, groupUUID string) ([]*model.GroupMember, error) {
	return f.cache.GetGroupMembers(ctx, groupUUID)
}

// SearchGroups 按群名或完整群号搜索，走 MySQL 分页。
func (f *Facade) SearchGroups(ctx context.Context, keyword string, page, pageSize int) ([]*model.GroupInfo, int64, error) {
	return f.store.SearchGroups(ctx, keyword, page, pageSize)
}

// CheckGroupMember 检查成员身份，走 Hash field 点查；缓存 miss 才回源。
func (f *Facade) CheckGroupMember(ctx context.Context, groupUUID, userUUID string) (bool, int8, error) {
	return f.cache.CheckGroupMember(ctx, groupUUID, userUUID)
}

// CheckGroupSendPermission 检查群发言权限，走最终一致的群资料 + 成员 Hash field。
func (f *Facade) CheckGroupSendPermission(ctx context.Context, groupUUID, userUUID string) (repository.CheckGroupSendPermissionResult, error) {
	return f.cache.CheckGroupSendPermission(ctx, groupUUID, userUUID)
}

// ListUserGroups 读取用户所属群列表，走 READY 反向索引；命中后只提交低频对账意图。
func (f *Facade) ListUserGroups(ctx context.Context, userUUID string) ([]*model.GroupInfo, error) {
	return f.cache.ListUserGroups(ctx, userUUID)
}

// GetUserProfiles 按用户 UUID 批量补齐资料，走 MySQL。
func (f *Facade) GetUserProfiles(ctx context.Context, userUUIDs []string) (map[string]*model.UserProfile, error) {
	return f.store.GetUserProfiles(ctx, userUUIDs)
}

var _ repository.IGroupRepository = (*Facade)(nil)
