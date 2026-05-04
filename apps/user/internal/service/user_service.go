package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/013677890/LCchat-Backend/apps/user/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/util"
	"regexp"
	"time"
)

// userServiceImpl 资料域服务实现。
type userServiceImpl struct {
	userRepo repository.IUserRepository
}

// NewProfileUserService 创建仅包含资料域职责的用户服务实例。
func NewProfileUserService(userRepo repository.IUserRepository) UserService {
	return &userServiceImpl{userRepo: userRepo}
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
		UserInfo: buildUserInfoProto(userInfo),
	}, nil
}

// GetOtherProfile 获取他人资料。
// 业务流程：
//  1. 查询目标用户资料
//  2. 若不存在则返回用户不存在
//  3. 返回公开资料视图（隐私字段不在 user_profile 中维护）
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
		UserInfo: buildUserInfoProto(targetUserInfo),
	}, nil
}

// SearchUser 搜索用户。
// 业务流程：
//  1. 从 context 中获取当前用户 UUID 用于鉴权
//  2. 按昵称前缀或 user_uuid 前缀搜索资料
//  3. 组装公开搜索结果返回给上游网关
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
			Pagination: buildUserPaginationInfoProto(req.Page, req.PageSize, total),
		}, nil
	}

	// 3. 构建响应（不返回 email，isFriend 由网关聚合）
	items := make([]*pb.SimpleUserItem, len(users))
	for i, user := range users {
		items[i] = buildSearchUserItemProto(user, false)
	}

	// 5. 返回搜索结果
	return &pb.SearchUserResponse{
		Items: items,
		Pagination: buildUserPaginationInfoProto(req.Page, req.PageSize, total),
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
		UserInfo: buildUserInfoProto(userInfo),
	}, nil
}

// UploadAvatar 上传头像。
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

// BatchGetProfile 批量获取用户资料。
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
		if simpleUser := buildSimpleUserInfoProto(user); simpleUser != nil {
			simpleUsers = append(simpleUsers, simpleUser)
		}
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
