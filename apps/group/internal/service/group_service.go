package service

import (
	"context"
	"errors"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/group/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/util"
)

// groupServiceImpl 是 group-service 业务层实现。
//
// 当前阶段策略是“先读后写”：
//  1. 只读查询链路（群资料、成员、群列表、成员 ID）先落地，便于 msg / gateway 尽快复用；
//  2. 写操作仍然保持占位，避免在群管理规则尚未定稿前过早固化实现；
//  3. 所有对外错误都在 service 层统一做语义映射，避免上游直接感知 repository 细节。
//
// 这样既能让 group-service 尽快形成“可被调用”的内部能力，
// 也不会影响后续继续扩展群创建、成员管理、审批、角色控制等写路径。
type groupServiceImpl struct {
	groupRepo repository.IGroupRepository
}

// NewGroupService 创建 group 服务实例。
//
// 当前只保存仓储依赖，不做额外初始化动作；
// 一方面保持构造函数足够薄，另一方面也避免把未来尚未确认的副作用提前塞进骨架层。
func NewGroupService(groupRepo repository.IGroupRepository) IGroupService {
	return &groupServiceImpl{groupRepo: groupRepo}
}

// CreateGroup 创建群。
func (s *groupServiceImpl) CreateGroup(ctx context.Context, req *pb.CreateGroupRequest) (*pb.CreateGroupResponse, error) {
	return nil, notImplemented(ctx, "创建群")
}

// DismissGroup 解散群。
func (s *groupServiceImpl) DismissGroup(ctx context.Context, req *pb.DismissGroupRequest) error {
	return notImplemented(ctx, "解散群")
}

// GetGroupInfo 获取群资料。
func (s *groupServiceImpl) GetGroupInfo(ctx context.Context, req *pb.GetGroupInfoRequest) (*pb.GetGroupInfoResponse, error) {
	if req == nil || req.GetGroupUuid() == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	groupInfo, err := s.groupRepo.GetGroupInfo(ctx, req.GetGroupUuid())
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
	return notImplemented(ctx, "更新群资料")
}

// AddMember 添加群成员。
func (s *groupServiceImpl) AddMember(ctx context.Context, req *pb.AddMemberRequest) error {
	return notImplemented(ctx, "添加群成员")
}

// RemoveMember 移除群成员。
func (s *groupServiceImpl) RemoveMember(ctx context.Context, req *pb.RemoveMemberRequest) error {
	return notImplemented(ctx, "移除群成员")
}

// GetMemberList 获取群成员列表。
func (s *groupServiceImpl) GetMemberList(ctx context.Context, req *pb.GetMemberListRequest) (*pb.GetMemberListResponse, error) {
	if req == nil || req.GetGroupUuid() == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	members, err := s.groupRepo.GetGroupMembers(ctx, req.GetGroupUuid())
	if err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return nil, apperr.New(consts.CodeGroupNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取群成员列表失败")
	}

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
	if req == nil || req.GetGroupUuid() == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	members, err := s.groupRepo.GetGroupMembers(ctx, req.GetGroupUuid())
	if err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return nil, apperr.New(consts.CodeGroupNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取群成员 ID 列表失败")
	}

	return &pb.GetGroupMemberIdsResponse{UserUuids: collectMemberUUIDs(members)}, nil
}

// collectMemberUUIDs 从成员列表中提取去重后的用户 UUID。
//
// service 层既要给仓储批量查资料，也要直接返回成员 UUID 列表；
// 把这段去重逻辑抽出来，可以避免两处手写重复循环。
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

// notImplemented 统一返回“暂未实现”的占位错误。
//
// 这里复用 CodeMethodNotAllowed，而不是伪造领域错误码，原因是：
//  1. 当前不是业务失败，而是能力尚未交付；
//  2. 上游可以明确识别这是“接口边界已预留，但实现未开放”；
//  3. 等真实逻辑落地后，再替换为精确的领域错误，不会污染当前骨架阶段的语义。
func notImplemented(_ context.Context, action string) error {
	return apperr.NewWithMessage(consts.CodeMethodNotAllowed, action+"功能暂未实现")
}
