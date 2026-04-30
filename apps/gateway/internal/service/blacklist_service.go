package service

import (
	"context"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/pb"
	relationpb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/pkg/logger"
)

// BlacklistServiceImpl 黑名单服务实现
type BlacklistServiceImpl struct {
	userClient pb.UserServiceClient
}

// NewBlacklistService 创建黑名单服务实例
// userClient: 用户服务 gRPC 客户端
func NewBlacklistService(userClient pb.UserServiceClient) BlacklistService {
	return &BlacklistServiceImpl{
		userClient: userClient,
	}
}

// AddBlacklist 拉黑用户
func (s *BlacklistServiceImpl) AddBlacklist(ctx context.Context, req *dto.AddBlacklistRequest) (*dto.AddBlacklistResponse, error) {

	grpcReq := dto.ConvertToProtoAddBlacklistRequest(req)
	grpcResp, err := s.userClient.AddBlacklist(ctx, grpcReq)
	if err != nil {
		return nil, err
	}

	return dto.ConvertAddBlacklistResponseFromProto(grpcResp), nil
}

// RemoveBlacklist 取消拉黑
func (s *BlacklistServiceImpl) RemoveBlacklist(ctx context.Context, req *dto.RemoveBlacklistRequest) (*dto.RemoveBlacklistResponse, error) {

	grpcReq := dto.ConvertToProtoRemoveBlacklistRequest(req)
	grpcResp, err := s.userClient.RemoveBlacklist(ctx, grpcReq)
	if err != nil {
		return nil, err
	}

	return dto.ConvertRemoveBlacklistResponseFromProto(grpcResp), nil
}

// GetBlacklistList 获取黑名单列表
func (s *BlacklistServiceImpl) GetBlacklistList(ctx context.Context, req *dto.GetBlacklistListRequest) (*dto.GetBlacklistListResponse, error) {

	grpcReq := &relationpb.GetBlacklistListRequest{
		Page:     req.Page,
		PageSize: req.PageSize,
	}
	grpcResp, err := s.userClient.GetBlacklistList(ctx, grpcReq)
	if err != nil {
		return nil, err
	}

	resp := dto.ConvertGetBlacklistListResponseFromProto(grpcResp)
	if resp == nil || len(resp.Items) == 0 {
		return resp, nil
	}

	uuids := make([]string, 0, len(resp.Items))
	for _, item := range resp.Items {
		if item != nil && item.UUID != "" {
			uuids = append(uuids, item.UUID)
		}
	}

	userMap, err := s.batchGetSimpleUserInfo(ctx, uuids)
	if err != nil {
		logger.Warn(ctx, "批量获取黑名单用户信息失败，降级返回",
			logger.Int("count", len(uuids)),
			logger.ErrorField("error", err),
		)
		return resp, nil
	}

	for _, item := range resp.Items {
		if item == nil {
			continue
		}
		if info, ok := userMap[item.UUID]; ok && info != nil {
			item.Nickname = info.Nickname
			item.Avatar = info.Avatar
		}
	}

	return resp, nil
}

// batchGetSimpleUserInfo 批量获取用户信息（含去重与分片）
// 失败时返回错误，由调用方决定是否降级
func (s *BlacklistServiceImpl) batchGetSimpleUserInfo(ctx context.Context, uuids []string) (map[string]*dto.SimpleUserInfo, error) {
	const batchSize = 100
	result := make(map[string]*dto.SimpleUserInfo)
	if len(uuids) == 0 {
		return result, nil
	}

	unique := make([]string, 0, len(uuids))
	seen := make(map[string]struct{}, len(uuids))
	for _, uuid := range uuids {
		if uuid == "" {
			continue
		}
		if _, ok := seen[uuid]; ok {
			continue
		}
		seen[uuid] = struct{}{}
		unique = append(unique, uuid)
	}

	for i := 0; i < len(unique); i += batchSize {
		end := i + batchSize
		if end > len(unique) {
			end = len(unique)
		}

		grpcResp, err := s.userClient.BatchGetProfile(ctx, &userpb.BatchGetProfileRequest{
			UserUuids: unique[i:end],
		})
		if err != nil {
			return result, err
		}

		for _, user := range grpcResp.Users {
			if user == nil || user.Uuid == "" {
				continue
			}
			result[user.Uuid] = dto.ConvertSimpleUserInfoFromProto(user)
		}
	}

	return result, nil
}

// CheckIsBlacklist 判断是否拉黑
func (s *BlacklistServiceImpl) CheckIsBlacklist(ctx context.Context, req *dto.CheckIsBlacklistRequest) (*dto.CheckIsBlacklistResponse, error) {

	grpcReq := dto.ConvertToProtoCheckIsBlacklistRequest(req)
	grpcResp, err := s.userClient.CheckIsBlacklist(ctx, grpcReq)
	if err != nil {
		return nil, err
	}

	return dto.ConvertCheckIsBlacklistResponseFromProto(grpcResp), nil
}
