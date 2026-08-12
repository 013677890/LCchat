package service

import (
	"context"
	"errors"
	"sort"

	"github.com/013677890/LCchat-Backend/apps/auth/internal/repository"
	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/presence"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
	"github.com/013677890/LCchat-Backend/pkg/util"
)

// deviceServiceImpl 实现设备会话与在线状态查询逻辑。
//
// 在线状态的事实源是 connect 维护的 presence 路由投影（`user:routing:{user}`）：
// 设备在窗口内有存活 WS 连接才算在线；device_sessions 只提供平台等展示元数据，
// 以及离线设备的 last_seen（最后一次登录/断连/登出等状态迁移时刻）。
type deviceServiceImpl struct {
	deviceRepo   repository.IDeviceRepository
	presenceRepo presence.Repository
}

// NewDeviceService 创建设备服务实例。
//
// 该服务聚焦“设备会话 / 在线状态”两个高频读写场景：
//  1. 读取当前用户的设备列表与在线态；
//  2. 执行踢设备、设备状态刷新；
//  3. 统一把 presence 路由与设备会话的聚合结果组装成 auth proto。
func NewDeviceService(deviceRepo repository.IDeviceRepository, presenceRepo presence.Repository) DeviceService {
	return &deviceServiceImpl{deviceRepo: deviceRepo, presenceRepo: presenceRepo}
}

// listUserRoutesDegraded 读取单用户 presence 路由；读取失败按离线降级并告警。
// 在线状态属于展示能力，presence 读失败不应让整个查询失败。
func (s *deviceServiceImpl) listUserRoutesDegraded(ctx context.Context, userUUID string) []presence.DeviceRoute {
	if s.presenceRepo == nil {
		return nil
	}

	routes, err := s.presenceRepo.ListUserRoutes(ctx, userUUID)
	if err != nil {
		logger.Warn(ctx, "读取 presence 路由失败，按离线降级",
			logger.String("user_uuid", userUUID),
			logger.ErrorField("error", err),
		)
		return nil
	}
	return routes
}

// listUsersRoutesDegraded 批量读取 presence 路由；读取失败按全员离线降级并告警。
func (s *deviceServiceImpl) listUsersRoutesDegraded(ctx context.Context, userUUIDs []string) map[string][]presence.DeviceRoute {
	if s.presenceRepo == nil {
		return map[string][]presence.DeviceRoute{}
	}

	routesByUser, err := s.presenceRepo.ListUsersRoutes(ctx, userUUIDs)
	if err != nil {
		logger.Warn(ctx, "批量读取 presence 路由失败，按离线降级",
			logger.Int("user_count", len(userUUIDs)),
			logger.ErrorField("error", err),
		)
		return map[string][]presence.DeviceRoute{}
	}
	return routesByUser
}

// computeUserPresence 依据 presence 路由与设备会话推导单用户在线视图。
// 语义：
//  1. 在线 = 存在窗口内路由，且对应会话未处于已登出/被踢终态；
//  2. platforms 取在线设备对应会话的平台集合（会话缺失时平台未知，不影响在线判定）；
//  3. last_seen = max(在线设备路由 lastActiveMs, 全部会话的最后状态迁移时刻)。
func computeUserPresence(routes []presence.DeviceRoute, sessions []*model.DeviceSession) (bool, []string, int64) {
	sessionByDevice := make(map[string]*model.DeviceSession, len(sessions))
	var lastSeenSec int64
	for _, session := range sessions {
		if session == nil || session.DeviceId == "" {
			continue
		}
		sessionByDevice[session.DeviceId] = session

		if !session.UpdatedAt.IsZero() {
			if sec := session.UpdatedAt.Unix(); sec > lastSeenSec {
				lastSeenSec = sec
			}
		}
	}

	isOnline := false
	platformSet := make(map[string]struct{})
	for _, route := range routes {
		session := sessionByDevice[route.DeviceID]

		// 防御性排除已登出/被踢终态：踢线尚未强制断开 WS 前路由可能仍新鲜，
		// 该设备不应继续对外展示为在线。
		if session != nil && (session.Status == model.DeviceStatusLoggedOut || session.Status == model.DeviceStatusKicked) {
			continue
		}
		isOnline = true

		if session != nil && session.Platform != "" {
			platformSet[session.Platform] = struct{}{}
		}
		if sec := route.LastActiveMs / 1000; sec > lastSeenSec {
			lastSeenSec = sec
		}
	}

	platforms := make([]string, 0, len(platformSet))
	for platform := range platformSet {
		platforms = append(platforms, platform)
	}
	sort.Strings(platforms)

	return isOnline, platforms, lastSeenSec
}

// GetDeviceList 获取当前登录用户的设备列表。
// 业务流程：
//  1. 从上下文中提取当前用户 UUID 与当前设备 ID；
//  2. 批量查询该用户的设备会话快照；
//  3. 读取 presence 路由，得到各在线设备的心跳级活跃时间；
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

	// last_seen：presence 路由中的设备取心跳级 lastActiveMs；
	// 其余设备回退到会话的最后状态迁移时刻。
	routeActiveSec := make(map[string]int64)
	for _, route := range s.listUserRoutesDegraded(ctx, userUUID) {
		if route.LastActiveMs > 0 {
			routeActiveSec[route.DeviceID] = route.LastActiveMs / 1000
		}
	}

	devices := make([]*authpb.DeviceItem, 0, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}

		lastSeenSec := routeActiveSec[session.DeviceId]
		if lastSeenSec == 0 && !session.UpdatedAt.IsZero() {
			lastSeenSec = session.UpdatedAt.Unix()
		}
		if device := buildDeviceItemProto(session, deviceID, lastSeenSec); device != nil {
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
//  4. 删除该设备的 RefreshToken，阻止后续续期；
//  5. 将设备状态更新为 kicked，记录设备管理结果。
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
		if errors.Is(err, repoerr.ErrRecordNotFound) {
			return apperr.New(consts.CodeDeviceNotFound)
		}
		return apperr.Wrap(err, consts.CodeInternalError, "踢出设备失败：查询设备会话失败")
	}

	if session == nil {
		return apperr.New(consts.CodeDeviceNotFound)
	}

	// AccessToken 是无状态 JWT，不维护 Redis 黑名单；被踢设备的旧 AccessToken 仍会
	// 在自身 exp 前有效。撤销 RefreshToken 可以保证它无法越过该窗口继续续期。
	if err := s.deviceRepo.DeleteRefreshToken(ctx, userUUID, req.DeviceId); err != nil {
		return apperr.Wrap(err, consts.CodeInternalError, "踢出设备失败：撤销 RefreshToken 失败")
	}
	if session.Status == model.DeviceStatusOnline || session.Status == model.DeviceStatusOffline {
		if err := s.deviceRepo.UpdateOnlineStatus(ctx, userUUID, req.DeviceId, model.DeviceStatusKicked); err != nil {
			if errors.Is(err, repoerr.ErrRecordNotFound) {
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
//  2. 查询该用户当前所有设备会话（平台元数据与终态排除依据）；
//  3. 读取 presence 路由（窗口内有存活 WS 连接的设备集）；
//  4. 聚合出在线状态、在线平台与 last_seen；
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

	routes := s.listUserRoutesDegraded(ctx, req.UserUuid)
	isOnline, platforms, lastSeenSec := computeUserPresence(routes, sessionsByUser[req.UserUuid])

	return &authpb.GetOnlineStatusResponse{Status: buildOnlineStatusProto(req.UserUuid, isOnline, lastSeenSec, platforms)}, nil
}

// BatchGetOnlineStatus 批量获取在线状态。
// 业务流程：
//  1. 校验 user_uuids 非空且数量不超过上限；
//  2. 对请求 UUID 去重，批量查询设备会话与 presence 路由；
//  3. 逐个用户聚合在线状态；
//  4. 按原始请求顺序返回在线状态列表。
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

	routesByUser := s.listUsersRoutesDegraded(ctx, unique)

	users := make([]*authpb.OnlineStatusItem, 0, len(req.UserUuids))
	for _, userUUID := range req.UserUuids {
		isOnline, _, lastSeenSec := computeUserPresence(routesByUser[userUUID], sessionsByUser[userUUID])
		users = append(users, buildOnlineStatusItemProto(userUUID, isOnline, lastSeenSec))
	}

	return &authpb.BatchGetOnlineStatusResponse{Users: users}, nil
}

// UpdateDeviceStatus 更新设备在线状态。
// 业务流程：
//  1. 校验 user_uuid、device_id 与目标状态；
//  2. 仅允许 online / offline 两种在线态更新；
//  3. 调用仓储层更新设备会话状态（该迁移时刻是离线设备 last_seen 的数据来源）；
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
		if errors.Is(err, repoerr.ErrRecordNotFound) {
			return nil
		}
		return apperr.Wrap(err, consts.CodeInternalError, "更新设备状态失败")
	}
	return nil
}
