package repository

import (
	"context"
	"time"

	"github.com/013677890/LCchat-Backend/model"
)

// IUserRepository 资料域数据访问接口。
type IUserRepository interface {
	// GetByUUID 根据 UUID 查询用户资料。
	GetByUUID(ctx context.Context, uuid string) (*model.UserProfile, error)

	// CreateProfile 创建或确认用户资料存在。
	CreateProfile(ctx context.Context, userUUID, nickname, avatar string) (*model.UserProfile, error)

	// BatchGetByUUIDs 批量查询用户资料。
	BatchGetByUUIDs(ctx context.Context, uuids []string) ([]*model.UserProfile, error)

	// UpdateAvatarWithDisplayEvent 更新头像并在同一事务中写入展示字段变更事件。
	UpdateAvatarWithDisplayEvent(ctx context.Context, userUUID, avatar string) (*model.UserProfile, error)

	// UpdateBasicInfoWithDisplayEvent 更新基本信息并在同一事务中写入展示字段变更事件。
	UpdateBasicInfoWithDisplayEvent(ctx context.Context, userUUID string, nickname, signature, birthday string, gender int8) (*model.UserProfile, error)

	// Delete 软删除用户（注销账号）
	Delete(ctx context.Context, userUUID string) error

	// SaveQRCode 保存用户二维码
	// 将二维码 token 与用户 UUID 的映射关系存储到 Redis
	// 同时保存反向映射: user:qrcode:user:{userUUID} -> token
	// 过期时间: 48小时
	SaveQRCode(ctx context.Context, userUUID, token string) error

	// GetUUIDByQRCodeToken 根据 token 获取用户 UUID
	GetUUIDByQRCodeToken(ctx context.Context, token string) (string, error)

	// GetQRCodeTokenByUserUUID 根据用户 UUID 获取二维码 token
	GetQRCodeTokenByUserUUID(ctx context.Context, userUUID string) (string, time.Time, error)

	// SearchUser 按昵称前缀或 user_uuid 前缀搜索资料。
	SearchUser(ctx context.Context, keyword string, page, pageSize int) ([]*model.UserProfile, int64, error)
}
