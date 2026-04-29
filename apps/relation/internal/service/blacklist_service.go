package service

// TODO(Phase 2): 从 apps/user/internal/service/blacklist_service.go 迁移完整实现。

import (
	"context"

	pb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	"github.com/013677890/LCchat-Backend/apps/relation/internal/repository"

	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type blacklistServiceImpl struct {
	db             *gorm.DB
	redisClient    *goredis.Client
	blacklistRepo  repository.IBlacklistRepository
	friendRepo     repository.IFriendRepository
}

func NewBlacklistService(
	db *gorm.DB,
	redisClient *goredis.Client,
	blacklistRepo repository.IBlacklistRepository,
	friendRepo repository.IFriendRepository,
) IBlacklistService {
	return &blacklistServiceImpl{
		db:            db,
		redisClient:   redisClient,
		blacklistRepo: blacklistRepo,
		friendRepo:    friendRepo,
	}
}

func (s *blacklistServiceImpl) AddBlacklist(ctx context.Context, req *pb.AddBlacklistRequest) error {
	panic("TODO: migrate from apps/user/internal/service/blacklist_service.go")
}

func (s *blacklistServiceImpl) RemoveBlacklist(ctx context.Context, req *pb.RemoveBlacklistRequest) error {
	panic("TODO: migrate from apps/user/internal/service/blacklist_service.go")
}

func (s *blacklistServiceImpl) GetBlacklistList(ctx context.Context, req *pb.GetBlacklistListRequest) (*pb.GetBlacklistListResponse, error) {
	panic("TODO: migrate from apps/user/internal/service/blacklist_service.go")
}

func (s *blacklistServiceImpl) CheckIsBlacklist(ctx context.Context, req *pb.CheckIsBlacklistRequest) (*pb.CheckIsBlacklistResponse, error) {
	panic("TODO: migrate from apps/user/internal/service/blacklist_service.go")
}
