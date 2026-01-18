package repository

import (
	"ChatServer/model"
	"context"
)

// ==================== 用户信息 Repository ====================

// UserRepository 用户数据访问接口
// 负责用户基础信息的 CRUD 操作
// 实现层集成了 Redis 缓存 + MySQL 持久化
type UserRepository interface {
	// GetByPhone 根据手机号查询用户信息
	GetByPhone(ctx context.Context, telephone string) (*model.UserInfo, error)

	// GetByUUID 根据UUID查询用户信息
	GetByUUID(ctx context.Context, uuid string) (*model.UserInfo, error)

	// Create 创建新用户
	Create(ctx context.Context, user *model.UserInfo) (*model.UserInfo, error)

	// Update 更新用户信息
	Update(ctx context.Context, user *model.UserInfo) (*model.UserInfo, error)

	// ExistsByPhone 检查手机号是否已存在
	ExistsByPhone(ctx context.Context, telephone string) (bool, error)

	// UpdateLastLogin 更新最后登录时间
	UpdateLastLogin(ctx context.Context, userUUID string) error

	// BatchGetByUUIDs 批量查询用户信息
	BatchGetByUUIDs(ctx context.Context, uuids []string) ([]*model.UserInfo, error)
}

// RelationRepository 好友关系数据访问接口
type RelationRepository interface {
	// GetFriendRelation 获取好友关系
	GetFriendRelation(ctx context.Context, userUUID, friendUUID string) (*model.UserRelation, error)

	// GetFriendList 获取好友列表
	GetFriendList(ctx context.Context, userUUID string, page, pageSize int) ([]*model.UserRelation, int64, error)

	// CreateFriendRelation 创建好友关系（双向）
	CreateFriendRelation(ctx context.Context, userUUID, friendUUID string) error

	// DeleteFriendRelation 删除好友关系（单向）
	DeleteFriendRelation(ctx context.Context, userUUID, friendUUID string) error

	// SetFriendRemark 设置好友备注
	SetFriendRemark(ctx context.Context, userUUID, friendUUID, remark string) error

	// BlockUser 拉黑用户
	BlockUser(ctx context.Context, userUUID, targetUUID string) error

	// UnblockUser 解除拉黑
	UnblockUser(ctx context.Context, userUUID, targetUUID string) error

	// GetBlacklist 获取黑名单列表
	GetBlacklist(ctx context.Context, userUUID string, page, pageSize int) ([]*model.UserRelation, int64, error)

	// IsBlocked 检查是否被拉黑
	IsBlocked(ctx context.Context, userUUID, targetUUID string) (bool, error)

	// IsFriend 检查是否是好友
	IsFriend(ctx context.Context, userUUID, friendUUID string) (bool, error)
}

// ApplyRequestRepository 好友申请数据访问接口
type ApplyRequestRepository interface {
	// Create 创建好友申请
	Create(ctx context.Context, apply *model.ApplyRequest) (*model.ApplyRequest, error)

	// GetByID 根据ID获取好友申请
	GetByID(ctx context.Context, id int64) (*model.ApplyRequest, error)

	// GetPendingList 获取待处理的好友申请列表
	GetPendingList(ctx context.Context, targetUUID string, page, pageSize int) ([]*model.ApplyRequest, int64, error)

	// UpdateStatus 更新申请状态
	UpdateStatus(ctx context.Context, id int64, status int, remark string) error

	// ExistsPendingRequest 检查是否存在待处理的申请
	ExistsPendingRequest(ctx context.Context, applicantUUID, targetUUID string) (bool, error)
}

// DeviceSessionRepository 设备会话数据访问接口
type DeviceSessionRepository interface {
	// Create 创建设备会话
	Create(ctx context.Context, session *model.DeviceSession) error

	// GetByUserUUID 获取用户的所有设备会话
	GetByUserUUID(ctx context.Context, userUUID string) ([]*model.DeviceSession, error)

	// GetByDeviceID 根据设备ID获取会话
	GetByDeviceID(ctx context.Context, userUUID, deviceID string) (*model.DeviceSession, error)

	// UpdateOnlineStatus 更新在线状态
	UpdateOnlineStatus(ctx context.Context, userUUID, deviceID string, status int) error

	// UpdateLastSeen 更新最后活跃时间
	UpdateLastSeen(ctx context.Context, userUUID, deviceID string) error

	// Delete 删除设备会话
	Delete(ctx context.Context, userUUID, deviceID string) error

	// GetOnlineDevices 获取在线设备列表
	GetOnlineDevices(ctx context.Context, userUUID string) ([]*model.DeviceSession, error)

	// BatchGetOnlineStatus 批量获取用户在线状态
	BatchGetOnlineStatus(ctx context.Context, userUUIDs []string) (map[string][]*model.DeviceSession, error)
}
