# MinIO 对象存储使用指南

## 概述

本项目使用 MinIO 作为对象存储服务，用于存储用户上传的图片、文件等。MinIO 是一个高性能的对象存储服务，兼容 Amazon S3 API。

## 目录结构

```
config/
  └── minio.go          # MinIO 配置
pkg/
  └── minio/
      └── minio.go      # MinIO 客户端封装
```

## 配置说明

### 配置结构 (`config/minio.go`)

```go
type MinIOConfig struct {
    // 连接配置
    Endpoint        string        // MinIO 服务地址，如: localhost:9000
    AccessKeyID     string        // Access Key
    SecretAccessKey string        // Secret Key
    UseSSL          bool          // 是否使用 HTTPS

    // Bucket 配置
    BucketName string             // 默认存储桶名称
    Location   string             // Bucket 区域

    // 上传配置
    MaxFileSize   int64           // 最大文件大小（字节）
    AllowedTypes  []string        // 允许的文件类型
    UploadTimeout time.Duration   // 上传超时时间

    // 访问配置
    PublicRead bool               // 是否公开读取
    BaseURL    string             // 外部访问的基础 URL

    // 连接池配置
    MaxIdleConns        int
    MaxIdleConnsPerHost int
    IdleConnTimeout     time.Duration
}
```

### 默认配置

```go
// 本地开发配置
cfg := config.DefaultMinIOConfig()

// 生产环境配置
cfg := config.ProductionMinIOConfig()
```

## 初始化

### 在 main.go 中初始化

```go
package main

import (
    "github.com/013677890/LCchat-Backend/config"
    pkgminio "github.com/013677890/LCchat-Backend/pkg/minio"
    "github.com/013677890/LCchat-Backend/pkg/logger"
    "context"
)

func main() {
    // 1. 初始化日志
    // ...

    // 2. 初始化 MinIO
    minioConfig := config.DefaultMinIOConfig()
    minioClient, err := pkgminio.Build(minioConfig)
    if err != nil {
        logger.Error(context.Background(), "MinIO 初始化失败",
            logger.ErrorField("error", err),
        )
        panic(err)
    }
    
    // 设置全局 MinIO 客户端
    pkgminio.ReplaceGlobal(minioClient)
    
    logger.Info(context.Background(), "MinIO 初始化成功",
        logger.String("endpoint", minioConfig.Endpoint),
        logger.String("bucket", minioConfig.BucketName),
    )

    // 3. 初始化其他组件
    // ...
}
```

## 基础使用

### 1. 上传文件

```go
package handler

import (
    "context"
    "io"
    pkgminio "github.com/013677890/LCchat-Backend/pkg/minio"
    "github.com/013677890/LCchat-Backend/pkg/logger"
)

func UploadImage(ctx context.Context, reader io.Reader, fileSize int64, fileName string) (*pkgminio.UploadResult, error) {
    // 获取全局 MinIO 客户端
    client := pkgminio.Client()
    if client == nil {
        return nil, errors.New("MinIO 客户端未初始化")
    }

    // 上传文件
    result, err := client.Upload(ctx, reader, fileSize, pkgminio.UploadOptions{
        PathPrefix:  "images/",              // 文件路径前缀
        FileName:    fileName,               // 自定义文件名（可选）
        ContentType: "image/jpeg",           // 内容类型（可选，会自动检测）
        Metadata: map[string]string{         // 自定义元数据（可选）
            "user_id": "123",
            "upload_time": time.Now().String(),
        },
    })
    
    if err != nil {
        logger.Error(ctx, "上传文件失败", logger.ErrorField("error", err))
        return nil, err
    }

    logger.Info(ctx, "上传文件成功",
        logger.String("object", result.ObjectName),
        logger.String("url", result.URL),
        logger.Int64("size", result.Size),
    )

    return result, nil
}
```

### 2. 在 Gin Handler 中上传图片

```go
package handler

import (
    "net/http"
    pkgminio "github.com/013677890/LCchat-Backend/pkg/minio"
    "github.com/gin-gonic/gin"
)

func UploadAvatarHandler(c *gin.Context) {
    ctx := c.Request.Context()

    // 1. 解析上传的文件
    file, header, err := c.Request.FormFile("avatar")
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "code":    400,
            "message": "无法读取上传的文件",
        })
        return
    }
    defer file.Close()

    // 2. 验证文件大小
    const maxSize = 5 * 1024 * 1024 // 5MB
    if header.Size > maxSize {
        c.JSON(http.StatusBadRequest, gin.H{
            "code":    400,
            "message": "文件大小超过限制（最大 5MB）",
        })
        return
    }

    // 3. 验证文件类型
    contentType := header.Header.Get("Content-Type")
    if !strings.HasPrefix(contentType, "image/") {
        c.JSON(http.StatusBadRequest, gin.H{
            "code":    400,
            "message": "只支持图片格式",
        })
        return
    }

    // 4. 获取 MinIO 客户端
    client := pkgminio.Client()
    if client == nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "code":    500,
            "message": "文件存储服务不可用",
        })
        return
    }

    // 5. 生成文件名（使用原始文件扩展名）
    ext := filepath.Ext(header.Filename)
    fileName := uuid.New().String() + ext

    // 6. 上传文件
    result, err := client.Upload(ctx, file, header.Size, pkgminio.UploadOptions{
        PathPrefix:  "avatars/",
        FileName:    fileName,
        ContentType: contentType,
    })
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "code":    500,
            "message": "上传失败",
        })
        return
    }

    // 7. 返回上传结果
    c.JSON(http.StatusOK, gin.H{
        "code":    0,
        "message": "上传成功",
        "data": gin.H{
            "url":          result.URL,
            "object_name":  result.ObjectName,
            "size":         result.Size,
            "content_type": result.ContentType,
        },
    })
}
```

### 3. 下载文件

```go
func DownloadFile(ctx context.Context, objectName string) (io.ReadCloser, error) {
    client := pkgminio.Client()
    if client == nil {
        return nil, errors.New("MinIO 客户端未初始化")
    }

    // 下载文件
    reader, info, err := client.Download(ctx, objectName)
    if err != nil {
        return nil, err
    }

    logger.Info(ctx, "下载文件成功",
        logger.String("object", objectName),
        logger.Int64("size", info.Size),
        logger.String("content_type", info.ContentType),
    )

    // 注意：调用者需要关闭 reader
    return reader, nil
}
```

### 4. 删除文件

```go
func DeleteFile(ctx context.Context, objectName string) error {
    client := pkgminio.Client()
    if client == nil {
        return errors.New("MinIO 客户端未初始化")
    }

    // 删除文件
    err := client.Delete(ctx, objectName)
    if err != nil {
        return err
    }

    logger.Info(ctx, "删除文件成功", logger.String("object", objectName))
    return nil
}
```

### 5. 批量删除文件

```go
func DeleteMultipleFiles(ctx context.Context, objectNames []string) error {
    client := pkgminio.Client()
    if client == nil {
        return errors.New("MinIO 客户端未初始化")
    }

    // 批量删除
    errors := client.DeleteMultiple(ctx, objectNames)
    if len(errors) > 0 {
        logger.Error(ctx, "批量删除部分失败",
            logger.Int("total", len(objectNames)),
            logger.Int("failed", len(errors)),
        )
        return fmt.Errorf("批量删除失败: %d/%d", len(errors), len(objectNames))
    }

    return nil
}
```

### 6. 检查文件是否存在

```go
func CheckFileExists(ctx context.Context, objectName string) (bool, error) {
    client := pkgminio.Client()
    if client == nil {
        return false, errors.New("MinIO 客户端未初始化")
    }

    exists, err := client.Exists(ctx, objectName)
    if err != nil {
        return false, err
    }

    return exists, nil
}
```

### 7. 获取预签名 URL（临时访问私有文件）

```go
func GetTemporaryURL(ctx context.Context, objectName string) (string, error) {
    client := pkgminio.Client()
    if client == nil {
        return "", errors.New("MinIO 客户端未初始化")
    }

    // 生成 1 小时有效的预签名 URL
    url, err := client.GetPresignedURL(ctx, objectName, 1*time.Hour)
    if err != nil {
        return "", err
    }

    return url, nil
}
```

## 高级用法

### 1. 自定义路径前缀（按日期分类）

```go
func UploadWithDatePrefix(ctx context.Context, reader io.Reader, size int64) (*pkgminio.UploadResult, error) {
    client := pkgminio.Client()
    
    // 按日期生成路径: images/2024/01/29/
    now := time.Now()
    pathPrefix := fmt.Sprintf("images/%d/%02d/%02d/", now.Year(), now.Month(), now.Day())
    
    result, err := client.Upload(ctx, reader, size, pkgminio.UploadOptions{
        PathPrefix: pathPrefix,
    })
    
    return result, err
}
```

### 2. 按用户分类存储

```go
func UploadUserFile(ctx context.Context, userID string, reader io.Reader, size int64, fileType string) (*pkgminio.UploadResult, error) {
    client := pkgminio.Client()
    
    // 路径: users/{userID}/{fileType}/
    pathPrefix := fmt.Sprintf("users/%s/%s/", userID, fileType)
    
    result, err := client.Upload(ctx, reader, size, pkgminio.UploadOptions{
        PathPrefix: pathPrefix,
        Metadata: map[string]string{
            "user_id":   userID,
            "file_type": fileType,
        },
    })
    
    return result, err
}
```

### 3. 带进度的上传（大文件）

```go
type ProgressReader struct {
    reader   io.Reader
    total    int64
    uploaded int64
    callback func(uploaded, total int64)
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
    n, err := pr.reader.Read(p)
    pr.uploaded += int64(n)
    if pr.callback != nil {
        pr.callback(pr.uploaded, pr.total)
    }
    return n, err
}

func UploadWithProgress(ctx context.Context, reader io.Reader, size int64) error {
    client := pkgminio.Client()
    
    // 创建带进度的 reader
    progressReader := &ProgressReader{
        reader: reader,
        total:  size,
        callback: func(uploaded, total int64) {
            progress := float64(uploaded) / float64(total) * 100
            logger.Info(ctx, "上传进度",
                logger.Float64("progress", progress),
                logger.Int64("uploaded", uploaded),
                logger.Int64("total", total),
            )
        },
    }
    
    _, err := client.Upload(ctx, progressReader, size, pkgminio.UploadOptions{
        PathPrefix: "large-files/",
    })
    
    return err
}
```

## Docker Compose 配置

在 `docker-compose.yml` 中添加 MinIO 服务：

```yaml
version: '3.8'

services:
  # ... 其他服务 ...

  minio:
    image: minio/minio:latest
    container_name: chatserver-minio
    ports:
      - "9000:9000"      # API 端口
      - "9001:9001"      # 控制台端口
    environment:
      MINIO_ROOT_USER: minioadmin
      MINIO_ROOT_PASSWORD: minioadmin
    command: server /data --console-address ":9001"
    volumes:
      - ./data/minio:/data
    networks:
      - chatserver-network
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9000/minio/health/live"]
      interval: 30s
      timeout: 10s
      retries: 3

networks:
  chatserver-network:
    driver: bridge
```

### 访问 MinIO 控制台

启动后访问：`http://localhost:9001`
- 用户名：`minioadmin`
- 密码：`minioadmin`

## 最佳实践

### 1. 文件命名规范

```go
// 推荐：使用 UUID + 扩展名
fileName := uuid.New().String() + ".jpg"

// 推荐：使用时间戳 + 随机字符串
fileName := fmt.Sprintf("%d_%s.jpg", time.Now().Unix(), randString(8))

// 不推荐：直接使用用户上传的文件名（可能包含特殊字符）
fileName := header.Filename  // ❌
```

### 2. 路径组织

```
avatars/              # 用户头像
  ├── {uuid}.jpg
  └── {uuid}.png

images/               # 一般图片（按日期分类）
  └── 2024/
      └── 01/
          └── 29/
              └── {uuid}.jpg

files/                # 文件（按用户分类）
  └── {user_id}/
      ├── documents/
      └── media/

temp/                 # 临时文件（定期清理）
  └── {uuid}
```

### 3. 错误处理

```go
result, err := client.Upload(ctx, reader, size, opts)
if err != nil {
    // 记录详细错误日志
    logger.Error(ctx, "MinIO 上传失败",
        logger.String("path_prefix", opts.PathPrefix),
        logger.Int64("size", size),
        logger.ErrorField("error", err),
    )
    
    // 返回用户友好的错误信息
    return nil, errors.New("文件上传失败，请稍后重试")
}
```

### 4. 资源清理

```go
// 下载文件后记得关闭 reader
reader, _, err := client.Download(ctx, objectName)
if err != nil {
    return err
}
defer reader.Close()  // ⚠️ 重要：必须关闭

// 读取内容
data, err := io.ReadAll(reader)
```

### 5. 并发上传

```go
func UploadMultipleFiles(ctx context.Context, files []FileData) error {
    client := pkgminio.Client()
    
    var wg sync.WaitGroup
    errors := make(chan error, len(files))
    
    for _, file := range files {
        wg.Add(1)
        go func(f FileData) {
            defer wg.Done()
            
            _, err := client.Upload(ctx, f.Reader, f.Size, pkgminio.UploadOptions{
                PathPrefix: "batch/",
            })
            
            if err != nil {
                errors <- err
            }
        }(file)
    }
    
    wg.Wait()
    close(errors)
    
    // 检查是否有错误
    var uploadErrors []error
    for err := range errors {
        uploadErrors = append(uploadErrors, err)
    }
    
    if len(uploadErrors) > 0 {
        return fmt.Errorf("批量上传失败: %d/%d", len(uploadErrors), len(files))
    }
    
    return nil
}
```

## 安全建议

### 1. 文件大小限制

```go
// 在配置中设置
cfg := config.MinIOConfig{
    MaxFileSize: 10 * 1024 * 1024,  // 10MB
}

// 在上传前验证
if fileSize > cfg.MaxFileSize {
    return errors.New("文件大小超过限制")
}
```

### 2. 文件类型限制

```go
// 在配置中设置
cfg := config.MinIOConfig{
    AllowedTypes: []string{
        "image/jpeg",
        "image/png",
        "image/gif",
        "image/webp",
    },
}

// 自动验证（在 Upload 方法中）
```

### 3. 访问控制

```go
// 公开文件：使用公开 Bucket
cfg := config.MinIOConfig{
    PublicRead: true,
}

// 私有文件：使用预签名 URL
url, err := client.GetPresignedURL(ctx, objectName, 1*time.Hour)
```

### 4. 敏感信息保护

```go
// 不要硬编码密钥
cfg := config.MinIOConfig{
    AccessKeyID:     os.Getenv("MINIO_ACCESS_KEY"),     // ✅
    SecretAccessKey: os.Getenv("MINIO_SECRET_KEY"),     // ✅
}

// 不要这样做
cfg := config.MinIOConfig{
    AccessKeyID:     "minioadmin",                      // ❌
    SecretAccessKey: "minioadmin",                      // ❌
}
```

## 性能优化

### 1. 连接池配置

```go
cfg := config.MinIOConfig{
    MaxIdleConns:        100,               // 最大空闲连接数
    MaxIdleConnsPerHost: 10,                // 每个 host 的最大空闲连接数
    IdleConnTimeout:     90 * time.Second,  // 空闲连接超时
}
```

### 2. 上传超时

```go
cfg := config.MinIOConfig{
    UploadTimeout: 30 * time.Second,  // 小文件
}

cfg := config.MinIOConfig{
    UploadTimeout: 5 * time.Minute,   // 大文件
}
```

### 3. CDN 加速

```go
// 使用 CDN 地址作为 BaseURL
cfg := config.MinIOConfig{
    BaseURL: "https://cdn.example.com",  // CDN 地址
}

// 生成的 URL 会使用 CDN 域名
// https://cdn.example.com/chatserver/avatars/xxx.jpg
```

## 监控和日志

所有操作都会自动记录日志：
- 上传成功/失败
- 下载成功/失败
- 删除成功/失败
- 错误详情

建议监控的指标：
- 上传成功率
- 上传延迟
- 存储空间使用情况
- 流量统计

## 常见问题

### 1. 连接失败

**问题**：`failed to create minio client: dial tcp: connection refused`

**解决**：
- 检查 MinIO 服务是否启动
- 检查 Endpoint 配置是否正确
- 检查网络连接

### 2. Bucket 不存在

**问题**：`The specified bucket does not exist`

**解决**：
- Build 方法会自动创建 Bucket
- 检查 BucketName 配置
- 检查用户权限

### 3. 上传失败

**问题**：`upload failed: context deadline exceeded`

**解决**：
- 增加 UploadTimeout
- 检查网络速度
- 检查文件大小

### 4. 无法访问文件

**问题**：`Access Denied`

**解决**：
- 检查 PublicRead 配置
- 使用预签名 URL
- 检查 Bucket 策略

## 总结

MinIO 客户端提供了完整的文件管理功能：
- ✅ 文件上传（支持自定义路径、元数据）
- ✅ 文件下载
- ✅ 文件删除（单个/批量）
- ✅ 文件存在检查
- ✅ 预签名 URL（临时访问）
- ✅ 自动类型检测
- ✅ 文件大小和类型验证
- ✅ 完善的错误处理和日志
- ✅ 连接池优化

适用于：
- 用户头像上传
- 聊天图片/文件上传
- 临时文件存储
- 任何需要对象存储的场景
