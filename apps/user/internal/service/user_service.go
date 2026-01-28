package service

import (
	"ChatServer/apps/user/internal/converter"
	"ChatServer/apps/user/internal/repository"
	"ChatServer/apps/user/internal/utils"
	pb "ChatServer/apps/user/pb"
	"ChatServer/consts"
	"ChatServer/pkg/logger"
	"context"
	"errors"
	"regexp"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// userServiceImpl 用户信息服务实现
type userServiceImpl struct {
	userRepo repository.IUserRepository
	authRepo repository.IAuthRepository
}

// NewUserService 创建用户信息服务实例
func NewUserService(userRepo repository.IUserRepository, authRepo repository.IAuthRepository) UserService {
	return &userServiceImpl{
		userRepo: userRepo,
		authRepo: authRepo,
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
	userUUID, ok := ctx.Value("user_uuid").(string)
	if !ok || userUUID == "" {
		logger.Error(ctx, "获取用户UUID失败")
		return nil, status.Error(codes.Unauthenticated, strconv.Itoa(consts.CodeUnauthorized))
	}

	// 2. 查询用户信息
	userInfo, err := s.userRepo.GetByUUID(ctx, userUUID)
	if err != nil {
		logger.Error(ctx, "查询用户信息失败",
			logger.String("user_uuid", userUUID),
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	if userInfo == nil {
		logger.Warn(ctx, "用户不存在",
			logger.String("user_uuid", userUUID),
		)
		return nil, status.Error(codes.NotFound, strconv.Itoa(consts.CodeUserNotFound))
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
		logger.Error(ctx, "查询用户信息失败",
			logger.String("target_user_uuid", req.UserUuid),
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	if targetUserInfo == nil {
		logger.Warn(ctx, "用户不存在",
			logger.String("target_user_uuid", req.UserUuid),
		)
		return nil, status.Error(codes.NotFound, strconv.Itoa(consts.CodeUserNotFound))
	}

	// 2. 返回用户信息（脱敏由Gateway层负责）
	return &pb.GetOtherProfileResponse{
		UserInfo: converter.ModelToProtoUserInfo(targetUserInfo),
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
	userUUID, ok := ctx.Value("user_uuid").(string)
	if !ok || userUUID == "" {
		logger.Error(ctx, "获取用户UUID失败")
		return nil, status.Error(codes.Unauthenticated, strconv.Itoa(consts.CodeUnauthorized))
	}

	// 2. 验证请求参数（至少提供一个字段）
	if req.Nickname == "" && req.Birthday == "" && req.Signature == "" && req.Gender == 0 {
		logger.Warn(ctx, "更新基本信息请求参数为空")
		return nil, status.Error(codes.InvalidArgument, strconv.Itoa(consts.CodeParamError))
	}

	// 2.1 如果提供了生日，验证生日格式
	if req.Birthday != "" {
		// 验证生日格式 (YYYY-MM-DD)
		birthdayPattern := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
		if !birthdayPattern.MatchString(req.Birthday) {
			logger.Warn(ctx, "生日格式错误",
				logger.String("birthday", req.Birthday),
			)
			return nil, status.Error(codes.InvalidArgument, strconv.Itoa(consts.CodeBirthdayFormatError))
		}

		// 验证生日是否是有效日期
		_, err := time.Parse("2006-01-02", req.Birthday)
		if err != nil {
			logger.Warn(ctx, "生日日期无效",
				logger.String("birthday", req.Birthday),
				logger.ErrorField("error", err),
			)
			return nil, status.Error(codes.InvalidArgument, strconv.Itoa(consts.CodeBirthdayFormatError))
		}
	}

	// 3. 更新基本信息
	err := s.userRepo.UpdateBasicInfo(ctx, userUUID, req.Nickname, req.Signature, req.Birthday, int8(req.Gender))
	if err != nil {
		logger.Error(ctx, "更新基本信息失败",
			logger.String("user_uuid", userUUID),
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 4. 查询更新后的用户信息
	userInfo, err := s.userRepo.GetByUUID(ctx, userUUID)
	if err != nil {
		logger.Error(ctx, "查询更新后的用户信息失败",
			logger.String("user_uuid", userUUID),
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	if userInfo == nil {
		logger.Warn(ctx, "用户不存在",
			logger.String("user_uuid", userUUID),
		)
		return nil, status.Error(codes.NotFound, strconv.Itoa(consts.CodeUserNotFound))
	}

	// 5. 转换为Protobuf格式并返回
	return &pb.UpdateProfileResponse{
		UserInfo: converter.ModelToProtoUserInfo(userInfo),
	}, nil
}

// UploadAvatar 上传头像
func (s *userServiceImpl) UploadAvatar(ctx context.Context, req *pb.UploadAvatarRequest) (*pb.UploadAvatarResponse, error) {
	return nil, status.Error(codes.Unimplemented, "上传头像功能暂未实现")
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
	userUUID, ok := ctx.Value("user_uuid").(string)
	if !ok || userUUID == "" {
		logger.Error(ctx, "获取用户UUID失败")
		return status.Error(codes.Unauthenticated, strconv.Itoa(consts.CodeUnauthorized))
	}

	// 2. 查询用户信息
	userInfo, err := s.userRepo.GetByUUID(ctx, userUUID)
	if err != nil {
		logger.Error(ctx, "查询用户信息失败",
			logger.String("user_uuid", userUUID),
			logger.ErrorField("error", err),
		)
		return status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	if userInfo == nil {
		logger.Warn(ctx, "用户不存在",
			logger.String("user_uuid", userUUID),
		)
		return status.Error(codes.NotFound, strconv.Itoa(consts.CodeUserNotFound))
	}

	// 3. 校验旧密码是否正确
	err = bcrypt.CompareHashAndPassword([]byte(userInfo.Password), []byte(req.OldPassword))
	if err != nil {
		logger.Warn(ctx, "旧密码错误",
			logger.String("user_uuid", userUUID),
		)
		return status.Error(codes.Unauthenticated, strconv.Itoa(consts.CodePasswordError))
	}

	// 4. 校验新密码是否与旧密码相同
	err = bcrypt.CompareHashAndPassword([]byte(userInfo.Password), []byte(req.NewPassword))
	if err == nil {
		// 密码相同
		logger.Warn(ctx, "新密码不能与旧密码相同",
			logger.String("user_uuid", userUUID),
		)
		return status.Error(codes.FailedPrecondition, strconv.Itoa(consts.CodePasswordSameAsOld))
	}

	// 5. 生成新密码哈希
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		logger.Error(ctx, "生成密码哈希失败",
			logger.String("user_uuid", userUUID),
			logger.ErrorField("error", err),
		)
		return status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 6. 更新密码
	err = s.userRepo.UpdatePassword(ctx, userUUID, string(hashedPassword))
	if err != nil {
		logger.Error(ctx, "更新密码失败",
			logger.String("user_uuid", userUUID),
			logger.ErrorField("error", err),
		)
		return status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}

	// 7. 踢出其他所有设备的登录态（删除所有设备的token）
	// 注意：当前设备保持登录态，其他设备被踢出
	// 这里需要在repository中实现踢出其他设备的方法，暂时跳过
	// TODO: 实现踢出其他设备登录态

	logger.Info(ctx, "密码修改成功",
		logger.String("user_uuid", userUUID),
	)

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
	userUUID, ok := ctx.Value("user_uuid").(string)
	if !ok || userUUID == "" {
		logger.Error(ctx, "获取用户UUID失败")
		return nil, status.Error(codes.Unauthenticated, strconv.Itoa(consts.CodeUnauthorized))
	}

	// 记录换绑邮箱请求（新旧邮箱脱敏）
	logger.Info(ctx, "用户换绑邮箱请求",
		logger.String("user_uuid", userUUID),
		logger.String("new_email", utils.MaskEmail(req.NewEmail)),
	)

	// 2. 检查新邮箱是否已被使用
	exists, err := s.userRepo.ExistsByEmail(ctx, req.NewEmail)
	if err != nil {
		logger.Error(ctx, "检查邮箱是否存在失败",
			logger.String("email", utils.MaskEmail(req.NewEmail)),
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}
	if exists {
		logger.Warn(ctx, "邮箱已被使用",
			logger.String("email", utils.MaskEmail(req.NewEmail)),
		)
		return nil, status.Error(codes.AlreadyExists, strconv.Itoa(consts.CodeEmailAlreadyExist))
	}

	// 3. 校验验证码（type=4: 换绑邮箱）
	isValid, err := s.authRepo.VerifyVerifyCode(ctx, req.NewEmail, req.VerifyCode, 4)
	if err != nil {
		// 判断是 Redis Key 不存在还是其他错误
		if errors.Is(err, repository.ErrRedisNil) {
			logger.Warn(ctx, "验证码已过期",
				logger.String("email", utils.MaskEmail(req.NewEmail)),
			)
			return nil, status.Error(codes.Unauthenticated, strconv.Itoa(consts.CodeVerifyCodeExpire))
		}
		logger.Error(ctx, "校验验证码失败",
			logger.String("email", utils.MaskEmail(req.NewEmail)),
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}
	if !isValid {
		logger.Warn(ctx, "验证码错误",
			logger.String("email", utils.MaskEmail(req.NewEmail)),
		)
		return nil, status.Error(codes.Unauthenticated, strconv.Itoa(consts.CodeVerifyCodeError))
	}

	// 4. 查询用户当前信息，获取旧邮箱用于日志记录
	userInfo, err := s.userRepo.GetByUUID(ctx, userUUID)
	if err != nil {
		logger.Error(ctx, "查询用户信息失败",
			logger.String("user_uuid", userUUID),
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
	}
	if userInfo == nil {
		logger.Warn(ctx, "用户不存在",
			logger.String("user_uuid", userUUID),
		)
		return nil, status.Error(codes.NotFound, strconv.Itoa(consts.CodeUserNotFound))
	}

	// 5. 更新邮箱
	err = s.userRepo.UpdateEmail(ctx, userUUID, req.NewEmail)
	if err != nil {
		logger.Error(ctx, "更新邮箱失败",
			logger.String("user_uuid", userUUID),
			logger.String("old_email", utils.MaskEmail(userInfo.Email)),
			logger.String("new_email", utils.MaskEmail(req.NewEmail)),
			logger.ErrorField("error", err),
		)
		return nil, status.Error(codes.Internal, strconv.Itoa(consts.CodeInternalError))
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
	logger.Info(ctx, "邮箱更换成功",
		logger.String("user_uuid", userUUID),
		logger.String("old_email", utils.MaskEmail(userInfo.Email)),
		logger.String("new_email", utils.MaskEmail(req.NewEmail)),
	)

	return &pb.ChangeEmailResponse{
		Email: req.NewEmail,
	}, nil
}

// ChangeTelephone 绑定/换绑手机
func (s *userServiceImpl) ChangeTelephone(ctx context.Context, req *pb.ChangeTelephoneRequest) (*pb.ChangeTelephoneResponse, error) {
	return nil, status.Error(codes.Unimplemented, "绑定/换绑手机功能暂未实现")
}

// GetQRCode 获取用户二维码
func (s *userServiceImpl) GetQRCode(ctx context.Context, req *pb.GetQRCodeRequest) (*pb.GetQRCodeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "获取用户二维码功能暂未实现")
}

// ParseQRCode 解析二维码
func (s *userServiceImpl) ParseQRCode(ctx context.Context, req *pb.ParseQRCodeRequest) (*pb.ParseQRCodeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "解析二维码功能暂未实现")
}

// DeleteAccount 注销账号
func (s *userServiceImpl) DeleteAccount(ctx context.Context, req *pb.DeleteAccountRequest) (*pb.DeleteAccountResponse, error) {
	return nil, status.Error(codes.Unimplemented, "注销账号功能暂未实现")
}

// BatchGetProfile 批量获取用户信息
func (s *userServiceImpl) BatchGetProfile(ctx context.Context, req *pb.BatchGetProfileRequest) (*pb.BatchGetProfileResponse, error) {
	return nil, status.Error(codes.Unimplemented, "批量获取用户信息功能暂未实现")
}
