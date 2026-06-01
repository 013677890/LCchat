package repository

import (
	"context"

	"github.com/013677890/LCchat-Backend/model"
)

// ==================== 好友关系 Repository ====================

// IFriendRepository 好友关系数据访问接口
type IFriendRepository interface {
	GetFriendList(ctx context.Context, userUUID, groupTag string, page, pageSize int) ([]*model.UserRelation, int64, int64, error)
	GetFriendRelation(ctx context.Context, userUUID, friendUUID string) (*model.UserRelation, error)
	CreateFriendRelation(ctx context.Context, userUUID, friendUUID string) error
	DeleteFriendRelation(ctx context.Context, userUUID, friendUUID string) error
	CleanupAccountRelations(ctx context.Context, userUUID string) error
	SetFriendRemark(ctx context.Context, userUUID, friendUUID, remark string) error
	SetFriendTag(ctx context.Context, userUUID, friendUUID, groupTag string) error
	IsFriend(ctx context.Context, userUUID, friendUUID string) (bool, error)
	CheckIsFriendRelation(ctx context.Context, userUUID, peerUUID string) (bool, error)
	BatchCheckIsFriend(ctx context.Context, userUUID string, peerUUIDs []string) (map[string]bool, error)
	GetRelationStatus(ctx context.Context, userUUID, peerUUID string) (*model.UserRelation, error)
	SyncFriendList(ctx context.Context, userUUID string, cursor FriendSyncCursor, limit int) ([]*model.UserRelation, FriendSyncCursor, bool, error)
}

// ==================== 好友申请 Repository ====================

// IApplyRepository 好友申请数据访问接口
type IApplyRepository interface {
	Create(ctx context.Context, apply *model.ApplyRequest) (*model.ApplyRequest, error)
	GetByID(ctx context.Context, id int64) (*model.ApplyRequest, error)
	GetPendingList(ctx context.Context, targetUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error)
	GetSentList(ctx context.Context, applicantUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error)
	CleanupAccountApplies(ctx context.Context, userUUID string) error
	UpdateStatus(ctx context.Context, id int64, status int, remark string) error
	AcceptApplyAndCreateRelation(ctx context.Context, applyId int64, userUUID, friendUUID, remark string) (alreadyProcessed bool, err error)
	MarkAsRead(ctx context.Context, targetUUID string, ids []int64) (int64, error)
	MarkAllAsRead(ctx context.Context, targetUUID string) (int64, error)
	MarkAsReadAsync(ctx context.Context, ids []int64)
	GetUnreadCount(ctx context.Context, targetUUID string) (int64, error)
	ClearUnreadCount(ctx context.Context, targetUUID string) error
	ExistsPendingRequest(ctx context.Context, applicantUUID, targetUUID string) (bool, error)
	GetByIDWithInfo(ctx context.Context, id int64) (*model.ApplyRequest, error)
}

// ==================== 黑名单 Repository ====================

// IBlacklistRepository 黑名单数据访问接口
type IBlacklistRepository interface {
	AddBlacklist(ctx context.Context, userUUID, targetUUID string) error
	RemoveBlacklist(ctx context.Context, userUUID, targetUUID string) error
	GetBlacklistList(ctx context.Context, userUUID string, page, pageSize int) ([]*model.UserRelation, int64, error)
	IsBlocked(ctx context.Context, userUUID, targetUUID string) (bool, error)
	GetBlacklistRelation(ctx context.Context, userUUID, targetUUID string) (*model.UserRelation, error)
}
