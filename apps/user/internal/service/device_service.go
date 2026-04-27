package service

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/013677890/LCchat-Backend/apps/user/internal/repository"
	pb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	pkgdeviceactive "github.com/013677890/LCchat-Backend/pkg/deviceactive"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/util"
)

// deviceServiceImpl 设备会话服务实现
type deviceServiceImpl struct {
	deviceRepo repository.IDeviceRepository
}

// NewDeviceService 创建设备服务实例
func NewDeviceService(deviceRepo repository.IDeviceRepository) DeviceService {
	return &deviceServiceImpl{
		deviceRepo: deviceRepo,
	}
}

// GetDeviceList 获取设备列表
func (s *deviceServiceImpl) GetDeviceList(ctx context.Context, req *pb.GetDeviceListRequest) (*pb.GetDeviceListResponse, error) {
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

	devices := make([]*pb.DeviceItem, 0, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}
		sec, ok := activeTimes[session.DeviceId]
		if !ok || sec <= 0 {
			sec = 0
		}
		lastSeenAt := sec * 1000
		devices = append(devices, &pb.DeviceItem{
			DeviceId:        session.DeviceId,
			DeviceName:      session.DeviceName,
			Platform:        session.Platform,
			AppVersion:      session.AppVersion,
			IsCurrentDevice: deviceID != "" && session.DeviceId == deviceID,
			Status:          int32(session.Status),
			LastSeenAt:      lastSeenAt,
		})
	}

	sort.Slice(devices, func(i, j int) bool {
		if devices[i].LastSeenAt == devices[j].LastSeenAt {
			return devices[i].DeviceId < devices[j].DeviceId
		}
		return devices[i].LastSeenAt > devices[j].LastSeenAt
	})

	return &pb.GetDeviceListResponse{Devices: devices}, nil
}

// KickDevice 踢出设备
func (s *deviceServiceImpl) KickDevice(ctx context.Context, req *pb.KickDeviceRequest) error {
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

	// 幂等语义：无论 token 是否已删除，都返回成功；仅 Redis 异常才报错。
	if err := s.deviceRepo.DeleteTokens(ctx, userUUID, req.DeviceId); err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "踢出设备失败：删除设备 Token 失败")
	}

	// status 语义：0=在线, 1=离线, 2=注销, 3=被踢出。
	// 踢设备时：在线/离线 -> 被踢出；注销/已被踢出保持原状态，按幂等成功。
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

// GetOnlineStatus 获取用户在线状态
func (s *deviceServiceImpl) GetOnlineStatus(ctx context.Context, req *pb.GetOnlineStatusRequest) (*pb.GetOnlineStatusResponse, error) {
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

	// 无设备会话，直接离线返回（lastSeenAt 仍取 active zset 最近值）。
	if len(sessions) == 0 {
		return &pb.GetOnlineStatusResponse{
			Status: &pb.OnlineStatus{
				UserUuid:        req.UserUuid,
				IsOnline:        false,
				LastSeenAt:      lastSeenSec * 1000,
				OnlinePlatforms: []string{},
			},
		}, nil
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

		// 在线判定：状态=在线 且 (当前时间 - Redis 活跃时间) <= 窗口。
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

	return &pb.GetOnlineStatusResponse{
		Status: &pb.OnlineStatus{
			UserUuid:        req.UserUuid,
			IsOnline:        isOnline,
			LastSeenAt:      lastSeenSec * 1000,
			OnlinePlatforms: platforms,
		},
	}, nil
}

// BatchGetOnlineStatus 批量获取在线状态
func (s *deviceServiceImpl) BatchGetOnlineStatus(ctx context.Context, req *pb.BatchGetOnlineStatusRequest) (*pb.BatchGetOnlineStatusResponse, error) {
	if req == nil || len(req.UserUuids) == 0 || len(req.UserUuids) > 100 {
		return nil, apperr.New(consts.CodeParamError)
	}

	// 去重后查询，返回结果按请求顺序组装。
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

	users := make([]*pb.OnlineStatusItem, 0, len(req.UserUuids))
	for _, userUUID := range req.UserUuids {
		sessions := sessionsByUser[userUUID]
		lastSeenSec := lastSeenByUser[userUUID]
		if len(sessions) == 0 {
			users = append(users, &pb.OnlineStatusItem{
				UserUuid:   userUUID,
				IsOnline:   false,
				LastSeenAt: lastSeenSec * 1000,
			})
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

		users = append(users, &pb.OnlineStatusItem{
			UserUuid:   userUUID,
			IsOnline:   isOnline,
			LastSeenAt: lastSeenSec * 1000,
		})
	}

	return &pb.BatchGetOnlineStatusResponse{
		Users: users,
	}, nil
}

// UpdateDeviceActive 批量更新设备活跃时间（内部调用）。
// 由 gateway/connect 在本地节流命中后调用，仅更新 Redis 活跃时间。
func (s *deviceServiceImpl) UpdateDeviceActive(ctx context.Context, req *pb.UpdateDeviceActiveRequest) error {
	if req == nil || len(req.Items) == 0 {
		return apperr.New(consts.CodeParamError)
	}

	nowSec := time.Now().Unix()
	repoItems := make([]repository.DeviceActiveItem, 0, len(req.Items))
	for _, item := range req.Items {
		if item == nil || item.UserUuid == "" || item.DeviceId == "" {
			return apperr.New(consts.CodeParamError)
		}
		repoItems = append(repoItems, repository.DeviceActiveItem{
			UserUUID: item.UserUuid,
			DeviceID: item.DeviceId,
		})
	}

	if err := s.deviceRepo.BatchSetActiveTimestamps(ctx, repoItems, nowSec); err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "批量更新设备活跃时间失败")
	}

	return nil
}

// UpdateDeviceStatus 更新设备在线状态（内部调用）。
// 由 connect 服务在连接建立/断开时调用。
// 幂等语义：设备不存在时视为成功（可能设备已被踢出或注销）。
func (s *deviceServiceImpl) UpdateDeviceStatus(ctx context.Context, req *pb.UpdateDeviceStatusRequest) error {
	if req == nil || req.UserUuid == "" || req.DeviceId == "" {
		return apperr.New(consts.CodeParamError)
	}

	// 仅允许 0(在线) 和 1(离线) 两种状态。
	targetStatus := int8(req.Status)
	if targetStatus != model.DeviceStatusOnline && targetStatus != model.DeviceStatusOffline {
		return apperr.New(consts.CodeParamError)
	}

	if err := s.deviceRepo.UpdateOnlineStatus(ctx, req.UserUuid, req.DeviceId, targetStatus); err != nil {
		// 幂等语义：设备不存在时不视为错误。
		// 场景：设备已被踢出/注销后 connect 的延迟断连通知。
		if errors.Is(err, repository.ErrRecordNotFound) {
			return nil
		}
		return apperr.Wrap(err, consts.CodeInternalError, "更新设备状态失败")
	}

	return nil
}
