package v1

import (
	"ChatServer/apps/gateway/internal/dto"
	"ChatServer/apps/gateway/internal/middleware"
	"ChatServer/apps/gateway/internal/service"
	"ChatServer/apps/gateway/internal/utils"
	"ChatServer/consts"
	"ChatServer/pkg/logger"
	"ChatServer/pkg/result"

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

		// 其他内部错误
		logger.Error(ctx, "获取个人信息服务内部错误",
			logger.ErrorField("error", err),
		)
		result.Fail(c, nil, consts.CodeInternalError)
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

		// 其他内部错误
		logger.Error(ctx, "获取他人信息服务内部错误",
			logger.ErrorField("error", err),
		)
		result.Fail(c, nil, consts.CodeInternalError)
		return
	}

	// 4. 返回成功响应
	result.Success(c, profileResp)
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

		// 其他内部错误
		logger.Error(ctx, "修改密码服务内部错误",
			logger.ErrorField("error", err),
		)
		result.Fail(c, nil, consts.CodeInternalError)
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
	if req.Nickname == "" && req.Gender == 0 && req.Birthday == "" && req.Signature == "" {
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

		// 其他内部错误
		logger.Error(ctx, "更新基本信息服务内部错误",
			logger.ErrorField("error", err),
		)
		result.Fail(c, nil, consts.CodeInternalError)
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

		// 其他内部错误
		logger.Error(ctx, "换绑邮箱服务内部错误",
			logger.ErrorField("error", err),
		)
		result.Fail(c, nil, consts.CodeInternalError)
		return
	}

	// 3. 返回成功响应
	result.Success(c, emailResp)
}
