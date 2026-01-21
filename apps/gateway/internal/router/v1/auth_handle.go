package v1

import (
	"ChatServer/apps/gateway/internal/dto"
	"ChatServer/apps/gateway/internal/middleware"
	"ChatServer/apps/gateway/internal/service"
	"ChatServer/apps/gateway/internal/utils"
	"ChatServer/consts"
	"ChatServer/pkg/logger"
	"ChatServer/pkg/result"
	"ChatServer/pkg/util"
	"time"

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
	traceId := c.GetString("trace_id")
	ip := c.ClientIP()

	startTime := time.Now()

	// 1. 绑定请求数据
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 参数错误由客户端输入导致,属于正常业务流程,不记录日志
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	// 2. 获取设备ID
	// 优先从 Header 获取设备唯一标识，若无则生成一个新的 UUID 标识当前设备
	deviceId := c.GetHeader("X-Device-ID")
	if deviceId == "" {
		deviceId = util.NewUUID()
		logger.Debug(ctx, "请求头中无设备ID,生成新设备ID",
			logger.String("device_id", deviceId),
		)
	}

	// 3. 调用服务层处理业务逻辑（依赖注入）
	loginResp, err := h.authService.Login(ctx, &req, deviceId)
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

	// 4. 记录登录成功日志
	totalDuration := time.Since(startTime)
	logger.Info(ctx, "登录成功",
		logger.String("trace_id", traceId),
		logger.String("ip", ip),
		logger.String("user_uuid", utils.MaskUUID(loginResp.UserInfo.UUID)),
		logger.String("telephone", utils.MaskTelephone(loginResp.UserInfo.Telephone)),
		logger.String("nickname", loginResp.UserInfo.Nickname),
		logger.String("platform", req.DeviceInfo.Platform),
		logger.Duration("total_duration", totalDuration),
	)

	// 5. 返回成功响应
	result.Success(c, loginResp)
}
