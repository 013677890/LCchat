package service

import (
	"context"
	"strings"

	"time"

	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/pb"
	relationpb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/async"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/util"
)

// UserServiceImpl 用户服务实现。
type UserServiceImpl struct {
	userClient pb.UserServiceClient
}

// NewUserService 创建用户服务实例
// userClient: 用户服务 gRPC 客户端
func NewUserService(userClient pb.UserServiceClient) UserService {
	return &UserServiceImpl{userClient: userClient}
}

// GetProfile 获取个人信息
// ctx: 请求上下文
// 返回: 个人信息响应
func (s *UserServiceImpl) GetProfile(ctx context.Context) (*dto.GetProfileResponse, error) {
	// 1. 调用用户服务获取个人信息(gRPC)
	grpcResp, err := s.userClient.GetProfile(ctx, &userpb.GetProfileRequest{})
	if err != nil {
		return nil, err
	}
	// 2. 检查用户信息是否为空
	if grpcResp.UserInfo == nil {
		return nil, apperr.New(consts.CodeInternalError)
	}
	return dto.ConvertGetProfileResponseFromProto(grpcResp), nil
}

// GetOtherProfile 获取他人信息
// ctx: 请求上下文
// req: 获取他人信息请求
// 返回: 他人信息响应
func (s *UserServiceImpl) GetOtherProfile(ctx context.Context, req *dto.GetOtherProfileRequest) (*dto.GetOtherProfileResponse, error) {
	// 1. 调用用户服务获取个人信息(gRPC)
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	// 2. 调用用户服务获取他人信息(gRPC)

	grpcReq := dto.ConvertToProtoGetOtherProfileRequest(req)

	// 3. 请求内 fan-out 仍走协程池，但必须继承父请求取消和超时语义。
	group := async.NewGroup(ctx, 5*time.Second)

	var userResp *userpb.GetOtherProfileResponse
	var friendResp *relationpb.CheckIsFriendResponse
	var friendErr error

	if err := group.Go(func(asyncCtx context.Context) error {
		resp, err := s.userClient.GetOtherProfile(asyncCtx, grpcReq)
		if err != nil {
			return err
		}
		userResp = resp
		return nil
	}); err != nil {
		return nil, err
	}

	if err := group.Go(func(asyncCtx context.Context) error {
		friendReq := &relationpb.CheckIsFriendRequest{
			UserUuid: currentUserUUID,
			PeerUuid: req.UserUUID,
		}
		resp, err := s.userClient.CheckIsFriend(asyncCtx, friendReq)
		if err != nil {
			friendErr = err
			return nil
		}
		friendResp = resp
		return nil
	}); err != nil {
		if waitErr := group.Wait(); waitErr != nil {
			return nil, waitErr
		}
		return nil, err
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}

	// 4. 检查用户服务返回的用户信息是否为空
	if userResp == nil || userResp.UserInfo == nil {
		return nil, apperr.New(consts.CodeInternalError)
	}

	// 5. 检查好友服务调用结果
	isFriend := false
	if friendErr != nil {
		logger.Warn(ctx, "查询好友关系失败，按非好友降级",
			logger.String("target_user_uuid", req.UserUUID),
			logger.ErrorField("error", friendErr),
		)
	} else if friendResp != nil {
		isFriend = friendResp.IsFriend
	}

	// 6. 资料域当前只返回公开资料字段，非好友场景无需再裁剪账号域信息。

	// 7. 返回响应
	return dto.ConvertGetOtherProfileResponseFromProto(userResp, isFriend), nil
}

// SearchUser 搜索用户
// ctx: 请求上下文
// req: 搜索用户请求
// 返回: 搜索用户响应
func (s *UserServiceImpl) SearchUser(ctx context.Context, req *dto.SearchUserRequest) (*dto.SearchUserResponse, error) {
	keyword := strings.TrimSpace(req.Keyword)
	if util.ValidateEmail(keyword) {
		return s.searchUserByEmail(ctx, req, keyword)
	}

	// 1. 非邮箱关键词继续走 user-service 搜索昵称和 UUID。
	grpcResp, err := s.userClient.SearchUser(ctx, dto.ConvertToProtoSearchUserRequest(req))
	if err != nil {
		return nil, err
	}

	// 2. 转换响应。
	resp := dto.ConvertSearchUserResponseFromProto(grpcResp)
	if resp == nil || len(resp.Items) == 0 {
		return resp, nil
	}
	return s.fillSearchUserFriendStatus(ctx, resp), nil

}

// searchUserByEmail 按邮箱走 auth-service 查账号，再到 user-service 拉资料。
func (s *UserServiceImpl) searchUserByEmail(ctx context.Context, req *dto.SearchUserRequest, email string) (*dto.SearchUserResponse, error) {
	accountResp, err := s.userClient.FindAccountByEmail(ctx, &authpb.FindAccountByEmailRequest{Email: email})
	if err != nil {
		return nil, err
	}
	if accountResp == nil || !accountResp.Found || accountResp.UserUuid == "" {
		return buildEmptySearchUserResponse(req), nil
	}

	profileResp, err := s.userClient.BatchGetProfile(ctx, &userpb.BatchGetProfileRequest{UserUuids: []string{accountResp.UserUuid}})
	if err != nil {
		return nil, err
	}
	if profileResp == nil || len(profileResp.Users) == 0 || profileResp.Users[0] == nil {
		return buildEmptySearchUserResponse(req), nil
	}

	items := make([]*dto.SimpleUserItem, 0, 1)
	if req.Page <= 1 && req.PageSize > 0 {
		profile := profileResp.Users[0]
		items = append(items, &dto.SimpleUserItem{
			UUID:      profile.Uuid,
			Nickname:  profile.Nickname,
			Avatar:    profile.Avatar,
			Signature: profile.Signature,
		})
	}

	resp := &dto.SearchUserResponse{
		Items: items,
		Pagination: &dto.PaginationInfo{
			Page:       req.Page,
			PageSize:   req.PageSize,
			Total:      1,
			TotalPages: 1,
		},
	}

	return s.fillSearchUserFriendStatus(ctx, resp), nil
}

// buildEmptySearchUserResponse 构造空的搜索响应，保持分页结构稳定。
func buildEmptySearchUserResponse(req *dto.SearchUserRequest) *dto.SearchUserResponse {
	return &dto.SearchUserResponse{
		Items: []*dto.SimpleUserItem{},
		Pagination: &dto.PaginationInfo{
			Page:       req.Page,
			PageSize:   req.PageSize,
			Total:      0,
			TotalPages: 0,
		},
	}
}

// fillSearchUserFriendStatus 尝试批量补充好友关系，失败时按非好友降级。
func (s *UserServiceImpl) fillSearchUserFriendStatus(ctx context.Context, resp *dto.SearchUserResponse) *dto.SearchUserResponse {
	if resp == nil || len(resp.Items) == 0 {
		return resp
	}

	// 3. 尝试补充好友关系（批量判断，失败则降级不填）
	currentUserUUID := util.GetUserUUIDFromContext(ctx)
	if currentUserUUID != "" {
		peerUUIDs := make([]string, 0, len(resp.Items))
		seen := make(map[string]struct{}, len(resp.Items))
		for _, item := range resp.Items {
			if item == nil || item.UUID == "" {
				continue
			}
			if _, exists := seen[item.UUID]; exists {
				continue
			}
			seen[item.UUID] = struct{}{}
			peerUUIDs = append(peerUUIDs, item.UUID)
		}

		if len(peerUUIDs) > 0 {
			batchResp, err := s.userClient.BatchCheckIsFriend(ctx, &relationpb.BatchCheckIsFriendRequest{UserUuid: currentUserUUID, PeerUuids: peerUUIDs})
			if err != nil {
				logger.Warn(ctx, "批量判断好友关系失败，按普通搜索结果返回",
					logger.Int("count", len(peerUUIDs)),
					logger.ErrorField("error", err),
				)
			} else if batchResp != nil {
				result := make(map[string]bool, len(batchResp.Items))
				for _, item := range batchResp.Items {
					if item == nil || item.PeerUuid == "" {
						continue
					}
					result[item.PeerUuid] = item.IsFriend
				}

				for _, item := range resp.Items {
					if item == nil || item.UUID == "" {
						continue
					}
					item.IsFriend = result[item.UUID]
				}
			}
		}
	}

	return resp
}

// UpdateProfile 更新基本信息
// ctx: 请求上下文
// req: 更新基本信息请求
// 返回: 更新后的个人信息响应
func (s *UserServiceImpl) UpdateProfile(ctx context.Context, req *dto.UpdateProfileRequest) (*dto.UpdateProfileResponse, error) {

	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoUpdateProfileRequest(req)

	// 2. 调用用户服务更新基本信息(gRPC)
	grpcResp, err := s.userClient.UpdateProfile(ctx, grpcReq)
	if err != nil {
		return nil, err
	}
	// 3. 检查用户服务返回的用户信息是否为空
	if grpcResp.UserInfo == nil {
		// 如果用户信息为空，则返回内部错误
		return nil, apperr.New(consts.CodeInternalError)
	}
	return dto.ConvertUpdateProfileResponseFromProto(grpcResp), nil
}

// ChangePassword 修改密码
// ctx: 请求上下文
// req: 修改密码请求
// 返回: 修改密码响应
func (s *UserServiceImpl) ChangePassword(ctx context.Context, req *dto.ChangePasswordRequest) error {
	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoChangePasswordRequest(req)
	// 2. 调用用户服务修改密码(gRPC)
	_, err := s.userClient.ChangePassword(ctx, grpcReq)
	return err
}

// ChangeEmail 换绑邮箱
// ctx: 请求上下文
// req: 换绑邮箱请求
// 返回: 换绑邮箱响应
func (s *UserServiceImpl) ChangeEmail(ctx context.Context, req *dto.ChangeEmailRequest) (*dto.ChangeEmailResponse, error) {
	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoChangeEmailRequest(req)
	// 2. 调用用户服务换绑邮箱(gRPC)
	grpcResp, err := s.userClient.ChangeEmail(ctx, grpcReq)
	if err != nil {
		return nil, err
	}
	return dto.ConvertChangeEmailResponseFromProto(grpcResp), nil
}

// UploadAvatar 上传头像
// ctx: 请求上下文
// avatarURL: 头像URL
// 返回: 上传头像响应
func (s *UserServiceImpl) UploadAvatar(ctx context.Context, avatarURL string) (string, error) {
	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := &userpb.UploadAvatarRequest{AvatarUrl: avatarURL}
	// 2. 调用用户服务上传头像(gRPC)
	grpcResp, err := s.userClient.UploadAvatar(ctx, grpcReq)
	if err != nil {
		return "", err
	}
	return grpcResp.AvatarUrl, nil
}

// BatchGetProfile 批量获取用户信息
// ctx: 请求上下文
// req: 批量获取用户信息请求
// 返回: 批量获取用户信息响应
func (s *UserServiceImpl) BatchGetProfile(ctx context.Context, req *dto.BatchGetProfileRequest) (*dto.BatchGetProfileResponse, error) {
	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoBatchGetProfileRequest(req)
	// 2. 调用用户服务批量获取用户信息(gRPC)
	grpcResp, err := s.userClient.BatchGetProfile(ctx, grpcReq)
	if err != nil {
		return nil, err
	}
	return dto.ConvertBatchGetProfileResponseFromProto(grpcResp), nil
}

// GetQRCode 获取用户二维码
// ctx: 请求上下文
// 返回: 获取用户二维码响应
func (s *UserServiceImpl) GetQRCode(ctx context.Context) (*dto.GetQRCodeResponse, error) {
	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := &userpb.GetQRCodeRequest{}
	// 2. 调用用户服务获取用户二维码(gRPC)
	grpcResp, err := s.userClient.GetQRCode(ctx, grpcReq)
	if err != nil {
		return nil, err
	}
	return dto.ConvertGetQRCodeResponseFromProto(grpcResp), nil
}

// ParseQRCode 解析二维码
// ctx: 请求上下文
// req: 解析二维码请求
// 返回: 解析二维码响应
func (s *UserServiceImpl) ParseQRCode(ctx context.Context, req *dto.ParseQRCodeRequest) (*dto.ParseQRCodeResponse, error) {
	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoParseQRCodeRequest(req)
	// 2. 调用用户服务解析二维码(gRPC)
	grpcResp, err := s.userClient.ParseQRCode(ctx, grpcReq)
	if err != nil {
		return nil, err
	}
	return dto.ConvertParseQRCodeResponseFromProto(grpcResp), nil
}

// DeleteAccount 注销账号
// ctx: 请求上下文
// req: 注销账号请求
// 返回: 注销账号响应
func (s *UserServiceImpl) DeleteAccount(ctx context.Context, req *dto.DeleteAccountRequest) (*dto.DeleteAccountResponse, error) {
	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoDeleteAccountRequest(req)
	// 2. 调用用户服务注销账号(gRPC)
	grpcResp, err := s.userClient.DeleteAccount(ctx, grpcReq)
	if err != nil {
		return nil, err
	}
	return dto.ConvertDeleteAccountResponseFromProto(grpcResp), nil
}
