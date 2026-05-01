package handler

import (
	"context"

	"github.com/013677890/LCchat-Backend/apps/user/internal/service"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
)

// InternalProfileHandler 负责承接内部资料服务请求。
type InternalProfileHandler struct {
	pb.UnimplementedInternalProfileServiceServer

	internalProfileService service.IInternalProfileService
}

// NewInternalProfileHandler 创建内部资料 Handler 实例。
func NewInternalProfileHandler(internalProfileService service.IInternalProfileService) *InternalProfileHandler {
	return &InternalProfileHandler{internalProfileService: internalProfileService}
}

// CreateProfile 创建或确认默认资料存在。
func (h *InternalProfileHandler) CreateProfile(ctx context.Context, req *pb.CreateProfileRequest) (*pb.CreateProfileResponse, error) {
	return h.internalProfileService.CreateProfile(ctx, req)
}

// BatchGetUserCard 批量获取用户卡片信息。
func (h *InternalProfileHandler) BatchGetUserCard(ctx context.Context, req *pb.BatchGetUserCardRequest) (*pb.BatchGetUserCardResponse, error) {
	return h.internalProfileService.BatchGetUserCard(ctx, req)
}

// BatchGetPublicProfile 批量获取公开资料信息。
func (h *InternalProfileHandler) BatchGetPublicProfile(ctx context.Context, req *pb.BatchGetPublicProfileRequest) (*pb.BatchGetPublicProfileResponse, error) {
	return h.internalProfileService.BatchGetPublicProfile(ctx, req)
}
