package cache

import (
	"context"
	"time"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/model"
	"github.com/013677890/LCchat-Backend/pkg/repoerr"
)

// CheckGroupSendPermission 检查指定用户在群内是否允许发送消息。
//
// 架构定位与性能优化：
//  1. 面向 msg-service 等消息链路极高频调用点，每次群消息投递前都会同步调用该方法进行权限校验；
//  2. 采用纯内存策略判定：先从 Redis String 读群资料（状态与全员禁言标记），再通过 Lua 单点读取成员 Hash 字段；
//  3. 严格禁止在权限判定链路使用 HGETALL，以保证千人大群中发言权限判定始终保持 O(1) 耗时与极低内存开销；
//  4. 三层权限判定矩阵：
//     - 判定 1: 用户是否为有效群成员（非成员直接拒绝 GroupSendDenyNotMember）；
//     - 判定 2: 群是否开启全员禁言且用户为普通成员（管理员/群主具备豁免特权，普通成员拒绝 GroupSendDenyGroupMuted）；
//     - 判定 3: 用户是否处于单人禁言期内（MuteUntil > Now，被禁言者拒绝 GroupSendDenyMemberMuted）；
//     - 若以上均通过，则返回 CanSend=true。
func (r *Reader) CheckGroupSendPermission(ctx context.Context, groupUUID, userUUID string) (repository.CheckGroupSendPermissionResult, error) {
	result := repository.CheckGroupSendPermissionResult{CanSend: false, Role: -1}
	if r == nil || r.store == nil || groupUUID == "" || userUUID == "" {
		result.Reason = repository.GroupSendDenyNotMember
		return result, nil
	}
	// 1. 读取群状态与全员禁言开关
	group, err := r.loadReadableGroupInfo(ctx, groupUUID)
	if err != nil {
		return result, err
	}
	// 2. 点查用户的成员身份快照（包含角色和单人禁言截止时间）
	member, err := r.loadActiveMemberForRead(ctx, groupUUID, userUUID)
	if err != nil {
		return result, err
	}
	// 判定 1: 非群成员拦截
	if member == nil {
		result.Reason = repository.GroupSendDenyNotMember
		return result, nil
	}
	result.Role = member.Role
	result.MuteAll = group.MuteAll

	// 判定 2: 全员禁言拦截（群主和管理员豁免）
	if group.MuteAll && member.Role == repository.MemberRoleMember {
		result.Reason = repository.GroupSendDenyGroupMuted
		return result, nil
	}
	// 判定 3: 单人禁言拦截（未过期则拒绝）
	if member.MuteUntil != nil && member.MuteUntil.After(time.Now()) {
		result.Reason = repository.GroupSendDenyMemberMuted
		result.MuteUntil = member.MuteUntil.UnixMilli()
		return result, nil
	}
	// 权限校验全部通过
	result.CanSend = true
	return result, nil
}

// loadActiveMemberForRead 点查发送权限所需的单个成员快照（优先读 Hash 单 field 缓存，miss 时查 DB）。
func (r *Reader) loadActiveMemberForRead(ctx context.Context, groupUUID, userUUID string) (*model.GroupMember, error) {
	member, cacheHit, err := r.getGroupMemberFromCache(ctx, groupUUID, userUUID)
	if err != nil {
		repoerr.LogRedisError(ctx, err)
	} else if cacheHit {
		return member, nil
	}
	return r.store.LoadActiveMemberFromDB(ctx, groupUUID, userUUID)
}
