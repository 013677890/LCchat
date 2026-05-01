package service

import (
	"context"

	"github.com/013677890/LCchat-Backend/apps/user/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
)

// internalProfileServiceImpl 实现面向内部服务的最小资料视图能力。
type internalProfileServiceImpl struct {
	userRepo repository.IUserRepository
}

// NewInternalProfileService 创建内部资料服务实例。
func NewInternalProfileService(userRepo repository.IUserRepository) InternalProfileService {
	return &internalProfileServiceImpl{userRepo: userRepo}
}

// CreateProfile 创建或确认默认资料存在。
func (s *internalProfileServiceImpl) CreateProfile(ctx context.Context, req *pb.CreateProfileRequest) (*pb.CreateProfileResponse, error) {
	if req == nil || req.UserUuid == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	// 当前迁移阶段仍由注册流程直接写入 user_info，因此这里只做存在性确认。
	userInfo, err := s.userRepo.GetByUUID(ctx, req.UserUuid)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "确认用户资料失败")
	}
	if userInfo == nil {
		return nil, apperr.New(consts.CodeUserNotFound)
	}

	return &pb.CreateProfileResponse{}, nil
}

// BatchGetUserCard 批量获取用户卡片信息。
func (s *internalProfileServiceImpl) BatchGetUserCard(ctx context.Context, req *pb.BatchGetUserCardRequest) (*pb.BatchGetUserCardResponse, error) {
	if req == nil || len(req.UserUuids) == 0 || len(req.UserUuids) > 200 {
		return nil, apperr.New(consts.CodeParamError)
	}

	users, err := s.userRepo.BatchGetByUUIDs(ctx, req.UserUuids)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "批量获取用户卡片失败")
	}

	cards := make([]*pb.UserCard, 0, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}
		cards = append(cards, &pb.UserCard{
			Uuid:     user.Uuid,
			Nickname: user.Nickname,
			Avatar:   user.Avatar,
		})
	}

	return &pb.BatchGetUserCardResponse{Cards: cards}, nil
}

// BatchGetPublicProfile 批量获取公开资料信息。
func (s *internalProfileServiceImpl) BatchGetPublicProfile(ctx context.Context, req *pb.BatchGetPublicProfileRequest) (*pb.BatchGetPublicProfileResponse, error) {
	if req == nil || len(req.UserUuids) == 0 || len(req.UserUuids) > 100 {
		return nil, apperr.New(consts.CodeParamError)
	}

	users, err := s.userRepo.BatchGetByUUIDs(ctx, req.UserUuids)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "批量获取公开资料失败")
	}

	profiles := make([]*pb.PublicProfile, 0, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}
		profiles = append(profiles, &pb.PublicProfile{
			Uuid:      user.Uuid,
			Nickname:  user.Nickname,
			Avatar:    user.Avatar,
			Gender:    int32(user.Gender),
			Signature: user.Signature,
		})
	}

	return &pb.BatchGetPublicProfileResponse{Profiles: profiles}, nil
}
