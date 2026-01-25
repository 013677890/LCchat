package repository

import (
	"ChatServer/model"
	"context"
	"time"
)

// ==================== 认证相关 Repository ====================

// IAuthRepository 认证相关数据访问接口
type IAuthRepository interface {
	// GetByPhone 根据手机号查询用户信息
	GetByPhone(ctx context.Context, telephone string) (*model.UserInfo, error)

	// GetByEmail 根据邮箱查询用户信息
	GetByEmail(ctx context.Context, email string) (*model.UserInfo, error)

	// ExistsByPhone 检查手机号是否已存在
	ExistsByPhone(ctx context.Context, telephone string) (bool, error)

	// ExistsByEmail 检查邮箱是否已存在
	ExistsByEmail(ctx context.Context, email string) (bool, error)

	// Create 创建新用户
	Create(ctx context.Context, user *model.UserInfo) (*model.UserInfo, error)

	// VerifyVerifyCode 校验验证码
	// type: 验证码类型 (1:注册 2:登录 3:重置密码 4:换绑邮箱)
	VerifyVerifyCode(ctx context.Context, email, verifyCode string, codeType int32) (bool, error)

	// StoreVerifyCode 存储验证码到Redis（带过期时间）
	// type: 验证码类型 (1:注册 2:登录 3:重置密码 4:换绑邮箱)
	StoreVerifyCode(ctx context.Context, email, verifyCode string, codeType int32, expireDuration time.Duration) error

	// DeleteVerifyCode 删除验证码（消耗验证码）
	// type: 验证码类型 (1:注册 2:登录 3:重置密码 4:换绑邮箱)
	DeleteVerifyCode(ctx context.Context, email string, codeType int32) error

	// UpdateLastLogin 更新最后登录时间
	UpdateLastLogin(ctx context.Context, userUUID string) error

	// UpdatePassword 更新密码
	UpdatePassword(ctx context.Context, userUUID, password string) error

	// VerifyVerifyCodeRateLimit 验证码限流校验
	// 返回值: true=触发限流(不允许发送), false=未触发限流(允许发送)
	VerifyVerifyCodeRateLimit(ctx context.Context, email string, ip string) (bool, error)

	// IncrementVerifyCodeCount 递增验证码发送计数（发送验证码时调用）
	IncrementVerifyCodeCount(ctx context.Context, email string, ip string) error
}

// ==================== 用户信息 Repository ====================

// IUserRepository 用户信息数据访问接口
type IUserRepository interface {
	// GetByUUID 根据UUID查询用户信息
	GetByUUID(ctx context.Context, uuid string) (*model.UserInfo, error)

	// GetByPhone 根据手机号查询用户信息
	GetByPhone(ctx context.Context, telephone string) (*model.UserInfo, error)

	// BatchGetByUUIDs 批量查询用户信息
	BatchGetByUUIDs(ctx context.Context, uuids []string) ([]*model.UserInfo, error)

	// Update 更新用户信息
	Update(ctx context.Context, user *model.UserInfo) (*model.UserInfo, error)

	// UpdateAvatar 更新用户头像
	UpdateAvatar(ctx context.Context, userUUID, avatar string) error

	// UpdateBasicInfo 更新基本信息（昵称、性别、生日、签名）
	UpdateBasicInfo(ctx context.Context, userUUID string, nickname, signature, birthday string, gender int8) error

	// UpdateEmail 更新邮箱
	UpdateEmail(ctx context.Context, userUUID, email string) error

	// UpdateTelephone 更新手机号
	UpdateTelephone(ctx context.Context, userUUID, telephone string) error

	// Delete 软删除用户（注销账号）
	Delete(ctx context.Context, userUUID string) error

	// ExistsByPhone 检查手机号是否已存在
	ExistsByPhone(ctx context.Context, telephone string) (bool, error)

	// ExistsByEmail 检查邮箱是否已存在
	ExistsByEmail(ctx context.Context, email string) (bool, error)
}

// ==================== 好友关系 Repository ====================

// IFriendRepository 好友关系数据访问接口
type IFriendRepository interface {
	// SearchUser 搜索用户（按手机号或昵称）
	SearchUser(ctx context.Context, keyword string, page, pageSize int) ([]*model.UserInfo, int64, error)

	// GetFriendList 获取好友列表
	GetFriendList(ctx context.Context, userUUID, groupTag string, page, pageSize int) ([]*model.UserRelation, int64, error)

	// GetFriendRelation 获取好友关系
	GetFriendRelation(ctx context.Context, userUUID, friendUUID string) (*model.UserRelation, error)

	// CreateFriendRelation 创建好友关系（双向）
	CreateFriendRelation(ctx context.Context, userUUID, friendUUID string) error

	// DeleteFriendRelation 删除好友关系（单向）
	DeleteFriendRelation(ctx context.Context, userUUID, friendUUID string) error

	// SetFriendRemark 设置好友备注
	SetFriendRemark(ctx context.Context, userUUID, friendUUID, remark string) error

	// SetFriendTag 设置好友标签
	SetFriendTag(ctx context.Context, userUUID, friendUUID, groupTag string) error

	// GetTagList 获取标签列表
	GetTagList(ctx context.Context, userUUID string) ([]string, error)

	// IsFriend 检查是否是好友
	IsFriend(ctx context.Context, userUUID, friendUUID string) (bool, error)

	// GetRelationStatus 获取关系状态
	GetRelationStatus(ctx context.Context, userUUID, peerUUID string) (*model.UserRelation, error)

	// SyncFriendList 增量同步好友列表
	SyncFriendList(ctx context.Context, userUUID string, version int64, limit int) ([]*model.UserRelation, int64, error)
}

// ==================== 好友申请 Repository ====================

// IApplyRepository 好友申请数据访问接口
type IApplyRepository interface {
	// Create 创建好友申请
	Create(ctx context.Context, apply *model.ApplyRequest) (*model.ApplyRequest, error)

	// GetByID 根据ID获取好友申请
	GetByID(ctx context.Context, id int64) (*model.ApplyRequest, error)

	// GetPendingList 获取待处理的好友申请列表
	GetPendingList(ctx context.Context, targetUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error)

	// GetSentList 获取发出的好友申请列表
	GetSentList(ctx context.Context, applicantUUID string, status, page, pageSize int) ([]*model.ApplyRequest, int64, error)

	// UpdateStatus 更新申请状态
	UpdateStatus(ctx context.Context, id int64, status int, remark string) error

	// MarkAsRead 标记申请已读
	MarkAsRead(ctx context.Context, ids []int64) error

	// GetUnreadCount 获取未读申请数量
	GetUnreadCount(ctx context.Context, targetUUID string) (int64, error)

	// ExistsPendingRequest 检查是否存在待处理的申请
	ExistsPendingRequest(ctx context.Context, applicantUUID, targetUUID string) (bool, error)

	// GetByIDWithInfo 根据ID获取好友申请（包含申请人信息）
	GetByIDWithInfo(ctx context.Context, id int64) (*model.ApplyRequest, *model.UserInfo, error)
}

// ==================== 黑名单 Repository ====================

// IBlacklistRepository 黑名单数据访问接口
type IBlacklistRepository interface {
	// AddBlacklist 拉黑用户
	AddBlacklist(ctx context.Context, userUUID, targetUUID string) error

	// RemoveBlacklist 取消拉黑
	RemoveBlacklist(ctx context.Context, userUUID, targetUUID string) error

	// GetBlacklistList 获取黑名单列表
	GetBlacklistList(ctx context.Context, userUUID string, page, pageSize int) ([]*model.UserRelation, int64, error)

	// IsBlocked 检查是否被拉黑
	IsBlocked(ctx context.Context, userUUID, targetUUID string) (bool, error)

	// GetBlacklistRelation 获取拉黑关系
	GetBlacklistRelation(ctx context.Context, userUUID, targetUUID string) (*model.UserRelation, error)
}

// ==================== 设备会话 Repository ====================

// IDeviceRepository 设备会话数据访问接口
type IDeviceRepository interface {
	// Create 创建设备会话
	Create(ctx context.Context, session *model.DeviceSession) error

	// GetByUserUUID 获取用户的所有设备会话
	GetByUserUUID(ctx context.Context, userUUID string) ([]*model.DeviceSession, error)

	// GetByDeviceID 根据设备ID获取会话
	GetByDeviceID(ctx context.Context, userUUID, deviceID string) (*model.DeviceSession, error)

	// UpsertSession 创建或更新设备会话（Upsert）
	UpsertSession(ctx context.Context, session *model.DeviceSession) error

	// UpdateOnlineStatus 更新在线状态
	UpdateOnlineStatus(ctx context.Context, userUUID, deviceID string, status int8) error

	// UpdateLastSeen 更新最后活跃时间
	UpdateLastSeen(ctx context.Context, userUUID, deviceID string) error

	// Delete 删除设备会话
	Delete(ctx context.Context, userUUID, deviceID string) error

	// GetOnlineDevices 获取在线设备列表
	GetOnlineDevices(ctx context.Context, userUUID string) ([]*model.DeviceSession, error)

	// BatchGetOnlineStatus 批量获取用户在线状态
	BatchGetOnlineStatus(ctx context.Context, userUUIDs []string) (map[string][]*model.DeviceSession, error)

	// UpdateToken 更新Token
	UpdateToken(ctx context.Context, userUUID, deviceID, token, refreshToken string, expireAt *time.Time) error

	// DeleteByUserUUID 删除用户所有设备会话（登出所有设备）
	DeleteByUserUUID(ctx context.Context, userUUID string) error

	// ==================== Redis Token 管理 ====================

	// StoreAccessToken 将 AccessToken 存入 Redis
	StoreAccessToken(ctx context.Context, userUUID, deviceID, accessToken string, expireDuration time.Duration) error

	// StoreRefreshToken 将 RefreshToken 存入 Redis
	StoreRefreshToken(ctx context.Context, userUUID, deviceID, refreshToken string, expireDuration time.Duration) error

	// VerifyAccessToken 验证 AccessToken 是否有效
	VerifyAccessToken(ctx context.Context, userUUID, deviceID, accessToken string) (bool, error)

	// GetRefreshToken 获取 RefreshToken
	GetRefreshToken(ctx context.Context, userUUID, deviceID string) (string, error)

	// DeleteTokens 删除设备的所有 Token（用于踢出设备）
	DeleteTokens(ctx context.Context, userUUID, deviceID string) error
}
