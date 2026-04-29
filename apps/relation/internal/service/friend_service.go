package service

// TODO(Phase 2): 从 apps/user/internal/service/friend_service.go 迁移完整实现。
// 当前文件是结构占位，service 接口和 handler 已就位。
// 迁移时需要：
// 1. 复制 friend_service.go 全部逻辑
// 2. 将 pb import 从 apps/user/pb 改为 apps/relation/pb
// 3. 将 repository import 从 apps/user/internal/repository 改为 apps/relation/internal/repository
// 4. 更新 PaginationInfo 引用到 commonpb.PaginationInfo

import (
	"context"

	pb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	"github.com/013677890/LCchat-Backend/apps/relation/internal/repository"

	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type friendServiceImpl struct {
	db          *gorm.DB
	redisClient *goredis.Client
	friendRepo  repository.IFriendRepository
	applyRepo   repository.IApplyRepository
}

func NewFriendService(
	db *gorm.DB,
	redisClient *goredis.Client,
	friendRepo repository.IFriendRepository,
	applyRepo repository.IApplyRepository,
) IFriendService {
	return &friendServiceImpl{
		db:          db,
		redisClient: redisClient,
		friendRepo:  friendRepo,
		applyRepo:   applyRepo,
	}
}

func (s *friendServiceImpl) SendFriendApply(ctx context.Context, req *pb.SendFriendApplyRequest) (*pb.SendFriendApplyResponse, error) {
	panic("TODO: migrate from apps/user/internal/service/friend_service.go")
}

func (s *friendServiceImpl) GetFriendApplyList(ctx context.Context, req *pb.GetFriendApplyListRequest) (*pb.GetFriendApplyListResponse, error) {
	panic("TODO: migrate from apps/user/internal/service/friend_service.go")
}

func (s *friendServiceImpl) GetSentApplyList(ctx context.Context, req *pb.GetSentApplyListRequest) (*pb.GetSentApplyListResponse, error) {
	panic("TODO: migrate from apps/user/internal/service/friend_service.go")
}

func (s *friendServiceImpl) HandleFriendApply(ctx context.Context, req *pb.HandleFriendApplyRequest) error {
	panic("TODO: migrate from apps/user/internal/service/friend_service.go")
}

func (s *friendServiceImpl) GetUnreadApplyCount(ctx context.Context, req *pb.GetUnreadApplyCountRequest) (*pb.GetUnreadApplyCountResponse, error) {
	panic("TODO: migrate from apps/user/internal/service/friend_service.go")
}

func (s *friendServiceImpl) MarkApplyAsRead(ctx context.Context, req *pb.MarkApplyAsReadRequest) error {
	panic("TODO: migrate from apps/user/internal/service/friend_service.go")
}

func (s *friendServiceImpl) GetFriendList(ctx context.Context, req *pb.GetFriendListRequest) (*pb.GetFriendListResponse, error) {
	panic("TODO: migrate from apps/user/internal/service/friend_service.go")
}

func (s *friendServiceImpl) SyncFriendList(ctx context.Context, req *pb.SyncFriendListRequest) (*pb.SyncFriendListResponse, error) {
	panic("TODO: migrate from apps/user/internal/service/friend_service.go")
}

func (s *friendServiceImpl) DeleteFriend(ctx context.Context, req *pb.DeleteFriendRequest) error {
	panic("TODO: migrate from apps/user/internal/service/friend_service.go")
}

func (s *friendServiceImpl) SetFriendRemark(ctx context.Context, req *pb.SetFriendRemarkRequest) error {
	panic("TODO: migrate from apps/user/internal/service/friend_service.go")
}

func (s *friendServiceImpl) SetFriendTag(ctx context.Context, req *pb.SetFriendTagRequest) error {
	panic("TODO: migrate from apps/user/internal/service/friend_service.go")
}

func (s *friendServiceImpl) GetTagList(ctx context.Context, req *pb.GetTagListRequest) (*pb.GetTagListResponse, error) {
	panic("TODO: migrate from apps/user/internal/service/friend_service.go")
}

func (s *friendServiceImpl) CheckIsFriend(ctx context.Context, req *pb.CheckIsFriendRequest) (*pb.CheckIsFriendResponse, error) {
	panic("TODO: migrate from apps/user/internal/service/friend_service.go")
}

func (s *friendServiceImpl) BatchCheckIsFriend(ctx context.Context, req *pb.BatchCheckIsFriendRequest) (*pb.BatchCheckIsFriendResponse, error) {
	panic("TODO: migrate from apps/user/internal/service/friend_service.go")
}

func (s *friendServiceImpl) GetRelationStatus(ctx context.Context, req *pb.GetRelationStatusRequest) (*pb.GetRelationStatusResponse, error) {
	panic("TODO: migrate from apps/user/internal/service/friend_service.go")
}
