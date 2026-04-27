package minio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/013677890/LCchat-Backend/config"
	"github.com/013677890/LCchat-Backend/pkg/logger"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client 全局 MinIO 客户端实例
var global *MinIOClient

// MinIOClient MinIO 客户端封装
type MinIOClient struct {
	client *minio.Client
	config config.MinIOConfig
}

// Client 返回全局 MinIO 客户端（未初始化时为 nil）
func Client() *MinIOClient {
	return global
}

// ReplaceGlobal 设置全局 MinIO 客户端
func ReplaceGlobal(c *MinIOClient) {
	global = c
}

// Build 基于配置创建 MinIO 客户端
func Build(cfg config.MinIOConfig) (*MinIOClient, error) {
	// 1. 验证必填配置
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, errors.New("minio endpoint is empty")
	}
	if strings.TrimSpace(cfg.AccessKeyID) == "" {
		return nil, errors.New("minio accessKeyId is empty")
	}
	if strings.TrimSpace(cfg.SecretAccessKey) == "" {
		return nil, errors.New("minio secretAccessKey is empty")
	}
	if strings.TrimSpace(cfg.BucketName) == "" {
		return nil, errors.New("minio bucketName is empty")
	}

	// 2. 创建 MinIO 客户端
	minioClient, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSSL,
		Transport: &http.Transport{
			MaxIdleConns:        cfg.MaxIdleConns,
			MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
			IdleConnTimeout:     cfg.IdleConnTimeout,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create minio client: %w", err)
	}

	// 3. 创建封装的客户端
	client := &MinIOClient{
		client: minioClient,
		config: cfg,
	}

	// 4. 确保 Bucket 存在
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	exists, err := minioClient.BucketExists(ctx, cfg.BucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket exists: %w", err)
	}

	if !exists {
		// 创建 Bucket
		err = minioClient.MakeBucket(ctx, cfg.BucketName, minio.MakeBucketOptions{
			Region: cfg.Location,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}

		logger.Info(ctx, "MinIO Bucket 创建成功",
			logger.String("bucket", cfg.BucketName),
			logger.String("location", cfg.Location),
		)

		// 设置 Bucket 策略（公开读）
		if cfg.PublicRead {
			policy := fmt.Sprintf(`{
				"Version": "2012-10-17",
				"Statement": [
					{
						"Effect": "Allow",
						"Principal": {"AWS": ["*"]},
						"Action": ["s3:GetObject"],
						"Resource": ["arn:aws:s3:::%s/*"]
					}
				]
			}`, cfg.BucketName)

			err = minioClient.SetBucketPolicy(ctx, cfg.BucketName, policy)
			if err != nil {
				logger.Warn(ctx, "设置 Bucket 公开策略失败",
					logger.String("bucket", cfg.BucketName),
					logger.ErrorField("error", err),
				)
			}
		}
	}

	return client, nil
}

// UploadOptions 上传选项
type UploadOptions struct {
	// 文件路径前缀（如: "avatars/", "images/2024/01/"）
	PathPrefix string
	// 自定义文件名（如果为空则自动生成 UUID）
	FileName string
	// 内容类型（如: "image/jpeg"，如果为空则自动检测）
	ContentType string
	// 元数据（可选）
	Metadata map[string]string
}

// UploadResult 上传结果
type UploadResult struct {
	// 对象名称（完整路径，如: avatars/uuid.jpg）
	ObjectName string
	// 文件大小（字节）
	Size int64
	// ETag（文件的 MD5 哈希）
	ETag string
	// 完整访问 URL
	URL string
	// 内容类型
	ContentType string
}

// Upload 上传文件
// ctx: 上下文
// reader: 文件内容流
// fileSize: 文件大小（字节）
// opts: 上传选项
func (c *MinIOClient) Upload(ctx context.Context, reader io.Reader, fileSize int64, opts UploadOptions) (*UploadResult, error) {
	// 1. 验证文件大小
	if c.config.MaxFileSize > 0 && fileSize > c.config.MaxFileSize {
		return nil, fmt.Errorf("文件大小超过限制: %d bytes (最大: %d bytes)", fileSize, c.config.MaxFileSize)
	}

	// 2. 生成对象名称
	objectName := c.generateObjectName(opts)

	// 3. 检测文件的真实 Content-Type（基于文件内容的 Magic Bytes）
	// 读取前 512 字节用于类型检测（http.DetectContentType 的要求）
	buffer := make([]byte, 512)
	n, err := io.ReadFull(reader, buffer)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("读取文件内容失败: %w", err)
	}
	buffer = buffer[:n] // 截取实际读取的字节数

	// 基于文件内容检测真实的 MIME 类型
	detectedContentType := http.DetectContentType(buffer)

	// 4. 确定最终使用的 Content-Type
	contentType := opts.ContentType
	if contentType == "" {
		// 如果未指定，优先使用检测到的类型
		contentType = detectedContentType
	} else {
		// 如果指定了类型，验证是否与检测到的类型匹配
		// 允许一定的灵活性（如 image/jpg 和 image/jpeg）
		if !c.isContentTypeMatch(contentType, detectedContentType) {
			logger.Warn(ctx, "指定的文件类型与实际检测到的类型不一致",
				logger.String("specified", contentType),
				logger.String("detected", detectedContentType),
				logger.String("object", objectName),
			)
			// 使用检测到的真实类型
			contentType = detectedContentType
		}
	}

	// 5. 验证文件类型是否在允许列表中
	if len(c.config.AllowedTypes) > 0 && !c.isAllowedType(contentType) {
		// 同时记录检测到的类型，方便排查
		logger.Warn(ctx, "文件类型不在允许列表中",
			logger.String("detected_type", detectedContentType),
			logger.String("content_type", contentType),
			logger.String("file_name", opts.FileName),
			logger.Any("allowed_types", c.config.AllowedTypes),
		)
		return nil, fmt.Errorf("不支持的文件类型: %s (检测到: %s, 允许: %v)",
			contentType, detectedContentType, c.config.AllowedTypes)
	}

	// 6. 验证扩展名是否与内容类型匹配（防止恶意文件伪装）
	if opts.FileName != "" || objectName != "" {
		fileName := opts.FileName
		if fileName == "" {
			fileName = objectName
		}
		if !c.validateFileExtension(fileName, detectedContentType) {
			logger.Warn(ctx, "文件扩展名与实际内容类型不匹配（可能是恶意文件）",
				logger.String("file_name", fileName),
				logger.String("detected_type", detectedContentType),
			)
			return nil, fmt.Errorf("文件扩展名与实际内容类型不匹配（检测到: %s）", detectedContentType)
		}
	}

	// 7. 重新组合 reader（已读取的 512 字节 + 剩余内容）
	multiReader := io.MultiReader(strings.NewReader(string(buffer)), reader)

	// 8. 设置超时
	uploadCtx := ctx
	if c.config.UploadTimeout > 0 {
		var cancel context.CancelFunc
		uploadCtx, cancel = context.WithTimeout(ctx, c.config.UploadTimeout)
		defer cancel()
	}

	// 9. 执行上传（使用重新组合的 reader）
	uploadInfo, err := c.client.PutObject(
		uploadCtx,
		c.config.BucketName,
		objectName,
		multiReader,
		fileSize,
		minio.PutObjectOptions{
			ContentType:  contentType,
			UserMetadata: opts.Metadata,
		},
	)
	if err != nil {
		logger.Error(ctx, "MinIO 上传失败",
			logger.String("object", objectName),
			logger.String("content_type", contentType),
			logger.String("detected_type", detectedContentType),
			logger.Int64("size", fileSize),
			logger.ErrorField("error", err),
		)
		return nil, fmt.Errorf("上传失败: %w", err)
	}

	// 10. 生成访问 URL
	url := c.generateURL(objectName)

	logger.Info(ctx, "MinIO 上传成功",
		logger.String("object", objectName),
		logger.String("url", url),
		logger.String("content_type", contentType),
		logger.String("detected_type", detectedContentType),
		logger.Int64("size", uploadInfo.Size),
		logger.String("etag", uploadInfo.ETag),
	)

	return &UploadResult{
		ObjectName:  objectName,
		Size:        uploadInfo.Size,
		ETag:        uploadInfo.ETag,
		URL:         url,
		ContentType: contentType,
	}, nil
}

// Download 下载文件
// ctx: 上下文
// objectName: 对象名称（完整路径）
// 返回: io.ReadCloser（记得关闭）, 文件信息, 错误
func (c *MinIOClient) Download(ctx context.Context, objectName string) (io.ReadCloser, *minio.ObjectInfo, error) {
	// 1. 获取对象
	object, err := c.client.GetObject(ctx, c.config.BucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		logger.Error(ctx, "MinIO 下载失败",
			logger.String("object", objectName),
			logger.ErrorField("error", err),
		)
		return nil, nil, fmt.Errorf("下载失败: %w", err)
	}

	// 2. 获取对象信息
	info, err := object.Stat()
	if err != nil {
		object.Close()
		logger.Error(ctx, "MinIO 获取对象信息失败",
			logger.String("object", objectName),
			logger.ErrorField("error", err),
		)
		return nil, nil, fmt.Errorf("获取对象信息失败: %w", err)
	}

	logger.Info(ctx, "MinIO 下载成功",
		logger.String("object", objectName),
		logger.Int64("size", info.Size),
	)

	return object, &info, nil
}

// Delete 删除文件
// ctx: 上下文
// objectName: 对象名称（完整路径）
func (c *MinIOClient) Delete(ctx context.Context, objectName string) error {
	err := c.client.RemoveObject(ctx, c.config.BucketName, objectName, minio.RemoveObjectOptions{})
	if err != nil {
		logger.Error(ctx, "MinIO 删除失败",
			logger.String("object", objectName),
			logger.ErrorField("error", err),
		)
		return fmt.Errorf("删除失败: %w", err)
	}

	logger.Info(ctx, "MinIO 删除成功",
		logger.String("object", objectName),
	)
	return nil
}

// DeleteMultiple 批量删除文件
// ctx: 上下文
// objectNames: 对象名称列表
// 返回: 删除失败的对象列表
func (c *MinIOClient) DeleteMultiple(ctx context.Context, objectNames []string) []error {
	if len(objectNames) == 0 {
		return nil
	}

	// 创建删除通道
	objectsCh := make(chan minio.ObjectInfo, len(objectNames))
	go func() {
		defer close(objectsCh)
		for _, name := range objectNames {
			objectsCh <- minio.ObjectInfo{Key: name}
		}
	}()

	// 批量删除
	errorsCh := c.client.RemoveObjects(ctx, c.config.BucketName, objectsCh, minio.RemoveObjectsOptions{})

	// 收集错误
	var errors []error
	for err := range errorsCh {
		if err.Err != nil {
			logger.Error(ctx, "MinIO 批量删除失败",
				logger.String("object", err.ObjectName),
				logger.ErrorField("error", err.Err),
			)
			errors = append(errors, fmt.Errorf("删除 %s 失败: %w", err.ObjectName, err.Err))
		}
	}

	if len(errors) == 0 {
		logger.Info(ctx, "MinIO 批量删除成功",
			logger.Int("count", len(objectNames)),
		)
	}

	return errors
}

// Exists 检查文件是否存在
// ctx: 上下文
// objectName: 对象名称（完整路径）
func (c *MinIOClient) Exists(ctx context.Context, objectName string) (bool, error) {
	_, err := c.client.StatObject(ctx, c.config.BucketName, objectName, minio.StatObjectOptions{})
	if err != nil {
		// 检查是否为 NotFound 错误
		errResponse := minio.ToErrorResponse(err)
		if errResponse.Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("检查对象存在失败: %w", err)
	}
	return true, nil
}

// GetPresignedURL 获取预签名 URL（用于临时访问私有文件）
// ctx: 上下文
// objectName: 对象名称（完整路径）
// expires: 有效期（如: 1小时）
func (c *MinIOClient) GetPresignedURL(ctx context.Context, objectName string, expires time.Duration) (string, error) {
	url, err := c.client.PresignedGetObject(ctx, c.config.BucketName, objectName, expires, nil)
	if err != nil {
		logger.Error(ctx, "MinIO 生成预签名 URL 失败",
			logger.String("object", objectName),
			logger.Duration("expires", expires),
			logger.ErrorField("error", err),
		)
		return "", fmt.Errorf("生成预签名 URL 失败: %w", err)
	}

	logger.Info(ctx, "MinIO 生成预签名 URL 成功",
		logger.String("object", objectName),
		logger.Duration("expires", expires),
	)

	return url.String(), nil
}

// ==================== 辅助方法 ====================

// generateObjectName 生成对象名称
func (c *MinIOClient) generateObjectName(opts UploadOptions) string {
	var fileName string

	// 使用自定义文件名或生成 UUID
	if opts.FileName != "" {
		fileName = opts.FileName
	} else {
		// 生成 UUID 作为文件名
		fileName = uuid.New().String()
	}

	// 添加路径前缀
	if opts.PathPrefix != "" {
		// 确保前缀以 / 结尾
		prefix := strings.TrimSuffix(opts.PathPrefix, "/")
		return prefix + "/" + fileName
	}

	return fileName
}

// generateURL 生成访问 URL
func (c *MinIOClient) generateURL(objectName string) string {
	baseURL := strings.TrimSuffix(c.config.BaseURL, "/")
	bucketName := c.config.BucketName
	objectName = strings.TrimPrefix(objectName, "/")

	return fmt.Sprintf("%s/%s/%s", baseURL, bucketName, objectName)
}

// detectContentType 根据文件扩展名检测 Content-Type
// ⚠️ 注意：此方法仅用于后备，不应作为主要的类型检测方式
// 优先使用 http.DetectContentType 基于文件内容检测
func (c *MinIOClient) detectContentType(fileName string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".pdf":
		return "application/pdf"
	case ".txt":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".zip":
		return "application/zip"
	case ".mp4":
		return "video/mp4"
	case ".mp3":
		return "audio/mpeg"
	default:
		return "application/octet-stream"
	}
}

// isContentTypeMatch 检查两个 Content-Type 是否匹配
// 允许一定的灵活性，如 image/jpg 和 image/jpeg 视为相同
func (c *MinIOClient) isContentTypeMatch(specified, detected string) bool {
	// 标准化类型（转小写）
	specified = strings.ToLower(strings.TrimSpace(specified))
	detected = strings.ToLower(strings.TrimSpace(detected))

	// 完全匹配
	if specified == detected {
		return true
	}

	// 特殊情况：image/jpg 和 image/jpeg 视为相同
	if (specified == "image/jpg" || specified == "image/jpeg") &&
		(detected == "image/jpg" || detected == "image/jpeg") {
		return true
	}

	// 检查主类型是否匹配（如 image/*）
	specifiedMain := strings.Split(specified, "/")[0]
	detectedMain := strings.Split(detected, "/")[0]

	return specifiedMain == detectedMain
}

// validateFileExtension 验证文件扩展名是否与检测到的内容类型匹配
// 防止恶意文件伪装（如 .exe 改名为 .jpg）
func (c *MinIOClient) validateFileExtension(fileName, detectedContentType string) bool {
	ext := strings.ToLower(filepath.Ext(fileName))
	detectedContentType = strings.ToLower(detectedContentType)

	// 定义扩展名与 MIME 类型的映射
	validExtensions := map[string][]string{
		// 图片类型
		"image/jpeg":    {".jpg", ".jpeg"},
		"image/png":     {".png"},
		"image/gif":     {".gif"},
		"image/webp":    {".webp"},
		"image/bmp":     {".bmp"},
		"image/svg+xml": {".svg"},
		"image/x-icon":  {".ico"},

		// 文档类型
		"application/pdf":  {".pdf"},
		"text/plain":       {".txt"},
		"application/json": {".json"},
		"application/xml":  {".xml"},

		// 压缩文件
		"application/zip":              {".zip"},
		"application/x-rar-compressed": {".rar"},
		"application/x-7z-compressed":  {".7z"},

		// 视频类型
		"video/mp4":       {".mp4"},
		"video/mpeg":      {".mpeg", ".mpg"},
		"video/quicktime": {".mov"},

		// 音频类型
		"audio/mpeg": {".mp3"},
		"audio/wav":  {".wav"},
		"audio/ogg":  {".ogg"},

		// 其他
		"application/octet-stream": {}, // 允许任何扩展名
	}

	// 查找允许的扩展名列表
	allowedExts, exists := validExtensions[detectedContentType]
	if !exists {
		// 未知类型，记录警告但允许（可根据安全需求调整）
		return true
	}

	// 如果允许列表为空，表示任何扩展名都可以
	if len(allowedExts) == 0 {
		return true
	}

	// 检查扩展名是否在允许列表中
	for _, allowedExt := range allowedExts {
		if ext == allowedExt {
			return true
		}
	}

	return false
}

// isAllowedType 检查文件类型是否允许
func (c *MinIOClient) isAllowedType(contentType string) bool {
	for _, allowed := range c.config.AllowedTypes {
		if strings.EqualFold(contentType, allowed) {
			return true
		}
	}
	return false
}

// GetBucketName 获取当前使用的 Bucket 名称
func (c *MinIOClient) GetBucketName() string {
	return c.config.BucketName
}

// GetConfig 获取配置
func (c *MinIOClient) GetConfig() config.MinIOConfig {
	return c.config
}
