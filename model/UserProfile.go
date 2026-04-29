package model

import (
	"time"
)

// UserProfile 用户资料表（user-service 归属）
// 存储用户公开资料信息。nickname / avatar 是权威数据。
type UserProfile struct {
	Id          int64      `gorm:"column:id;primaryKey;comment:自增id"`
	UserUuid    string     `gorm:"column:user_uuid;uniqueIndex;type:char(20);not null;comment:用户唯一id"`
	Nickname    string     `gorm:"column:nickname;type:varchar(20);not null;default:'';comment:昵称"`
	Avatar      string     `gorm:"column:avatar;type:varchar(255);not null;default:'';comment:头像URL"`
	Gender      int8       `gorm:"column:gender;not null;default:3;comment:性别,1=男 2=女 3=未知"`
	Birthday    *time.Time `gorm:"column:birthday;type:date;default:null;comment:生日"`
	Signature   string     `gorm:"column:signature;type:varchar(100);not null;default:'';comment:个性签名"`
	QrcodeToken string     `gorm:"column:qrcode_token;type:varchar(64);default:null;comment:二维码token"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null;comment:创建时间"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;not null;comment:更新时间"`
}

func (UserProfile) TableName() string {
	return "user_profile"
}
