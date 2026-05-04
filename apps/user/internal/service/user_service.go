package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/013677890/LCchat-Backend/apps/user/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/util"
)

// userServiceImpl 资料域服务实现。
type userServiceImpl struct {
	userRepo repository.IUserRepository
}

// NewProfileUserService 创建仅包含资料域职责的用户服务实例。
//
// 该服务在四拆后只保留 user_profile 相关职责：
//  1. 读取和更新个人资料；
//  2. 批量查询与搜索公开资料；
//  3. 管理二维码名片等资料域附属能力。
func NewProfileUserService(userRepo repository.IUserRepository) UserService {
	return &userServiceImpl{userRepo: userRepo}
}

// GetProfile 获取当前登录用户的资料快照。
// 业务流程：
//  1. 从上下文中提取当前登录用户 UUID；
//  2. 查询 user_profile 里的权威资料记录；
//  3. 组装 user proto 后返回给上游。
//
// 错误码映射：
//   - codes.NotFound: 用户不存在
//   - codes.Internal: 系统内部错误
func (s *userServiceImpl) GetProfile(ctx context.Context, req *pb.GetProfileRequest) (*pb.GetProfileResponse, error) {
	// 资料接口必须绑定登录身份；没有 user_uuid 时不允许继续查询自己的资料。
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 直接按当前用户 UUID 读取资料域权威快照。
	userInfo, err := s.userRepo.GetByUUID(ctx, userUUID)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询用户信息失败")
	}

	// 资料域把 nil 视为“用户不存在”，与数据库错误明确区分开。
	if userInfo == nil {
		return nil, apperr.New(consts.CodeUserNotFound)
	}

	// 仓储层返回的是资料域模型；这里统一转换成对外 proto 响应。
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
	// 他人资料是显式按目标 UUID 查询，不依赖当前用户身份参与过滤。
	targetUserInfo, err := s.userRepo.GetByUUID(ctx, req.UserUuid)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "查询用户信息失败")
	}

	// 若仓储层没有返回资料快照，则统一映射为“目标用户不存在”。
	if targetUserInfo == nil {
		return nil, apperr.New(consts.CodeUserNotFound)
	}

	// 资料服务只返回资料域快照；更细粒度的脱敏与聚合由上游边界层负责。
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
	// 搜索入口仍要求登录态，避免匿名批量枚举资料数据。
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 搜索逻辑完全下沉到 repository；service 层只保留鉴权、错误映射与响应重组。
	users, total, err := s.userRepo.SearchUser(ctx, req.Keyword, int(req.Page), int(req.PageSize))
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "搜索用户失败")
	}

	if len(users) == 0 {
		// 搜索结果为空时返回空切片而不是 nil，方便 gateway 直接序列化空数组。
		return &pb.SearchUserResponse{
			Items:      []*pb.SimpleUserItem{},
			Pagination: buildUserPaginationInfoProto(req.Page, req.PageSize, total),
		}, nil
	}

	// 搜索结果只回传公开资料字段；邮箱、好友态等跨域字段由上游继续聚合。
	items := make([]*pb.SimpleUserItem, len(users))
	for i, user := range users {
		// 每个命中结果独立映射；这里固定把 is_friend 置为 false，交给网关二次补齐。
		items[i] = buildSearchUserItemProto(user, false)
	}

	// 返回分页后的公开搜索结果集合。
	return &pb.SearchUserResponse{
		Items:      items,
		Pagination: buildUserPaginationInfoProto(req.Page, req.PageSize, total),
	}, nil
}

// UpdateProfile 更新当前用户的基础资料。
// 业务流程：
//  1. 从上下文中提取当前登录用户 UUID；
//  2. 校验请求中至少包含一个可更新字段；
//  3. 若携带生日，则额外校验格式与日期合法性；
//  4. 调用 repository 在事务中更新资料并写展示字段事件；
//  5. 返回更新后的资料快照。
//
// 错误码映射：
//   - codes.NotFound: 用户不存在
//   - codes.AlreadyExists: 昵称已被使用
//   - codes.InvalidArgument: 参数验证失败
//   - codes.Internal: 系统内部错误
func (s *userServiceImpl) UpdateProfile(ctx context.Context, req *pb.UpdateProfileRequest) (*pb.UpdateProfileResponse, error) {
	// 更新资料必须绑定当前登录用户；匿名请求直接拒绝。
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 四个展示字段都没传时没有更新意义，按参数错误返回。
	if req.Nickname == "" && req.Birthday == "" && req.Signature == "" && req.Gender == 0 {
		return nil, apperr.New(consts.CodeParamError)
	}

	// 生日字段额外做双重校验：先校验格式，再校验是否是日历中的真实日期。
	if req.Birthday != "" {
		// 先要求严格的 YYYY-MM-DD 文本格式，避免仓储层落入脏字符串。
		birthdayPattern := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
		if !birthdayPattern.MatchString(req.Birthday) {
			return nil, apperr.New(consts.CodeBirthdayFormatError)
		}

		// 再用 time.Parse 校验真实日期，拦住诸如 2024-02-31 这类非法值。
		_, err := time.Parse("2006-01-02", req.Birthday)
		if err != nil {
			return nil, apperr.New(consts.CodeBirthdayFormatError)
		}
	}

	// 真正的资料写入和展示字段事件由 repository 在事务内统一完成，service 不重复拼事务。
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

	// 返回事务提交后确认过的最新资料快照，避免上游继续使用旧值。
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
	// 上传头像本质是更新当前用户资料里的 avatar 字段，因此必须先拿登录身份。
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 头像地址为空时没有更新意义，也会污染展示字段，因此直接拦截。
	if req.AvatarUrl == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	// 仓储层会在同一条写链路里同时更新头像字段并写入 profile_display_changed 事件。
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

	// 返回仓储层确认后的最终头像地址，避免上游和数据库状态不一致。
	return &pb.UploadAvatarResponse{
		AvatarUrl: userInfo.Avatar,
	}, nil
}

// GetQRCode 获取当前用户的二维码名片链接。
// 业务流程：
//  1. 从上下文中提取当前登录用户 UUID；
//  2. 先尝试复用 Redis 中仍有效的既有二维码 token；
//  3. 若缓存 miss，则生成新的二维码 token 并落 Redis；
//  4. 组装对外二维码链接和过期时间后返回。
//
// 错误码映射：
//   - codes.Unauthenticated: 未认证
//   - codes.Internal: 系统内部错误
func (s *userServiceImpl) GetQRCode(ctx context.Context, req *pb.GetQRCodeRequest) (*pb.GetQRCodeResponse, error) {
	// 二维码名片属于当前用户私有资源，没有登录态时不允许生成或读取。
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 先尝试复用仍在有效期内的 token，避免二维码链接频繁变化影响分享体验。
	token, expireTime, err := s.userRepo.GetQRCodeTokenByUserUUID(ctx, userUUID)
	if err == nil {
		// 命中已存在 token 时直接复用，避免频繁生成新二维码导致扫码链接不断变化。
		return &pb.GetQRCodeResponse{
			Qrcode:   fmt.Sprintf("https://www.LCchat.top/q/%s", token),
			ExpireAt: expireTime.Format(time.RFC3339),
		}, nil
	} else if errors.Is(err, repository.ErrRedisNil) {
		// token 不存在属于正常 miss，继续走新建二维码逻辑。
	} else {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取用户二维码 token失败")
	}

	// 缓存 miss 时重新生成全局唯一 token，建立新的二维码映射关系。
	token = util.GenIDString()

	// 同时保存 token -> user_uuid 与 user_uuid -> token 两个方向映射，便于后续生成与解析复用。
	err = s.userRepo.SaveQRCode(ctx, userUUID, token)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "保存二维码到Redis失败")
	}

	// 对外统一返回短链接形式，扫码端再走解析接口获取 user_uuid。
	qrcodeURL := fmt.Sprintf("https://www.LCchat.top/q/%s", token)

	// 过期时间与仓储层保存二维码的 TTL 保持一致，避免上游展示时间和实际缓存生命周期错位。
	expireAt := time.Now().Add(48 * time.Hour).Format(time.RFC3339)

	// 返回新的二维码链接和显式过期时间，供客户端展示倒计时或缓存策略。
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
	// 空批量请求直接返回空数组，避免让上游额外判断 nil。
	if len(req.UserUuids) == 0 {
		return &pb.BatchGetProfileResponse{
			Users: []*pb.SimpleUserInfo{},
		}, nil
	}

	// 当前接口约束单次最多查询 100 个 UUID，避免放大跨服务聚合压力。
	if len(req.UserUuids) > 100 {
		return nil, apperr.New(consts.CodeParamError)
	}

	// 仓储层统一处理缓存和数据库回源；service 层只做结果裁剪与 proto 转换。
	users, err := s.userRepo.BatchGetByUUIDs(ctx, req.UserUuids)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "批量查询用户信息失败")
	}

	// 过滤 nil 后再映射，保证返回切片里的每一项都可安全序列化。
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

// ParseQRCode 解析二维码 token 对应的用户身份。
// 业务流程：
//  1. 校验请求中的 token 是否为空；
//  2. 从 Redis 中读取 token -> user_uuid 映射；
//  3. 将缓存 miss 统一映射为二维码过期；
//  4. 返回解析出的 user_uuid。
//
// 错误码映射：
//   - codes.InvalidArgument: 二维码格式错误（token 为空）
//   - codes.NotFound: 二维码已过期或用户不存在
//   - codes.Internal: 系统内部错误
func (s *userServiceImpl) ParseQRCode(ctx context.Context, req *pb.ParseQRCodeRequest) (*pb.ParseQRCodeResponse, error) {
	// token 为空时连最基本的二维码标识都没有，直接按格式错误返回。
	if req.Token == "" {
		return nil, apperr.New(consts.CodeQRCodeFormatError)
	}

	// 二维码解析完全依赖 Redis 中的短期映射；命中后只返回 user_uuid 供上游继续拉资料。
	userUUID, err := s.userRepo.GetUUIDByQRCodeToken(ctx, req.Token)
	if err != nil {
		if errors.Is(err, repository.ErrRedisNil) {
			// Redis 中不存在该 token，说明二维码已过期或无效。
			return nil, apperr.New(consts.CodeQRCodeExpired)
		}
		return nil, apperr.Wrap(err, consts.CodeInternalError, "从 Redis 获取二维码 token 失败")
	}

	// 解析二维码只返回 user_uuid；目标用户资料由上游再按需拉取。
	return &pb.ParseQRCodeResponse{
		UserUuid: userUUID,
	}, nil
}
