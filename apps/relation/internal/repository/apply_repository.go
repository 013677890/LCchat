package repository

// TODO(Phase 2): 从 apps/user/internal/repository/apply_repository.go 迁移完整实现。

import (
	"context"

	"github.com/013677890/LCchat-Backend/model"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type applyRepositoryImpl struct {
	db          *gorm.DB
	redisClient *goredis.Client
}

func NewApplyRepository(db *gorm.DB, redisClient *goredis.Client) IApplyRepository {
	return &applyRepositoryImpl{db: db, redisClient: redisClient}
}

func (r *applyRepositoryImpl) Create(ctx context.Context, apply *model.ApplyRequest) (*model.ApplyRequest, error) {
	panic("TODO: migrate from apps/user/internal/repository/apply_repository.go")
}

func (r *applyRepositoryImpl) GetByID(ctx context.Context, id int64) (*model.ApplyRequest, error) {
	panic("TODO: migrate")
}

func (r *applyRepositoryImpl) GetPendingList(ctx context.Context, targetUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error) {
	panic("TODO: migrate")
}

func (r *applyRepositoryImpl) GetSentList(ctx context.Context, applicantUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error) {
	panic("TODO: migrate")
}

func (r *applyRepositoryImpl) UpdateStatus(ctx context.Context, id int64, status int, remark string) error {
	panic("TODO: migrate")
}

func (r *applyRepositoryImpl) AcceptApplyAndCreateRelation(ctx context.Context, applyId int64, userUUID, friendUUID, remark string) (bool, error) {
	panic("TODO: migrate")
}

func (r *applyRepositoryImpl) MarkAsRead(ctx context.Context, targetUUID string, ids []int64) (int64, error) {
	panic("TODO: migrate")
}

func (r *applyRepositoryImpl) MarkAllAsRead(ctx context.Context, targetUUID string) (int64, error) {
	panic("TODO: migrate")
}

func (r *applyRepositoryImpl) MarkAsReadAsync(ctx context.Context, ids []int64) {
	panic("TODO: migrate")
}

func (r *applyRepositoryImpl) GetUnreadCount(ctx context.Context, targetUUID string) (int64, error) {
	panic("TODO: migrate")
}

func (r *applyRepositoryImpl) ClearUnreadCount(ctx context.Context, targetUUID string) error {
	panic("TODO: migrate")
}

func (r *applyRepositoryImpl) ExistsPendingRequest(ctx context.Context, applicantUUID, targetUUID string) (bool, error) {
	panic("TODO: migrate")
}

func (r *applyRepositoryImpl) GetByIDWithInfo(ctx context.Context, id int64) (*model.ApplyRequest, error) {
	panic("TODO: migrate")
}
