package v1

import (
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/middleware"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/service"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/utils"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/result"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	authService service.AuthService
}

func resolveDeviceID(c *gin.Context) string {
	if deviceID := ctxmeta.DeviceIDFromGin(c); deviceID != "" {
		return deviceID
	}

	return strings.TrimSpace(c.GetHeader(ctxmeta.HeaderDeviceID))
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
	// 1. 绑定请求数据
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 设备 ID 优先取 context，其次取 X-Device-ID；空值视为参数错误
	deviceID := resolveDeviceID(c)
	if deviceID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	ctxmeta.SetDeviceID(c, deviceID)

	// 3. 在 device_id 就绪后创建上下文，确保能透传到 user 服务。
	ctx := middleware.NewContextWithGin(c)

	// 4. 调用服务层处理业务逻辑（依赖注入）
	loginResp, err := h.authService.Login(ctx, &req, deviceID)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败（如密码错误、账号锁定等）
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 6. 返回成功响应
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

		result.FailServer(c, err, consts.CodeInternalError)
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

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 3. 返回成功响应
	result.Success(c, sendVerifyCodeResp)
}

// LoginByCode 验证码登录接口
// @Summary 验证码登录
// @Description 用户通过邮箱和验证码直接登录（无需密码）
// @Tags 认证接口
// @Accept json
// @Produce json
// @Param request body dto.LoginByCodeRequest true "验证码登录请求"
// @Success 200 {object} dto.LoginByCodeResponse
// @Router /api/v1/public/login-by-code [post]
func (h *AuthHandler) LoginByCode(c *gin.Context) {
	// 1. 绑定请求数据
	var req dto.LoginByCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 设备 ID 优先取 context，其次取 X-Device-ID；空值视为参数错误
	deviceID := resolveDeviceID(c)
	if deviceID == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	ctxmeta.SetDeviceID(c, deviceID)

	// 3. 在 device_id 就绪后创建上下文，确保能透传到 user 服务。
	ctx := middleware.NewContextWithGin(c)

	// 4. 调用服务层处理业务逻辑（依赖注入）
	loginResp, err := h.authService.LoginByCode(ctx, &req, deviceID)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败（如验证码错误、用户不存在等）
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 5. 返回成功响应
	result.Success(c, loginResp)
}

// Logout 用户登出接口
// @Summary 用户登出
// @Description 用户退出当前设备登录态
// @Tags 认证接口
// @Accept json
// @Produce json
// @Param request body dto.LogoutRequest true "登出请求"
// @Success 200 {object} dto.LogoutResponse
// @Router /api/v1/user/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 调用服务层处理业务逻辑（依赖注入）
	_, err := h.authService.Logout(ctx, &req)
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

	// 3. 返回成功响应
	result.Success(c, nil)
}

// ResetPassword 重置密码接口
// @Summary 重置密码
// @Description 通过邮箱验证码重置密码（忘记密码场景）
// @Tags 认证接口
// @Accept json
// @Produce json
// @Param request body dto.ResetPasswordRequest true "重置密码请求"
// @Success 200 {object} dto.ResetPasswordResponse
// @Router /api/v1/public/reset-password [post]
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 调用服务层处理业务逻辑（依赖注入）
	_, err := h.authService.ResetPassword(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败（如验证码错误、用户不存在等）
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 3. 返回成功响应
	result.Success(c, nil)
}

// RefreshToken 刷新Token接口
// @Summary 刷新Token
// @Description 使用 Refresh Token 刷新 Access Token
// @Tags 认证接口
// @Accept json
// @Produce json
// @Param request body dto.RefreshTokenRequest true "刷新Token请求"
// @Success 200 {object} dto.RefreshTokenResponse
// @Router /api/v1/public/user/refresh-token [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	// 1. 绑定请求数据
	var req dto.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	req.UserUUID = strings.TrimSpace(req.UserUUID)
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	req.RefreshToken = strings.TrimSpace(req.RefreshToken)
	if req.UserUUID == "" || req.DeviceID == "" || req.RefreshToken == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 若请求头提供了设备 ID，必须与 body 保持一致，避免身份来源混乱。
	if headerDeviceID := strings.TrimSpace(c.GetHeader(ctxmeta.HeaderDeviceID)); headerDeviceID != "" && headerDeviceID != req.DeviceID {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 3. 仅在参数校验通过后将身份信息写入上下文。
	ctxmeta.SetUserUUID(c, req.UserUUID)
	ctxmeta.SetDeviceID(c, req.DeviceID)

	// 4. 创建带有用户信息的 context
	ctx := middleware.NewContextWithGin(c)

	// 5. 调用服务层处理业务逻辑（依赖注入）
	resp, err := h.authService.RefreshToken(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败（如Token无效、已过期等）
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 6. 返回成功响应
	result.Success(c, resp)
}

// VerifyCode 校验验证码接口
// @Summary 校验验证码
// @Description 校验验证码是否正确（不消耗验证码）
// @Tags 认证接口
// @Accept json
// @Produce json
// @Param request body dto.VerifyCodeRequest true "校验验证码请求"
// @Success 200 {object} dto.VerifyCodeResponse
// @Router /api/v1/public/user/verify-code [post]
func (h *AuthHandler) VerifyCode(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	// 1. 绑定请求数据
	var req dto.VerifyCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 调用服务层处理业务逻辑（依赖注入）
	resp, err := h.authService.VerifyCode(ctx, &req)
	if err != nil {
		// 检查是否为业务错误
		if consts.IsNonServerError(utils.ExtractErrorCode(err)) {
			// 业务逻辑失败（如验证码错误、已过期等）
			result.Fail(c, nil, utils.ExtractErrorCode(err))
			return
		}

		result.FailServer(c, err, consts.CodeInternalError)
		return
	}

	// 3. 返回成功响应
	result.Success(c, resp)
}
