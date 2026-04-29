package repository

// TODO(Phase 2): 从 apps/user/internal/repository/friend_repository.go 迁移完整实现。
// Repository 层不引用 pb 类型，只使用 model 类型，因此迁移相对简单：
// 1. 复制 friend_repository.go 全部逻辑
// 2. 将 package 从 repository (apps/user) 改为 repository (apps/relation)
// 3. 更新 import 中的内部引用（如 errors.go, util.go, lua_scripts.go）

import (
	"context"

	"github.com/013677890/LCchat-Backend/model"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type friendRepositoryImpl struct {
	db          *gorm.DB
	redisClient *goredis.Client
}

func NewFriendRepository(db *gorm.DB, redisClient *goredis.Client) IFriendRepository {
	return &friendRepositoryImpl{db: db, redisClient: redisClient}
}

func (r *friendRepositoryImpl) GetFriendList(ctx context.Context, userUUID, groupTag string, page, pageSize int) ([]*model.UserRelation, int64, int64, error) {
	panic("TODO: migrate from apps/user/internal/repository/friend_repository.go")
}

func (r *friendRepositoryImpl) GetFriendRelation(ctx context.Context, userUUID, friendUUID string) (*model.UserRelation, error) {
	panic("TODO: migrate")
}

func (r *friendRepositoryImpl) CreateFriendRelation(ctx context.Context, userUUID, friendUUID string) error {
	panic("TODO: migrate")
}

func (r *friendRepositoryImpl) DeleteFriendRelation(ctx context.Context, userUUID, friendUUID string) error {
	panic("TODO: migrate")
}

func (r *friendRepositoryImpl) SetFriendRemark(ctx context.Context, userUUID, friendUUID, remark string) error {
	panic("TODO: migrate")
}

func (r *friendRepositoryImpl) SetFriendTag(ctx context.Context, userUUID, friendUUID, groupTag string) error {
	panic("TODO: migrate")
}

func (r *friendRepositoryImpl) GetTagList(ctx context.Context, userUUID string) ([]string, error) {
	panic("TODO: migrate")
}

func (r *friendRepositoryImpl) IsFriend(ctx context.Context, userUUID, friendUUID string) (bool, error) {
	panic("TODO: migrate")
}

func (r *friendRepositoryImpl) CheckIsFriendRelation(ctx context.Context, userUUID, peerUUID string) (bool, error) {
	panic("TODO: migrate")
}

func (r *friendRepositoryImpl) BatchCheckIsFriend(ctx context.Context, userUUID string, peerUUIDs []string) (map[string]bool, error) {
	panic("TODO: migrate")
}

func (r *friendRepositoryImpl) GetRelationStatus(ctx context.Context, userUUID, peerUUID string) (*model.UserRelation, error) {
	panic("TODO: migrate")
}

func (r *friendRepositoryImpl) SyncFriendList(ctx context.Context, userUUID string, version int64, limit int) ([]*model.UserRelation, int64, bool, error) {
	panic("TODO: migrate")
}
