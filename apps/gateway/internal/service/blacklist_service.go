package service

import (
	"context"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/pb"
	relationpb "github.com/013677890/LCchat-Backend/apps/relation/pb"
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

	userMap, err := batchGetSimpleUserInfo(ctx, s.userClient, uuids)
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

// CheckIsBlacklist 判断是否拉黑
func (s *BlacklistServiceImpl) CheckIsBlacklist(ctx context.Context, req *dto.CheckIsBlacklistRequest) (*dto.CheckIsBlacklistResponse, error) {

	grpcReq := dto.ConvertToProtoCheckIsBlacklistRequest(req)
	grpcResp, err := s.userClient.CheckIsBlacklist(ctx, grpcReq)
	if err != nil {
		return nil, err
	}

	return dto.ConvertCheckIsBlacklistResponseFromProto(grpcResp), nil
}
