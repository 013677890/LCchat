package config

import "time"

// MinIOConfig MinIO 对象存储配置
type MinIOConfig struct {
	// 连接配置
	Endpoint        string `json:"endpoint" yaml:"endpoint"`               // MinIO 服务地址，如: localhost:9000
	AccessKeyID     string `json:"accessKeyId" yaml:"accessKeyId"`         // Access Key
	SecretAccessKey string `json:"secretAccessKey" yaml:"secretAccessKey"` // Secret Key
	UseSSL          bool   `json:"useSSL" yaml:"useSSL"`                   // 是否使用 HTTPS

	// Bucket 配置
	BucketName string `json:"bucketName" yaml:"bucketName"` // 默认存储桶名称
	Location   string `json:"location" yaml:"location"`     // Bucket 区域，如: us-east-1

	// 上传配置
	MaxFileSize   int64         `json:"maxFileSize" yaml:"maxFileSize"`     // 最大文件大小（字节），默认 10MB
	AllowedTypes  []string      `json:"allowedTypes" yaml:"allowedTypes"`   // 允许的文件类型，如: ["image/jpeg", "image/png"]
	UploadTimeout time.Duration `json:"uploadTimeout" yaml:"uploadTimeout"` // 上传超时时间

	// 访问配置
	PublicRead bool   `json:"publicRead" yaml:"publicRead"` // 是否公开读取
	BaseURL    string `json:"baseUrl" yaml:"baseUrl"`       // 外部访问的基础 URL，用于返回给客户端的文件地址

	// 连接池配置
	MaxIdleConns        int           `json:"maxIdleConns" yaml:"maxIdleConns"`               // 最大空闲连接数
	MaxIdleConnsPerHost int           `json:"maxIdleConnsPerHost" yaml:"maxIdleConnsPerHost"` // 每个 host 的最大空闲连接数
	IdleConnTimeout     time.Duration `json:"idleConnTimeout" yaml:"idleConnTimeout"`         // 空闲连接超时时间
}

// DefaultMinIOConfig 返回本地开发的默认配置
func DefaultMinIOConfig() MinIOConfig {
	return MinIOConfig{
		// 连接配置（优先读取环境变量）
		Endpoint:        getenvString("MINIO_ENDPOINT", "minio:9000"),
		AccessKeyID:     getenvString("MINIO_ACCESS_KEY", "minioadmin"),
		SecretAccessKey: getenvString("MINIO_SECRET_KEY", "minioadmin"),
		UseSSL:          getenvBool("MINIO_USE_SSL", false),

		// Bucket 配置
		BucketName: getenvString("MINIO_BUCKET", "chatserver"),
		Location:   "us-east-1",

		// 上传配置
		MaxFileSize:   10 * 1024 * 1024, // 10MB
		AllowedTypes:  []string{"image/jpeg", "image/jpg", "image/png", "image/gif", "image/webp"},
		UploadTimeout: 30 * time.Second,

		// 访问配置
		PublicRead: getenvBool("MINIO_PUBLIC_READ", true),                   // 图片公开访问
		BaseURL:    getenvString("MINIO_BASE_URL", "http://localhost:9000"), // 本地开发访问地址

		// 连接池配置
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
}
