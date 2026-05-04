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
//
// 当前 user-service 中保留的 group 只承担“群成员只读查询”职责：
//  1. 供 msg-service 校验发言权限；
//  2. 供 gateway / 其他内部调用链拉取成员清单；
//  3. 不扩展群管理写操作，保持四拆后的职责边界清晰。
func NewGroupService(groupRepo repository.IGroupRepository) GroupService {
	return &groupServiceImpl{groupRepo: groupRepo}
}

// GetGroupMembers 获取群组有效成员列表。
// 业务流程：
//  1. 校验请求中的 group_uuid；
//  2. 调用仓储层确认群存在并查询有效成员；
//  3. 将成员模型转换为只读群成员 proto；
//  4. 返回给 msg-service / gateway 等上游调用方。
//
// 错误码映射：
//   - codes.InvalidArgument: 参数错误
//   - codes.NotFound: 群不存在
//   - codes.Internal: 系统内部错误
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
