package model

import (
	"time"
	"gorm.io/gorm"
)

// DeviceSession 设备会话表
// 核心用途：管理“我的设备”列表，支持多端登录与互踢。
// 注意：高频的鉴权验证建议走 Redis，此表主要用于持久化记录和管理后台。
type DeviceSession struct {
	Id        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	
	// 核心索引：确定 "谁" 在 "哪个设备"
	UserUuid  string `gorm:"column:user_uuid;type:char(20);not null;index;uniqueIndex:uidx_user_device;comment:用户uuid"`
	DeviceId  string `gorm:"column:device_id;type:varchar(64);not null;uniqueIndex:uidx_user_device;comment:设备唯一指纹"`
	
	// 展示给用户看的信息
	DeviceName string `gorm:"column:device_name;type:varchar(64);not null;default:'Unknown Device';comment:设备名称(如 iPhone 13 Pro)"`
	Platform   string `gorm:"column:platform;type:varchar(32);not null;comment:平台(iOS/Android/Web/Win/Mac)"`
	
	// 鉴权数据
	// 【修正】JWT通常很长，255不够用，建议 1024 或 text
	Token        string `gorm:"column:token;type:varchar(2048);comment:Access Token"`
	RefreshToken string `gorm:"column:refresh_token;type:varchar(2048);comment:Refresh Token"`
	
	// 环境信息 (风控用)
	AppVersion string `gorm:"column:app_version;type:varchar(32);comment:APP版本"`
	IP         string `gorm:"column:ip;type:varchar(64);comment:登录IP"`
	UserAgent  string `gorm:"column:user_agent;type:varchar(255);comment:User Agent"`
	
	// 时间与状态
	ExpireAt   *time.Time `gorm:"column:expire_at;index;comment:过期时间(用于清理过期会话)"`
	LastSeenAt *time.Time `gorm:"column:last_seen_at;comment:最后活跃时间(注意减少写入频率)"`
	
	// 0在线 1下线(被踢) 2注销
	Status     int8       `gorm:"column:status;not null;default:0;comment:状态"`
	
	CreatedAt time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (DeviceSession) TableName() string {
	return "device_session"
}