package service

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/013677890/LCchat-Backend/apps/auth/internal/repository"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	pkgdeviceactive "github.com/013677890/LCchat-Backend/pkg/deviceactive"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/util"
)

// deviceServiceImpl 实现设备会话与在线状态查询逻辑。
type deviceServiceImpl struct {
	deviceRepo repository.IDeviceRepository
}

// NewDeviceService 创建设备服务实例。
//
// 该服务聚焦“设备会话 / 在线状态”两个高频读写场景：
//  1. 读取当前用户的设备列表与在线态；
//  2. 执行踢设备、状态刷新、活跃时间回写；
//  3. 统一把仓储层的设备状态结果组装成 auth proto。
func NewDeviceService(deviceRepo repository.IDeviceRepository) DeviceService {
	return &deviceServiceImpl{deviceRepo: deviceRepo}
}

// GetDeviceList 获取当前登录用户的设备列表。
// 业务流程：
//  1. 从上下文中提取当前用户 UUID 与当前设备 ID；
//  2. 批量查询该用户的设备会话快照；
//  3. 按 device_id 查询各设备最近活跃时间；
//  4. 组装 DeviceItem 并按最近活跃时间倒序排序；
//  5. 返回设备列表给上游网关。
//
// 错误码映射：
//   - codes.Unauthenticated: 未登录
//   - codes.Internal: 系统内部错误
func (s *deviceServiceImpl) GetDeviceList(ctx context.Context, req *authpb.GetDeviceListRequest) (*authpb.GetDeviceListResponse, error) {
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return nil, apperr.New(consts.CodeUnauthorized)
	}

	deviceID := util.GetDeviceIDFromContext(ctx)
	sessionsByUser, err := s.deviceRepo.BatchGetOnlineStatus(ctx, []string{userUUID})
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取设备列表失败")
	}
	sessions := sessionsByUser[userUUID]

	deviceIDs := make([]string, 0, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}
		deviceIDs = append(deviceIDs, session.DeviceId)
	}

	activeTimes, err := s.deviceRepo.GetActiveTimestamps(ctx, userUUID, deviceIDs)
	if err != nil {
		logger.Warn(ctx, "获取设备活跃时间失败，按空活跃时间返回",
			logger.String("user_uuid", userUUID),
			logger.ErrorField("error", err),
		)
		activeTimes = map[string]int64{}
	}

	devices := make([]*authpb.DeviceItem, 0, len(sessions))
	for _, session := range sessions {
		if device := buildDeviceItemProto(session, deviceID, activeTimes[session.DeviceId]); device != nil {
			devices = append(devices, device)
		}
	}

	sort.Slice(devices, func(i, j int) bool {
		if devices[i].LastSeenAt == devices[j].LastSeenAt {
			return devices[i].DeviceId < devices[j].DeviceId
		}
		return devices[i].LastSeenAt > devices[j].LastSeenAt
	})

	return &authpb.GetDeviceListResponse{Devices: devices}, nil
}

// KickDevice 踢出指定设备。
// 业务流程：
//  1. 从上下文中提取当前用户 UUID；
//  2. 校验目标 device_id 合法且不是当前设备；
//  3. 查询目标设备会话并确认其存在；
//  4. 删除该设备的 AccessToken / RefreshToken；
//  5. 将设备状态更新为 kicked，完成远端登录态失效。
//
// 错误码映射：
//   - codes.Unauthenticated: 未登录
//   - codes.InvalidArgument: device_id 缺失或试图踢出当前设备
//   - codes.NotFound: 设备不存在
//   - codes.Internal: 系统内部错误
func (s *deviceServiceImpl) KickDevice(ctx context.Context, req *authpb.KickDeviceRequest) error {
	userUUID := util.GetUserUUIDFromContext(ctx)
	if userUUID == "" {
		return apperr.New(consts.CodeUnauthorized)
	}
	if req == nil || req.DeviceId == "" {
		return apperr.New(consts.CodeParamError)
	}

	currentDeviceID := util.GetDeviceIDFromContext(ctx)
	if currentDeviceID != "" && currentDeviceID == req.DeviceId {
		return apperr.New(consts.CodeCannotKickCurrent)
	}

	session, err := s.deviceRepo.GetByDeviceID(ctx, userUUID, req.DeviceId)
	if err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return apperr.New(consts.CodeDeviceNotFound)
		}
		return apperr.Wrap(err, consts.CodeInternalError, "踢出设备失败：查询设备会话失败")
	}
	if session == nil {
		return apperr.New(consts.CodeDeviceNotFound)
	}

	if err := s.deviceRepo.DeleteTokens(ctx, userUUID, req.DeviceId); err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "踢出设备失败：删除设备 Token 失败")
	}
	if session.Status == model.DeviceStatusOnline || session.Status == model.DeviceStatusOffline {
		if err := s.deviceRepo.UpdateOnlineStatus(ctx, userUUID, req.DeviceId, model.DeviceStatusKicked); err != nil {
			if errors.Is(err, repository.ErrRecordNotFound) {
				return apperr.New(consts.CodeDeviceNotFound)
			}
			return apperr.Wrap(err, consts.CodeInternalError, "踢出设备失败：更新设备状态失败")
		}
	}
	return nil
}

// GetOnlineStatus 获取单用户在线状态。
// 业务流程：
//  1. 校验请求中的 user_uuid；
//  2. 查询该用户当前所有设备会话；
//  3. 读取用户最近活跃时间与各设备活跃时间；
//  4. 按在线窗口计算是否在线，并汇总在线平台；
//  5. 返回单用户在线状态视图。
//
// 错误码映射：
//   - codes.InvalidArgument: 参数错误
//   - codes.Internal: 系统内部错误
func (s *deviceServiceImpl) GetOnlineStatus(ctx context.Context, req *authpb.GetOnlineStatusRequest) (*authpb.GetOnlineStatusResponse, error) {
	if req == nil || req.UserUuid == "" {
		return nil, apperr.New(consts.CodeParamError)
	}

	sessionsByUser, err := s.deviceRepo.BatchGetOnlineStatus(ctx, []string{req.UserUuid})
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "获取在线状态失败：查询设备会话失败")
	}
	sessions := sessionsByUser[req.UserUuid]

	lastSeenMap, err := s.deviceRepo.BatchGetLastSeenTimestamps(ctx, []string{req.UserUuid})
	if err != nil {
		logger.Warn(ctx, "获取在线状态失败：读取最近活跃时间失败，按 0 返回",
			logger.String("user_uuid", req.UserUuid),
			logger.ErrorField("error", err),
		)
		lastSeenMap = map[string]int64{}
	}
	lastSeenSec := lastSeenMap[req.UserUuid]
	if len(sessions) == 0 {
		return &authpb.GetOnlineStatusResponse{Status: buildOnlineStatusProto(req.UserUuid, false, lastSeenSec, []string{})}, nil
	}

	deviceIDs := make([]string, 0, len(sessions))
	for _, session := range sessions {
		if session == nil || session.DeviceId == "" {
			continue
		}
		deviceIDs = append(deviceIDs, session.DeviceId)
	}

	activeTimes, err := s.deviceRepo.GetActiveTimestamps(ctx, req.UserUuid, deviceIDs)
	if err != nil {
		logger.Warn(ctx, "获取在线状态失败：读取设备活跃时间失败，按离线处理",
			logger.String("user_uuid", req.UserUuid),
			logger.ErrorField("error", err),
		)
		activeTimes = map[string]int64{}
	}

	nowSec := time.Now().Unix()
	windowSec := int64(pkgdeviceactive.OnlineWindow().Seconds())
	platformSet := make(map[string]struct{})
	isOnline := false
	for _, session := range sessions {
		if session == nil || session.DeviceId == "" {
			continue
		}
		seenSec, ok := activeTimes[session.DeviceId]
		if !ok || seenSec <= 0 {
			continue
		}
		if session.Status == model.DeviceStatusOnline && nowSec-seenSec <= windowSec {
			isOnline = true
			if session.Platform != "" {
				platformSet[session.Platform] = struct{}{}
			}
		}
	}

	platforms := make([]string, 0, len(platformSet))
	for p := range platformSet {
		platforms = append(platforms, p)
	}
	sort.Strings(platforms)

	return &authpb.GetOnlineStatusResponse{Status: buildOnlineStatusProto(req.UserUuid, isOnline, lastSeenSec, platforms)}, nil
}

// BatchGetOnlineStatus 批量获取在线状态。
// 业务流程：
//  1. 校验 user_uuids 非空且数量不超过上限；
//  2. 对请求 UUID 去重，批量查询设备会话；
//  3. 批量读取设备活跃时间与最近活跃时间；
//  4. 逐个用户按在线窗口计算在线状态；
//  5. 按原始请求顺序返回在线状态列表。
//
// 错误码映射：
//   - codes.InvalidArgument: 参数错误
//   - codes.Internal: 系统内部错误
func (s *deviceServiceImpl) BatchGetOnlineStatus(ctx context.Context, req *authpb.BatchGetOnlineStatusRequest) (*authpb.BatchGetOnlineStatusResponse, error) {
	if req == nil || len(req.UserUuids) == 0 || len(req.UserUuids) > 100 {
		return nil, apperr.New(consts.CodeParamError)
	}

	unique := make([]string, 0, len(req.UserUuids))
	seen := make(map[string]struct{}, len(req.UserUuids))
	for _, userUUID := range req.UserUuids {
		if userUUID == "" {
			return nil, apperr.New(consts.CodeParamError)
		}
		if _, ok := seen[userUUID]; ok {
			continue
		}
		seen[userUUID] = struct{}{}
		unique = append(unique, userUUID)
	}

	sessionsByUser, err := s.deviceRepo.BatchGetOnlineStatus(ctx, unique)
	if err != nil {
		return nil, apperr.Wrap(err, consts.CodeInternalError, "批量获取在线状态失败")
	}

	nowSec := time.Now().Unix()
	windowSec := int64(pkgdeviceactive.OnlineWindow().Seconds())
	userDeviceIDs := make(map[string][]string, len(unique))
	for _, userUUID := range unique {
		sessions := sessionsByUser[userUUID]
		if len(sessions) == 0 {
			continue
		}
		deviceIDs := make([]string, 0, len(sessions))
		for _, session := range sessions {
			if session == nil || session.DeviceId == "" {
				continue
			}
			deviceIDs = append(deviceIDs, session.DeviceId)
		}
		if len(deviceIDs) > 0 {
			userDeviceIDs[userUUID] = deviceIDs
		}
	}

	activeByUser, err := s.deviceRepo.BatchGetActiveTimestamps(ctx, userDeviceIDs)
	if err != nil {
		logger.Warn(ctx, "批量获取在线状态：批量读取设备活跃时间失败，按离线处理",
			logger.Int("user_count", len(userDeviceIDs)),
			logger.ErrorField("error", err),
		)
		activeByUser = map[string]map[string]int64{}
	}
	lastSeenByUser, err := s.deviceRepo.BatchGetLastSeenTimestamps(ctx, unique)
	if err != nil {
		logger.Warn(ctx, "批量获取在线状态：读取最近活跃时间失败，按 0 返回",
			logger.Int("user_count", len(unique)),
			logger.ErrorField("error", err),
		)
		lastSeenByUser = map[string]int64{}
	}

	users := make([]*authpb.OnlineStatusItem, 0, len(req.UserUuids))
	for _, userUUID := range req.UserUuids {
		sessions := sessionsByUser[userUUID]
		lastSeenSec := lastSeenByUser[userUUID]
		if len(sessions) == 0 {
			users = append(users, buildOnlineStatusItemProto(userUUID, false, lastSeenSec))
			continue
		}

		activeTimes := activeByUser[userUUID]
		if activeTimes == nil {
			activeTimes = map[string]int64{}
		}
		isOnline := false
		for _, session := range sessions {
			if session == nil || session.DeviceId == "" {
				continue
			}
			seenSec, ok := activeTimes[session.DeviceId]
			if !ok || seenSec <= 0 {
				continue
			}
			if session.Status == model.DeviceStatusOnline && nowSec-seenSec <= windowSec {
				isOnline = true
			}
		}
		users = append(users, buildOnlineStatusItemProto(userUUID, isOnline, lastSeenSec))
	}

	return &authpb.BatchGetOnlineStatusResponse{Users: users}, nil
}

// UpdateDeviceActive 批量更新设备活跃时间。
// 业务流程：
//  1. 校验请求体与活跃项列表非空；
//  2. 将 proto 活跃项转换为仓储层批量写入结构；
//  3. 使用统一的当前秒级时间戳回写设备活跃时间；
//  4. 由仓储层负责 Redis ZSet 的批量更新与过期清理。
//
// 错误码映射：
//   - codes.InvalidArgument: 参数错误
//   - codes.Internal: 系统内部错误
func (s *deviceServiceImpl) UpdateDeviceActive(ctx context.Context, req *authpb.UpdateDeviceActiveRequest) error {
	if req == nil || len(req.Items) == 0 {
		return apperr.New(consts.CodeParamError)
	}

	nowSec := time.Now().Unix()
	repoItems := make([]repository.DeviceActiveItem, 0, len(req.Items))
	for _, item := range req.Items {
		if item == nil || item.UserUuid == "" || item.DeviceId == "" {
			return apperr.New(consts.CodeParamError)
		}
		repoItems = append(repoItems, repository.DeviceActiveItem{UserUUID: item.UserUuid, DeviceID: item.DeviceId})
	}
	if err := s.deviceRepo.BatchSetActiveTimestamps(ctx, repoItems, nowSec); err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "批量更新设备活跃时间失败")
	}
	return nil
}

// UpdateDeviceStatus 更新设备在线状态。
// 业务流程：
//  1. 校验 user_uuid、device_id 与目标状态；
//  2. 仅允许 online / offline 两种在线态更新；
//  3. 调用仓储层更新设备会话状态；
//  4. 若设备不存在则按幂等成功处理。
//
// 错误码映射：
//   - codes.InvalidArgument: 参数错误或状态非法
//   - codes.Internal: 系统内部错误
func (s *deviceServiceImpl) UpdateDeviceStatus(ctx context.Context, req *authpb.UpdateDeviceStatusRequest) error {
	if req == nil || req.UserUuid == "" || req.DeviceId == "" {
		return apperr.New(consts.CodeParamError)
	}
	targetStatus := int8(req.Status)
	if targetStatus != model.DeviceStatusOnline && targetStatus != model.DeviceStatusOffline {
		return apperr.New(consts.CodeParamError)
	}
	if err := s.deviceRepo.UpdateOnlineStatus(ctx, req.UserUuid, req.DeviceId, targetStatus); err != nil {
		if errors.Is(err, repository.ErrRecordNotFound) {
			return nil
		}
		return apperr.Wrap(err, consts.CodeInternalError, "更新设备状态失败")
	}
	return nil
}
