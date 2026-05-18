package service

import (
	"context"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/pb"
	relationpb "github.com/013677890/LCchat-Backend/apps/relation/pb"
	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/pkg/logger"
)

// FriendServiceImpl 好友服务实现
type FriendServiceImpl struct {
	userClient pb.UserServiceClient
}

// NewFriendService 创建好友服务实例
// userClient: 用户服务 gRPC 客户端
func NewFriendService(userClient pb.UserServiceClient) FriendService {
	return &FriendServiceImpl{
		userClient: userClient,
	}
}

// SendFriendApply 发送好友申请
func (s *FriendServiceImpl) SendFriendApply(ctx context.Context, req *dto.SendFriendApplyRequest) (*dto.SendFriendApplyResponse, error) {
	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoSendFriendApplyRequest(req)

	// 2. 调用用户服务发送好友申请(gRPC)
	grpcResp, err := s.userClient.SendFriendApply(ctx, grpcReq)
	if err != nil {
		// gRPC 调用失败，提取业务错误码
		// 返回业务错误（作为 Go error 返回，由 Handler 层处理）
		return nil, err
	}

	// 3. gRPC 调用成功，返回结果
	return dto.ConvertFriendApplyResponseFromProto(grpcResp), nil
}

// GetFriendApplyList 获取好友申请列表
func (s *FriendServiceImpl) GetFriendApplyList(ctx context.Context, req *dto.GetFriendApplyListRequest) (*dto.GetFriendApplyListResponse, error) {
	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := &relationpb.GetFriendApplyListRequest{
		Status:   req.Status,
		Page:     req.Page,
		PageSize: req.PageSize,
	}

	// 2. 调用用户服务获取好友申请列表(gRPC)
	grpcResp, err := s.userClient.GetFriendApplyList(ctx, grpcReq)
	if err != nil {
		// gRPC 调用失败，提取业务错误码
		// 返回业务错误（作为 Go error 返回，由 Handler 层处理）
		return nil, err
	}

	// 3. gRPC 调用成功，构建响应
	resp := dto.ConvertGetFriendApplyListResponseFromProto(grpcResp)
	if resp == nil || len(resp.Items) == 0 {
		return resp, nil
	}

	// 4. 批量补全申请人信息（失败则降级返回已有数据）
	applicantUUIDs := make([]string, 0, len(resp.Items))
	for _, item := range resp.Items {
		if item != nil && item.ApplicantUUID != "" {
			applicantUUIDs = append(applicantUUIDs, item.ApplicantUUID)
		}
	}

	userMap, err := s.batchGetSimpleUserInfo(ctx, applicantUUIDs)
	if err != nil {
		logger.Warn(ctx, "批量获取申请人信息失败，降级返回",
			logger.Int("count", len(applicantUUIDs)),
			logger.ErrorField("error", err),
		)
		return resp, nil
	}

	for _, item := range resp.Items {
		if item == nil {
			continue
		}
		if info, ok := userMap[item.ApplicantUUID]; ok && info != nil {
			item.ApplicantNickname = info.Nickname
			item.ApplicantAvatar = info.Avatar
		}
	}

	return resp, nil
}

// GetSentApplyList 获取发出的申请列表
func (s *FriendServiceImpl) GetSentApplyList(ctx context.Context, req *dto.GetSentApplyListRequest) (*dto.GetSentApplyListResponse, error) {
	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := &relationpb.GetSentApplyListRequest{
		Status:   req.Status,
		Page:     req.Page,
		PageSize: req.PageSize,
	}

	// 2. 调用用户服务获取发出的申请列表(gRPC)
	grpcResp, err := s.userClient.GetSentApplyList(ctx, grpcReq)
	if err != nil {
		// gRPC 调用失败，提取业务错误码
		// 返回业务错误（作为 Go error 返回，由 Handler 层处理）
		return nil, err
	}

	// 3. gRPC 调用成功，构建响应
	resp := dto.ConvertGetSentApplyListResponseFromProto(grpcResp)
	if resp == nil || len(resp.Items) == 0 {
		return resp, nil
	}

	// 4. 批量补全目标用户信息（失败则降级返回已有数据）
	targetUUIDs := make([]string, 0, len(resp.Items))
	for _, item := range resp.Items {
		if item != nil && item.TargetUUID != "" {
			targetUUIDs = append(targetUUIDs, item.TargetUUID)
		}
	}

	userMap, err := s.batchGetSimpleUserInfo(ctx, targetUUIDs)
	if err != nil {
		logger.Warn(ctx, "批量获取目标用户信息失败，降级返回",
			logger.Int("count", len(targetUUIDs)),
			logger.ErrorField("error", err),
		)
		return resp, nil
	}

	for _, item := range resp.Items {
		if item == nil {
			continue
		}
		if item.TargetInfo == nil {
			item.TargetInfo = &dto.SimpleUserInfo{UUID: item.TargetUUID}
		}
		if info, ok := userMap[item.TargetUUID]; ok && info != nil {
			item.TargetInfo.Nickname = info.Nickname
			item.TargetInfo.Avatar = info.Avatar
			item.TargetInfo.Gender = info.Gender
			item.TargetInfo.Signature = info.Signature
		}
	}

	return resp, nil
}

// HandleFriendApply 处理好友申请
func (s *FriendServiceImpl) HandleFriendApply(ctx context.Context, req *dto.HandleFriendApplyRequest) (*dto.HandleFriendApplyResponse, error) {
	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoHandleFriendApplyRequest(req)

	// 2. 调用用户服务处理好友申请(gRPC)
	_, err := s.userClient.HandleFriendApply(ctx, grpcReq)
	if err != nil {
		// gRPC 调用失败，提取业务错误码
		// 返回业务错误（作为 Go error 返回，由 Handler 层处理）
		return nil, err
	}

	// 3. gRPC 调用成功，返回结果
	return dto.ConvertHandleFriendApplyResponseFromProto(nil), nil
}

// GetUnreadApplyCount 获取未读申请数量
func (s *FriendServiceImpl) GetUnreadApplyCount(ctx context.Context, req *dto.GetUnreadApplyCountRequest) (*dto.GetUnreadApplyCountResponse, error) {
	// 1. 调用用户服务获取未读申请数量(gRPC)
	grpcResp, err := s.userClient.GetUnreadApplyCount(ctx, &relationpb.GetUnreadApplyCountRequest{})
	if err != nil {
		// gRPC 调用失败，提取业务错误码
		// 返回业务错误（作为 Go error 返回，由 Handler 层处理）
		return nil, err
	}

	// 2. gRPC 调用成功，返回结果
	return dto.ConvertGetUnreadApplyCountResponseFromProto(grpcResp), nil
}

// MarkApplyAsRead 标记申请已读
func (s *FriendServiceImpl) MarkApplyAsRead(ctx context.Context, req *dto.MarkApplyAsReadRequest) (*dto.MarkApplyAsReadResponse, error) {
	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoMarkApplyAsReadRequest(req)

	// 2. 调用用户服务标记申请已读(gRPC)
	_, err := s.userClient.MarkApplyAsRead(ctx, grpcReq)
	if err != nil {
		// gRPC 调用失败，提取业务错误码
		// 返回业务错误（作为 Go error 返回，由 Handler 层处理）
		return nil, err
	}

	// 3. gRPC 调用成功，返回结果
	return dto.ConvertMarkApplyAsReadResponseFromProto(nil), nil
}

// GetFriendList 获取好友列表
func (s *FriendServiceImpl) GetFriendList(ctx context.Context, req *dto.GetFriendListRequest) (*dto.GetFriendListResponse, error) {
	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := &relationpb.GetFriendListRequest{
		GroupTag: req.GroupTag,
		Page:     req.Page,
		PageSize: req.PageSize,
	}

	// 2. 调用用户服务获取好友列表(gRPC)
	grpcResp, err := s.userClient.GetFriendList(ctx, grpcReq)
	if err != nil {
		// gRPC 调用失败，提取业务错误码
		// 返回业务错误（作为 Go error 返回，由 Handler 层处理）
		return nil, err
	}

	// 3. gRPC 调用成功，构建响应
	resp := dto.ConvertGetFriendListResponseFromProto(grpcResp)
	if resp == nil || len(resp.Items) == 0 {
		return resp, nil
	}

	// 4. 批量补全好友信息（失败则降级返回已有数据）
	peerUUIDs := make([]string, 0, len(resp.Items))
	for _, item := range resp.Items {
		if item != nil && item.UUID != "" {
			peerUUIDs = append(peerUUIDs, item.UUID)
		}
	}

	userMap, err := s.batchGetSimpleUserInfo(ctx, peerUUIDs)
	if err != nil {
		logger.Warn(ctx, "批量获取好友信息失败，降级返回",
			logger.Int("count", len(peerUUIDs)),
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
			item.Gender = info.Gender
			item.Signature = info.Signature
		}
	}

	return resp, nil
}

// SyncFriendList 好友增量同步
func (s *FriendServiceImpl) SyncFriendList(ctx context.Context, req *dto.SyncFriendListRequest) (*dto.SyncFriendListResponse, error) {
	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := &relationpb.SyncFriendListRequest{
		Version: req.Version,
		Limit:   req.Limit,
	}

	// 2. 调用用户服务同步好友列表(gRPC)
	grpcResp, err := s.userClient.SyncFriendList(ctx, grpcReq)
	if err != nil {
		// gRPC 调用失败，提取业务错误码
		// 返回业务错误（作为 Go error 返回，由 Handler 层处理）
		return nil, err
	}

	// 3. gRPC 调用成功，构建响应
	resp := dto.ConvertSyncFriendListResponseFromProto(grpcResp)
	if resp == nil || len(resp.Changes) == 0 {
		return resp, nil
	}

	// 4. 批量补全好友信息（失败则降级返回已有数据）
	peerUUIDs := make([]string, 0, len(resp.Changes))
	for _, change := range resp.Changes {
		if change == nil || change.UUID == "" || change.ChangeType == "delete" {
			continue
		}
		peerUUIDs = append(peerUUIDs, change.UUID)
	}

	userMap, err := s.batchGetSimpleUserInfo(ctx, peerUUIDs)
	if err != nil {
		logger.Warn(ctx, "批量获取好友信息失败，降级返回",
			logger.Int("count", len(peerUUIDs)),
			logger.ErrorField("error", err),
		)
		return resp, nil
	}

	for _, change := range resp.Changes {
		if change == nil || change.UUID == "" || change.ChangeType == "delete" {
			continue
		}
		if info, ok := userMap[change.UUID]; ok && info != nil {
			change.Nickname = info.Nickname
			change.Avatar = info.Avatar
			change.Gender = info.Gender
			change.Signature = info.Signature
		}
	}

	return resp, nil
}

// DeleteFriend 删除好友
func (s *FriendServiceImpl) DeleteFriend(ctx context.Context, req *dto.DeleteFriendRequest) (*dto.DeleteFriendResponse, error) {
	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoDeleteFriendRequest(req)

	// 2. 调用用户服务删除好友(gRPC)
	_, err := s.userClient.DeleteFriend(ctx, grpcReq)
	if err != nil {
		// gRPC 调用失败，提取业务错误码
		// 返回业务错误（作为 Go error 返回，由 Handler 层处理）
		return nil, err
	}

	// 3. gRPC 调用成功，返回结果
	return dto.ConvertDeleteFriendResponseFromProto(nil), nil
}

// SetFriendRemark 设置好友备注
func (s *FriendServiceImpl) SetFriendRemark(ctx context.Context, req *dto.SetFriendRemarkRequest) (*dto.SetFriendRemarkResponse, error) {
	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoSetFriendRemarkRequest(req)

	// 2. 调用用户服务设置好友备注(gRPC)
	_, err := s.userClient.SetFriendRemark(ctx, grpcReq)
	if err != nil {
		// gRPC 调用失败，提取业务错误码
		// 返回业务错误（作为 Go error 返回，由 Handler 层处理）
		return nil, err
	}

	// 3. gRPC 调用成功，返回结果
	return dto.ConvertSetFriendRemarkResponseFromProto(nil), nil
}

// SetFriendTag 设置好友标签
func (s *FriendServiceImpl) SetFriendTag(ctx context.Context, req *dto.SetFriendTagRequest) (*dto.SetFriendTagResponse, error) {
	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoSetFriendTagRequest(req)

	// 2. 调用用户服务设置好友标签(gRPC)
	_, err := s.userClient.SetFriendTag(ctx, grpcReq)
	if err != nil {
		// gRPC 调用失败，提取业务错误码
		// 返回业务错误（作为 Go error 返回，由 Handler 层处理）
		return nil, err
	}

	// 3. gRPC 调用成功，返回结果
	return dto.ConvertSetFriendTagResponseFromProto(nil), nil
}

// CheckIsFriend 判断是否好友
func (s *FriendServiceImpl) CheckIsFriend(ctx context.Context, req *dto.CheckIsFriendRequest) (*dto.CheckIsFriendResponse, error) {
	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoCheckIsFriendRequest(req)

	// 2. 调用用户服务判断是否好友(gRPC)
	grpcResp, err := s.userClient.CheckIsFriend(ctx, grpcReq)
	if err != nil {
		// gRPC 调用失败，提取业务错误码
		// 返回业务错误（作为 Go error 返回，由 Handler 层处理）
		return nil, err
	}

	// 3. gRPC 调用成功，返回结果
	return dto.ConvertCheckIsFriendResponseFromProto(grpcResp), nil
}

// GetRelationStatus 获取关系状态
func (s *FriendServiceImpl) GetRelationStatus(ctx context.Context, req *dto.GetRelationStatusRequest) (*dto.GetRelationStatusResponse, error) {
	// 1. 转换 DTO 为 Protobuf 请求
	grpcReq := dto.ConvertToProtoGetRelationStatusRequest(req)

	// 2. 调用用户服务获取关系状态(gRPC)
	grpcResp, err := s.userClient.GetRelationStatus(ctx, grpcReq)
	if err != nil {
		// gRPC 调用失败，提取业务错误码
		// 返回业务错误（作为 Go error 返回，由 Handler 层处理）
		return nil, err
	}

	// 3. gRPC 调用成功，返回结果
	return dto.ConvertGetRelationStatusResponseFromProto(grpcResp), nil
}

// batchGetSimpleUserInfo 批量获取用户信息（含去重与分片）
// 失败时返回错误，由调用方决定是否降级
func (s *FriendServiceImpl) batchGetSimpleUserInfo(ctx context.Context, uuids []string) (map[string]*dto.SimpleUserInfo, error) {
	const batchSize = 100
	result := make(map[string]*dto.SimpleUserInfo)
	if len(uuids) == 0 {
		return result, nil
	}

	// 去重
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
