package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/013677890/LCchat-Backend/apps/user/internal/converter"
	"github.com/013677890/LCchat-Backend/apps/user/internal/repository"
	"github.com/013677890/LCchat-Backend/apps/user/internal/utils"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/util"
	"regexp"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// userServiceImpl 用户信息服务实现
type userServiceImpl struct {
	userRepo   repository.IUserRepository
	authRepo   repository.IAuthRepository
	deviceRepo repository.IDeviceRepository
}

// NewUserService 创建用户信息服务实例
func NewUserService(userRepo repository.IUserRepository, authRepo repository.IAuthRepository, deviceRepo repository.IDeviceRepository) UserService {
	return &userServiceImpl{
		userRepo:   userRepo,
		authRepo:   authRepo,
		deviceRepo: deviceRepo,
	}
}

// GetProfile 获取个人信息
// 业务流程：
//  1. 从context中获取用户UUID
//  2. 查询用户信息
//  3. 转换为Protobuf格式并返回
//
// 错误码映射：
//   - codes.NotFound: 用户不存在
//   - codes.Internal: 系统内部错误
func (s *userServiceImpl) GetProfile(ctx context.Context, req *pb.GetProfileRequest) (*pb.GetProfileResponse, error) {
	// 1. 从context中获取用户UUID
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 2. 查询用户信息
	userInfo, err := s.userRepo.GetByUUID(ctx, userUUID)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询用户信息失败")
	}

	if userInfo == nil {
		return nil, apperr.New(consts.CodeUserNotFound)
	}

	// 3. 转换为Protobuf格式并返回
	return &pb.GetProfileResponse{
		UserInfo: converter.ModelToProtoUserInfo(userInfo),
	}, nil
}

// GetOtherProfile 获取他人信息
// 业务流程：
//  1. 从context中获取当前用户UUID
//  2. 查询目标用户信息
//  3. 判断是否为好友关系
//  4. 非好友时脱敏邮箱和手机号
//  5. 返回用户信息
//
// 错误码映射：
//   - codes.NotFound: 用户不存在
//   - codes.Internal: 系统内部错误
func (s *userServiceImpl) GetOtherProfile(ctx context.Context, req *pb.GetOtherProfileRequest) (*pb.GetOtherProfileResponse, error) {
	// 1. 查询目标用户信息
	targetUserInfo, err := s.userRepo.GetByUUID(ctx, req.UserUuid)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询用户信息失败")
	}

	if targetUserInfo == nil {
		return nil, apperr.New(consts.CodeUserNotFound)
	}

	// 2. 返回用户信息（脱敏由Gateway层负责）
	return &pb.GetOtherProfileResponse{
		UserInfo: converter.ModelToProtoUserInfo(targetUserInfo),
	}, nil
}

// SearchUser 搜索用户
// 业务流程：
//  1. 从context中获取当前用户UUID（用于鉴权）
//  2. 调用userRepo搜索用户（按邮箱、昵称、UUID）
//  3. 组装响应（不返回 email）
//
// 错误码映射：
//   - codes.InvalidArgument: 关键词太短
//   - codes.Internal: 系统内部错误
func (s *userServiceImpl) SearchUser(ctx context.Context, req *pb.SearchUserRequest) (*pb.SearchUserResponse, error) {
	// 1. 从context中获取当前用户UUID
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 2. 调用搜索用户
	users, total, err := s.userRepo.SearchUser(ctx, req.Keyword, int(req.Page), int(req.PageSize))
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "搜索用户失败")
	}

	if len(users) == 0 {
		// 没有搜索到结果，返回空列表
		return &pb.SearchUserResponse{
			Items: []*pb.SimpleUserItem{},
			Pagination: &pb.PaginationInfo{
				Page:       req.Page,
				PageSize:   req.PageSize,
				Total:      total,
				TotalPages: int32((total + int64(req.PageSize) - 1) / int64(req.PageSize)),
			},
		}, nil
	}

	// 3. 构建响应（不返回 email，isFriend 由网关聚合）
	items := make([]*pb.SimpleUserItem, len(users))
	for i, user := range users {
		items[i] = &pb.SimpleUserItem{
			Uuid:      user.Uuid,
			Nickname:  user.Nickname,
			Avatar:    user.Avatar,
			Signature: user.Signature,
			IsFriend:  false,
		}
	}

	// 4. 计算总页数
	totalPages := int32((total + int64(req.PageSize) - 1) / int64(req.PageSize))

	// 5. 返回搜索结果
	return &pb.SearchUserResponse{
		Items: items,
		Pagination: &pb.PaginationInfo{
			Page:       req.Page,
			PageSize:   req.PageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	}, nil
}

// UpdateProfile 更新基本信息
// 业务流程：
//  1. 从context中获取用户UUID
//  2. 验证请求参数（至少提供一个字段）
//  3. 如果更新昵称，检查昵称是否已被使用（排除自己）
//  4. 更新基本信息
//  5. 查询更新后的用户信息
//  6. 转换为Protobuf格式并返回
//
// 错误码映射：
//   - codes.NotFound: 用户不存在
//   - codes.AlreadyExists: 昵称已被使用
//   - codes.InvalidArgument: 参数验证失败
//   - codes.Internal: 系统内部错误
func (s *userServiceImpl) UpdateProfile(ctx context.Context, req *pb.UpdateProfileRequest) (*pb.UpdateProfileResponse, error) {
	// 1. 从context中获取用户UUID
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 2. 验证请求参数（至少提供一个字段）
	if req.Nickname == "" && req.Birthday == "" && req.Signature == "" && req.Gender == 0 {
		return nil, apperr.New(consts.CodeParamError)
	}

	// 2.1 如果提供了生日，验证生日格式
	if req.Birthday != "" {
		// 验证生日格式 (YYYY-MM-DD)
		birthdayPattern := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
		if !birthdayPattern.MatchString(req.Birthday) {
			return nil, apperr.New(consts.CodeBirthdayFormatError)
		}

		// 验证生日是否是有效日期
		_, err := time.Parse("2006-01-02", req.Birthday)
		if err != nil {
			return nil, apperr.New(consts.CodeBirthdayFormatError)
		}
	}

	// 3. 更新基本信息
	userInfo, err := s.userRepo.UpdateBasicInfoWithDisplayEvent(ctx, userUUID, req.Nickname, req.Signature, req.Birthday, int8(req.Gender))
	if err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return nil, apperr.New(consts.CodeUserNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "更新基本信息失败")
	}

	// 4. 仓储层已经返回更新后的资料快照，这里只保留空值兜底判断。
	if userInfo == nil {
		return nil, apperr.New(consts.CodeUserNotFound)
	}

	// 5. 转换为Protobuf格式并返回
	return &pb.UpdateProfileResponse{
		UserInfo: converter.ModelToProtoUserInfo(userInfo),
	}, nil
}

// UploadAvatar 上传头像
// UploadAvatar 上传头像
// 业务流程：
//  1. 从context中获取用户UUID
//  2. 验证头像URL不为空
//  3. 更新数据库中的头像字段
//  4. 返回新的头像URL
//
// 错误码映射：
//   - codes.InvalidArgument: 头像URL为空
//   - codes.Internal: 系统内部错误
func (s *userServiceImpl) UploadAvatar(ctx context.Context, req *pb.UploadAvatarRequest) (*pb.UploadAvatarResponse, error) {
	// 1. 从context中获取用户UUID
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 2. 验证头像URL不为空
	if req.AvatarUrl == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	// 3. 更新数据库中的头像字段，并写入登录展示字段回写事件。
	userInfo, err := s.userRepo.UpdateAvatarWithDisplayEvent(ctx, userUUID, req.AvatarUrl)
	if err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return nil, apperr.New(consts.CodeUserNotFound)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "更新头像失败")
	}
	if userInfo == nil {
		return nil, apperr.New(consts.CodeUserNotFound)
	}

	// 4. 返回新的头像URL
	return &pb.UploadAvatarResponse{
		AvatarUrl: userInfo.Avatar,
	}, nil
}

// ChangePassword 修改密码
// 业务流程：
//  1. 从context中获取用户UUID
//  2. 查询用户信息
//  3. 验证旧密码是否正确
//  4. 验证新密码不能与旧密码相同
//  5. 生成新密码哈希
//  6. 更新密码
//  7. 踢出其他所有设备的登录态
//
// 错误码映射：
//   - codes.NotFound: 用户不存在
//   - codes.Unauthenticated: 旧密码错误
//   - codes.FailedPrecondition: 新密码不能与旧密码相同
//   - codes.Internal: 系统内部错误
func (s *userServiceImpl) ChangePassword(ctx context.Context, req *pb.ChangePasswordRequest) error {
	// 1. 从context中获取用户UUID
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}

	// 2. 查询用户信息
	userInfo, err := s.userRepo.GetByUUID(ctx, userUUID)
	if err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "查询用户信息失败")
	}

	if userInfo == nil {
		return apperr.New(consts.CodeUserNotFound)
	}

	// 3. 校验旧密码是否正确
	err = bcrypt.CompareHashAndPassword([]byte(userInfo.Password), []byte(req.OldPassword))
	if err != nil {
		return apperr.New(consts.CodePasswordError)
	}

	// 4. 校验新密码是否与旧密码相同
	err = bcrypt.CompareHashAndPassword([]byte(userInfo.Password), []byte(req.NewPassword))
	if err == nil {
		// 密码相同
		return apperr.New(consts.CodePasswordSameAsOld)
	}

	// 5. 生成新密码哈希
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "生成密码哈希失败")
	}

	// 6. 更新密码
	err = s.userRepo.UpdatePassword(ctx, userUUID, string(hashedPassword))
	if err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "更新密码失败")
	}

	// 7. 踢出其他所有设备的登录态（删除所有设备的token）
	// 注意：当前设备保持登录态，其他设备被踢出
	// 这里需要在repository中实现踢出其他设备的方法，暂时跳过
	// TODO: 实现踢出其他设备登录态

	return nil
}

// ChangeEmail 绑定/换绑邮箱
// 业务流程：
//  1. 从context中获取用户UUID
//  2. 检查新邮箱是否已被使用
//  3. 校验验证码是否正确
//  4. 更新邮箱
//  5. 删除验证码
//
// 错误码映射：
//   - codes.NotFound: 用户不存在
//   - codes.AlreadyExists: 邮箱已被使用
//   - codes.Unauthenticated: 验证码错误或已过期
//   - codes.Internal: 系统内部错误
func (s *userServiceImpl) ChangeEmail(ctx context.Context, req *pb.ChangeEmailRequest) (*pb.ChangeEmailResponse, error) {
	// 1. 从context中获取用户UUID
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 访问日志已经由统一拦截器记录，这里不重复记录换绑邮箱入口日志。
	// 保留下方业务校验、错误处理和成功结果日志即可。

	// 2. 检查新邮箱是否已被使用
	exists, err := s.userRepo.ExistsByEmail(ctx, req.NewEmail)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "检查邮箱是否存在失败")
	}
	if exists {
		return nil, apperr.New(consts.CodeEmailAlreadyExist)
	}

	// 3. 校验验证码（type=4: 换绑邮箱）
	isValid, err := s.authRepo.VerifyVerifyCode(ctx, req.NewEmail, req.VerifyCode, 4)
	if err != nil {
		// 判断是 Redis Key 不存在还是其他错误
		if errors.Is(err, repository.ErrRedisNil) {
			return nil, apperr.New(consts.CodeVerifyCodeExpire)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "校验验证码失败")
	}
	if !isValid {
		return nil, apperr.New(consts.CodeVerifyCodeError)
	}

	// 4. 查询用户当前信息，获取旧邮箱用于日志记录
	userInfo, err := s.userRepo.GetByUUID(ctx, userUUID)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询用户信息失败")
	}
	if userInfo == nil {
		return nil, apperr.New(consts.CodeUserNotFound)
	}

	// 5. 更新邮箱
	err = s.userRepo.UpdateEmail(ctx, userUUID, req.NewEmail)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "更新邮箱失败")
	}

	// 6. 删除验证码（type=4: 换绑邮箱）
	if err := s.authRepo.DeleteVerifyCode(ctx, req.NewEmail, 4); err != nil {
		logger.Warn(ctx, "删除验证码失败",
			logger.String("email", utils.MaskEmail(req.NewEmail)),
			logger.ErrorField("error", err),
		)
		// 删除失败不影响换绑邮箱流程，只记录警告日志
	}

	// 7. 换绑成功
	return &pb.ChangeEmailResponse{
		Email: req.NewEmail,
	}, nil
}

// ChangeTelephone 绑定/换绑手机
func (s *userServiceImpl) ChangeTelephone(ctx context.Context, req *pb.ChangeTelephoneRequest) (*pb.ChangeTelephoneResponse, error) {
	return nil, apperr.NewWithMessage(consts.CodeInternalError, "绑定/换绑手机功能暂未实现")
}

// GetQRCode 获取用户二维码
// 业务流程：
//  1. 从context中获取用户UUID
//  2. 使用雪花算法生成唯一的二维码 token
//  3. 在 Redis 中保存 token -> userUUID 和 userUUID -> token 的映射关系（48小时过期）
//  4. 构造二维码 URL，格式为: https://LCchat.top/api/v1/auth/user/parse-qrcode/{token}
//  5. 计算过期时间（当前时间 + 48小时）
//  6. 返回二维码 URL 和过期时间
//
// 错误码映射：
//   - codes.Unauthenticated: 未认证
//   - codes.Internal: 系统内部错误
func (s *userServiceImpl) GetQRCode(ctx context.Context, req *pb.GetQRCodeRequest) (*pb.GetQRCodeResponse, error) {
	// 1. 从context中获取用户UUID
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 2. 如果已有二维码 token，则直接返回
	token, expireTime, err := s.userRepo.GetQRCodeTokenByUserUUID(ctx, userUUID)
	if err == nil {
		return &pb.GetQRCodeResponse{
			Qrcode:   fmt.Sprintf("https://www.LCchat.top/q/%s", token),
			ExpireAt: expireTime.Format(time.RFC3339),
		}, nil
	} else if errors.Is(err, repository.ErrRedisNil) {
	} else {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取用户二维码 token失败")
	}

	// 3. 使用雪花算法生成唯一的二维码 token
	token = util.GenIDString()

	// 3. 在 Redis 中保存 token -> userUUID 和 userUUID -> token 的映射关系
	err = s.userRepo.SaveQRCode(ctx, userUUID, token)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "保存二维码到Redis失败")
	}

	// 4. 构造二维码 URL
	qrcodeURL := fmt.Sprintf("https://www.LCchat.top/q/%s", token)

	// 5. 计算过期时间（当前时间 + 48小时）
	expireAt := time.Now().Add(48 * time.Hour).Format(time.RFC3339)

	// 6. 返回二维码 URL 和过期时间
	return &pb.GetQRCodeResponse{
		Qrcode:   qrcodeURL,
		ExpireAt: expireAt,
	}, nil
}

// DeleteAccount 注销账号
// 业务流程：
//  1. 从context中获取用户UUID
//  2. 查询用户信息
//  3. 验证密码是否正确
//  4. 软删除用户（设置 deleted_at 时间戳）
//  5. 删除用户的所有设备会话（登出所有设备）
//  6. 返回注销时间和恢复截止时间
//
// 错误码映射：
//   - codes.NotFound: 用户不存在
//   - codes.Unauthenticated: 密码错误
//   - codes.Internal: 系统内部错误
func (s *userServiceImpl) DeleteAccount(ctx context.Context, req *pb.DeleteAccountRequest) (*pb.DeleteAccountResponse, error) {
	// 1. 从context中获取用户UUID
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 2. 查询用户信息
	userInfo, err := s.userRepo.GetByUUID(ctx, userUUID)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询用户信息失败")
	}

	if userInfo == nil {
		return nil, apperr.New(consts.CodeUserNotFound)
	}

	// 3. 校验密码是否正确
	err = bcrypt.CompareHashAndPassword([]byte(userInfo.Password), []byte(req.Password))
	if err != nil {
		return nil, apperr.New(consts.CodePasswordError)
	}

	// 4. 软删除用户（设置 deleted_at 时间戳）
	err = s.userRepo.Delete(ctx, userUUID)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "注销账号失败")
	}

	// 5. 异步清理用户所有设备的 Redis 会话（不阻塞返回）
	async.RunSafe(ctx, func(asyncCtx context.Context) {
		if err := s.deviceRepo.DeleteByUserUUID(asyncCtx, userUUID); err != nil {
			logger.Warn(asyncCtx, "清理用户 Redis 会话失败",
				logger.String("user_uuid", userUUID),
				logger.ErrorField("error", err),
			)
		}
	}, 5*time.Second)

	// 6. 计算恢复截止时间（30天后）
	deleteAt := time.Now()
	recoverDeadline := deleteAt.Add(30 * 24 * time.Hour)

	// 7. 返回注销时间和恢复截止时间
	return &pb.DeleteAccountResponse{
		DeleteAt:        deleteAt.Format(time.RFC3339),
		RecoverDeadline: recoverDeadline.Format(time.RFC3339),
	}, nil
}

// BatchGetProfile 批量获取用户信息
// BatchGetProfile 批量获取用户信息
// 业务流程：
//  1. 验证请求参数（UUID列表不为空，最多100个）
//  2. 批量查询用户信息
//  3. 转换为SimpleUserInfo格式并返回
//
// 错误码映射：
//   - codes.InvalidArgument: 参数错误
//   - codes.Internal: 系统内部错误
func (s *userServiceImpl) BatchGetProfile(ctx context.Context, req *pb.BatchGetProfileRequest) (*pb.BatchGetProfileResponse, error) {
	// 1. 验证请求参数
	if len(req.UserUuids) == 0 {
		return &pb.BatchGetProfileResponse{
			Users: []*pb.SimpleUserInfo{},
		}, nil
	}

	if len(req.UserUuids) > 100 {
		return nil, apperr.New(consts.CodeParamError)
	}

	// 2. 批量查询用户信息
	users, err := s.userRepo.BatchGetByUUIDs(ctx, req.UserUuids)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "批量查询用户信息失败")
	}

	// 3. 转换为SimpleUserInfo格式
	simpleUsers := make([]*pb.SimpleUserInfo, 0, len(users))
	for _, user := range users {
		simpleUsers = append(simpleUsers, &pb.SimpleUserInfo{
			Uuid:      user.Uuid,
			Nickname:  user.Nickname,
			Avatar:    user.Avatar,
			Gender:    int32(user.Gender),
			Signature: user.Signature,
		})
	}

	return &pb.BatchGetProfileResponse{
		Users: simpleUsers,
	}, nil
}

// ParseQRCode 解析二维码
// 业务流程：
//  1. 验证 token 是否为空
//  2. 从 Redis 中根据 token 获取用户 UUID
//  3. 验证用户是否存在
//  4. 返回用户 UUID
//
// 错误码映射：
//   - codes.InvalidArgument: 二维码格式错误（token 为空）
//   - codes.NotFound: 二维码已过期或用户不存在
//   - codes.Internal: 系统内部错误
func (s *userServiceImpl) ParseQRCode(ctx context.Context, req *pb.ParseQRCodeRequest) (*pb.ParseQRCodeResponse, error) {
	// 1. 验证 token 是否为空
	if req.Token == "" {
		return nil, apperr.New(consts.CodeQRCodeFormatError)
	}

	// 2. 从 Redis 中根据 token 获取用户 UUID
	userUUID, err := s.userRepo.GetUUIDByQRCodeToken(ctx, req.Token)
	if err != nil {
		if errors.Is(err, repository.ErrRedisNil) {
			// Redis 中不存在该 token，说明二维码已过期或无效
			return nil, apperr.New(consts.CodeQRCodeExpired)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "从 Redis 获取二维码 token 失败")
	}

	// 4. 返回用户 UUID
	return &pb.ParseQRCodeResponse{
		UserUuid: userUUID,
	}, nil
}
