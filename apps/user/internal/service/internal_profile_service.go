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

	_, err := s.userRepo.CreateProfile(ctx, req.UserUuid, req.Nickname, req.Avatar)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "创建用户资料失败")
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
		if card := buildUserCardProto(user); card != nil {
			cards = append(cards, card)
		}
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
		if profile := buildPublicProfileProto(user); profile != nil {
			profiles = append(profiles, profile)
		}
	}

	return &pb.BatchGetPublicProfileResponse{Profiles: profiles}, nil
}
