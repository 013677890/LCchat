package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/013677890/LCchat-Backend/apps/auth/internal/repository"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/accountevent"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/ctxmeta"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
	"github.com/013677890/LCchat-Backend/pkg/util"
	"golang.org/x/crypto/bcrypt"
)

// authServiceImpl 实现认证服务的核心登录注册流程。
type authServiceImpl struct {
	authRepo   repository.IAuthRepository
	deviceRepo repository.IDeviceRepository
}

const passwordLoginSlowThreshold = time.Second

// passwordLoginCostSnapshot 记录密码登录各关键步骤的耗时快照，用于定位真实瓶颈。
type passwordLoginCostSnapshot struct {
	totalCost             time.Duration
	queryUserCost         time.Duration
	comparePasswordCost   time.Duration
	getDeviceIDCost       time.Duration
	generateTokenCost     time.Duration
	storeRefreshTokenCost time.Duration
	upsertSessionCost     time.Duration
}

// NewAuthService 创建认证服务实例。
//
// 该服务聚焦 auth 域最核心的登录注册主链路：
//  1. 编排验证码、密码哈希与 Token 生成；
//  2. 读写 user_account 与设备登录态；
//  3. 在注册 / 注销等关键节点写入 outbox 事件，驱动跨服务一致性。
func NewAuthService(authRepo repository.IAuthRepository, deviceRepo repository.IDeviceRepository) AuthService {
	return &authServiceImpl{
		authRepo:   authRepo,
		deviceRepo: deviceRepo,
	}
}

// logPasswordLoginFlow 在密码登录出现系统失败或整体偏慢时输出分段耗时，便于定位慢点。
func logPasswordLoginFlow(ctx context.Context, stage string, err error, snapshot passwordLoginCostSnapshot) {
	if err == nil && snapshot.totalCost < passwordLoginSlowThreshold {
		return
	}

	if err != nil {
		logger.Warn(ctx, "密码登录链路执行异常",
			logger.String("stage", stage),
			logger.Duration("total_cost", snapshot.totalCost),
			logger.Duration("query_user_cost", snapshot.queryUserCost),
			logger.Duration("compare_password_cost", snapshot.comparePasswordCost),
			logger.Duration("get_device_id_cost", snapshot.getDeviceIDCost),
			logger.Duration("generate_token_cost", snapshot.generateTokenCost),
			logger.Duration("store_refresh_token_cost", snapshot.storeRefreshTokenCost),
			logger.Duration("upsert_session_cost", snapshot.upsertSessionCost),
			logger.ErrorField("error", err),
		)
		return
	}

	logger.Warn(ctx, "密码登录链路耗时偏慢",
		logger.String("stage", stage),
		logger.Duration("total_cost", snapshot.totalCost),
		logger.Duration("query_user_cost", snapshot.queryUserCost),
		logger.Duration("compare_password_cost", snapshot.comparePasswordCost),
		logger.Duration("get_device_id_cost", snapshot.getDeviceIDCost),
		logger.Duration("generate_token_cost", snapshot.generateTokenCost),
		logger.Duration("store_refresh_token_cost", snapshot.storeRefreshTokenCost),
		logger.Duration("upsert_session_cost", snapshot.upsertSessionCost),
	)
}

// Register 用户注册。
// 业务流程：
//  1. 校验邮箱对应的注册验证码；
//  2. 生成密码哈希并组装 user_account 初始快照；
//  3. 在同一事务中写入账号记录与 user_created outbox 事件；
//  4. 返回注册后的账号展示信息。
//
// 错误码映射：
//   - codes.AlreadyExists: 账号已存在
//   - codes.InvalidArgument: 验证码错误或已失效
//   - codes.Internal: 系统内部错误
func (s *authServiceImpl) Register(ctx context.Context, req *authpb.RegisterRequest) (*authpb.RegisterResponse, error) {
	// 先校验注册验证码，避免无效请求占用数据库写资源。
	isValid, err := s.authRepo.VerifyVerifyCode(ctx, req.Email, req.VerifyCode, 1)
	if err != nil {
		if errors.Is(err, repoerr.ErrRedisNil) {
			return nil, apperr.New(consts.CodeVerifyCodeError)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "校验验证码失败")
	}

	if !isValid {
		return nil, apperr.New(consts.CodeVerifyCodeError)
	}

	// 只有验证码通过后才计算 bcrypt，避免对明显无效请求浪费 CPU。
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "生成密码哈希失败")
	}

	// 注册时由 auth-service 直接写入 user_account，并冗余登录所需展示字段。
	user := &model.UserAccount{
		UserUuid:      util.GenIDString(),
		Email:         req.Email,
		PasswordHash:  string(hashedPassword),
		Telephone:     strings.TrimSpace(req.Telephone),
		Status:        0,
		IsAdmin:       0,
		LoginNickname: req.Nickname,
		LoginAvatar:   "",
	}

	// 注册成功后需要异步打通资料初始化闭环，因此在同一事务里写入 user_created outbox 事件。
	eventID := util.GenIDString()
	payload, err := accountevent.Encode(accountevent.UserCreatedPayload{
		EventID:  eventID,
		UserUUID: user.UserUuid,
		Nickname: user.LoginNickname,
		Avatar:   user.LoginAvatar,
	})

	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "序列化注册事件失败")
	}

	// 把 user_account 与 user_created 事件放进同一事务，确保资料初始化链路有可靠事实来源。
	createdUser, err := s.authRepo.CreateWithOutboxEvent(ctx, user, accountevent.EventTypeUserCreated, payload)
	if err != nil {
		if errors.Is(err, repoerr.ErrDuplicateKey) {
			return nil, apperr.New(consts.CodeUserAlreadyExist)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "创建用户失败")
	}

	return buildRegisterResponse(createdUser), nil
}

// Login 用户密码登录。
// 业务流程：
//  1. 为缺失的设备信息补默认值，避免后续空指针；
//  2. 按账号查询 user_account 并校验账号状态；
//  3. 校验密码、提取 device_id 与客户端 IP；
//  4. 生成 AccessToken / RefreshToken，只把承担续期职责的 RefreshToken 写入 Redis；
//  5. 尽力更新设备会话，最后返回登录响应。
//
// 错误码映射：
//   - codes.NotFound: 用户不存在
//   - codes.PermissionDenied: 账号被禁用或密码错误
//   - codes.InvalidArgument: 设备 ID 缺失
//   - codes.Internal: 系统内部错误
func (s *authServiceImpl) Login(ctx context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error) {
	loginStartedAt := time.Now()
	snapshot := passwordLoginCostSnapshot{}

	// 设备信息允许缺省；这里补默认值，避免后续 GetXxx 调用遇到 nil 指针。
	if req.DeviceInfo == nil {
		req.DeviceInfo = &authpb.DeviceInfo{DeviceName: "Unknown", Platform: "Unknown"}
	}

	// 第一步先按账号读取 user_account，后续密码校验和状态判断都依赖这份快照。
	queryUserStartedAt := time.Now()
	user, err := s.authRepo.GetByEmail(ctx, req.Account)
	snapshot.queryUserCost = time.Since(queryUserStartedAt)
	if err != nil {
		if errors.Is(err, repoerr.ErrRecordNotFound) {
			return nil, apperr.New(consts.CodeUserNotFound)
		}
		snapshot.totalCost = time.Since(loginStartedAt)
		logPasswordLoginFlow(ctx, "get_user", err, snapshot)
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询用户失败")
	}

	if user.Status == 1 {
		return nil, apperr.New(consts.CodeUserDisabled)
	}

	// 把 user_uuid 写回上下文，便于后续降级日志和 repository 调用复用同一身份信息。
	ctx = ctxmeta.WithUserUUID(ctx, user.UserUuid)

	// 密码错误时直接返回业务错误，不继续生成 token。
	comparePasswordStartedAt := time.Now()
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		snapshot.comparePasswordCost = time.Since(comparePasswordStartedAt)
		return nil, apperr.New(consts.CodePasswordError)
	}
	snapshot.comparePasswordCost = time.Since(comparePasswordStartedAt)

	// device_id 是设备级登录态隔离主键，缺失时直接拒绝登录。
	getDeviceIDStartedAt := time.Now()
	deviceID, err := getRequiredDeviceID(ctx)
	snapshot.getDeviceIDCost = time.Since(getDeviceIDStartedAt)
	if err != nil {
		return nil, err
	}

	// IP 仅用于设备会话落库展示，不参与 token 生成。
	clientIP := util.GetClientIPFromContext(ctx)

	// AccessToken 是短期、自包含的 JWT，只返回给客户端，不在 Redis 保留副本；
	// RefreshToken 是设备级长期续期凭据，服务端必须持有其状态以支持主动撤销。
	generateTokenStartedAt := time.Now()
	accessToken, err := util.GenerateToken(user.UserUuid, deviceID)
	if err != nil {
		snapshot.generateTokenCost = time.Since(generateTokenStartedAt)
		snapshot.totalCost = time.Since(loginStartedAt)
		logPasswordLoginFlow(ctx, "generate_token", err, snapshot)
		return nil, apperr.Wrap(err, consts.CodeInternalError, "生成访问令牌失败")
	}

	refreshToken, err := util.GenerateRefreshToken()
	snapshot.generateTokenCost = time.Since(generateTokenStartedAt)
	if err != nil {
		snapshot.totalCost = time.Since(loginStartedAt)
		logPasswordLoginFlow(ctx, "generate_refresh_token", err, snapshot)
		return nil, apperr.Wrap(err, consts.CodeInternalError, "生成 RefreshToken 失败")
	}

	// RefreshToken 是服务端唯一持久化的 Token 状态。即使 AccessToken 已经在内存中生成，
	// 写入失败时也不能把任何 Token 返回客户端，否则会形成无法续期的不完整登录态。
	storeRefreshTokenStartedAt := time.Now()
	if err := s.deviceRepo.StoreRefreshToken(ctx, user.UserUuid, deviceID, refreshToken, util.RefreshExpire); err != nil {
		snapshot.storeRefreshTokenCost = time.Since(storeRefreshTokenStartedAt)
		snapshot.totalCost = time.Since(loginStartedAt)
		logPasswordLoginFlow(ctx, "store_refresh_token", err, snapshot)
		return nil, apperr.Wrap(err, consts.CodeInternalError, "写入 RefreshToken 失败")
	}

	snapshot.storeRefreshTokenCost = time.Since(storeRefreshTokenStartedAt)

	// 设备会话只承载设备展示与在线态所需的最小字段。
	deviceSession := &model.DeviceSession{
		UserUuid:   user.UserUuid,
		DeviceId:   deviceID,
		DeviceName: req.DeviceInfo.GetDeviceName(),
		Platform:   req.DeviceInfo.GetPlatform(),
		AppVersion: req.DeviceInfo.GetAppVersion(),
		IP:         clientIP,
		UserAgent:  buildDeviceUserAgent(req.DeviceInfo),
		Status:     model.DeviceStatusOnline,
	}

	// 设备会话落库失败只做告警，不回滚已经成功生成的登录态。
	upsertSessionStartedAt := time.Now()
	if err := s.deviceRepo.UpsertSession(ctx, deviceSession); err != nil {
		snapshot.upsertSessionCost = time.Since(upsertSessionStartedAt)
		logger.Warn(ctx, "设备会话落库失败，按降级处理继续登录", logger.ErrorField("error", err))
	}
	snapshot.upsertSessionCost = time.Since(upsertSessionStartedAt)

	snapshot.totalCost = time.Since(loginStartedAt)
	logPasswordLoginFlow(ctx, "completed", nil, snapshot)

	return buildLoginResponse(accessToken, refreshToken, int64(util.AccessExpire.Seconds()), user), nil
}

// LoginByCode 用户验证码登录。
// 业务流程：
//  1. 为缺失的设备信息补默认值；
//  2. 按邮箱查询账号并校验账号状态；
//  3. 校验登录验证码并在成功后尽力删除验证码；
//  4. 生成两类 Token，只保存设备级 RefreshToken；
//  5. 尽力回写设备会话和活跃时间，再返回登录结果。
//
// 错误码映射：
//   - codes.NotFound: 用户不存在
//   - codes.InvalidArgument: 验证码错误、已过期或设备 ID 缺失
//   - codes.PermissionDenied: 账号被禁用
//   - codes.Internal: 系统内部错误
func (s *authServiceImpl) LoginByCode(ctx context.Context, req *authpb.LoginByCodeRequest) (*authpb.LoginByCodeResponse, error) {
	// 验证码登录与密码登录共用同一套设备会话模型，因此同样先补默认设备信息。
	if req.DeviceInfo == nil {
		req.DeviceInfo = &authpb.DeviceInfo{DeviceName: "Unknown", Platform: "Unknown"}
	}

	// 先查账号快照，避免对不存在账号继续做验证码校验和 token 写入。
	user, err := s.authRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, repoerr.ErrRecordNotFound) {
			return nil, apperr.New(consts.CodeUserNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询用户失败")
	}

	if user.Status == 1 {
		return nil, apperr.New(consts.CodeUserDisabled)
	}

	// 设备级验证码登录同样要求 device_id，用于隔离多端 RefreshToken。
	deviceID, err := getRequiredDeviceID(ctx)
	if err != nil {
		return nil, err
	}

	// type=2 表示登录验证码；这里显式区分过期和错误两种业务反馈。
	isValid, err := s.authRepo.VerifyVerifyCode(ctx, req.Email, req.VerifyCode, 2)
	if err != nil {
		if errors.Is(err, repoerr.ErrRedisNil) {
			return nil, apperr.New(consts.CodeVerifyCodeExpire)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "校验验证码失败")
	}

	if !isValid {
		return nil, apperr.New(consts.CodeVerifyCodeError)
	}

	// 验证码消费成功后尽力删除；即使删除失败，也不影响本次登录结果。
	if err := s.authRepo.DeleteVerifyCode(ctx, req.Email, 2); err != nil {
		logger.Warn(ctx, "删除验证码失败", logger.ErrorField("error", err))
	}

	// 从这里开始进入与密码登录一致的设备级 token 签发流程。
	ctx = ctxmeta.WithUserUUID(ctx, user.UserUuid)
	clientIP := util.GetClientIPFromContext(ctx)
	accessToken, err := util.GenerateToken(user.UserUuid, deviceID)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "生成访问令牌失败")
	}
	refreshToken, err := util.GenerateRefreshToken()
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "生成 RefreshToken 失败")
	}

	// AccessToken 不落 Redis；RefreshToken 写入成功才表示设备获得了完整、可续期的登录态。
	if err := s.deviceRepo.StoreRefreshToken(ctx, user.UserUuid, deviceID, refreshToken, util.RefreshExpire); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "写入 RefreshToken 失败")
	}

	deviceSession := &model.DeviceSession{
		UserUuid:   user.UserUuid,
		DeviceId:   deviceID,
		DeviceName: req.DeviceInfo.GetDeviceName(),
		Platform:   req.DeviceInfo.GetPlatform(),
		AppVersion: req.DeviceInfo.GetAppVersion(),
		IP:         clientIP,
		UserAgent:  buildDeviceUserAgent(req.DeviceInfo),
		Status:     model.DeviceStatusOnline,
	}

	// 会话仍按 best-effort 处理，失败不影响已经成功签发的登录态。
	if err := s.deviceRepo.UpsertSession(ctx, deviceSession); err != nil {
		logger.Warn(ctx, "设备会话落库失败，按降级处理继续登录", logger.ErrorField("error", err))
	}

	return buildLoginByCodeResponse(accessToken, refreshToken, int64(util.AccessExpire.Seconds()), user), nil
}

// SendVerifyCode 发送验证码。
// 业务流程：
//  1. 校验邮箱格式；
//  2. 基于邮箱和 IP 原子执行分钟级 / 天级 / IP 级限流检查与计数占位；
//  3. 生成验证码并写入 Redis；
//  4. 发送邮件并返回过期时间。
//
// 错误码映射：
//   - codes.InvalidArgument: 邮箱非法
//   - codes.ResourceExhausted: 发送过于频繁
//   - codes.Internal: 系统内部错误
func (s *authServiceImpl) SendVerifyCode(ctx context.Context, req *authpb.SendVerifyCodeRequest) (*authpb.SendVerifyCodeResponse, error) {
	// 邮箱格式错误时直接返回，避免无意义地进入 Redis 限流和邮件发送流程。
	if !util.ValidateEmail(req.Email) {
		return nil, apperr.New(consts.CodeInvalidEmail)
	}

	// 限流依赖“邮箱 + IP”双维度，检查与计数占位必须原子完成。
	ip := util.GetClientIPFromContext(ctx)
	isLimited, err := s.authRepo.CheckAndIncrementVerifyCodeRateLimit(ctx, req.Email, ip)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "验证码限流检查失败")
	}
	if isLimited {
		return nil, apperr.New(consts.CodeSendTooFrequent)
	}

	// 验证码随机生成，后续会带着 2 分钟 TTL 写入 Redis。
	code, err := util.GenerateVerifyCode(6)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "生成验证码失败")
	}

	// 先存 Redis，再发邮件，保证邮件中携带的是当前真实生效验证码。
	if err := s.authRepo.StoreVerifyCode(ctx, req.Email, code, req.Type, 2*time.Minute); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "存储验证码失败")
	}

	// 最后才真正发邮件，避免 Redis 未落盘时出现“用户收到验证码但系统不认”的情况。
	if err := util.SendVerifyCodeEmail(req.Email, code, 2); err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "发送验证码邮件失败")
	}

	return &authpb.SendVerifyCodeResponse{ExpireSeconds: 120}, nil
}

// VerifyCode 校验验证码。
// 业务流程：
//  1. 按邮箱和验证码类型查询 Redis 中的验证码；
//  2. 区分“验证码过期”和“验证码不匹配”；
//  3. 返回布尔校验结果给上游调用方。
//
// 错误码映射：
//   - codes.InvalidArgument: 验证码已过期
//   - codes.Internal: 系统内部错误
func (s *authServiceImpl) VerifyCode(ctx context.Context, req *authpb.VerifyCodeRequest) (*authpb.VerifyCodeResponse, error) {
	// 校验接口本身不消费验证码，只返回当前验证码是否仍然匹配。
	isValid, err := s.authRepo.VerifyVerifyCode(ctx, req.Email, req.VerifyCode, req.Type)
	if err != nil {
		if errors.Is(err, repoerr.ErrRedisNil) {
			return nil, apperr.New(consts.CodeVerifyCodeExpire)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "校验验证码失败")
	}
	return &authpb.VerifyCodeResponse{Valid: isValid}, nil
}

// RefreshToken 刷新 AccessToken。
// 业务流程：
//  1. 从上下文中提取 user_uuid 与 device_id；
//  2. 校验当前设备保存的 RefreshToken；
//  3. 重新查询账号，阻止已注销或禁用账号通过旧 RefreshToken 延长登录态；
//  4. 生成无状态 AccessToken；
//  5. 尽力续期设备信息缓存 TTL，并返回新的访问令牌信息。
//
// 错误码映射：
//   - codes.Unauthenticated: Token 非法或设备不存在
//   - codes.Internal: 系统内部错误
func (s *authServiceImpl) RefreshToken(ctx context.Context, req *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error) {
	// refresh token 链路必须依赖上下文中的 user_uuid 与 device_id 两个维度定位设备登录态。
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return nil, apperr.New(consts.CodeInvalidToken)
	}
	deviceID := util.GetDeviceIDFromContext(ctx)
	if deviceID == "" {
		return nil, apperr.New(consts.CodeInvalidToken)
	}

	// 先读取当前设备保存的 RefreshToken，再做字面量比较，防止跨设备串用续签。
	storedRefreshToken, err := s.deviceRepo.GetRefreshToken(ctx, userUUID, deviceID)
	if err != nil {
		if errors.Is(err, repoerr.ErrRedisNil) {
			return nil, apperr.New(consts.CodeDeviceNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取 Refresh Token 失败")
	}

	if storedRefreshToken != req.RefreshToken {
		return nil, apperr.New(consts.CodeInvalidToken)
	}

	// RefreshToken 只能证明客户端持有某台设备的续期凭据，不能证明账号当前仍允许登录。
	// 账号状态必须在每次续期时重新读取，否则注销或禁用只能阻止新登录，旧 RefreshToken
	// 仍可持续签发新的 AccessToken，突破 AccessToken 原本有限的过期窗口。
	user, err := s.authRepo.GetByUserUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repoerr.ErrRecordNotFound) {
			// 账号已经不存在时，该续期凭据不应保留。撤销失败只记录告警，因为当前请求已经
			// 被拒绝，不会签发 AccessToken；后续请求仍会重复执行同一账号状态校验。
			if revokeErr := s.deviceRepo.DeleteRefreshToken(ctx, userUUID, deviceID); revokeErr != nil {
				logger.Warn(ctx, "账号不存在且撤销 RefreshToken 失败",
					logger.String("user_uuid", userUUID),
					logger.String("device_id", deviceID),
					logger.ErrorField("error", revokeErr),
				)
			}
			return nil, apperr.New(consts.CodeInvalidToken)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "刷新 Token 时查询账号失败")
	}
	if user.Status == 1 {
		// 禁用/注销状态不允许保留可在未来复用的续期凭据。这里不影响已签发 AccessToken
		// 的有限窗口，只切断后续续期能力。
		if revokeErr := s.deviceRepo.DeleteRefreshToken(ctx, userUUID, deviceID); revokeErr != nil {
			logger.Warn(ctx, "账号不可用且撤销 RefreshToken 失败",
				logger.String("user_uuid", userUUID),
				logger.String("device_id", deviceID),
				logger.ErrorField("error", revokeErr),
			)
		}
		return nil, apperr.New(consts.CodeUserDisabled)
	}

	// 通过设备凭据和账号状态双重校验后重新签发 AccessToken。AccessToken 是自包含 JWT，
	// 不写 Redis；同一设备先前签发的 AccessToken 按设计继续有效到各自的 exp。
	newAccessToken, err := util.GenerateToken(userUUID, deviceID)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "生成 Access Token 失败")
	}

	// 设备信息 TTL 续期只影响设备列表缓存命中率，不影响 Refresh 主流程成功与否。
	if err := s.deviceRepo.TouchDeviceInfoTTL(ctx, userUUID); err != nil {
		logger.Warn(ctx, "续期设备信息缓存失败",
			logger.String("user_uuid", userUUID),
			logger.ErrorField("error", err),
		)
	}

	return &authpb.RefreshTokenResponse{
		AccessToken: newAccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int64(util.AccessExpire.Seconds()),
	}, nil
}

// Logout 退出当前设备登录态。
// 业务流程：
//  1. 校验请求中的 device_id；
//  2. 从上下文中获取当前登录用户；
//  3. 删除该设备的 RefreshToken，阻止后续续期；
//  4. 将设备会话更新为已登出状态（该迁移时刻即设备的 last_seen）。
//
// 错误码映射：
//   - codes.InvalidArgument: device_id 缺失
//   - codes.Internal: 系统内部错误
func (s *authServiceImpl) Logout(ctx context.Context, req *authpb.LogoutRequest) error {
	// 退出登录必须显式指定 device_id，避免误删其他设备的登录态。
	if req == nil || req.DeviceId == "" {
		return apperr.New(consts.CodeParamError)
	}

	// 当前实现默认只允许已认证用户主动登出自己某个设备。
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return apperr.New(consts.CodeInternalError)
	}

	// AccessToken 是无状态 JWT，登出不会建立黑名单；它会在自身 exp 到达后自然失效。
	// 此处撤销 RefreshToken，保证设备不能跨过这个有限窗口继续签发新的 AccessToken。
	if err := s.deviceRepo.DeleteRefreshToken(ctx, userUUID, req.DeviceId); err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "撤销 RefreshToken 失败")
	}

	// 再把设备会话标记为 logged_out；如果会话本身不存在，则按幂等成功处理。
	if err := s.deviceRepo.UpdateOnlineStatus(ctx, userUUID, req.DeviceId, model.DeviceStatusLoggedOut); err != nil {
		if !errors.Is(err, repoerr.ErrRecordNotFound) {
			return apperr.Wrap(err, consts.CodeInternalError, "更新设备注销状态失败")
		}
		logger.Warn(ctx, "登出时设备会话不存在，按幂等成功处理",
			logger.String("user_uuid", userUUID),
			logger.String("device_id", req.DeviceId),
		)
	}

	return nil
}

// ResetPassword 通过验证码重置密码。
// 业务流程：
//  1. 根据邮箱查询账号；
//  2. 校验重置密码验证码；
//  3. 拒绝把新密码设置成旧密码；
//  4. 生成新的密码哈希并更新数据库；
//  5. 撤销全部设备的 RefreshToken，并尽力删除已消费的验证码。
//
// 错误码映射：
//   - codes.NotFound: 用户不存在
//   - codes.InvalidArgument: 验证码错误、已过期或新旧密码相同
//   - codes.Internal: 系统内部错误
func (s *authServiceImpl) ResetPassword(ctx context.Context, req *authpb.ResetPasswordRequest) error {
	// 先按邮箱定位账号，避免对不存在账号做验证码与哈希计算。
	user, err := s.authRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, repoerr.ErrRecordNotFound) {
			return apperr.New(consts.CodeUserNotFound)
		}
		return apperr.Wrap(err, consts.CodeInternalError, "查询用户失败")
	}

	// type=3 对应重置密码验证码；这里同样区分“过期”和“错误”。
	isValid, err := s.authRepo.VerifyVerifyCode(ctx, req.Email, req.VerifyCode, 3)
	if err != nil {
		if errors.Is(err, repoerr.ErrRedisNil) {
			return apperr.New(consts.CodeVerifyCodeExpire)
		}
		return apperr.Wrap(err, consts.CodeInternalError, "校验验证码失败")
	}

	if !isValid {
		return apperr.New(consts.CodeVerifyCodeError)
	}

	// 阻止把新密码重置成旧密码，保持与修改密码链路一致的业务语义。
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.NewPassword)); err == nil {
		return apperr.New(consts.CodePasswordSameAsOld)
	}

	// 验证通过后再生成新的 bcrypt 哈希。
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "生成密码哈希失败")
	}

	// 密码重置通常意味着旧凭据可能已经泄漏。AccessToken 按无状态设计保留到 exp，
	// 但所有 RefreshToken 必须撤销，避免旧设备在过期窗口之后继续续期。
	if err := s.authRepo.UpdatePassword(ctx, user.UserUuid, string(hashedPassword)); err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "更新密码失败")
	}
	if err := s.deviceRepo.DeleteByUserUUID(ctx, user.UserUuid); err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "密码已重置，但撤销设备续期凭据失败")
	}

	// 验证码是一次性凭据。删除失败只影响重复提交防护，不应回滚已经完成的密码更新
	// 和设备续期凭据撤销，因此沿用 best-effort 处理。
	if err := s.authRepo.DeleteVerifyCode(ctx, req.Email, 3); err != nil {
		logger.Warn(ctx, "删除验证码失败", logger.ErrorField("error", err))
	}
	return nil
}
