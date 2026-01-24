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

// AuthHandler 认证处理器
type AuthHandler struct {
	authService service.AuthService
}

// NewAuthHandler 创建认证处理器
// authService: 认证服务
func NewAuthHandler(authService service.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

// Login 用户登录接口
// @Summary 用户登录
// @Description 用户通过手机号和密码登录
// @Tags 认证接口
// @Accept json
// @Produce json
// @Param request body dto.LoginRequest true "登录请求"
// @Success 200 {object} dto.LoginResponse
// @Router /api/v1/public/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 如果context中没有device_id,则从X-Device-ID中获取
	deviceId, exists := c.Get("device_id")
	if !exists {
		// 从X-Device-ID中获取
		deviceId = c.GetHeader("X-Device-ID")
		//如果为空直接返回
		if deviceId == "" {
			logger.Error(ctx, "请求头中无设备ID",
				logger.String("device_id", deviceId.(string)),
			)
			result.Fail(c, nil, consts.CodeParamError)
			return
		}
		// 写入context
		c.Set("device_id", deviceId)
	}

	// 3. 调用服务层处理业务逻辑（依赖注入）
	loginResp, err := h.authService.Login(ctx, &req, deviceId.(string))
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败（如密码错误、账号锁定等）
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		// 其他内部错误
		logger.Error(ctx, "登录服务内部错误",
			logger.ErrorField("error", err),
		)
		result.Fail(c, nil, consts.CodeInternalError)
		return
	}

	// 5. 返回成功响应
	result.Success(c, loginResp)
}

// Register 用户注册接口
// @Summary 用户注册
// @Description 用户通过邮箱和密码注册
// @Tags 认证接口
// @Accept json
// @Produce json
// @Param request body dto.RegisterRequest true "注册请求"
// @Success 200 {object} dto.RegisterResponse
// @Router /api/v1/public/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 调用服务层处理业务逻辑（依赖注入）
	registerResp, err := h.authService.Register(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败（如密码错误、账号锁定等）
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		// 其他内部错误
		logger.Error(ctx, "注册服务内部错误",
			logger.ErrorField("error", err),
		)
		result.Fail(c, nil, consts.CodeInternalError)
		return
	}

	// 5. 返回成功响应
	result.Success(c, registerResp)
}

// SendVerifyCode 发送验证码接口
// @Summary 发送验证码
// @Description 发送验证码
// @Tags 认证接口
// @Accept json
// @Produce json
// @Param request body dto.SendVerifyCodeRequest true "发送验证码请求"
// @Success 200 {object} dto.SendVerifyCodeResponse
// @Router /api/v1/public/send-verify-code [post]
func (h *AuthHandler) SendVerifyCode(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.SendVerifyCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 调用服务层处理业务逻辑（依赖注入）
	sendVerifyCodeResp, err := h.authService.SendVerifyCode(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败（如验证码发送失败等）
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		// 其他内部错误
		logger.Error(ctx, "发送验证码服务内部错误",
			logger.ErrorField("error", err),
		)
		result.Fail(c, nil, consts.CodeInternalError)
		return
	}

	// 3. 返回成功响应
	result.Success(c, sendVerifyCodeResp)
}