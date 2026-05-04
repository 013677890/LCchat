package service

import (
	"context"
	"errors"

	"github.com/013677890/LCchat-Backend/apps/user/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
)

// groupServiceImpl 群组服务实现。
type groupServiceImpl struct {
	groupRepo repository.IGroupRepository
}

// NewGroupService 创建群组服务实例。
func NewGroupService(groupRepo repository.IGroupRepository) GroupService {
	return &groupServiceImpl{groupRepo: groupRepo}
}

// GetGroupMembers 获取群组有效成员列表。
func (s *groupServiceImpl) GetGroupMembers(ctx context.Context, req *pb.GetGroupMembersRequest) (*pb.GetGroupMembersResponse, error) {
	if req == nil || req.GroupUuid == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	members, err := s.groupRepo.GetGroupMembers(ctx, req.GroupUuid)
	if err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return nil, apperr.New(consts.CodeGroupNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取群成员失败")
	}

	items := make([]*pb.GroupMemberItem, 0, len(members))
	for _, member := range members {
		if item := buildGroupMemberItemProto(member); item != nil {
			items = append(items, item)
		}
	}

	return &pb.GetGroupMembersResponse{Members: items}, nil
}
