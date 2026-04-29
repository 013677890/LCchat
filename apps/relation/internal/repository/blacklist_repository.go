package repository

// TODO(Phase 2): 从 apps/user/internal/repository/blacklist_repository.go 迁移完整实现。

import (
	"context"

	"github.com/013677890/LCchat-Backend/model"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type blacklistRepositoryImpl struct {
	db          *gorm.DB
	redisClient *goredis.Client
}

func NewBlacklistRepository(db *gorm.DB, redisClient *goredis.Client) IBlacklistRepository {
	return &blacklistRepositoryImpl{db: db, redisClient: redisClient}
}

func (r *blacklistRepositoryImpl) AddBlacklist(ctx context.Context, userUUID, targetUUID string) error {
	panic("TODO: migrate from apps/user/internal/repository/blacklist_repository.go")
}

func (r *blacklistRepositoryImpl) RemoveBlacklist(ctx context.Context, userUUID, targetUUID string) error {
	panic("TODO: migrate")
}

func (r *blacklistRepositoryImpl) GetBlacklistList(ctx context.Context, userUUID string, page, pageSize int) ([]*model.UserRelation, int64, error) {
	panic("TODO: migrate")
}

func (r *blacklistRepositoryImpl) IsBlocked(ctx context.Context, userUUID, targetUUID string) (bool, error) {
	panic("TODO: migrate")
}

func (r *blacklistRepositoryImpl) GetBlacklistRelation(ctx context.Context, userUUID, targetUUID string) (*model.UserRelation, error) {
	panic("TODO: migrate")
}
