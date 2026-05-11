package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/group/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/util"
)

// groupServiceImpl 是 group-service 业务层实现。
type groupServiceImpl struct {
	groupRepo repository.IGroupRepository
}

const defaultGroupAvatarURL = "https://cube.elemecdn.com/0/88/03b0d39583f48206768a7534e55bcpng.png"

// NewGroupService 创建 group 服务实例。
func NewGroupService(groupRepo repository.IGroupRepository) IGroupService {
	return &groupServiceImpl{groupRepo: groupRepo}
}

// CreateGroup 创建群。
func (s *groupServiceImpl) CreateGroup(ctx context.Context, req *pb.CreateGroupRequest) (*pb.CreateGroupResponse, error) {
	// 群写操作必须绑定当前登录用户，群主身份只信任 context 中的认证结果。
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}
	if req == nil {
		return nil, apperr.New(consts.CodeParamError)
	}

	// service 层保留一次兜底校验，避免绕过 proto validate 的内部调用写入脏数据。
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return nil, apperr.New(consts.CodeParamError)
	}
	if len([]rune(name)) > 64 {
		return nil, apperr.New(consts.CodeGroupNameTooLong)
	}

	avatar := strings.TrimSpace(req.GetAvatar())
	if avatar == "" {
		// 默认头像在 service 层补齐，保证 repository 只处理完整业务实体。
		avatar = defaultGroupAvatarURL
	}

	// 群 UUID 只生成一次，后续 group 与初始成员关系必须共享同一个事实 ID。
	groupUUID := util.GenIDString()
	// 初始成员使用同一时间戳，避免列表排序时因逐条生成时间出现无意义抖动。
	now := time.Now()
	memberUUIDs := normalizeGroupMemberUUIDs(req.GetMemberUuids(), currentUserUUID)
	members := buildCreateGroupMembers(groupUUID, currentUserUUID, memberUUIDs, now)
	group := &model.GroupInfo{
		Uuid:      groupUUID,
		Name:      name,
		Avatar:    avatar,
		OwnerUuid: currentUserUUID,
		MemberCnt: len(members),
		Status:    0,
	}

	if err := s.groupRepo.CreateGroup(ctx, group, members); err != nil {
		// 系统错误统一包中文上下文，具体日志留给边界拦截器输出。
		return nil, apperr.Wrap(err, consts.CodeInternalError, "创建群失败")
	}

	return &pb.CreateGroupResponse{GroupUuid: group.Uuid}, nil
}

// DismissGroup 解散群。
func (s *groupServiceImpl) DismissGroup(ctx context.Context, req *pb.DismissGroupRequest) error {
	// 操作者必须来自认证上下文，不能信任请求体传入的任何身份字段。
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	if req == nil {
		return apperr.New(consts.CodeParamError)
	}

	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	if groupUUID == "" {
		return apperr.New(consts.CodeParamError)
	}

	// 群主权限、重复解散幂等和缓存强失效都下沉到 repository 事务内处理。
	return mapGroupWriteError(
		s.groupRepo.DismissGroup(ctx, groupUUID, currentUserUUID),
		"解散群失败",
	)
}

// GetGroupInfo 获取群资料。
func (s *groupServiceImpl) GetGroupInfo(ctx context.Context, req *pb.GetGroupInfoRequest) (*pb.GetGroupInfoResponse, error) {
	if req == nil {
		return nil, apperr.New(consts.CodeParamError)
	}

	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	if groupUUID == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	groupInfo, err := s.groupRepo.GetGroupInfo(ctx, groupUUID)
	if err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return nil, apperr.New(consts.CodeGroupNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取群资料失败")
	}

	return buildGroupInfoProto(groupInfo), nil
}

// UpdateGroupInfo 更新群资料。
func (s *groupServiceImpl) UpdateGroupInfo(ctx context.Context, req *pb.UpdateGroupInfoRequest) error {
	// 资料更新是写操作，必须先确认调用方已登录。
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	if req == nil {
		return apperr.New(consts.CodeParamError)
	}

	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	if groupUUID == "" {
		return apperr.New(consts.CodeParamError)
	}

	// proto 中 name/avatar 是普通 string，这里统一解析“空值不更新”和字段合法性。
	name, avatar, err := buildUpdateGroupInfoFields(req)
	if err != nil {
		return err
	}
	if name == nil && avatar == nil {
		// 无字段变更直接成功，避免开启无意义事务和刷新 updated_at。
		return nil
	}

	// 管理员/群主权限与群状态在 repository 内再次校验，防止并发状态变化。
	return mapGroupWriteError(
		s.groupRepo.UpdateGroupInfo(ctx, groupUUID, currentUserUUID, name, avatar),
		"更新群资料失败",
	)
}

// AddMember 添加群成员。
func (s *groupServiceImpl) AddMember(ctx context.Context, req *pb.AddMemberRequest) error {
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	if req == nil || len(req.GetUserUuids()) == 0 {
		return apperr.New(consts.CodeParamError)
	}

	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	if groupUUID == "" {
		return apperr.New(consts.CodeParamError)
	}

	// service 只做去空去重，是否已在群由 repository 在锁内按幂等规则处理。
	memberUUIDs := normalizeGroupMemberUUIDs(req.GetUserUuids())
	if len(memberUUIDs) == 0 {
		return nil
	}

	// 这里只构造最小成员意图，角色、邀请人和入群时间以 repository 事务结果为准。
	members := buildAddGroupMembers(groupUUID, memberUUIDs)
	return mapGroupWriteError(
		s.groupRepo.AddMembers(ctx, groupUUID, currentUserUUID, members),
		"添加群成员失败",
	)
}

// RemoveMember 移除群成员。
func (s *groupServiceImpl) RemoveMember(ctx context.Context, req *pb.RemoveMemberRequest) error {
	// 操作者来自 context，目标成员来自请求体，二者相同表示主动退群。
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	if req == nil {
		return apperr.New(consts.CodeParamError)
	}

	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	targetUUID := strings.TrimSpace(req.GetUserUuid())
	if groupUUID == "" || targetUUID == "" {
		return apperr.New(consts.CodeParamError)
	}

	// 退群、踢人、不能踢群主等角色规则由 repository 在事务锁内统一裁决。
	return mapGroupWriteError(
		s.groupRepo.RemoveMember(ctx, groupUUID, currentUserUUID, targetUUID),
		"移除群成员失败",
	)
}

// GetMemberList 获取群成员列表。
func (s *groupServiceImpl) GetMemberList(ctx context.Context, req *pb.GetMemberListRequest) (*pb.GetMemberListResponse, error) {
	if req == nil {
		return nil, apperr.New(consts.CodeParamError)
	}

	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	if groupUUID == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	members, err := s.groupRepo.GetGroupMembers(ctx, groupUUID)
	if err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return nil, apperr.New(consts.CodeGroupNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取群成员列表失败")
	}

	// 成员关系与用户资料分属不同表，展示信息用批量查询补齐，避免 N+1 查询。
	profiles, err := s.groupRepo.GetUserProfiles(ctx, collectMemberUUIDs(members))
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询群成员资料失败")
	}

	items := make([]*pb.GroupMemberItem, 0, len(members))
	for _, member := range members {
		if member == nil {
			continue
		}
		if item := buildGroupMemberItemProto(member, profiles[member.UserUuid]); item != nil {
			items = append(items, item)
		}
	}

	return &pb.GetMemberListResponse{Members: items}, nil
}

// GetGroupList 获取当前用户的群列表。
func (s *groupServiceImpl) GetGroupList(ctx context.Context, req *pb.GetGroupListRequest) (*pb.GetGroupListResponse, error) {
	_ = req

	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	groups, err := s.groupRepo.ListUserGroups(ctx, currentUserUUID)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取群列表失败")
	}

	return buildGroupListResponse(groups), nil
}

// GetGroupMemberIds 获取群成员 UUID 列表。
func (s *groupServiceImpl) GetGroupMemberIds(ctx context.Context, req *pb.GetGroupMemberIdsRequest) (*pb.GetGroupMemberIdsResponse, error) {
	if req == nil {
		return nil, apperr.New(consts.CodeParamError)
	}

	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	if groupUUID == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	members, err := s.groupRepo.GetGroupMembers(ctx, groupUUID)
	if err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return nil, apperr.New(consts.CodeGroupNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取群成员 ID 列表失败")
	}

	return &pb.GetGroupMemberIdsResponse{UserUuids: collectMemberUUIDs(members)}, nil
}

// CheckGroupMember 检查指定用户是否为群成员并返回角色。
func (s *groupServiceImpl) CheckGroupMember(ctx context.Context, req *pb.CheckGroupMemberRequest) (*pb.CheckGroupMemberResponse, error) {
	if req == nil {
		return nil, apperr.New(consts.CodeParamError)
	}

	groupUUID := strings.TrimSpace(req.GetGroupUuid())
	userUUID := strings.TrimSpace(req.GetUserUuid())
	if groupUUID == "" || userUUID == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	// 高频权限链路交给 repository 使用缓存加速，并在读缓存前确认群未解散。
	isMember, role, err := s.groupRepo.CheckGroupMember(ctx, groupUUID, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrGroupDismissed) {
			return nil, apperr.New(consts.CodeGroupAlreadyDismiss)
		}
		if errors.Is(err, repository.ErrRecordNotFound) {
			return nil, apperr.New(consts.CodeGroupNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "检查群成员关系失败")
	}
	if !isMember {
		role = -1
	}

	return &pb.CheckGroupMemberResponse{IsMember: isMember, Role: int32(role)}, nil
}

func normalizeGroupMemberUUIDs(raw []string, exclusions ...string) []string {
	if len(raw) == 0 {
		return []string{}
	}

	// 先写入排除集合，可用于创建群时排除群主，避免重复插入 owner 成员关系。
	seen := make(map[string]struct{}, len(raw)+len(exclusions))
	for _, exclusion := range exclusions {
		userUUID := strings.TrimSpace(exclusion)
		if userUUID == "" {
			continue
		}
		seen[userUUID] = struct{}{}
	}

	userUUIDs := make([]string, 0, len(raw))
	for _, item := range raw {
		userUUID := strings.TrimSpace(item)
		if userUUID == "" {
			continue
		}
		// 同一请求内重复成员只保留第一份，写路径保持幂等且减少事务内判断成本。
		if _, exists := seen[userUUID]; exists {
			continue
		}
		seen[userUUID] = struct{}{}
		userUUIDs = append(userUUIDs, userUUID)
	}

	return userUUIDs
}

func buildUpdateGroupInfoFields(req *pb.UpdateGroupInfoRequest) (*string, *string, error) {
	var namePtr *string
	if rawName := req.GetName(); rawName != "" {
		// 传了 name 但 trim 后为空视为非法；完全不传则表示不更新 name。
		name := strings.TrimSpace(rawName)
		if name == "" {
			return nil, nil, apperr.New(consts.CodeParamError)
		}
		if len([]rune(name)) > 64 {
			return nil, nil, apperr.New(consts.CodeGroupNameTooLong)
		}
		namePtr = &name
	}

	var avatarPtr *string
	if rawAvatar := req.GetAvatar(); rawAvatar != "" {
		// avatar 目前只做 trim，不强校验 URL，避免 service 层过早绑定存储策略。
		avatar := strings.TrimSpace(rawAvatar)
		if avatar != "" {
			avatarPtr = &avatar
		}
	}

	return namePtr, avatarPtr, nil
}

func mapGroupWriteError(err error, fallbackMessage string) error {
	if err == nil {
		return nil
	}
	// 写路径统一在这里做 repository 语义错误到业务码的映射，避免各接口分散判断。
	if errors.Is(err, repository.ErrGroupDismissed) {
		return apperr.New(consts.CodeGroupAlreadyDismiss)
	}
	if errors.Is(err, repository.ErrRecordNotFound) {
		return apperr.New(consts.CodeGroupNotFound)
	}
	if errors.Is(err, repository.ErrNoPermission) {
		return apperr.New(consts.CodeNoPermission)
	}
	if errors.Is(err, repository.ErrCannotKickOwner) {
		return apperr.New(consts.CodeCannotKickOwner)
	}
	if errors.Is(err, repository.ErrCannotQuitAsOwner) {
		return apperr.New(consts.CodeCannotQuitAsOwner)
	}
	return apperr.Wrap(err, consts.CodeInternalError, fallbackMessage)
}

func buildCreateGroupMembers(groupUUID, ownerUUID string, memberUUIDs []string, now time.Time) []*model.GroupMember {
	members := make([]*model.GroupMember, 0, len(memberUUIDs)+1)
	if groupUUID == "" || ownerUUID == "" {
		return members
	}

	// 群主必须作为第一条有效成员关系写入，保证 member_cnt 与权限判断都有基础事实。
	members = append(members, &model.GroupMember{
		GroupUuid: groupUUID,
		UserUuid:  ownerUUID,
		Role:      2,
		Status:    0,
		JoinedAt:  now,
	})

	for _, userUUID := range memberUUIDs {
		if userUUID == "" {
			continue
		}
		// 普通初始成员记录邀请人，便于后续审计入群来源。
		members = append(members, &model.GroupMember{
			GroupUuid: groupUUID,
			UserUuid:  userUUID,
			Role:      0,
			Status:    0,
			Inviter:   ownerUUID,
			JoinedAt:  now,
		})
	}

	return members
}

func buildAddGroupMembers(groupUUID string, memberUUIDs []string) []*model.GroupMember {
	if groupUUID == "" || len(memberUUIDs) == 0 {
		return []*model.GroupMember{}
	}

	members := make([]*model.GroupMember, 0, len(memberUUIDs))
	for _, userUUID := range memberUUIDs {
		if userUUID == "" {
			continue
		}
		members = append(members, &model.GroupMember{
			GroupUuid: groupUUID,
			UserUuid:  userUUID,
		})
	}

	return members
}

func collectMemberUUIDs(members []*model.GroupMember) []string {
	if len(members) == 0 {
		return []string{}
	}

	userUUIDs := make([]string, 0, len(members))
	seen := make(map[string]struct{}, len(members))
	for _, member := range members {
		if member == nil || member.UserUuid == "" {
			continue
		}
		if _, exists := seen[member.UserUuid]; exists {
			continue
		}
		seen[member.UserUuid] = struct{}{}
		userUUIDs = append(userUUIDs, member.UserUuid)
	}

	return userUUIDs
}
