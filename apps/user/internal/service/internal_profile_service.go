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
//
// internal-profile 只暴露给其他服务最小资料视图：
//  1. 补建默认资料记录；
//  2. 批量返回用户卡片；
//  3. 批量返回公开资料，避免外部直接碰 profile 表。
func NewInternalProfileService(userRepo repository.IUserRepository) InternalProfileService {
	return &internalProfileServiceImpl{userRepo: userRepo}
}

// CreateProfile 创建或确认默认资料存在。
// 业务流程：
//  1. 校验请求中的 user_uuid；
//  2. 调用仓储层创建资料，若已存在则直接复用；
//  3. 返回空响应给上游，表示资料闭环已经完成。
//
// 错误码映射：
//   - codes.InvalidArgument: 参数错误
//   - codes.Internal: 系统内部错误
func (s *internalProfileServiceImpl) CreateProfile(ctx context.Context, req *pb.CreateProfileRequest) (*pb.CreateProfileResponse, error) {
	// internal-profile 的创建请求必须至少携带 user_uuid，否则无法建立资料主键。
	if req == nil || req.UserUuid == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	// 仓储层会负责“已存在直接复用 / 不存在时创建”的幂等语义。
	_, err := s.userRepo.CreateProfile(ctx, req.UserUuid, req.Nickname, req.Avatar)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "创建用户资料失败")
	}

	return &pb.CreateProfileResponse{}, nil
}

// BatchGetUserCard 批量获取用户卡片信息。
// 业务流程：
//  1. 校验 user_uuids 非空且不超过批量上限；
//  2. 调用仓储层批量查询资料快照；
//  3. 仅提取卡片场景需要的 uuid / nickname / avatar；
//  4. 返回内部服务可直接消费的卡片列表。
//
// 错误码映射：
//   - codes.InvalidArgument: 参数错误
//   - codes.Internal: 系统内部错误
func (s *internalProfileServiceImpl) BatchGetUserCard(ctx context.Context, req *pb.BatchGetUserCardRequest) (*pb.BatchGetUserCardResponse, error) {
	// 用户卡片是内部批量接口，因此同时限制空批和超大批量请求。
	if req == nil || len(req.UserUuids) == 0 || len(req.UserUuids) > 200 {
		return nil, apperr.New(consts.CodeParamError)
	}

	// 先批量拉取资料快照，再在 service 层裁成卡片所需字段。
	users, err := s.userRepo.BatchGetByUUIDs(ctx, req.UserUuids)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "批量获取用户卡片失败")
	}

	cards := make([]*pb.UserCard, 0, len(users))
	for _, user := range users {
		// 过滤空用户后再做 proto 转换，避免返回带 nil 元素的切片。
		if card := buildUserCardProto(user); card != nil {
			cards = append(cards, card)
		}
	}

	return &pb.BatchGetUserCardResponse{Cards: cards}, nil
}

// BatchGetPublicProfile 批量获取公开资料信息。
// 业务流程：
//  1. 校验 user_uuids 非空且不超过上限；
//  2. 批量查询 user_profile 快照；
//  3. 组装对外可见的公开资料字段；
//  4. 返回给 relation/msg 等内部调用方做聚合。
//
// 错误码映射：
//   - codes.InvalidArgument: 参数错误
//   - codes.Internal: 系统内部错误
func (s *internalProfileServiceImpl) BatchGetPublicProfile(ctx context.Context, req *pb.BatchGetPublicProfileRequest) (*pb.BatchGetPublicProfileResponse, error) {
	// 公开资料批量读取沿用较小上限，避免 relation/msg 一次性拉过多资料造成放大。
	if req == nil || len(req.UserUuids) == 0 || len(req.UserUuids) > 100 {
		return nil, apperr.New(consts.CodeParamError)
	}

	// 仓储层统一处理缓存与数据库回源；service 这里只负责公开资料字段的再组装。
	users, err := s.userRepo.BatchGetByUUIDs(ctx, req.UserUuids)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "批量获取公开资料失败")
	}

	profiles := make([]*pb.PublicProfile, 0, len(users))
	for _, user := range users {
		// 仅把存在的资料快照转成 public profile，跳过 nil 结果。
		if profile := buildPublicProfileProto(user); profile != nil {
			profiles = append(profiles, profile)
		}
	}

	return &pb.BatchGetPublicProfileResponse{Profiles: profiles}, nil
}
