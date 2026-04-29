package model

import (
	"time"

	"gorm.io/gorm"
)

// UserAccount 用户账号表（auth-service 归属）
// 存储认证凭证、账号状态、登录展示冗余字段。
// login_nickname / login_avatar 是冗余字段，权威数据在 user_profile。
type UserAccount struct {
	Id            int64          `gorm:"column:id;primaryKey;comment:自增id"`
	UserUuid      string         `gorm:"column:user_uuid;uniqueIndex;type:char(20);not null;comment:用户唯一id"`
	Email         string         `gorm:"column:email;uniqueIndex;type:varchar(100);not null;comment:邮箱"`
	Telephone     string         `gorm:"column:telephone;uniqueIndex;type:varchar(20);default:null;comment:手机号"`
	PasswordHash  string         `gorm:"column:password_hash;type:char(60);not null;comment:bcrypt密码哈希"`
	Status        int8           `gorm:"column:status;not null;default:0;comment:状态,0=正常 1=注销"`
	IsAdmin       int8           `gorm:"column:is_admin;not null;default:0;comment:是否管理员"`
	LoginNickname string         `gorm:"column:login_nickname;type:varchar(20);not null;default:'';comment:登录展示昵称(冗余)"`
	LoginAvatar   string         `gorm:"column:login_avatar;type:varchar(255);not null;default:'';comment:登录展示头像(冗余)"`
	LastLoginAt   *time.Time     `gorm:"column:last_login_at;comment:最近登录时间"`
	CreatedAt     time.Time      `gorm:"column:created_at;not null;comment:创建时间"`
	UpdatedAt     time.Time      `gorm:"column:updated_at;not null;comment:更新时间"`
	DeletedAt     gorm.DeletedAt `gorm:"column:deleted_at;comment:注销时间"`
}

func (UserAccount) TableName() string {
	return "user_account"
}
