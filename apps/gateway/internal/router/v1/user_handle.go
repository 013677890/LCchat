package v1

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/middleware"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/service"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/utils"
	"github.com/013677890/LCchat-Backend/consts"
	pkgminio "github.com/013677890/LCchat-Backend/pkg/minio"
	"github.com/013677890/LCchat-Backend/pkg/result"

	"github.com/gin-gonic/gin"
)

// UserHandler 用户信息处理器
type UserHandler struct {
	userService service.UserService
}

// NewUserHandler 创建用户信息处理器
// userService: 用户信息服务
func NewUserHandler(userService service.UserService) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

// GetProfile 获取个人信息接口
// @Summary 获取个人信息
// @Description 获取当前登录用户的完整个人信息
// @Tags 用户信息接口
// @Accept json
// @Produce json
// @Success 200 {object} dto.GetProfileResponse
// @Router /api/v1/user/profile [get]
func (h *UserHandler) GetProfile(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 调用服务层处理业务逻辑（依赖注入）
	profileResp, err := h.userService.GetProfile(ctx)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败（如用户不存在等）
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 2. 返回成功响应
	result.Success(c, profileResp)
}

// GetOtherProfile 获取他人信息接口
// @Summary 获取他人信息
// @Description 获取其他用户的公开信息
// @Tags 用户信息接口
// @Accept json
// @Produce json
// @Param userUuid path string true "用户UUID"
// @Success 200 {object} dto.GetOtherProfileResponse
// @Router /api/v1/user/profile/{userUuid} [get]
func (h *UserHandler) GetOtherProfile(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 从路径参数中获取userUuid
	userUuid := c.Param("userUuid")
	if userUuid == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 构造请求DTO
	req := &dto.GetOtherProfileRequest{
		UserUUID: userUuid,
	}

	// 3. 调用服务层处理业务逻辑（依赖注入）
	profileResp, err := h.userService.GetOtherProfile(ctx, req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败（如用户不存在等）
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 4. 返回成功响应
	result.Success(c, profileResp)
}

// SearchUser 搜索用户接口
// @Summary 搜索用户
// @Description 通过邮箱、昵称、用户ID搜索用户
// @Tags 用户信息接口
// @Accept json
// @Produce json
// @Param keyword query string true "搜索关键词"
// @Param page query int false "页码(默认1)"
// @Param pageSize query int false "每页数量(默认20)"
// @Success 200 {object} dto.SearchUserResponse
// @Router /api/v1/auth/user/search [get]
func (h *UserHandler) SearchUser(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定查询参数
	var req dto.SearchUserRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 显式分页参数校验：
	// 未传分页时走默认值；显式传非法值时返回参数错误。
	if _, exists := c.GetQuery("page"); exists && req.Page < 1 {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	if _, exists := c.GetQuery("pageSize"); exists && (req.PageSize < 1 || req.PageSize > 100) {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 3. 设置默认值
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 100
	}

	// 4. 调用服务层处理业务逻辑（依赖注入）
	searchResp, err := h.userService.SearchUser(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 5. 返回成功响应
	result.Success(c, searchResp)
}

// ChangePassword 修改密码接口
// @Summary 修改密码
// @Description 通过旧密码修改新密码
// @Tags 用户信息接口
// @Accept json
// @Produce json
// @Param request body dto.ChangePasswordRequest true "修改密码请求"
// @Success 200 {object} dto.ChangePasswordResponse
// @Router /api/v1/user/change-password [post]
func (h *UserHandler) ChangePassword(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 调用服务层处理业务逻辑（依赖注入）
	err := h.userService.ChangePassword(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败（如旧密码错误、新密码与旧密码相同）
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 3. 返回成功响应
	result.Success(c, nil)
}

// UpdateProfile 更新基本信息接口
// @Summary 更新基本信息
// @Description 更新个人基本信息（昵称、性别、生日、签名）
// @Tags 用户信息接口
// @Accept json
// @Produce json
// @Param request body dto.UpdateProfileRequest true "更新基本信息请求"
// @Success 200 {object} dto.UpdateProfileResponse
// @Router /api/v1/user/profile [put]
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 至少提供一个字段
	if req.Nickname == "" && req.Gender == nil && req.Birthday == "" && req.Signature == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 3. 调用服务层处理业务逻辑（依赖注入）
	profileResp, err := h.userService.UpdateProfile(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败（如昵称已被使用等）
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 4. 返回成功响应
	result.Success(c, profileResp)
}

// ChangeEmail 换绑邮箱接口
// @Summary 换绑邮箱
// @Description 更换绑定邮箱（需验证码）
// @Tags 用户信息接口
// @Accept json
// @Produce json
// @Param request body dto.ChangeEmailRequest true "换绑邮箱请求"
// @Success 200 {object} dto.ChangeEmailResponse
// @Router /api/v1/user/change-email [post]
func (h *UserHandler) ChangeEmail(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.ChangeEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 调用服务层处理业务逻辑（依赖注入）
	emailResp, err := h.userService.ChangeEmail(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败（如邮箱已被使用、验证码错误等）
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 3. 返回成功响应
	result.Success(c, emailResp)
}

// UploadAvatar 上传头像接口
// @Summary 上传并更新用户头像
// @Description 上传图片文件到对象存储并更新用户头像
// @Tags 用户信息接口
// @Accept multipart/form-data
// @Produce json
// @Param avatar formData file true "头像文件(jpg/png,最大2MB)"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/auth/user/avatar [post]
func (h *UserHandler) UploadAvatar(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 解析上传的文件
	file, header, err := c.Request.FormFile("avatar")
	if err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	defer file.Close()

	// 2. 验证文件大小（最大 2MB）
	const maxSize = 2 * 1024 * 1024 // 2MB
	if header.Size > maxSize {
		result.Fail(c, nil, consts.CodeBodyTooLarge)
		return
	}

	// 3. 验证文件类型（只支持 jpg/png）
	contentType := header.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") ||
		(contentType != "image/jpeg" && contentType != "image/png") {
		result.Fail(c, nil, consts.CodeFileFormatNotSupport)
		return
	}

	// 4. 获取 MinIO 客户端
	minioClient := pkgminio.Client()
	if minioClient == nil {
		result.FailServer(c, errors.New("MinIO 客户端未初始化"), consts.CodeInternalError)
		return
	}

	// 5. 生成文件名（保留历史）
	// 格式: avatars/{user_uuid}/{timestamp}.{ext}
	userUUID, exists := middleware.GetUserUUID(c)
	if !exists || userUUID == "" {
		result.Fail(c, nil, consts.CodeUnauthorized)
		return
	}

	ext := filepath.Ext(header.Filename)
	if ext == "" {
		// 根据 Content-Type 推断扩展名
		if contentType == "image/jpeg" {
			ext = ".jpg"
		} else if contentType == "image/png" {
			ext = ".png"
		}
	}

	fileName := fmt.Sprintf("%d%s", time.Now().Unix(), ext)
	pathPrefix := fmt.Sprintf("avatars/%s/", userUUID)

	// 6. 上传到 MinIO
	uploadResult, err := minioClient.Upload(ctx, file, header.Size, pkgminio.UploadOptions{
		PathPrefix:  pathPrefix,
		FileName:    fileName,
		ContentType: contentType,
	})
	if err != nil {
		result.FailServer(c, err, consts.CodeFileUploadFail)
		return
	}

	// 7. 调用服务层更新数据库
	avatarURL, err := h.userService.UploadAvatar(ctx, uploadResult.URL)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 8. 返回成功响应
	result.Success(c, gin.H{
		"avatarUrl": avatarURL,
	})
}

// BatchGetProfile 批量获取用户信息接口
// @Summary 批量获取用户信息
// @Description 根据用户UUID列表批量查询用户基本信息（uuid、昵称、头像）
// @Tags 用户信息接口
// @Accept json
// @Produce json
// @Param request body dto.BatchGetProfileRequest true "批量获取用户信息请求"
// @Success 200 {object} dto.BatchGetProfileResponse
// @Router /api/v1/auth/user/batch-profile [post]
func (h *UserHandler) BatchGetProfile(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.BatchGetProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 验证用户UUID列表数量（最多100个）
	if len(req.UserUUIDs) == 0 {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	if len(req.UserUUIDs) > 100 {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 3. 调用服务层处理业务逻辑（依赖注入）
	batchResp, err := h.userService.BatchGetProfile(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 4. 返回成功响应
	result.Success(c, batchResp)
}

// GetQRCode 获取用户二维码接口
// @Summary 获取用户二维码
// @Description 生成用户二维码（用于加好友）
// @Tags 用户信息接口
// @Accept json
// @Produce json
// @Success 200 {object} dto.GetQRCodeResponse
// @Router /api/v1/user/qrcode [get]
func (h *UserHandler) GetQRCode(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 调用服务层处理业务逻辑（依赖注入）
	qrcodeResp, err := h.userService.GetQRCode(ctx)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 2. 返回成功响应
	result.Success(c, qrcodeResp)
}

// ParseQRCode 解析二维码接口
// @Summary 解析二维码
// @Description 通过二维码内容获取用户信息
// @Tags 用户信息接口
// @Accept json
// @Produce json
// @Param request body dto.ParseQRCodeRequest true "解析二维码请求"
// @Success 200 {object} dto.ParseQRCodeResponse
// @Router /api/v1/user/parse-qrcode [post]
func (h *UserHandler) ParseQRCode(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.ParseQRCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 调用服务层处理业务逻辑（依赖注入）
	parseResp, err := h.userService.ParseQRCode(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败（如二维码已过期、二维码格式错误等）
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 3. 返回成功响应
	result.Success(c, parseResp)
}

// DeleteAccount 注销账号接口
// @Summary 注销账号
// @Description 注销账号（软删除，30天内可恢复）
// @Tags 用户信息接口
// @Accept json
// @Produce json
// @Param request body dto.DeleteAccountRequest true "注销账号请求"
// @Success 200 {object} dto.DeleteAccountResponse
// @Router /api/v1/auth/user/delete-account [post]
func (h *UserHandler) DeleteAccount(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.DeleteAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 调用服务层处理业务逻辑（依赖注入）
	deleteResp, err := h.userService.DeleteAccount(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败（如密码错误等）
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 3. 返回成功响应
	result.Success(c, deleteResp)
}
